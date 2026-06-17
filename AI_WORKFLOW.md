# AI Native 开发工作流

这份文档记录 RedCart Copilot 中 AI 参与需求、设计、实现、测试、审查和交付的方式。AI 输出只能作为草案，最终决策、代码合并和验收结论必须由人工或主代理复核。

## 适用范围

以下情况必须记录 AI 使用过程：

- AI 影响了需求拆解、技术方案、接口契约、数据库结构或架构边界。
- AI 参与了代码、测试、文档、迁移、CI 脚本或生成产物的创建或修改。
- AI 用于排查订单、库存、金额、权限、迁移、支付、退款、AI 任务等高风险问题。
- Kimi 或其他子代理被委托执行前端实现、代码审查、并行排查或报告整理。

轻量问答如果不影响仓库产物，可以不记录；一旦进入实现或验收，就必须留下记录。

## 基本原则

- AI 输出是草案，不是最终决策。
- 不把密钥、生产数据、个人隐私、真实支付凭证或外部账号凭据交给 AI。
- 订单状态、库存、金额、权限、迁移和数据一致性相关改动必须人工复核。
- 架构、运行时、供应商或框架假设必须落到 `docs/architecture.md`、ADR 或对应工作流文档。
- 可执行验证优先于文字说明；没有跑过的命令不能写成“已通过”。
- 子代理不能直接修改主工作区；必须使用隔离 worktree 或 `/tmp` 副本，由主代理验收后合并。

## 使用矩阵

| 场景 | AI 可以负责 | 必须人工或主代理复核 |
|---|---|---|
| 需求拆解 | 起草用户故事、边界、验收点 | 删除非 MVP 范围，补齐业务约束 |
| 技术设计 | 提供模块划分、状态流转、异常分支草案 | 明确事务边界、幂等、库存、退款和权限规则 |
| API 设计 | 起草路径、请求体、响应体、错误语义 | 同步 `docs/api/openapi.yaml` 和接口文档表 |
| 数据库与适配器 | 起草迁移、仓储接口、事务方案 | 确认索引、约束、并发一致性和回滚路径 |
| 前端实现 | 起草页面结构、状态流、交互处理 | 验证真实 API、构建产物、响应式和错误状态 |
| 测试生成 | 枚举候选用例、边界输入、回归点 | 补齐并发、非法状态、金额和越权测试 |
| Code Review | 指出风险、遗漏和不一致 | 决定是否修改，并补验证证据 |
| 重构建议 | 给出拆分方向和依赖边界建议 | 保证外部行为不变，并同步架构或 ADR |

## 标准流程

1. 明确任务类型：功能、集成、调试、文档、审查或交付验证。
2. 读取对应工作流：`docs/workflows/add-feature.md`、`add-integration.md`、`debug.md` 或 `validate.md`。
3. 识别受影响层次和稳定契约，避免把框架、数据库、模型 Provider 细节泄漏进领域层。
4. 让 AI 产出候选方案、测试点或实现草案。
5. 主代理或人工复核高风险点，必要时先写失败测试再改实现。
6. 运行最小相关验证，再运行仓库要求的基础验证。
7. 每个 commit 都应同步更新 `CHANGELOG.md`，记录本次提交带来的用户可见变化、工程行为变化、验证门禁变化或文档资产变化；确实无可记录变化时，在 PR 描述或交付说明中说明原因。
8. 在本文件、风险归档、ADR 或 PR 描述中记录 AI 参与内容、修正点和验证结果。

## 子代理委托规则

使用 Kimi 等子代理时，主代理必须先给出窄边界任务说明：

- 允许修改的路径。
- 不允许修改的路径和凭据边界。
- 后端/API 契约、命令、验收标准。
- 预期返回格式：改了哪些文件、运行了哪些命令、如何验证、剩余风险。

子代理产物必须经过主代理验收：

- 先读交接摘要，不直接信任结论。
- 查看 changed files 和 diff stat。
- 运行最小有意义的测试、构建或冒烟命令。
- 只在失败或高风险文件上深读具体 diff。
- 未验收的子代理产物不得进入主工作区。

## 记录模板

每次 AI 参与仓库产物时，按下面格式追加记录或写入 PR 描述：

````markdown
## YYYY-MM-DD：<任务名称>

### AI 参与范围

- <AI 做了什么：需求拆解 / 方案草案 / 代码实现 / 测试生成 / 子代理实现 / 审查>

### 人工或主代理修正

- <删除了哪些不合适假设>
- <补了哪些边界、测试、文档或验证>

### 验证证据

```bash
<实际运行过的命令>
```

### 剩余风险

- <没有覆盖或刻意延期的风险；没有则写“无已知剩余风险”>
````

## 已归档案例

### 案例一：订单状态机

AI 任务：为内容电商订单流设计状态机，覆盖 `CREATED`、`PAID`、`SHIPPED`、`FINISHED`、`CANCELLED`、`REFUNDING`、`REFUNDED`，说明合法流转、非法流转、库存释放节点、幂等要求与测试点。

人工修正：

- 补充超时取消、重复支付拒绝、取消/退款与库存释放绑定、订单事件日志要求。
- 在 `backend/internal/order/domain` 中补显式状态流转校验和相关测试。
- 补充订单状态机与库存锁相关 ADR。

### 案例二：MVP 可执行交付

AI 任务：把 RedCart Copilot 的 MVP 拆成可运行的后端切面和边界清晰的前端交付简报，并显式保留幂等、状态流转和库存恢复规则。

人工修正：

- 运行时先使用内存适配层，把 PostgreSQL/Redis 保留为目标架构而不是硬前提。
- 后端补服务层和 HTTP 层测试，覆盖下单、支付、发货、退款和库存恢复。
- 前端委托改为隔离目录执行，最后由主代理验收并回收结果。

### 案例三：风险审计修复

AI 任务：根据测试审计中已确认的风险点，修复 HTTP 写操作 method gate、AI Copilot 权限隔离和 PostgreSQL 并发库存预锁问题，并补可执行回归测试。

人工修正：

- HTTP 状态变更路由显式要求 `POST`，并补 `GET` 不得触发写操作的回归测试。
- AI 生成入口要求商家角色，AI 任务读取按角色做所有权判断，避免 `merchant_id=0` 横向匹配。
- 新增 `SaveOrderWithInventoryLocks` 仓储契约，PostgreSQL 路径在事务内用条件更新原子预锁库存，内存路径在互斥锁内完成同样语义。
- 将已确认问题、修复状态、验证命令和剩余风险记录到 `docs/testing/2026-06-05-risk-audit.md`。

