# 更新记录

所有对 RedCart Copilot 有意义的变更都记录在这里。

## [未发布] - 2026-06-08

### 消费侧可靠性

- 新增 RabbitMQ 消费者实现（`backend/internal/event/rabbitmq/consumer.go`）：手动 ack（至少一次语义）、QoS prefetch 限流、`event_id` 幂等去重、DLX 死信队列。
- 新增 `Handler` / `Deduplicator` / `Acknowledger` 接口，使消费逻辑可测试且不耦合 AMQP SDK；`MemoryDeduplicator` 提供 demo 级进程内去重。
- 新增 5 个 consumer 单元测试：成功 ack+mark、handler 失败进 DLX、重复消息跳过、decode 错误进 DLX、dedup 瞬态错误 requeue。
- 更新 ADR 0006 新增第 7 节「消费侧可靠性」，记录手动 ack、prefetch、幂等顺序（MarkProcessed before Ack）和 DLX 路由决策。

### 发布器可靠性加固

- outbox 表新增 `published_at` 列与 `idx_outbox_pending` 部分索引（`backend/migrations/0003_outbox_published_at.sql`）；已发布事件改为软标记而非删除，保留审计轨迹。
- outbox relay 改为事务内轮询：`BeginTx → PollPendingInTx（FOR UPDATE SKIP LOCKED）→ 逐条发布 → MarkPublishedInTx / MarkFailedInTx → Commit`，防止多实例并发重复发布。
- 新增 `event.OutboxRelayStore` 与 `event.OutboxTx` 接口，提供事务感知的轮询与标记方法；`*Repository` 实现完整委托。
- RabbitMQ publisher 启用 publisher confirm 模式（`PublishWithDeferredConfirmWithContext` + `WaitContext`），并通过 `NotifyClose` 自动重连（3 秒间隔），`sync.Mutex` 保护并发 publish。
- 修复 `main.go` 中 `repo.(event.OutboxStore)` 类型断言始终失败的潜在 bug（`*Repository` 未实现完整 `OutboxStore`），改为 `event.OutboxRelayStore` 断言。
- 新增 `TestPublisherNoDuplicatePublishUnderConcurrency`（2 relay × 50 事件，零重复）与 `TestPublisherRollbackOnPublishFailure` 测试。
- 更新 ADR 0006 第 2、6 节，记录软标记、行锁、confirm 模式与自动重连决策。

### 工程

- 删除内存仓储实现、内存仓储单元测试、handler-only HTTP benchmark、空 publisher outbox benchmark 和模拟下游延迟 benchmark；后端测试与性能证据收束到 PostgreSQL/Redis/RabbitMQ-backed 路径和 live HTTP benchmark。
- 新增 live HTTP benchmark，要求 `LIVE_HTTP_BASE_URL` 指向已启动后端进程，通过真实 TCP 请求验证 `/healthz`、结算预览和下单写路径；README 性能表更新脚本拒绝非真实运行时 benchmark 名称。
- 建立本地 `main` 分支作为集成主干；删除已合并或停滞的 `feature/*` 分支以及过期的 `ai/live-*`、`ai/codex-*` 会话分支；清理所有非主工作区的 worktree；将 `.aidev-local/` 加入 `.gitignore`，保持主工作区干净。

### 优化

