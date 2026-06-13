# 架构说明

这个仓库按稳定职责分层组织，而不是围绕某个具体框架堆代码。

## 分层

### 1. 产品接口层

用户直接接触的 API、页面、命令、任务入口。

典型位置：

- `apps/`
- `cmd/`
- `services/`
- `examples/`

### 2. 领域能力层

可复用的业务规则、数据结构、校验逻辑、纯能力模块。

典型位置：

- `core/`
- `packages/`
- `internal/domain/`

### 3. 运行编排层

负责业务流程编排、依赖注入、状态推进、任务调度。

典型位置：

- `runtime/`
- `internal/runtime/`
- `workers/`

### 4. 集成适配层

负责数据库、缓存、消息队列、模型 Provider、外部 API、框架适配。

典型位置：

- `integrations/`
- `adapters/`
- `infra/`

## 依赖方向

依赖应按下面的方向流动：

```text
产品接口层 -> 运行编排层 -> 领域能力层
产品接口层 -> 集成适配层
运行编排层 -> 集成适配层
集成适配层 -> 仅依赖领域契约
```

领域能力层不能反向依赖产品接口、运行时、供应商 SDK 或部署环境。

## 当前实现映射

RedCart Copilot 当前 MVP 的代码映射如下：

- 产品接口层：`backend/cmd/api`、`backend/internal/redcart/interfaces/httpapi`、`frontend/`
- 运行编排层：`backend/internal/redcart/application`
- 领域能力层：`backend/internal/order/domain`、`backend/internal/redcart/domain`
- 集成适配层：`backend/internal/redcart/infrastructure/postgres`、`backend/internal/redcart/infrastructure/memory`、`backend/internal/redcart/infrastructure/redis`、`backend/internal/ai`

当前运行时数据库是 PostgreSQL，运行时缓存/会话源是 Redis。后端启动必须同时提供 `POSTGRES_DSN` 与 `REDIS_ADDR`：PostgreSQL 仓储适配器负责迁移、种子数据和业务真相；Redis 读侧适配器包裹 PostgreSQL 仓储，认证 token 以 Redis 为共享会话源并带本地热缓存，商品、SKU 和 SKU 列表读路径优先命中 Redis。订单、库存、购物车和业务真相仍以 PostgreSQL 为准。内存仓储保留为服务层、HTTP 层测试和契约对齐用适配器，不作为当前 Docker Compose MVP 的运行时数据源。

HTTP 入口当前由 Gin 负责路由和 method gate，但 Gin 只停留在产品接口层；应用层和领域层不依赖 Gin 类型。AI 能力当前使用 Mock Provider，RabbitMQ 仍是规划中的适配目标，不是当前运行前置依赖。Redis 当前只落地 session 与 catalog 热读适配，不承载库存预扣、购物车、幂等真相或订单事件总线职责。

运行时性能分析当前支持可选的 Grafana Pyroscope Go push mode。接入点位于后端启动装配层，依赖环境变量启用，不向应用层或领域层泄漏供应商类型。

## 扩展方式

- 新增用户能力时，优先在产品接口层暴露公共入口，再调用稳定的运行编排或领域契约。
- 新增外部集成时，优先实现已有契约，再补最小可执行验证。
- 只有在规则具备清晰输入输出和复用价值时，才放进领域能力层。
- 只要某类维护路径会被重复执行，就补对应工作流文档。

## 非目标

- 不预设某个框架必须成为最终形态。
- 不在没有适配器实现前，把数据库、中间件或云厂商写死到领域层。
- 不在没有可复现命令和环境说明前，写性能结论。
- agent 路由文件不是架构事实源，架构事实以 `docs/` 为准。
