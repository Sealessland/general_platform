# RedCart Copilot 展示介绍

> 面向第一次接触项目的人：5 分钟看懂「这是什么、为什么有趣、怎么体验」。
> 所有事实均来自仓库真实文件与代码，命令可直接执行；详细入口见 [docs/index.md](../docs/index.md)。

## 一句话定位

**RedCart Copilot 是一个「内容电商 + AI Copilot」的 AI Native 全栈示例项目**，把「内容种草 → 交易履约 → 商家经营 → AI 提效」的完整链路做成一个可运行、可演示、可解释的 MVP：后端 Go 单体 + PostgreSQL/Redis/RabbitMQ 运行时，前端无框架静态演示应用，AI 服务走 gRPC 契约并提供 A2UI 声明式界面。

项目不是功能堆砌，而是把**业务主链路、工程边界、AI 协作方式**讲清楚、跑起来——仓库本身就是 agent-native 工作流的产物：契约先行、可执行验证优先、AI 参与全程留痕。

## 展示页面与运行 demo

| 文件 | 内容 | 打开方式 |
|---|---|---|
| [index.html](./index.html) | 静态展示页：项目概览、架构、功能与关键数字的可视化介绍 | 直接用浏览器打开，或在 `showcase/` 下起静态服务后访问（见下） |
| [demo.html](./demo.html) | 可运行 demo：交互式体验消费者/商家/AI 业务路径（内置 mock 数据，复刻真实业务口径） | 零依赖，直接用浏览器打开即可运行，无需启动后端 |

本地静态预览两种方式任选其一：

```bash
# 方式一：直接双击 index.html / demo.html（两个文件均为自包含单文件，无后端依赖）
# 方式二：在 showcase/ 目录起静态服务后访问
cd showcase
python3 -m http.server 4173 --bind 127.0.0.1
# 浏览器打开 http://127.0.0.1:4173/index.html 与 http://127.0.0.1:4173/demo.html
```

> 截图占位：此处可放置 index.html 展示页截图与 demo.html 运行截图。

## 核心概念

### agent-native（以 AI 为原生参与者）

这套方法论把 AI agent 视为与 UI 用户对等的一等公民，设计原则包括：完整工具对等（full tool parity）、原子原语（atomic primitives）、显式完成信号（explicit completion signals）。体现在本仓库：

- 仓库按 agent-native 工作流组织：`AGENTS.md` 给出操作规则，`AI_WORKFLOW.md` 记录每次 AI 参与的范围、人工修正与验证证据；
- 稳定事实统一落在 `docs/`，agent 路由文件只负责指路，不重复维护事实（见 [docs/index.md](../docs/index.md)）；
- 每个功能、集成、调试、验证都有对应工作流文档（`docs/workflows/*`），并强制"可执行验证优先于文字说明"。

### A2UI（Agent-to-UI）

后端 AI 服务按 **A2UI v0.9** 协议返回声明式 UI surface JSON，前端 `/a2ui` 页面负责渲染。仓库实现了基础目录组件（Card/Column/Row/Text/Button）与增强组件（List/Image/Slider、函数调用、Slider 双向绑定与 `add_to_cart` action），并做了「智能导购专题页」演示：解析用户预算与场景意图，筛选在线商品注入上下文，生成导购页面。接口契约见 `api/proto/ai/v1/ai.proto`，HTTP 入口为 `POST /api/ai/a2ui`，契约与实现记录见 [AI_WORKFLOW.md](../AI_WORKFLOW.md)。

### 工具对等（tool parity）

AI Copilot 与前端共享**同一套后端能力入口**：卖点生成、经营复盘、A2UI 导购都通过 `backend/internal/ai.AIProvider` 契约接入，进程内 Mock Provider 与独立 gRPC `ai-service`（`AI_PROVIDER=grpc`）实现同一契约（见 [docs/architecture.md](../docs/architecture.md)）。代理与页面用户走同一条业务链路，AI 不绕过订单、库存、权限等既有规则。

## 架构一图流

```mermaid
graph TD
    FE["前端 frontend/（静态演示应用）<br/>内容流 · 购物车 · 订单 · 经营看板 · /a2ui"] -->|HTTP :18080| API["后端 API backend/cmd/api<br/>Gin 路由与 method gate（产品接口层）"]
    API --> APP["应用层 application<br/>下单 · 支付 · 退款 · 商家 · AI 编排"]
    APP --> DOM["领域层 domain<br/>订单状态机 · 库存锁 · 金额规则"]
    API --> PG[("PostgreSQL :15432<br/>业务真相 · 迁移 · 种子数据")]
    API --> RD[("Redis :6380<br/>session 会话源 · 读侧热缓存")]
    APP --> MQ[("RabbitMQ :5672<br/>事务性发件箱 → 订单事件")]
    API --> AI["AI 服务 ai-service（gRPC :50051）<br/>卖点 · 经营复盘 · A2UI surface"]
    API --> PROM["Prometheus :9090<br/>/metrics 指标采集"]
    API --> PYRO["Pyroscope :4040<br/>Go push mode 性能分析"]
```