### 案例四：Gin/GORM 迁移性能口径修正

AI 任务：分析现有 QPS 数据是否能评估 Gin/GORM 迁移成果，并补充能打到关键运行路径的测试和 CI 产物。

人工修正：

- 明确原有 `BenchmarkHTTPNotes` 和 `BenchmarkHTTPOrderPreview` 只使用内存仓储，只能代表 Gin 路由、JSON 编解码和应用层轻量路径。
- 新增 PostgreSQL-backed HTTP 集成测试，覆盖 Gin -> 应用层 -> PostgreSQL/GORM 的注册、商品/SKU、上架、结算预览、幂等下单、库存预锁、支付确认、发货、完成、看板和 AI 任务读取。
- 新增 PostgreSQL-backed HTTP 反向路径测试，覆盖取消释放库存、支付后退款恢复库存、库存不足无副作用、错误 method 不触发状态变化、消费者访问商家接口、跨用户读取订单和跨商家读取 AI 任务。
- 新增 HTTP 并发下单测试，确认真实 HTTP 路径下库存为 1 的 SKU 只能创建 1 笔订单，其余请求返回 `409 conflict`。
- 新增 PostgreSQL-backed benchmark 和 CI 产物，把 `backend-qps.txt` 与 `backend-postgres-http-qps.txt` 分开，避免用内存仓储数据评估 PostgreSQL/GORM 性能。

## 2026-06-06：测试用例补强

### AI 参与范围

- 审查现有后端、前端和 AI service 测试覆盖缺口。
- 补充 `cmd/api`、领域 helper、内存仓储、应用层、HTTP 层、前端守卫测试和 AI Provider 契约测试。
- 按工作流补充测试策略、端到端用例映射、测试指标门禁和剩余测试边界说明。

### 人工或主代理修正

- 将“空结算请求必须失败”的错误假设修正为：无选中购物车项时失败，有选中购物车项时可从购物车生成结算。
- 重点补订单、库存、权限、购物车、内容/商品读取、商家商品/SKU、dashboard、AI 任务和负向输入路径。
- 沿用仓库现有零依赖前端测试脚本，没有引入新的前端测试框架。
- 新增 `ci/scripts/backend-test-metrics.sh`，把覆盖率、关键包阈值、测试数量和 benchmark 数量变成可阻断回退的 CI 指标。
- 将新增测试覆盖和剩余非目标同步到 `docs/testing/test-strategy.md` 与 `docs/testing/e2e-cases.md`，方便后续 agent 追踪。

### 验证证据

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./internal/redcart/application ./internal/redcart/interfaces/httpapi
rtk env GOCACHE=/tmp/go-build-cache go test ./...
rtk env GOCACHE=/tmp/go-build-cache go test ./... -coverprofile=/tmp/redcart-cover.out
rtk env GOCACHE=/tmp/go-build-cache go tool cover -func=/tmp/redcart-cover.out
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 POSTGRES_BENCHTIME=1s GOCACHE=/tmp/go-build-cache bash ci/scripts/backend-ci.sh
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 GOCACHE=/tmp/go-build-cache go test ./internal/redcart/infrastructure/postgres -v
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 GOCACHE=/tmp/go-build-cache go test ./internal/redcart/interfaces/httpapi -run '^TestPostgresHTTP' -count=1 -v
rtk npm test
rtk npm run typecheck
rtk npm run lint
rtk npm run build
rtk python3 -m unittest discover -s tests -v
rtk bash ci/scripts/ai-service-ci.sh
rtk bash scripts/check-openapi.sh
rtk bash scripts/validate-workspace.sh
```

### 剩余风险

- 前端仍是源码守卫和构建检查，不是浏览器级 UI 自动化。
- QPS benchmark 仍是 `httptest` handler 基准，不是外部网络压测。

### 继续提升覆盖率

- 补充内存仓储公开方法测试，覆盖用户/商家按 ID 读取、目录列表、笔记更新、SKU 列表、购物车选中删除、订单读取、订单事件、行为事件和 AI task map 拷贝隔离。
- 补充 PostgreSQL 仓储离线 helper 测试，覆盖 row scanner、nullable helper、迁移文件解析、GORM result 适配和 scanner 错误传播；真实数据库路径仍由 `RUN_POSTGRES_INTEGRATION=1` 的集成测试覆盖。
- 将后端质量门禁提高到：总覆盖率 `65.0%`、内存仓储覆盖率 `90.0%`、PostgreSQL 仓储覆盖率 `15.0%`、后端测试数量 `54`。

### 继续验证证据

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./internal/redcart/infrastructure/memory ./internal/redcart/infrastructure/postgres
rtk bash ci/scripts/backend-ci.sh
rtk bash scripts/check-openapi.sh
rtk bash scripts/validate-workspace.sh
```

### PostgreSQL 覆盖率继续提升

- 新增 `TestRepositoryPostgresCRUDCoverage`，连接真实 PostgreSQL 覆盖用户/商家、笔记更新、商品/SKU、购物车、订单保存与列表、库存锁、行为事件和 AI task 读写。
- 将 PostgreSQL 仓储覆盖率门禁改为动态阈值：普通 CI 至少 `15.0%`，`RUN_POSTGRES_INTEGRATION=1` 时至少 `75.0%`。
- 连库后端 CI 显示 PostgreSQL 仓储包覆盖率提升到 `80.3%`，总覆盖率提升到 `80.5%`。

### PostgreSQL 覆盖率验证证据

```bash
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 GOCACHE=/tmp/go-build-cache go test ./internal/redcart/infrastructure/postgres -v
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 POSTGRES_BENCHTIME=1s GOCACHE=/tmp/go-build-cache bash ci/scripts/backend-ci.sh
```

## 2026-06-08：Pyroscope Go Push Mode 接入

### AI 参与范围

- 梳理 Grafana Pyroscope 官方文档，比较 Go push mode 与 pull mode 的接入差异。
- 设计最小改动接入方案，把 profiling 限定在后端启动装配层，以环境变量控制启停。
- 生成启动配置解析、最小文档更新和单元测试草案。

### 人工或主代理修正

- 选择 Go push mode，而不是 pull mode，避免当前 Gin 路由额外暴露 `/debug/pprof/*` 和引入 Alloy 采集链路。
- 将 Pyroscope 接入限制在 `backend/cmd/api`，不向应用层、领域层或仓储契约泄漏供应商类型。
- 保持默认关闭：只有设置 `PYROSCOPE_SERVER_ADDRESS` 时才启用 profiling，避免影响现有 Docker Compose 和本地运行路径。

