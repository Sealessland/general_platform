# 更新记录

所有对 RedCart Copilot 有意义的变更都记录在这里。

## [未发布] - 2026-06-08

### 优化

- 将 `git worktree` 升级为项目默认协作工作流，并增加 `scripts/git-worktree.sh` 与自动生成的 `BRANCH_STATUS.local.md` 本地状态板；Codex hook 会在相关 Git 操作后同步 worktree 状态和 AI 更改大纲。
- 增加 `.codex/skills/git-worktree/` 项目级 skill，供 agent 在需要多分支并行开发或分支隔离时引用。
- 仅在 PostgreSQL 适配层优化写路径：将运行时 SQL 调用切到 `database/sql`，用 `INSERT/UPDATE ... RETURNING` 取代写后回读，并让订单写入阶段直接回填 `order_items.id` 与时间戳，减少下单热路径的数据库往返和对象分配。
- 将 PR 与 `main` push 的统一门禁收敛到 `.github/workflows/ci.yml`。
- 将后端、前端、AI service、安全和 Docker 子 workflow 保留为复用与手动运行入口。
- 移除后端 CI 中未使用的 Redis service，保留当前 PostgreSQL 运行时依赖。
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