依赖方向约定（出处：[docs/architecture.md](../docs/architecture.md)）：

```text
产品接口层 -> 运行编排层 -> 领域能力层
产品接口层 -> 集成适配层
集成适配层 -> 仅依赖领域契约
```

要点：

- 订单、库存、购物车等交易真相在 PostgreSQL，事务边界不因 Redis 缓存命中而放宽；
- Redis 只负责 session 会话源与商品/SKU 读侧热缓存，是**读侧增益**不是写侧增益；
- RabbitMQ 承担异步解耦：订单状态变更通过事务性发件箱（Transactional Outbox）发布，核心交易仍保持数据库事务强一致（[ADR 0006](../docs/adr/0006-message-queue-and-event-driven.md)）；
- 领域层不依赖 Gin、GORM、供应商 SDK 或部署环境，集成细节全部收敛到适配层。

## 功能清单（与代码现状一致）

- **消费者**：笔记流、笔记详情、商品详情、购物车、结算预览、幂等下单、模拟支付、取消订单、确认收货、申请退款。
- **商家**：商品管理、SKU 管理、上下架、订单履约、退款审批、经营看板（漏斗/商品/汇总，进程内读 PostgreSQL 计算）。
- **AI Copilot**：商品卖点生成、经营复盘生成、AI 任务记录查询、A2UI 智能导购专题页。
- **工程链路**：Issue/PR 约束、OpenAPI 契约、版本化迁移、分层测试、ADR、AI 工作流沉淀、CI 门禁。

前端演示应用（`frontend/src/app.ts`）提供 hash 路由页面：`/notes` 内容流、`/orders` 我的订单、`/a2ui` A2UI 导购、`/merchant/dashboard` 经营看板、`/merchant/orders` 订单履约。

## 技术栈

| 层 | 选型 | 出处 |
|---|---|---|
| 后端 | Go 1.25 + Gin + GORM + gRPC | `backend/go.mod` |
| 数据库 | PostgreSQL 16（业务真相、迁移、种子数据） | `docker-compose.yml` |
| 缓存/会话 | Redis 7（session 会话源、读侧热缓存） | `docker-compose.yml` |
| 消息队列 | RabbitMQ 4（事务性发件箱发布订单事件） | `docker-compose.yml` |
| AI 服务 | Python + grpcio，Mock Provider（可替换为真实模型） | `ai-service/requirements.txt` |
| 前端 | 原生 TypeScript 单文件 demo，零运行时依赖，自定义 node 校验脚本 | `frontend/package.json` |
| 可观测 | Prometheus（`/metrics`）+ Grafana、Grafana Pyroscope（Go push mode） | `prometheus.yml`、`docker-compose.yml` |
| 契约 | OpenAPI（`docs/api/openapi.yaml`）、protobuf（`api/proto/ai/v1/ai.proto`） | `docs/api/endpoint-table.md` |

## 本地快速开始

基础校验（必跑，约数秒）：

```bash
bash scripts/validate-workspace.sh
bash scripts/check-openapi.sh
```

### 方式一：Docker Compose 一键启动（推荐）

```bash
bash scripts/local-dev.sh
```

该命令启动 `postgres / redis / pyroscope / backend / frontend`；`backend` 的 `depends_on` 会自动连带启动 `rabbitmq` 与 `ai-service`。如需完整可观测栈，可再补：

```bash
docker compose up -d prometheus grafana
```

启动后地址与演示账号：

- 后端 API：`http://127.0.0.1:18080`，健康检查 `GET /healthz`
- 前端演示：`http://127.0.0.1:4173`
- PostgreSQL：`127.0.0.1:15432`（库 `redcart`，用户/密码 `postgres/postgres`）
- Redis：`127.0.0.1:6380`；RabbitMQ：`amqp://redcart:redcart@127.0.0.1:5672/`
- Pyroscope：`http://127.0.0.1:4040`；Prometheus：`http://127.0.0.1:9090`
- 演示账号：消费者 `13800000001 / consumer-demo`；商家 `13800000002 / merchant-demo`

### 方式二：分服务启动

