# ADR 0006：消息队列与事件驱动边界

## 状态

已接受

## 背景

当前 RedCart Copilot 的后端是模块化单体，订单、库存、购物车、用户/商家认证等核心模块共享 PostgreSQL 事务。系统里已经存在两类事件记录：

- `order_events`：订单状态变更的领域事件，随 `UpdateOrderStatus` 的 side-effect 事务原子写入。
- `behavior_events`：用户行为事件，用于商家看板和经营复盘。

这些事件如果只留在数据库表中，会导致：

1. 行为分析、通知、库存同步、商家看板等能力只能直接读库或进程内调用。
2. 未来如果要拆分独立服务（通知服务、分析服务、库存服务），必须依赖同步 RPC 或数据库共享。
3. 订单状态变更后的异步动作（发送通知、更新搜索索引、触发 AI 复盘）与主交易路径耦合。

因此需要引入消息队列，把「事件产生」和「事件消费」解耦，同时不破坏现有强事务边界。

## 决策

### 1. 消息队列选型：Kafka

- 当前 MVP 以 Docker Compose 为运行时事实源，Kafka 通过 `apache/kafka` KRaft 单节点模式运行，不依赖 ZooKeeper。
- 项目事件天然按 topic 分发：`order.paid`、`behavior.note_view` 等逻辑 topic 可直接映射为 `KAFKA_TOPIC_PREFIX` 前缀下的 Kafka topic。
- Kafka 的 consumer group、显式 offset commit 和 topic 级持久化更贴近后续通知、分析、库存等独立消费者扩展路径。
- 事件发布抽象仍保持 broker-neutral：应用层只依赖 `backend/internal/event.Publisher`，Kafka SDK 类型只出现在集成适配层 `backend/internal/event/kafka`。

### 2. 发布模式：事务性发件箱（Transactional Outbox）

- 订单事件继续随订单状态变更在同一个数据库事务中写入 `order_events` 表。
- `outbox` 表作为「需要被发布到 MQ 的事件」的可靠缓冲区。
- side-effect 中不直接调用 MQ，而是把事件写入 `outbox` 表，保证**业务状态变更与事件记录原子一致**。
- 独立的 `outbox` relay 定时轮询 `outbox` 表，将事件发送到 Kafka。已发布事件通过 `published_at` 列软标记（不删除），保留审计轨迹。
- 轮询使用 `SELECT ... FOR UPDATE SKIP LOCKED` 在事务内加行锁，确保多个 relay 实例并发运行时不会重复发布同一事件。
- 每个 relay 周期是一个原子事务：`BeginTx → PollPendingInTx → 逐条发布 → MarkPublishedInTx / MarkFailedInTx → Commit`；发布失败时整个事务回滚，事件留在 outbox 等待下一轮重试。
- 行为事件同样先写 `behavior_events` 表，再同步写 `outbox` 表（同一事务）。

### 3. 事件分类与 Kafka topic

运行时 Kafka topic 由 `KAFKA_TOPIC_PREFIX`（默认 `redcart.events`）和逻辑 topic 组成，例如 `redcart.events.order.paid`。

| 逻辑 topic | Kafka topic 示例 | 事件类型 | 生产者 | 消费者（规划） |
|---|---|---|---|---|
| `order.created` | `redcart.events.order.created` | `ORDER_CREATED` | 订单服务 | 通知服务、分析服务、搜索服务 |
| `order.paid` | `redcart.events.order.paid` | `ORDER_PAID` | 订单服务 | 库存确认、通知服务、AI 复盘 |
| `order.cancelled` | `redcart.events.order.cancelled` | `ORDER_CANCELLED` | 订单服务 | 库存释放、通知服务 |
| `order.shipped` | `redcart.events.order.shipped` | `ORDER_SHIPPED` | 订单服务 | 通知服务 |
| `order.finished` | `redcart.events.order.finished` | `ORDER_FINISHED` | 订单服务 | 积分/会员服务、AI 复盘 |
| `order.refund_requested` | `redcart.events.order.refund_requested` | `ORDER_REFUND_REQUESTED` | 订单服务 | 风控/客服服务 |
| `behavior.order_create` | `redcart.events.behavior.order_create` | `BehaviorOrderCreate` | 订单/行为服务 | 商家看板、AI 复盘 |
| `behavior.order_pay` | `redcart.events.behavior.order_pay` | `BehaviorOrderPay` | 订单/行为服务 | 商家看板 |
| `behavior.note_view` | `redcart.events.behavior.note_view` | `BehaviorNoteView` | 笔记服务 | 推荐服务、商家看板 |
| `behavior.product_click` | `redcart.events.behavior.product_click` | `BehaviorProductClick` | 商品服务 | 推荐服务 |
| `behavior.add_to_cart` | `redcart.events.behavior.add_to_cart` | `BehaviorAddToCart` | 购物车服务 | 推荐服务、商家看板 |

> 当前阶段优先实现 `order.*` 事件发布；`behavior.*` 事件作为第二条线，保留表结构并按同样模式扩展。

### 4. 服务边界：单体内部先按「逻辑服务」拆分

在真正拆进程之前，先把 Kafka 事件作为服务间边界：