### 验证证据

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./cmd/api
rtk env GOCACHE=/tmp/go-build-cache go test ./...
rtk bash scripts/validate-workspace.sh
rtk podman run -d --name redcart-profile-pg --replace -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=redcart -p 15432:5432 postgres:16
rtk podman run -d --name redcart-profile-pyroscope --replace --network host grafana/pyroscope:latest
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable PYROSCOPE_SERVER_ADDRESS=http://127.0.0.1:4040 PYROSCOPE_APPLICATION_NAME=redcart.backend HTTP_PORT=18080 GOCACHE=/tmp/go-build-cache go run ./cmd/api
rtk curl -s http://127.0.0.1:18080/healthz
rtk curl -s http://127.0.0.1:18080/api/notes
rtk curl -s -X POST http://127.0.0.1:18080/api/auth/login -H Content-Type:application/json -d '{"phone":"13800000001","password":"consumer-demo"}'
```

### 剩余风险

- 当前只接入默认的 CPU/alloc/inuse profiles，尚未启用 mutex 和 block profiling。
- `docker compose up` 在当前环境里受 Podman compose provider 和 socket 状态影响，未能作为本次联调验证入口；实际联调用 `podman run + go run` 完成。

## 2026-06-08：CI/CD 最小影响优化

### AI 参与范围

- 审查现有 GitHub Actions、Dependabot 配置和 CI 说明。
- 按最小影响原则调整 workflow 触发策略，减少同一 PR 或 `main` push 的重复 CI 运行。
- 更新 CI/CD 文档，记录顶层门禁、子 workflow 复用入口和真实依赖扫描范围。

### 人工或主代理修正

- 保留 `.github/workflows/ci.yml` 作为 PR 与 `main` push 的统一入口，不改变现有门禁覆盖面。
- 将后端、前端、AI service、安全和 Docker workflow 收敛为 `workflow_call` 与 `workflow_dispatch`，保留手动单独运行能力。
- 移除后端 CI 中未使用的 Redis service，保持当前运行时 PostgreSQL 依赖边界。
- 将 Dependabot 的 `ai-service` 扫描从不存在的 Python manifest 改为 Dockerfile 依赖面。

### 验证证据

```bash
rtk bash scripts/validate-workspace.sh
rtk bash scripts/check-openapi.sh
rtk bash ci/scripts/security-ci.sh
rtk bash ci/scripts/frontend-ci.sh
rtk bash ci/scripts/ai-service-ci.sh
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 POSTGRES_BENCHTIME=1s GOCACHE=/tmp/go-build-cache bash ci/scripts/backend-ci.sh
rtk docker build -t redcart-backend:ci backend
rtk docker build -t redcart-frontend:ci frontend
rtk docker build -t redcart-ai-service:ci ai-service
rtk python3 -c "import yaml, pathlib; [yaml.safe_load(path.read_text()) for path in pathlib.Path('.github/workflows').glob('*.yml')]; yaml.safe_load(pathlib.Path('.github/dependabot.yml').read_text()); print('github yaml parse passed')"
```

### 剩余风险

- 本次只做 workflow 编排和依赖扫描配置优化，没有在 GitHub Actions 远端执行真实 workflow run。

## 2026-06-08：Pyroscope 可用性复核与图片资产回退

### AI 参与范围

- 复核 Pyroscope Go push mode 的验证口径，避免把 profile API 查询放进 CI/CD 门禁。
- 回退 README 和 CHANGELOG 中的本地 PNG 资产引用，并删除已生成的 PNG 文件。
- 按新增的 CHANGELOG 约束记录本次提交粒度变化。

### 人工或主代理修正

- 将 Pyroscope 判断口径调整为：CI 只验证后端 profiling 配置解析、默认关闭路径、启动错误传播和常规业务门禁；profile 数据保留为本地 UI 人工复核，不使用 curl 查询 profile API 作为自动化证据。
- 明确此前本地 PNG 是 Python + Pillow 绘制的确定性图片，不是 imagegen 模型输出；已按要求回退，不再作为仓库资产保留。
- 将可用性证据同步到 `docs/testing/performance-baseline.md`，避免只停留在对话结论。

### 验证证据

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./cmd/api
rtk curl -s http://127.0.0.1:18080/healthz
rtk docker compose logs --tail=40 backend
rtk curl -s http://127.0.0.1:18080/api/notes
rtk curl -s -X POST http://127.0.0.1:18080/api/auth/login -H Content-Type:application/json -d '{"phone":"13800000001","password":"consumer-demo"}'
```

### 剩余风险

- 远端 GitHub Actions workflow run 尚未在本地会话中触发。
- mutex、block 等 profile types 尚未纳入性能诊断基线。

## 2026-06-08：Pyroscope Mutex/Block 采样开关

### AI 参与范围

- 根据性能基线中保留的诊断边界，设计 Pyroscope mutex/block profile 的最小可选接入方案。
- 补充后端启动配置测试，覆盖默认关闭、可选采样、非法采样值、启动失败恢复和停止恢复。
- 更新运行配置文档、Compose 环境传递和性能基线口径。

### 人工或主代理修正

- 保持默认行为不变：只配置 `PYROSCOPE_SERVER_ADDRESS` 时仍只启用 CPU/alloc/inuse profiles。
- 将 mutex/block 采样限定为显式正整数环境变量，避免在常规运行和 CI 中默认增加采样开销。
- 在 Pyroscope start 前应用 runtime 采样设置，并在启动失败或停止时恢复 mutex 采样，避免全局 runtime 状态泄漏。

### 验证证据

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./cmd/api
rtk env GOCACHE=/tmp/go-build-cache go test ./...
rtk bash scripts/check-openapi.sh
rtk bash scripts/validate-workspace.sh
```

### 剩余风险

- mutex/block profile types 已可临时启用，但尚未纳入固定性能诊断基线或本地 UI 人工复核记录。

## 2026-06-08：下单流程构建逻辑拆分

### AI 参与范围

- 根据 RF-001 重构计划，选择不改变外部 API 的最小切片。
- 先补充 `CreateOrder` 行为测试，锁住订单创建事件、库存锁、locked stock 和行为事件副作用。
- 将订单草稿构建和库存锁构建从 `CreateOrder` 主流程抽为私有 helper。

### 人工或主代理修正

- 保持仓储契约、HTTP 契约和 OpenAPI 不变，本次只做应用层内部结构拆分。
- 没有引入新的 `OrderFactory` 公共类型，避免在一次小提交里扩大抽象面；RF-001 后续可继续按测试保护逐步拆分。
- 用内存仓储行为测试验证下单副作用，而不是只测试私有 helper。

### 验证证据

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./internal/redcart/application
rtk env GOCACHE=/tmp/go-build-cache go test ./...
rtk bash scripts/check-openapi.sh
rtk bash scripts/validate-workspace.sh
```

