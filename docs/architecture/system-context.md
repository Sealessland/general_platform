# 系统上下文

RedCart Copilot 面向四类直接可见的使用面：

- 消费者购物链路：笔记、商品、购物车、结算、订单、退款
- 商家经营链路：商品、SKU、库存、订单、看板
- AI Copilot 链路：卖点生成、经营复盘、任务记录
- 工程协作链路：任务、PR、CI、测试、迁移、ADR、AI 工作流记录

## 运行组件

- `gateway`：Nginx 统一入口，将请求分发到两个 Go API 实例
- `backend` / `backend-replica`：运行相同业务模块的 Go API 实例
- `frontend`：消费者与商家共用的前端演示应用
- `ai-service`：提示词与 AI Provider 占位实现
- `postgresql`：当前 MVP 的运行时业务数据源
- `redis`：一次性 Refresh 会话、Access JWT 撤销状态、商品/SKU 热读缓存与 Lua 令牌桶限流；不保存订单、库存和幂等真相
- `kafka`：事务性发件箱的发布目标，为后续通知与分析消费者保留可重放事件日志

## 当前 MVP 的运行边界

当前可执行 MVP 明确保持了分层边界：

- 产品接口层：`backend/cmd/api`、Gin HTTP 适配器 `backend/internal/redcart/interfaces/httpapi` 和 `frontend/`
- 运行编排层：`backend/internal/redcart/application`
- 领域能力层：`backend/internal/order/domain` 与 `backend/internal/redcart/domain`
- 集成适配层：PostgreSQL 仓储 `backend/internal/redcart/infrastructure/postgres`、Redis 适配器 `backend/internal/redcart/infrastructure/redis`、JWT 适配器 `backend/internal/redcart/infrastructure/auth`、迁移 `backend/migrations/` 与 AI Provider `backend/internal/ai`

后端运行时必须提供 `POSTGRES_DSN`、`REDIS_ADDR` 与 `JWT_SECRET`。PostgreSQL 仓储在启动时负责初始化迁移和演示种子数据，并在订单创建路径中用事务和条件更新完成库存预锁；多实例启动时，同一迁移版本由 transaction advisory lock 串行保护。仓储层不再保留内存测试适配器，服务层、HTTP 层和性能验证必须使用 PostgreSQL/Redis/Kafka 或真实运行中的 HTTP 服务作为证据来源。

Access JWT 由每个实例使用相同密钥本地验签；Redis 只协调 Refresh Token 单次轮换、Access `jti` 撤销、商品与 SKU 缓存和 Lua 原子限流，因此跨实例刷新、登出与流量配额保持一致。订单、库存、购物车和幂等真相仍然保留在 PostgreSQL。Kafka 作为事务性发件箱发布目标接入，应用层只依赖中立事件契约。相关能力不能把 JWT/Redis/Kafka SDK 类型混进领域逻辑。

## 核心数据流

1. 消费者浏览笔记与商品
2. 行为事件被记录
3. 消费者加购 SKU
4. 结算预览校验库存与金额
5. 下单时携带幂等键并创建库存锁
6. 支付成功后订单进入已支付状态
7. 订单与行为事件汇总到商家看板
8. AI Copilot 基于商品与经营数据返回草案建议
