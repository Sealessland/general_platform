# RedCart Copilot

RedCart Copilot 是一个面向内容电商场景的 AI Native 全栈项目，用来展示从内容种草到交易履约、再到商家经营与 AI 提效的完整链路。

仓库按真实工程项目组织，包含接口契约、迁移脚本、测试、CI、ADR、AI 工作流记录和重构计划，目标不是堆功能，而是把业务主链路、工程边界和演进方式讲清楚、跑起来。

## 当前范围

- 消费者链路：笔记流、笔记详情、商品详情、购物车、结算预览、幂等下单、模拟支付、取消订单、确认收货、申请退款。
- 商家链路：商品管理、SKU 管理、上下架、订单履约、退款审批、经营看板。
- AI Copilot：商品卖点生成、经营复盘生成、任务记录查询。
- 工程链路：Issue/PR 约束、OpenAPI、迁移、测试、ADR、AI 工作流沉淀。

## 先读约束

- 项目约束：`docs/project-constraints.md`
- CI/CD 说明：`ci/README.md`

## 仓库结构

```text
backend/       Go API、领域模块、迁移、后端测试
frontend/      静态前端演示应用与前端校验脚本
ai-service/    AI 提示词与 Provider 占位实现
docs/          PRD、架构、ADR、测试与工作流文档
ci/            CI/CD 说明与质量检查脚本
.github/       CI、Issue 模板、PR 模板
scripts/       校验、OpenAPI 检查、本地启动辅助脚本
```

## 快速开始

先执行仓库基础校验：

```bash
bash scripts/validate-workspace.sh
bash scripts/check-openapi.sh
```

后端测试：

```bash
cd backend
GOCACHE=/tmp/go-build-cache go test ./...
```

前端检查与构建：

```bash
cd frontend
npm test
npm run lint
npm run typecheck
npm run build
```

推荐直接使用 Docker Compose 启动完整 MVP 环境：

```bash
bash scripts/local-dev.sh
```

`local-dev` 默认会同时启动本地 Pyroscope，后端会以 Go push mode 把 profile 发送到 `http://127.0.0.1:4040`。如果你想关闭 profiling，可在启动前把 `PYROSCOPE_SERVER_ADDRESS` 设为空值。

如果只单独启动后端 API：

```bash
cd backend
POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable HTTP_PORT=18080 GOCACHE=/tmp/go-build-cache go run ./cmd/api
```

如需启用 Grafana Pyroscope Go push mode，可额外提供这些环境变量：

```bash
PYROSCOPE_SERVER_ADDRESS=http://127.0.0.1:4040
PYROSCOPE_APPLICATION_NAME=redcart.backend
PYROSCOPE_BASIC_AUTH_USER=
PYROSCOPE_BASIC_AUTH_PASSWORD=
PYROSCOPE_TENANT_ID=
PYROSCOPE_MUTEX_PROFILE_FRACTION=
PYROSCOPE_BLOCK_PROFILE_RATE=
```

只配置 `PYROSCOPE_SERVER_ADDRESS` 即可启用默认的 CPU/alloc/inuse profiling；未配置时 profiling 默认关闭，不影响现有运行路径。如果需要诊断锁竞争或阻塞，可额外把 `PYROSCOPE_MUTEX_PROFILE_FRACTION` 和 `PYROSCOPE_BLOCK_PROFILE_RATE` 设置为正整数，例如 `5`，后端会启用对应的 mutex/block profile types。

构建后的前端位于：

```text
frontend/dist/index.html
```

如果需要本地起一个静态服务，可在 `frontend/dist` 下执行：

```bash
python3 -m http.server 4173 --bind 127.0.0.1
```

## 演示地址与账号

- 后端地址：`http://127.0.0.1:18080`
- 前端静态服务示例：`http://127.0.0.1:4173`
- PostgreSQL 本地端口：`127.0.0.1:15432`
- Pyroscope：`http://127.0.0.1:4040`

内置演示账号：

- 消费者：`13800000001 / consumer-demo`
- 商家：`13800000002 / merchant-demo`

## 当前实现说明

当前 MVP 运行时采用纯 PostgreSQL 环境：

- 后端启动时必须提供 `POSTGRES_DSN`
- 后端连接 PostgreSQL 后会自动执行初始化迁移与演示种子数据
- AI 能力使用可重复的 Mock Provider
- Redis、RabbitMQ 暂不作为运行时前置依赖

## 工程规则

- 默认从 `main` 或当前稳定开发分支拉任务分支，并为每条工作线创建独立 `git worktree`。
- 主工作区保持干净；性能 spike、中间件试验、QPS 对照和流程文档修改放到独立 worktree。
- 订单、库存、金额、权限、数据库结构相关改动必须带测试。
- 数据库结构变化必须更新 `backend/migrations/`。
- API 行为变化必须同步更新 `docs/api/openapi.yaml`。
- 架构边界变化必须同步 `docs/architecture.md` 或相关 ADR。
- AI 参与的设计、实现或修正过程必须记录到 `AI_WORKFLOW.md` 或 PR 描述中。

常用命令：

```bash
rtk bash scripts/git-worktree.sh create feature/my-task
rtk bash scripts/git-worktree.sh list
```

主工作区根目录会自动生成 `BRANCH_STATUS.local.md`，记录当前 worktree 状态和 AI 生成的更改大纲；这是本地状态板，不提交进仓库。

## 验证要求

提交前至少执行：

```bash
bash scripts/validate-workspace.sh
```

这个检查会验证：

- 关键入口文件是否存在
- 核心文档是否齐全
- 工作流文档是否写明完成证据
- OpenAPI、迁移和仓库基础结构是否完整
