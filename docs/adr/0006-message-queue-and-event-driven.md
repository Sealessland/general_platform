# ADR 0006：消息队列与事件驱动边界

## 状态

已接受

## 背景

当前 RedCart Copilot 的后端是模块化单体，订单、库存、购物车、用户/商家认证等核心模块共享 PostgreSQL 事务。系统里已经存在两类事件记录：

- `order_events`：订单状态变更的领域事件，随 `UpdateOrderStatus` 的 side-effect 事务原子写入。
- `behavior_events`：用户行为事件，用于商家看板和经营复盘。

这些事件目前只作为数据库表存在，没有被发布到外部，导致：

1. 行为分析、通知、库存同步、商家看板等能力只能直接读库或进程内调用。
2. 未来如果要拆分独立服务（通知服务、分析服务、库存服务），必须依赖同步 RPC 或数据库共享。
3. 订单状态变更后的异步动作（发送通知、更新搜索索引、触发 AI 复盘）与主交易路径耦合。

因此需要引入消息队列，把「事件产生」和「事件消费」解耦，同时不破坏现有强事务边界。

## 决策

### 1. 消息队列选型：RabbitMQ

- 当前 MVP 以 Docker Compose 为运行时事实源，RabbitMQ 镜像成熟、启动快、本地开发友好。
- 项目README和 ADR 0005 已把 RabbitMQ 列为规划中的适配目标。
- 事件量在当前阶段不大，RabbitMQ 的 topic / direct routing 足够支撑订单、行为两类事件。
- 保留将来替换为 NATS / Kafka 的可能性：事件发布抽象不依赖具体 MQ SDK。

### 2. 发布模式：事务性发件箱（Transactional Outbox）

- 订单事件继续随订单状态变更在同一个数据库事务中写入 `order_events` 表。
- 新增 `outbox` 表，作为「需要被发布到 MQ 的事件」的可靠缓冲区。
- side-effect 中不再直接调用 MQ，而是把事件写入 `outbox` 表，保证**业务状态变更与事件记录原子一致**。
- 独立的 `outbox` 发布器定时轮询 `outbox` 表，将事件发送到 RabbitMQ，成功后删除或标记为已发布。
- 行为事件同样先写 `behavior_events` 表，再同步写 `outbox` 表（同一事务）。

### 3. 事件分类与主题

| 主题（Topic / Routing Key） | 事件类型 | 生产者 | 消费者（规划） |
|---|---|---|---|
| `order.created` | `ORDER_CREATED` | 订单服务 | 通知服务、分析服务、搜索服务 |
| `order.paid` | `ORDER_PAID` | 订单服务 | 库存确认、通知服务、AI 复盘 |
| `order.cancelled` | `ORDER_CANCELLED` | 订单服务 | 库存释放、通知服务 |
| `order.shipped` | `ORDER_SHIPPED` | 订单服务 | 通知服务 |
| `order.finished` | `ORDER_FINISHED` | 订单服务 | 积分/会员服务、AI 复盘 |
| `order.refund_requested` | `ORDER_REFUND_REQUESTED` | 订单服务 | 风控/客服服务 |
| `behavior.order_create` | `BehaviorOrderCreate` | 订单/行为服务 | 商家看板、AI 复盘 |
| `behavior.order_pay` | `BehaviorOrderPay` | 订单/行为服务 | 商家看板 |
| `behavior.note_view` | `BehaviorNoteView` | 笔记服务 | 推荐服务、商家看板 |
| `behavior.product_click` | `BehaviorProductClick` | 商品服务 | 推荐服务 |
| `behavior.add_to_cart` | `BehaviorAddToCart` | 购物车服务 | 推荐服务、商家看板 |

> 当前阶段优先实现 `order.*` 事件发布；`behavior.*` 事件作为第二条线，保留表结构但可按同样模式扩展。

### 4. 服务边界：单体内部先按「逻辑服务」拆分

在真正拆进程之前，先把 MQ 事件作为服务间边界：

