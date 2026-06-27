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
- 独立的 `outbox` 发布器定时轮询 `outbox` 表，将事件发送到 RabbitMQ。已发布事件通过 `published_at` 列软标记（不删除），保留审计轨迹。
- 轮询使用 `SELECT ... FOR UPDATE SKIP LOCKED` 在事务内加行锁，确保多个 relay 实例并发运行时不会重复发布同一事件。
- 每个 relay 周期是一个原子事务：`BeginTx → PollPendingInTx → 逐条发布 → MarkPublishedInTx / MarkFailedInTx → Commit`；发布失败时整个事务回滚，事件留在 outbox 等待下一轮重试。
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

- RabbitMQ publisher 启用 **publisher confirm 模式**：每条消息调用 `PublishWithDeferredConfirmWithContext` 并 `WaitContext` 等待 broker 确认，未确认或 NACK 时标记为发布失败。
- publisher 通过 `NotifyClose` 监听 channel/connection 断开，自动以 3 秒间隔重连，重连期间 `sync.Mutex` 保护 conn/channel 防止并发 publish 写入已关闭资源。
- 发布器失败后在同一事务内调用 `MarkFailedInTx`，递增 `retry_count`；超过最大重试次数（当前 5 次）的事件移动到 `outbox_dead_letter` 表，等待人工/补偿处理。
- 消费者侧错误处理见第 7 节。

### 7. 消费侧可靠性

- **手动 ack**：消费者以 `autoAck=false` 消费，处理成功后才 `Ack`；保证**至少一次**交付语义（at-least-once）。进程崩溃或网络中断时未 ack 的消息会被 RabbitMQ 重新投递。
- **QoS prefetch**：`channel.Qos(prefetch, 0, false)` 限制每个消费者未 ack 消息的上限（默认 10），防止慢 handler 被大量 in-flight 消息压垮。
- **幂等去重**：RabbitMQ 的至少一次语义意味着消息可能被重复投递（redelivery）。消费者使用 outbox `event_id` 做幂等检查：处理前调用 `Deduplicator.IsDuplicate`，处理后调用 `MarkProcessed`。`MarkProcessed` 必须在 `Ack` 之前完成——如果进程在 Ack 后、MarkProcessed 前崩溃，redelivery 会跳过已处理的副作用；反过来则可能执行两次。
- **死信队列（DLX）**：主队列声明 `x-dead-letter-exchange` 参数，指向 `redcart.events.dlx` fanout exchange。handler 返回错误时调用 `Nack(requeue=false)`，消息进入死信队列 `redcart.events.dlq`，等待人工/补偿处理。decode 失败同样进 DLX（消息格式错误，重试无意义）。dedup 检查失败（如 Redis 不可用）时 `Nack(requeue=true)` 重新入队，稍后重试——这是瞬态错误而非永久错误。
- 去重实现当前为进程内 `MemoryDeduplicator`（demo 用），生产环境应替换为 Redis `SETNX` 或数据库去重表，以在消费者重启后保持幂等性。

## 性能证据

性能证据只接受真实依赖路径，不再使用内存 outbox、空 publisher 或手工 sleep 模拟下游副作用作为吞吐来源。

当前保留两类 benchmark：

- `BenchmarkHTTPPostgresCreateOrderWithOutbox`：通过 Gin handler 进入应用层，使用 PostgreSQL 仓储创建订单，并在事务性发件箱中写入事件。benchmark 结束后会检查 PostgreSQL outbox 表中待发布事件数量。
- `BenchmarkPostgresRabbitMQOutboxRelay`：先向 PostgreSQL outbox 表写入真实事件，再用 outbox relay 通过 RabbitMQ publisher 发布并标记已发布。

README 性能表只由 GitHub Actions 的 benchmark workflow 更新，白名单限定为 PostgreSQL/Redis/RabbitMQ-backed 组件 benchmark 和 live HTTP benchmark。任何内存仓储、空 publisher 或模拟延迟 benchmark 结果都会被脚本拒绝。

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
