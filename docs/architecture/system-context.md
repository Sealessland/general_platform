# 系统上下文

RedCart Copilot 面向四类直接可见的使用面：

- 消费者购物链路：笔记、商品、购物车、结算、订单、退款
- 商家经营链路：商品、SKU、库存、订单、看板
- AI Copilot 链路：卖点生成、经营复盘、任务记录
- 工程协作链路：任务、PR、CI、测试、迁移、ADR、AI 工作流记录

## 运行组件

- `backend`：Go API 服务与业务模块
- `frontend`：消费者与商家共用的前端演示应用
- `ai-service`：提示词与 AI Provider 占位实现
- `postgresql`：当前 MVP 的运行时业务数据源
- `redis`：当前用于认证 session 共享与商品/SKU 热读缓存；购物车、库存预扣、幂等仍是后续适配目标
- `rabbitmq`：规划中的订单、库存、分析事件总线适配目标

## 当前 MVP 的运行边界

当前可执行 MVP 明确保持了分层边界：

- 产品接口层：`backend/cmd/api`、Gin HTTP 适配器 `backend/internal/redcart/interfaces/httpapi` 和 `frontend/`
- 运行编排层：`backend/internal/redcart/application`
- 领域能力层：`backend/internal/order/domain` 与 `backend/internal/redcart/domain`
- 集成适配层：PostgreSQL 仓储 `backend/internal/redcart/infrastructure/postgres`、迁移 `backend/migrations/`、内存测试仓储 `backend/internal/redcart/infrastructure/memory` 与 Mock AI Provider `backend/internal/ai`

后端运行时必须提供 `POSTGRES_DSN`。PostgreSQL 仓储在启动时负责初始化迁移和演示种子数据，并在订单创建路径中用事务和条件更新完成库存预锁。内存仓储只用于测试和契约对齐，不是当前 Docker Compose MVP 的运行时数据源。

Redis 当前以可选读侧适配器方式接入：session 以 Redis 为共享真相源，商品与 SKU 读路径可命中 Redis 缓存，但订单、库存、购物车和幂等真相仍然保留在 PostgreSQL。RabbitMQ 尚未作为运行前置依赖接入。相关能力仍通过适配层边界保留扩展点，不能把中间件细节混进领域逻辑。

## 核心数据流

1. 消费者浏览笔记与商品
2. 行为事件被记录
3. 消费者加购 SKU
4. 结算预览校验库存与金额
5. 下单时携带幂等键并创建库存锁
6. 支付成功后订单进入已支付状态
7. 订单与行为事件汇总到商家看板
8. AI Copilot 基于商品与经营数据返回草案建议