- **订单服务（Order Service）**：仍驻留在 `backend/internal/order` 与 `backend/internal/redcart/application/service_order.go`，但只发布事件，不直接调用分析/通知。
- **通知服务（Notification Service）**：新引入的独立逻辑消费者，监听 `order.*` 事件，未来可拆成独立进程。
- **分析服务（Analytics Service）**：消费 `behavior.*` 和 `order.*` 事件，为商家看板提供数据。
- **库存服务（Inventory Service）**：当前库存仍由订单服务在同一事务内处理；未来当库存需要独立扩缩容时，再基于 `order.paid` / `order.cancelled` 事件拆出。

### 5. 协议与序列化

- MQ 消息体使用 JSON，便于调试和前端/AI 服务消费。
- 每条消息包含：
  - `event_id`：发件箱表主键，用于幂等。
  - `event_type`：事件类型。
  - `occurred_at`：事件发生时间。
  - `payload`：领域相关负载。
  - `correlation_id`：可选，用于链路追踪占位。

### 6. 错误与重试

- 发布器失败按指数退避重试，最大重试次数可配置。
- 超过最大重试次数的事件进入死信队列（DLQ）或 `outbox_dead_letter` 表，等待人工/补偿处理。
- 消费者处理失败时消息不确认（nack），由 RabbitMQ 重新投递；达到重试上限后进入死信队列。

## 性能证据

### 为什么采用手工构建的 benchmark

调研后发现，目前没有直接评估「事务性发件箱 + 业务服务请求路径」的公开可信测试集：

- **SPECjms2007** 是 JMS/MOM 的行业标准 benchmark，但已于 2016 年退役；它评估的是消息中间件本身（供应链场景），不包含发件箱模式，也不把业务写入与事件写入放在同一个数据库事务内评估。
- **OpenMessaging Benchmark Framework** 支持 RabbitMQ、Kafka、Pulsar 等，但属于 broker-centric 测试，关注吞吐、延迟、稳定性，不模拟「订单 API 内同步执行下游副作用」这一典型电商场景。

因此，我们在 `backend/internal/redcart/application` 中手工构建了一个**控制变量**的对照 benchmark：业务逻辑完全相同，唯一变量是下游副作用发生在请求路径内（同步）还是通过发件箱异步处理。

### 测试设计

- `BenchmarkCreateOrderOutbox`：订单创建时只把事件写入事务性发件箱，下游副作用异步处理。
- `BenchmarkCreateOrderSyncSideEffects`：订单创建时同步模拟三个轻量下游调用（通知 + 分析 + 搜索索引），每个调用 500μs。500μs 代表同区域轻量 RPC / HTTP 通知 / 分析刷盘的典型耗时。

### 本地基准结果（Go 1.23，8 核 Intel i7-1185G7）

| Benchmark | QPS | ns/op | B/op | allocs/op |
|---|---|---|---|---|
| `BenchmarkCreateOrderOutbox` | ~168.7K | 5927 | 5269 | 53 |
| `BenchmarkCreateOrderSyncSideEffects` | ~462 | 2166098 | 3773 | 27 |

当请求路径中存在毫秒级下游调用时，发件箱模式把创建订单的吞吐提升了约 **365 倍**，而业务状态变更与事件记录仍在同一个数据库事务内保持原子。该测试会随 CI benchmark workflow 持续运行，结果写入 README 性能表格。

## 影响

- 订单等核心模块继续通过数据库事务保证一致性；MQ 只承担异步解耦，不承担分布式事务协调。
- 需要新增 `outbox` 表和发布器，增加一个后台 goroutine。
- Docker Compose 中新增 `rabbitmq` 服务，并更新健康检查。
- 新增 `backend/internal/mq` 或 `backend/internal/event` 抽象层，领域层只依赖契约，不依赖 RabbitMQ SDK。
- AI 复盘、商家看板等能力未来可以只订阅事件，不再直接查询订单/行为表。
- 当前阶段不引入服务发现、API 网关、Service Mesh；这些在 ADR 0005 中已有结论，保持后置。

## 非目标

- 不把订单、库存、购物车立刻拆成独立进程。
- 不引入 Saga 或 TCC 等分布式事务协议；强一致性仍由数据库事务保证。
- 不一次性实现所有 `behavior.*` 主题；先完成 `order.*` 事件链路，再按需扩展。