- **订单服务（Order Service）**：仍驻留在 `backend/internal/order` 与 `backend/internal/redcart/application/service_order.go`，但只发布事件，不直接调用分析/通知。
- **通知服务（Notification Service）**：独立逻辑消费者，订阅 `redcart.events.order.*` topic，未来可拆成独立进程。
- **分析服务（Analytics Service）**：消费 `redcart.events.behavior.*` 和 `redcart.events.order.*` topic，为商家看板提供数据。
- **库存服务（Inventory Service）**：当前库存仍由订单服务在同一事务内处理；未来当库存需要独立扩缩容时，再基于 `order.paid` / `order.cancelled` 事件拆出。

### 5. 协议与序列化

- Kafka 消息体使用 JSON，便于调试和前端/AI 服务消费。
- Kafka message key 使用 outbox `event_id`，让同一事件稳定落到同一分区。
- 每条消息包含：
  - `event_id`：发件箱表主键，用于幂等。
  - `event_type`：事件类型。
  - `topic`：不带前缀的逻辑 topic。
  - `occurred_at`：事件发生时间。
  - `payload`：领域相关负载。
  - `correlation_id`：可选，用于链路追踪占位。
- Kafka headers 额外携带 `event_type`、`event_topic`、`correlation_id`，便于消费者过滤与排障。

### 6. 错误与重试

- Kafka publisher 使用 `segmentio/kafka-go` 同步写入，`RequiredAcks=RequireAll`，`WriteMessages` 成功返回后才允许 relay 标记 outbox 已发布。
- publisher 写入失败后，relay 在同一事务内调用 `MarkFailedInTx`，递增 `retry_count`；超过最大重试次数（当前 5 次）的事件移动到 `outbox_dead_letter` 表，等待人工/补偿处理。
- `KAFKA_BROKERS` 未配置时，后端保持可启动但不启用 outbox relay；这只适用于本地最小运行路径，完整 Docker Compose 与 CI 都配置 Kafka。
- 消费者侧错误处理见第 7 节。

### 7. 消费侧可靠性

- **显式 offset commit**：消费者使用 `FetchMessage` 获取消息，只有处理成功、重复跳过或成功写入死信 topic 后才调用 `CommitMessages`。
- **至少一次语义**：进程崩溃、网络中断或 commit 失败时，Kafka 会在 consumer group 中重新投递未提交 offset 的消息。
- **幂等去重**：消费者使用 outbox `event_id` 做幂等检查：处理前调用 `Deduplicator.IsDuplicate`，处理后调用 `MarkProcessed`。`MarkProcessed` 必须在 `CommitMessages` 之前完成——如果进程在 Mark 后、Commit 前崩溃，redelivery 会跳过已处理副作用；反过来则可能执行两次。
- **死信 topic**：decode 失败或 handler 返回错误时，消费者把原始 Kafka record 写入 `redcart.events.dlq`（同样可被 `KAFKA_TOPIC_PREFIX` 命名空间化），附加 `dlq_reason`、`dlq_error`、`source_topic`、`source_partition`、`source_offset` headers，然后提交源 offset。去重检查或 MarkProcessed 失败属于瞬态/一致性错误，不提交 offset，让 Kafka 稍后重投。
- 去重实现当前为进程内 `MemoryDeduplicator`（demo 用），生产环境应替换为 Redis `SETNX` 或数据库去重表，以在消费者重启后保持幂等性。

## 性能证据

性能证据只接受真实依赖路径，不再使用内存 outbox、空 publisher 或手工 sleep 模拟下游副作用作为吞吐来源。

当前保留两类 benchmark：

- `BenchmarkHTTPPostgresCreateOrderWithOutbox`：通过 Gin handler 进入应用层，使用 PostgreSQL 仓储创建订单，并在事务性发件箱中写入事件。benchmark 结束后会检查 PostgreSQL outbox 表中待发布事件数量。
- `BenchmarkPostgresKafkaOutboxRelay`：先向 PostgreSQL outbox 表写入真实事件，再用 outbox relay 通过 Kafka publisher 发布并标记已发布。

README 性能表只由 GitHub Actions 的 benchmark workflow 更新，白名单限定为 PostgreSQL/Redis/Kafka-backed 组件 benchmark 和 live HTTP benchmark。任何内存仓储、空 publisher 或模拟延迟 benchmark 结果都会被脚本拒绝。

## 影响

- 订单等核心模块继续通过数据库事务保证一致性；MQ 只承担异步解耦，不承担分布式事务协调。
- 需要保留 `outbox` 表和 relay 后台 goroutine。
- Docker Compose 中新增 `kafka` 服务，并更新健康检查。
- `backend/internal/event/kafka` 是当前 MQ 集成适配器；领域层只依赖 `backend/internal/event` 契约，不依赖 Kafka SDK。
- AI 复盘、商家看板等能力未来可以只订阅事件，不再直接查询订单/行为表。
- 当前阶段不引入服务发现、API 网关、Service Mesh；这些在 ADR 0005 中已有结论，保持后置。

## 非目标

- 不把订单、库存、购物车立刻拆成独立进程。
- 不引入 Saga 或 TCC 等分布式事务协议；强一致性仍由数据库事务保证。
- 不一次性实现所有 `behavior.*` 主题；先完成 `order.*` 事件链路，再按需扩展。