先起依赖容器，再分别启动后端与前端：

```bash
docker compose up -d postgres redis pyroscope

# 后端（需显式提供 POSTGRES_DSN 与 REDIS_ADDR）
cd backend
POSTGRES_DSN=postgres://postgres:postgres@127.0.0.1:15432/redcart?sslmode=disable \
REDIS_ADDR=127.0.0.1:6380 HTTP_PORT=18080 \
GOCACHE=/tmp/go-build-cache go run ./cmd/api

# 前端构建产物（另开终端）
cd frontend/dist
python3 -m http.server 4173 --bind 127.0.0.1
```

### 本地测试与构建

```bash
# 后端测试
cd backend
GOCACHE=/tmp/go-build-cache go test ./...

# 前端检查与构建
cd frontend
npm test
npm run lint
npm run typecheck
npm run build
```

## 文档地图

完整文档索引在 [docs/index.md](../docs/index.md)，本文不复制其内容，仅指路重点：

- **项目与约束**：[README.md](../README.md)、[docs/project-constraints.md](../docs/project-constraints.md)
- **架构**：[docs/architecture.md](../docs/architecture.md)（分层与依赖方向）、[订单状态机](../docs/architecture/order-state-machine.md)
- **API 契约**：[docs/api/openapi.yaml](../docs/api/openapi.yaml)、[docs/api/endpoint-table.md](../docs/api/endpoint-table.md)
- **测试**：[docs/testing/test-strategy.md](../docs/testing/test-strategy.md)、[docs/testing/e2e-cases.md](../docs/testing/e2e-cases.md)、[性能基线](../docs/testing/performance-baseline.md)
- **架构决策（ADR）**：[ADR 0006 消息队列与事件驱动](../docs/adr/0006-message-queue-and-event-driven.md) 等，见 `docs/adr/`
- **AI 协作**：[AI_WORKFLOW.md](../AI_WORKFLOW.md)、[AGENTS.md](../AGENTS.md)

## 工程亮点

### CI 门禁（[ci/README.md](../ci/README.md)）

- `.github/workflows/ci.yml` 是 PR 与 `main` push 的统一门禁入口；子 workflow（后端/前端/AI/安全/Docker）只保留 `workflow_call` 与 `workflow_dispatch`，可复用可手动触发，避免同一事件重复跑。
- 后端 CI 连真实 PostgreSQL 与 Redis 运行，并产出 QPS 与 benchmark 产物到 `ci/artifacts/`（如 `backend-postgres-http-qps.txt`、`backend-rabbitmq-qps.txt`）；内存仓储 benchmark 不允许进入 CI 产物或性能表。
- 覆盖门禁由 [backend-test-metrics.sh](../ci/scripts/backend-test-metrics.sh) 固化：总覆盖率 ≥ 65.0%、应用层 ≥ 80.0%、AI 包 ≥ 95.0%、后端测试数量 ≥ 55 等，阻断测试规模与关键包覆盖率回退。

### 验证脚本与测试策略（[docs/testing/test-strategy.md](../docs/testing/test-strategy.md)）

- 仓库级结构门禁 `scripts/validate-workspace.sh`（转发到 `ci/scripts/validate-workspace.sh`）：必需文件清单、核心文档内容冒烟、Codex 项目 hook 自检、密钥扫描。
- 分层测试：领域层（订单状态机/金额/库存）→ 应用层 → HTTP 层 → PostgreSQL-backed HTTP 集成测试（`RUN_POSTGRES_INTEGRATION=1` 时走真实 Gin → 应用层 → PostgreSQL/GORM 路径）。
- 高风险场景专门覆盖：并发下单库存不超卖（200 并发抢 50 库存）、并发支付不重复扣减、越权与错误 method 无副作用、取消/退款库存恢复、非法状态流转。
- 性能基线只认真实运行路径：PostgreSQL-backed benchmark（`OrderPreview` / `CreateOrder`）与真实 RabbitMQ 的 outbox relay benchmark；前端当前是源码守卫与构建检查，未引入浏览器级 E2E（见「已知测试边界」）。

### 可复现、可解释的开发流

- 默认 `git worktree` 隔离每条工作线（[scripts/git-worktree.sh](../scripts/git-worktree.sh)），主工作区保持干净；
- 每次 AI 参与都有「范围 / 人工修正 / 验证证据 / 剩余风险」记录（[AI_WORKFLOW.md](../AI_WORKFLOW.md)），任何"已通过"都必须有跑过的命令佐证；
- 变更自动同步 `CHANGELOG.md`，架构边界变化必须同步 `docs/architecture.md` 或 ADR。
