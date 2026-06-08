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