### 剩余风险

- `CreateOrder` 仍负责 idempotency、校验编排、保存、事件记录和购物车清理；RF-001 还需要继续拆分 validator、locker 和 event publisher。

## 2026-06-08：下单创建事件记录拆分

### AI 参与范围

- 继续 RF-001 的低风险切片，聚焦订单创建后的事件记录逻辑。
- 补强 `CreateOrder` 行为测试，增加订单创建事件的 from/to status、operator、remark 和时间断言。
- 将订单创建事件与行为事件记录抽为 `recordOrderCreated` 私有 helper。

### 人工或主代理修正

- 保持原有事件记录失败不阻断下单的策略，不在本次切片中改变错误传播语义。
- 不新增外部 event publisher 契约，避免在单次提交中扩大接口面；后续可在更多状态事件有测试保护后继续抽象。
- 保持 HTTP、OpenAPI、仓储契约不变。

### 验证证据

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./internal/redcart/application
```

### 剩余风险

- `CreateOrder` 仍负责 idempotency、校验编排、库存预锁保存和购物车清理；RF-001 后续还需要继续拆 validator、locker 和 cart cleanup 边界。

## 2026-06-08：HTTP 测试文件按主题拆分

### AI 参与范围

- 将 `backend/internal/redcart/interfaces/httpapi/server_test.go` 按测试主题机械拆分为基础路由、购物车、订单、商家、AI、PostgreSQL 集成、benchmark 和公共 helper 文件。
- 保留原有测试和 benchmark 行为，只移动顶层声明与对应 imports。

### 人工或主代理修正

- 用原文件与拆分后文件的 `Test*`/`Benchmark*` 名称对比确认没有遗漏测试入口。
- 保持 HTTP 契约、OpenAPI、应用层行为和仓储契约不变，本次只做测试文件组织整理。

### 验证证据

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./internal/redcart/interfaces/httpapi
rtk env GOCACHE=/tmp/go-build-cache go test ./...
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 POSTGRES_BENCHTIME=1s GOCACHE=/tmp/go-build-cache bash ci/scripts/backend-ci.sh
rtk bash ci/scripts/frontend-ci.sh
rtk bash ci/scripts/ai-service-ci.sh
rtk bash ci/scripts/security-ci.sh
rtk bash ci/scripts/check-openapi.sh
rtk bash ci/scripts/validate-workspace.sh
rtk bash -lc 'tmp1=$(mktemp); tmp2=$(mktemp); git show HEAD:backend/internal/redcart/interfaces/httpapi/server_test.go | rg -o "^func (Test|Benchmark)[^(]+" | sed "s/^func //" | sort > "$tmp1"; rg -o "^func (Test|Benchmark)[^(]+" backend/internal/redcart/interfaces/httpapi/*_test.go | sed "s/.*func //" | sort > "$tmp2"; diff -u "$tmp1" "$tmp2"; status=$?; rm -f "$tmp1" "$tmp2"; exit $status'
rtk git diff --check
rtk bash scripts/validate-workspace.sh
rtk bash scripts/check-openapi.sh
rtk docker build -t redcart-backend:ci backend
rtk docker build -t redcart-frontend:ci frontend
rtk docker build -t redcart-ai-service:ci ai-service
```

### 剩余风险

- PostgreSQL HTTP 测试仍默认依赖 `RUN_POSTGRES_INTEGRATION=1` 和 `POSTGRES_DSN` 才会实际连库执行；本次默认后端测试覆盖的是 skip 路径。

## 2026-06-08：项目专用 Codex Hook 与验证状态归档

### AI 参与范围

- 根据本次测试拆分和全量验证经验，设计项目本地 Codex hook，避免后续 agent 在本仓库绕过 `rtk`、遗漏测试入口对比或只跑结构校验就结束交付。
- 更新测试策略、验证工作流、完成清单、项目技能入口和验证状态归档，让 hook 与本地全量验证状态可被后续 agent 发现。

### 人工或主代理修正

- 将 hook 限定在本仓库 `.codex/config.toml` 和 `.codex/hooks/redcart_project_hook.py`，不写入全局 Codex 配置，不影响其他 workspace。
- 用官方 Codex Hooks 文档核对项目级 hook 发现位置、`PreToolUse` deny 输出和 `Stop` 继续/停止语义。
- 将 hook 脚本纳入 `scripts/validate-workspace.sh` 的结构检查和自测，避免 hook 配置失效后无人察觉。

### 验证证据

```bash
rtk python3 .codex/hooks/redcart_project_hook.py --self-test
rtk python3 -m py_compile .codex/hooks/redcart_project_hook.py
rtk python3 -c 'import tomllib, pathlib; data=tomllib.loads(pathlib.Path(".codex/config.toml").read_text()); assert data["features"]["hooks"] is True; assert "PreToolUse" in data["hooks"]; assert "Stop" in data["hooks"]; print("project codex hook config shape passed")'
rtk python3 .codex/hooks/redcart_project_hook.py --mode quick
rtk python3 .codex/hooks/redcart_project_hook.py --mode full
rtk bash scripts/validate-workspace.sh
```

### 剩余风险

- 项目本地 Codex hook 需要在 Codex `/hooks` 面板中信任后才会自动运行；脚本或配置变更后需要重新信任。
- Hook quick gate 是交付兜底，不替代高风险改动后的完整 CI、PostgreSQL 集成测试和 Docker build。

## 2026-06-08：仅 PostgreSQL 适配层的写路径性能优化

### AI 参与范围

- 分析 `backend/internal/redcart/infrastructure/postgres/repository.go` 的写路径热点，定位 `SaveProduct`、`SaveSKU`、`SaveCartItem`、`SaveOrder`、`SaveOrderWithInventoryLocks` 中的写后回读和 GORM 运行时包装开销。
- 仅在 PostgreSQL 适配层内改写运行时 SQL 调用与返回值回填方式，不改应用层、HTTP 层、内存仓储、schema、migration 或 OpenAPI。
- 根据基准结果整理性能记录，说明优化动作、原因和收益口径。

### 人工或主代理修正

- 保持事务边界、库存条件更新 SQL、冲突语义和并发安全测试口径不变。
- 不把优化扩展到应用层批量接口或 schema 变更，避免超出“只改 pgsql 写性能”的范围。
- 对更新路径保留 `RETURNING created_at, updated_at`，避免因为去掉回读而丢失数据库侧时间戳。