- 将支付、取消、确认收货、退款申请、商家发货和退款审批这些高价值订单状态变更接口收敛为幂等语义：目标状态已达成时，重复请求直接返回当前订单视图，而不是返回冲突。
- 将 `git worktree` 升级为项目默认协作工作流，并增加 `scripts/git-worktree.sh` 与自动生成的 `BRANCH_STATUS.local.md` 本地状态板；Codex hook 会在相关 Git 操作后同步 worktree 状态和 AI 更改大纲。
- 新增 Redis 读侧适配层：在提供 `REDIS_ADDR` 时，认证 session 改为 Redis 真相源并带本地热缓存，同时为商品、SKU 和 SKU 列表增加 Redis 热读缓存与写后失效；`OrderPreview` 基准显著提升，而 `CreateOrder` 维持基本持平。
- 增加 `.codex/skills/git-worktree/` 项目级 skill，供 agent 在需要多分支并行开发或分支隔离时引用。
- 仅在 PostgreSQL 适配层优化写路径：将运行时 SQL 调用切到 `database/sql`，用 `INSERT/UPDATE ... RETURNING` 取代写后回读，并让订单写入阶段直接回填 `order_items.id` 与时间戳，减少下单热路径的数据库往返和对象分配。
- 将 PR 与 `main` push 的统一门禁收敛到 `.github/workflows/ci.yml`。
- 将后端、前端、AI service、安全和 Docker 子 workflow 保留为复用与手动运行入口。
- 将 Redis 从可选读侧适配提升为运行时必需依赖：后端启动缺少 `REDIS_ADDR` 直接失败；PostgreSQL HTTP 集成测试强制要求 `REDIS_ADDR`；同步更新 README、架构文档、项目约束、CI 说明与性能基线口径。
- 新增 PostgreSQL 极端并发稳定性测试：覆盖脏读、非重复读、幻读、死锁、丢失更新与写偏斜；测试暴露 Pay/Cancel/Finish/Refund 等状态转换存在丢失更新风险。
- 修复订单状态转换竞争：新增 `application.Repository.UpdateOrderStatus` 原子方法，PostgreSQL 实现通过 `SELECT ... FOR UPDATE` + `UPDATE ... WHERE status = 原状态` 保证并发状态迁移只有一个胜出；所有订单状态变更服务方法迁移到该方法，失败时返回幂等视图。
- 进一步把状态迁移与库存副作用收敛到同一事务：新增 `application.OrderTx` 接口并扩展 `UpdateOrderStatus` 支持事务内副作用回调；`PayOrder`、`CancelOrder`、`MerchantApproveRefund` 的库存确认/释放与订单事件写入全部在状态迁移同一事务中完成；新增 `TestPayOrderInventoryFailureRollsBackStatus` 验证副作用失败时状态回滚。
- 将 `ai-service` 的 Dependabot 扫描从不存在的 Python manifest 改为 Dockerfile 依赖面。
- 明确每个 commit 都应同步更新 `CHANGELOG.md`，并把例外说明纳入交付验证工作流。
- 回退 README 和 CHANGELOG 中的本地 PNG 图片资产与引用。
- 调整 Pyroscope 验证口径：不使用 curl profile 查询作为 CI/CD 门禁，profile 数据保留为本地人工/界面复核。
- 增加 Pyroscope mutex/block profiling 的可选采样配置，默认仍只启用 CPU/alloc/inuse profiling。
- 拆分下单流程中的订单草稿和库存锁构建逻辑，并补充订单创建副作用回归测试。
- 拆分下单流程中的订单创建事件记录逻辑，并补强事件元数据测试。
- 按主题拆分 HTTP 层测试文件，保留原有测试与 benchmark 行为不变。
- 增加项目专用 Codex hook，在本仓库交付前检查 `rtk` 命令约束、测试入口保留、OpenAPI 和 workspace 门禁。
- 归档 2026-06-08 本地全量验证状态，记录后端、前端、AI service、安全、OpenAPI、workspace 和 Docker build 通过证据。

### 新增

- 按 ADR 0005 把 AI Copilot 接入 gRPC：新增 `api/proto/ai/v1/ai.proto`、生成 Go/Python stub、`backend/internal/ai/grpc` 客户端、`ai-service/app/grpc_server.py` 服务端，以及 `scripts/generate-ai-grpc.sh` 生成脚本。
- 后端通过 `AI_PROVIDER=grpc` 与 `AI_GRPC_ADDR` 调用独立 `ai-service`；默认 Docker Compose 启动带 gRPC 端口的 `ai-service` 容器。
- 新增 gRPC 客户端与服务端单元测试；扫描脚本排除 `*/.venv/*` 以避免本地 Python 虚拟环境触发密钥误报。
- 更新 `docs/architecture.md`，记录 AI 服务作为内部 gRPC 边界。
- 新增 A2UI（Agent-to-UI）RPC：`api/proto/ai/v1/ai.proto` 增加 `A2UIService.GenerateA2UISurface`，按 A2UI v0.9 返回声明式 UI surface JSON；生成 Go/Python stub，后端 `AIProvider` 契约、`MockProvider`、`backend/internal/ai/grpc` 客户端、`ai-service/app/provider.py` 与 `ai-service/app/grpc_server.py` 均实现对应方法。
- 后端新增 `/api/ai/a2ui` HTTP 入口，登录用户可调用；前端新增 `/a2ui` 演示页与基础 A2UI 组件渲染器（Card/Column/Row/Text/Button）。
- 升级 A2UI 为智能导购专题页：应用层解析用户意图中的预算与场景，查询在线商品并筛选预算内商品，注入上下文后由 AI Provider 生成购物专题 surface；前端支持 `List`/`Image`/`Slider` 组件与加购 action。
- 新增宿舍书桌相关演示商品（LED 台灯、桌面收纳盒、USB 插线板）及 SKU，补充内存与 PostgreSQL 种子数据。
- 新增 `TestAIHTTPA2UIShoppingGuide` 验证导购页生成链路。
- 同步更新 `docs/api/openapi.yaml`、`docs/api/endpoint-table.md`、`docs/architecture.md`、`AI_WORKFLOW.md`。

## [0.1.0] - 2026-06-05

### 新增

- 初始化面向内容电商作品集的 monorepo 结构
- 补齐 Issue 模板、PR 模板、CODEOWNERS、Dependabot 与 CI 工作流
- 增加笔记、商品、购物车、订单、商家看板、AI Copilot 的 OpenAPI 草案
- 增加用户、商品、订单、库存锁、行为事件、AI 任务相关迁移草案
- 增加订单状态机及其单元测试
- 增加 AI Provider 抽象与 Mock Provider 测试
- 增加 AI 工作流、重构计划、ADR、测试策略与 PRD
- 增加可运行的内存版 Go HTTP API，覆盖消费者、商家和 AI 主链路
- 增加后端服务测试与 HTTP 测试，覆盖幂等下单、支付、发货、退款和库存恢复
- 增加可直接演示的静态前端页面，并与后端实际返回结构对齐