### 验证证据

```bash
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 GOCACHE=/tmp/go-build-cache go test ./internal/redcart/infrastructure/postgres
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 POSTGRES_BENCHTIME=2s GOCACHE=/tmp/go-build-cache go test ./internal/redcart/interfaces/httpapi -run '^$' -bench BenchmarkHTTPPostgresCreateOrder -benchmem
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 GOCACHE=/tmp/go-build-cache bash ci/scripts/backend-ci.sh
```

### 剩余风险

- 这次优化依赖 PostgreSQL `RETURNING` 回填当前真正会变化的字段；如果后续在数据库侧新增会改写更多业务字段的 trigger 或规则，需要扩大 `RETURNING` 列表或恢复针对性回读。
- 当前收益主要来自减少往返和分配，`CreateOrder` 仍然是多 SQL 事务写路径，若后续还要继续提吞吐，应直接分析 `SaveOrderWithInventoryLocks` 的 SQL 往返数和事件写入阶段耗时。

## 2026-06-09：Git Worktree 工作流与本地分支状态板

### AI 参与范围

- 将 `git worktree` 提升为项目默认协作工作流，新增 `docs/workflows/git-worktree.md` 和 `scripts/git-worktree.sh`，让功能开发、性能 spike 和文档修订默认走独立 worktree。
- 在项目本地 Codex hook 中增加分支状态同步能力：相关 Git 操作后的 `PostToolUse` 以及 `Stop` 阶段自动刷新主工作区根目录下的 `BRANCH_STATUS.local.md`。
- 为状态板增加 AI 生成的“更改大纲”，让主工作区能快速看到各活跃 worktree 的意图、关键文件、行为变化、验证状态和风险阻塞。

### 人工或主代理修正

- 将 `BRANCH_STATUS.local.md` 设计为本地生成文件并加入 `.gitignore`，避免动态状态污染仓库历史。
- 不让 hook 直接内联复杂摘要逻辑，而是调用独立脚本 `scripts/update-branch-status.py`；hook 只负责触发，脚本负责状态收集与 AI 摘要。
- 为 AI 摘要调用增加 `--disable hooks`、只读 sandbox 和超时回退，避免递归触发 hook 或因为摘要生成拖死交付流程。

### 验证证据

```bash
rtk bash scripts/git-worktree.sh list
rtk python3 scripts/update-branch-status.py
rtk bash scripts/validate-workspace.sh
```

### 剩余风险

- `BRANCH_STATUS.local.md` 中的 AI 更改大纲依赖本地 `codex exec` 可用；若本地 Codex 不可用或超时，脚本会回退到 Git 事实摘要，但细节会变粗。
- `PostToolUse` 只在可能改变 Git/worktree 状态的 Bash 命令后刷新状态板；纯查看类命令不会触发同步。

## 2026-06-08：Redis session 适配层接入

### AI 参与范围

- 按当前架构文档和运行边界，将 Redis 的第一刀收敛到认证 session/token 存储，不扩展到库存预扣、购物车、订单真相或幂等真相。
- 新增 Redis session 仓储装饰器，包裹 PostgreSQL 仓储；在提供 `REDIS_ADDR` 时把 token 写入 Redis，并在鉴权路径从 Redis 读取 user/merchant session 信息。
- 更新 Docker Compose、启动脚本、测试策略和运行说明，让本地 MVP 默认可以跑 Redis session 路径。

### 人工或主代理修正

- 避免 Redis 开启时继续双写基础仓储 session map，否则 TTL 失效后会被进程内存错误兜底成长期有效 token。
- 为 Redis 不可用场景保留带 TTL 的进程内 fallback，只做单实例兜底，不把它宣传成跨实例 session 方案。
- 明确 Redis 当前只负责 session；订单、库存、购物车和业务真相仍然在 PostgreSQL。

### 验证证据

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./cmd/api ./internal/redcart/infrastructure/redis ./internal/redcart/interfaces/httpapi
rtk env GOCACHE=/tmp/go-build-cache go test ./...
rtk bash scripts/validate-workspace.sh
```

### 剩余风险

- Redis session 当前不做持久化配置；Docker Compose 里的 Redis 采用无 AOF/无 RDB 的开发配置，重启后 session 会失效。
- Redis session 适配层当前只缓存鉴权所需的 user/merchant 最小字段；如果未来 `UserView` 的会话依赖字段扩展，需要同步扩展 session payload。

## 2026-06-09：Redis 读侧适配收敛为系统增益版本

### AI 参与范围

- 将 Redis 方案从“每次请求都走 Redis session 读取”收敛为“Redis 共享会话源 + 本地热缓存”，避免为写路径平白增加网络往返。
- 新增商品、SKU 和 SKU 列表的 Redis 热读缓存与写后失效，直接针对 `OrderPreview` 读路径和下单前的商品/SKU 校验路径提速。
- 基于 Redis on/off 的 PostgreSQL HTTP benchmark 对照，确认保留这条线的理由是读路径系统增益，而不是写路径吞吐幻想。

### 人工或主代理修正

- 保持订单、库存、购物车和幂等真相仍然在 PostgreSQL，不因为缓存命中就改变交易一致性边界。
- 在 `SaveOrderWithInventoryLocks` 后显式失效涉及下单 SKU 的缓存，避免用读缓存掩盖库存变化。
- 接受当前结论：Redis 对 `CreateOrder` 吞吐没有明显提升，但对 `OrderPreview` 已经有明确收益，因此将 Redis 定位为读侧增益而不是写侧增益。

### 验证证据

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./cmd/api ./internal/redcart/infrastructure/redis ./internal/redcart/interfaces/httpapi
rtk docker compose up -d postgres redis
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable RUN_POSTGRES_INTEGRATION=1 POSTGRES_BENCHTIME=3s GOCACHE=/tmp/go-build-cache go test ./internal/redcart/interfaces/httpapi -run '^$' -bench 'BenchmarkHTTPPostgres(OrderPreview|CreateOrder)$' -benchmem
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable REDIS_ADDR=127.0.0.1:6379 RUN_POSTGRES_INTEGRATION=1 POSTGRES_BENCHTIME=3s GOCACHE=/tmp/go-build-cache go test ./internal/redcart/interfaces/httpapi -run '^$' -bench 'BenchmarkHTTPPostgres(OrderPreview|CreateOrder)$' -benchmem
```

### 剩余风险

- 当前 Redis 读侧适配主要提升的是商品/SKU 热读路径；如果未来 SKU 更新频率升高，需要继续审视 TTL 与失效策略。
- `CreateOrder` 吞吐仍主要受 PostgreSQL 事务写路径支配；继续追写路径收益需要更重的 Redis 预占库存或幂等设计，而不是继续加只读缓存。

## 2026-06-12：Redis 从可选改为运行时必需依赖

### AI 参与范围

- 按用户要求将 Redis 从“可选读侧加速”改为“运行时必需依赖”。
- 修改后端装配层 `backend/cmd/api/repository_factory.go`，缺少 `REDIS_ADDR` 时直接返回错误。
- 调整装配层测试、Redis session/catalog 测试与 PostgreSQL HTTP 集成测试 helper，不再依赖无 Redis 的 fallback 路径。
- 同步更新 README、架构文档、项目约束、CI 说明、性能基线口径、CHANGELOG 与 AI 工作流记录。

### 人工或主代理修正

- 保留 `SessionRepository` 与 `CatalogCacheRepository` 内部的 `client == nil` 防御检查，仅作为最后一道防线，不再测试该路径。
- 历史性能基线中的“无 Redis”数据保留为对照说明，但明确标注 Redis 已是运行必需。

### 验证证据

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./cmd/api ./internal/redcart/infrastructure/redis ./internal/redcart/interfaces/httpapi
rtk env GOCACHE=/tmp/go-build-cache go test ./...
rtk bash scripts/validate-workspace.sh
rtk bash scripts/check-openapi.sh
```

### 剩余风险

- 本地仅启动 PostgreSQL 而未启动 Redis 的开发习惯会失败；需通过 `bash scripts/local-dev.sh` 或手动启动 Redis。
- 若 GitHub Actions 后端 workflow 未配置 Redis service，CI 会失败；已同步更新 `ci/README.md` 说明。

## 2026-06-12：极端并发稳定性测试与状态转换竞争修复

### AI 参与范围

- 按用户要求制造极端并发用例，覆盖脏读、幻读、非重复读、死锁、活锁、丢失更新与写偏斜等稳定性风险。
- 新增 `backend/internal/redcart/infrastructure/postgres/repository_stability_test.go`，包含：
  - `TestReadCommittedNoDirtyRead`：验证 READ COMMITTED 下不存在脏读。
  - `TestNonRepeatableReadInventory`、`TestPhantomInventoryLocks`：文档化 READ COMMITTED 下非重复读与幻读的预期行为。
  - `TestConcurrentPayOrderNoDoubleConfirm`：并发支付同一订单，检测库存重复确认（丢失更新）。
  - `TestConcurrentPayAndCancelNoInventoryCorruption`：并发支付与取消，检测写偏斜。
  - `TestConcurrentRefundAndFinishNoInventoryCorruption`：并发退款审批与确认收货，检测写偏斜。
  - `TestMassConcurrentStockReservation`：200 并发抢 50 库存，验证库存不超卖。
  - `TestCyclicSKUAccessNoDeadlock`：循环 SKU 访问模式，检测死锁。
- 测试运行后发现真实缺陷：并发 PayOrder 会出现多次成功并重复扣减库存；并发 Pay/Cancel 会同时成功导致库存不一致。

### 人工或主代理修正

- 定位根因：所有订单状态转换服务方法（Pay/Cancel/Finish/RequestRefund/Ship/ApproveRefund）都先 `GetOrder` 再 `SaveOrder`，`SaveOrder` 的 UPDATE 没有 `WHERE status = 原状态` 保护，导致丢失更新。
- 在 `application.Repository` 新增 `UpdateOrderStatus(orderID, fromStatus, toStatus, mutator)` 原子方法：PostgreSQL 实现使用 `SELECT ... FOR UPDATE` 并在 `UPDATE` 中追加 `AND status = fromStatus`，确保只有一个并发调用能完成状态迁移；内存实现同样做状态校验。
- 将所有订单状态转换服务方法迁移到 `UpdateOrderStatus`，失败后重读订单并返回幂等视图。
- 同步更新 `CHANGELOG.md` 与 `AI_WORKFLOW.md`。

### 验证证据

```bash
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable REDIS_ADDR=127.0.0.1:6379 RUN_POSTGRES_INTEGRATION=1 GOCACHE=/tmp/go-build-cache go test ./internal/redcart/infrastructure/postgres -run 'TestReadCommittedNoDirtyRead|TestNonRepeatableReadInventory|TestPhantomInventoryLocks|TestConcurrentPayOrderNoDoubleConfirm|TestConcurrentPayAndCancelNoInventoryCorruption|TestConcurrentRefundAndFinishNoInventoryCorruption|TestMassConcurrentStockReservation|TestCyclicSKUAccessNoDeadlock' -v -count=1
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable REDIS_ADDR=127.0.0.1:6379 RUN_POSTGRES_INTEGRATION=1 POSTGRES_BENCHTIME=1s GOCACHE=/tmp/go-build-cache bash ci/scripts/backend-ci.sh
rtk bash scripts/validate-workspace.sh
rtk bash scripts/check-openapi.sh
```

### 剩余风险

- `UpdateOrderStatus` 只保证状态迁移的原子性；库存确认/释放（Pay/Cancel/Refund）仍发生在状态迁移提交之后。进程在状态迁移后、库存更新前崩溃会导致订单状态与库存不一致，需后续通过补偿或事务包裹进一步收敛。
- 当前 PostgreSQL 默认隔离级别为 READ COMMITTED，已用测试文档化非重复读与幻读行为；若业务需要可重复读，需显式提升隔离级别并评估死锁风险。

## 2026-06-12：将状态迁移与库存副作用收敛到同一事务

### AI 参与范围

- 按用户"提升到最保证安全性的程度"要求，把订单状态迁移与库存/事件副作用进一步收敛到同一数据库事务。
- 扩展 `application.Repository`：新增 `OrderTx` 接口，修改 `UpdateOrderStatus` 增加 `sideEffect` 回调。
- PostgreSQL 实现 `pgOrderTx`，让回调内的 `GetSKU/SaveSKU/ListInventoryLocksByOrder/UpdateInventoryLock/AppendOrderEvent` 全部复用同一 `*sql.Tx`。
- 内存实现 `memOrderTx`，在 `UpdateOrderStatus` 全局锁内执行回调，并在副作用失败时回滚状态变更。
- 将 `PayOrder`、`CancelOrder`、`MerchantApproveRefund` 的库存确认/释放与事件写入移入 `sideEffect`；`ShipOrder`、`FinishOrder`、`RequestRefund` 继续使用 `UpdateOrderStatus` 但副作用为 `nil`。
- 新增 `TestPayOrderInventoryFailureRollsBackStatus`：人为破坏 SKU locked_stock 使支付副作用失败，验证订单状态回滚到 CREATED。
- 新增内存仓库 `TestUpdateOrderStatusWithSideEffect`，覆盖事务内副作用与回滚路径，恢复 memory 包覆盖率到阈值以上。

### 人工或主代理修正

- 将公共仓储操作（`GetSKU/SaveSKU/ListInventoryLocksByOrder/UpdateInventoryLock/AppendOrderEvent`）提取为基于 `dbQuerier` 的 helper，使主仓库与 `pgOrderTx` 共用同一份 SQL。
- 为 `gormTx` 补全 `Query` 方法以满足 `dbQuerier`。
- 确保 `releaseInventory` 改为接收 `OrderTx`，不再绕过事务。

### 验证证据

```bash
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable REDIS_ADDR=127.0.0.1:6379 RUN_POSTGRES_INTEGRATION=1 GOCACHE=/tmp/go-build-cache go test ./internal/redcart/infrastructure/postgres -run 'TestReadCommittedNoDirtyRead|TestNonRepeatableReadInventory|TestPhantomInventoryLocks|TestConcurrentPayOrderNoDoubleConfirm|TestConcurrentPayAndCancelNoInventoryCorruption|TestConcurrentRefundAndFinishNoInventoryCorruption|TestMassConcurrentStockReservation|TestCyclicSKUAccessNoDeadlock|TestPayOrderInventoryFailureRollsBackStatus' -v -count=1
rtk env GOCACHE=/tmp/go-build-cache go test ./internal/redcart/infrastructure/memory -coverprofile=/tmp/memory-cover.out
rtk env POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable REDIS_ADDR=127.0.0.1:6379 RUN_POSTGRES_INTEGRATION=1 POSTGRES_BENCHTIME=1s GOCACHE=/tmp/go-build-cache bash ci/scripts/backend-ci.sh
rtk bash scripts/check-openapi.sh
```

### 剩余风险

- 行为事件（`behavior_events`）仍写在事务外，若崩溃会丢失行为埋点，但不影响订单与库存一致性。
- 当前隔离级别仍为 READ COMMITTED；若未来引入可重复读或串行化，需要额外死锁测试与性能基线复核。

## 2026-06-12：AI Copilot gRPC 服务化

### AI 参与范围

- 按 ADR 0005 将 AI Copilot 从进程内 Mock Provider 升级为独立 gRPC 服务边界。
- 设计 `api/proto/ai/v1/ai.proto`，生成 Go/Python stub，实现 `backend/internal/ai/grpc` 客户端与 `ai-service/app/grpc_server.py` 服务端。
- 更新 `backend/cmd/api/main.go`、`docker-compose.yml`、`ai-service/Dockerfile` 与 `requirements.txt`，让后端可按 `AI_PROVIDER=grpc` 调用 `ai-service`。
- 补充 Go/Python gRPC 单元测试，更新 `docs/architecture.md`、扫描脚本排除 `.venv`，并新增 `.codex/skills/git-worktree/SKILL.md`。

### 人工或主代理修正

- 保持 `internal/ai.AIProvider` 契约不变，应用层无需修改；gRPC 客户端仅作为该契约的远程适配器。
- 保持订单、库存、购物车等核心交易模块仍在单体后端，不在本次提交中引入分布式事务。
- 把生成工具限定在本地 `backend/.bin` 与 `ai-service/.venv`，不将编译期依赖写死到生产镜像构建流程。
- 将 `ci/scripts/scan-secrets.sh` 增加 `*/.venv/*` 排除，避免本地虚拟环境文件触发误报。

### 验证证据

```bash
rtk bash scripts/generate-ai-grpc.sh
rtk env GOCACHE=/tmp/go-build-cache go test ./internal/ai/grpc -v
rtk env GOCACHE=/tmp/go-build-cache go build ./...
rtk bash -c "cd ai-service && .venv/bin/python -m unittest discover -s tests -v"
rtk bash scripts/validate-workspace.sh
```

### 剩余风险

- gRPC 服务当前未加 TLS/认证，后续若跨网络部署需要补充传输安全。
- `ai-service` 当前仍是 Mock Provider 逻辑；替换为真实模型推理时只需替换 `app/provider.py` 实现，gRPC 契约不变。

## 2026-06-13：A2UI（Agent-to-UI）RPC 注册

### AI 参与范围

- 在 `api/proto/ai/v1/ai.proto` 新增 `A2UIService` 与 `GenerateA2UISurface` RPC，按 A2UI v0.9 协议返回声明式 UI surface JSON。
- 生成 Go/Python stub，并在 `backend/internal/ai` 中扩展 `AIProvider` 契约与 `MockProvider`、`backend/internal/ai/grpc` 客户端实现。
- 在 `ai-service/app/provider.py` 与 `ai-service/app/grpc_server.py` 实现 A2UI surface 生成服务端逻辑。
- 在后端 `backend/internal/redcart/application/service_ai.go` 与 `backend/internal/redcart/interfaces/httpapi` 增加 `/api/ai/a2ui` HTTP 入口。
- 在 `frontend/src/app.ts` 增加 `/a2ui` 页面与基础 A2UI 组件渲染器（Card/Column/Row/Text/Button）。
- 同步更新 `docs/api/openapi.yaml`、`docs/api/endpoint-table.md`、`docs/architecture.md`。

### 人工或主代理修正

- 将 A2UI 实现限定为最小可运行 MVP：后端按 RPC 契约返回 JSONL，前端只渲染基础目录组件，不实现完整 A2UI 校验、双向绑定与事件回传。
- 保持现有 `AIProvider` 契约风格，将 A2UI 作为同一生成服务的新能力而非独立服务。
- 在 `scripts/generate-ai-grpc.sh` 执行前补齐 `backend/.bin` 插件与 `ai-service/.venv` 依赖。

### 验证证据

```bash
rtk bash scripts/generate-ai-grpc.sh
rtk env GOCACHE=/tmp/go-build-cache go test ./internal/ai/grpc ./internal/ai ./internal/redcart/application ./internal/redcart/interfaces/httpapi
rtk env GOCACHE=/tmp/go-build-cache go test ./...
rtk bash -c "cd ai-service && .venv/bin/python -m unittest discover -s tests -v"
rtk cd frontend && npm run lint && npm run typecheck && npm run build && npm test
rtk bash scripts/check-openapi.sh
rtk bash scripts/validate-workspace.sh
```

### 剩余风险

- 前端 A2UI 渲染器仅覆盖基础目录组件，复杂组件（List/Tabs/Modal/输入组件/函数调用）尚未实现。
- A2UI surface 当前由 Mock Provider 按固定模板生成，未接入真实 LLM；真实接入时需要按 A2UI v0.9 目录构造 prompt 并做 JSON schema 校验。
- gRPC 仍使用 insecure 传输，跨网络部署需补充 TLS。

## 2026-06-13：A2UI 智能导购专题页

### AI 参与范围

- 在 `backend/internal/redcart/application/service_ai.go` 增加 A2UI 上下文增强逻辑：解析用户意图中的预算与场景，调用 `ListProducts` 查询在线商品，筛选预算内商品后注入 `context_json`。
- 升级 `backend/internal/ai/mock_provider.go` 与 `ai-service/app/provider.py`，当上下文包含 `products` 时生成智能导购专题 surface（Header、预算 Slider、商品 List + Card、加购 Button）。
- 扩展前端 A2UI 渲染器，新增 `List`/`Image`/`Slider` 组件支持、`formatString` 函数调用解析、列表项子作用域、Slider 双向绑定与 `add_to_cart` action 处理。
- 新增宿舍书桌场景演示商品（LED Desk Lamp、Desk Storage Box Set、USB Power Strip）及 SKU，同步更新内存与 PostgreSQL 种子数据。
- 新增 `TestAIHTTPA2UIShoppingGuide` HTTP 集成测试。

### 人工或主代理修正

- 保持 A2UI 契约不变，将导购逻辑收敛到应用层与 Provider 模板生成，不修改 proto 与 gRPC 接口。
- 预算解析采用简单正则 + 上下文显式值的 fallback，不引入 NLP 库；真实场景可替换为 LLM 意图解析。
- 前端 List 模板渲染只实现相对路径的子作用域，复杂函数调用与输入双向绑定仍按最小可用实现。

### 验证证据

```bash
rtk env GOCACHE=/tmp/go-build-cache go test ./...
rtk env GOCACHE=/tmp/go-build-cache go test ./internal/redcart/interfaces/httpapi -run TestAIHTTP -v
rtk bash -c "cd ai-service && .venv/bin/python -m unittest discover -s tests -v"
rtk cd frontend && npm run lint && npm run typecheck && npm run build && npm test
rtk bash scripts/check-openapi.sh
rtk bash scripts/validate-workspace.sh
```

### 剩余风险

- 导购页仍使用固定模板，未接入真实 LLM 做商品匹配与页面布局决策。
- 预算 Slider 仅做前端展示与本地数据模型更新，未真正回传后端重新筛选商品。
- 商品图片使用示例 URL，本地无真实图片资源。

## 2026-06-16：消息队列与事件驱动边界

### AI 参与范围

- 基于 `main` 最新提交创建独立 git worktree `feature/microservice-boundaries-mq`，与当前未提交的 MQ 工作并行。
- 撰写 `docs/adr/0006-message-queue-and-event-driven.md`，确定 RabbitMQ + 事务性发件箱（Transactional Outbox）方案，把订单状态变更事件发布到消息队列。
- 更新 `docs/architecture.md` 与 `docs/index.md`，记录事件与异步边界成为当前运行时架构的一部分。
- 新增 `backend/internal/event` 事件发布契约、`backend/internal/event/rabbitmq` RabbitMQ 适配器、`backend/internal/event/outbox` 发件箱发布器。
- 新增 `backend/migrations/0002_outbox.sql` 发件箱与死信表，并将迁移机制升级为按文件名顺序执行的版本化迁移（`schema_migrations` 表）。
- 在 PostgreSQL 与内存仓储中实现 `event.Outbox` / `event.OutboxStore`；Redis 包装器（CatalogCacheRepository、SessionRepository）透传 Append 到基础仓储。
- 在应用层订单服务（CreateOrder、PayOrder、CancelOrder、FinishOrder、RequestRefund）与商家服务（MerchantShipOrder、MerchantApproveRefund）中写入发件箱事件。
- 更新 `docker-compose.yml` 加入 `rabbitmq` 服务，并配置后端 `RABBITMQ_ADDR` / `RABBITMQ_EXCHANGE`。
- 更新 `README.md`、`.env.example`，补充事件驱动相关说明。
- 新增单元测试：`backend/internal/event/event_test.go`、`backend/internal/event/outbox/publisher_test.go`、`backend/internal/redcart/infrastructure/postgres/outbox_repository_test.go`。
- 新增 MQ 对照 benchmark：`backend/internal/redcart/application/benchmark_test.go` 中 `BenchmarkCreateOrderOutbox` 与 `BenchmarkCreateOrderSyncSideEffects` 对比，证明在模拟 3 个 500μs 下游调用时，发件箱模式吞吐提升约 365 倍。
- 更新 `.github/workflows/benchmark.yml`，把 MQ benchmark 纳入 CI 持续采集。

### 人工或主代理修正

- 调研公开 benchmark 后确认：SPECjms2007（已退役）和 OpenMessaging Benchmark Framework 都是 broker-centric，不评估「事务性发件箱 + 业务请求路径」的收益；因此手工构建控制变量 benchmark，并在 ADR / 测试文件顶部说明理由与引用。
- 保持核心交易路径仍为数据库事务强一致；消息队列只承担异步解耦，不引入 Saga/TCC。
- 订单创建等暂无法纳入 `UpdateOrderStatus` 事务路径的写事件，使用非事务发件箱写入，后续可扩展 `SaveOrderWithInventoryLocks` 的 side-effect 机制实现完全原子性。
- gRPC 仍使用 insecure 传输，跨网络部署需补充 TLS；RabbitMQ 当前也使用 plain AMQP。

### 验证证据

```bash
rtk bash scripts/git-worktree.sh create feature/microservice-boundaries-mq main
cd /tmp/agent-native-shop-feature-microservice-boundaries-mq
rtk env GOCACHE=/tmp/go-build-cache go test ./...
rtk env GOCACHE=/tmp/go-build-cache go test ./internal/redcart/application -run '^$' -bench 'BenchmarkCreateOrder' -benchtime=1s -benchmem -count=1
rtk bash scripts/check-openapi.sh
rtk bash scripts/validate-workspace.sh
```

### 剩余风险

- 尚未实现 RabbitMQ 消费者；通知、分析、库存等下游服务仍停留在规划阶段。
- 行为事件（`behavior.*`）尚未接入发件箱，仍直接写入 `behavior_events` 表。
- 死信队列目前只写到 `outbox_dead_letter` 表，没有自动重放或告警机制。
- 未引入服务发现、API 网关、链路追踪、Service Mesh；这些按 ADR 0005 继续后置。
