# ADR 0006：Kafka 与事务性发件箱

## 状态

已接受（2026-08-02 修订：运行时从 RabbitMQ 迁移到 Kafka）

## 先说结论

订单状态和库存仍由 PostgreSQL 事务保证一致性。业务事务只写 `outbox` 表，后台 relay 再把事件发布到 Kafka 的 `redcart.events` topic。应用层只认识 `event.Publisher`，不依赖 Kafka SDK。

```text
订单事务
  ├─ 更新订单/库存
  └─ 写入 outbox
         ↓ 后台轮询
    Outbox Relay
         ↓ event.Publisher
    Kafka Adapter
         ↓ broker ack
    标记 outbox.published_at
```

这条链路解决的是“数据库成功但消息没发出去”的双写问题，不承诺端到端 exactly-once。

## 为什么选择 Kafka

- 订单、行为、分析事件适合按时间保留和重放，Kafka 的日志模型比临时队列更贴合。
- `correlation_id` 作为 record key，可让同一业务关联的消息稳定进入同一分区并保持分区内顺序。
- 单节点 KRaft 模式可直接放进 Docker Compose，不需要 ZooKeeper。
- Kafka 细节集中在 `backend/internal/event/kafka`；以后更换 broker 时，应用层和 outbox relay 不变。

当前只实现真实使用的 producer。仓库没有运行中的业务 consumer，因此不保留一套“看起来完整但没有调用方”的通用消费框架。新增通知或分析消费者时，应同时定义真实 handler、持久化 Inbox 去重和失败隔离策略。

## 代码如何对应

| 职责 | 文件 | 初学者先看什么 |
|---|---|---|
| 中立事件与发布契约 | `backend/internal/event/event.go` | `Event`、`Publisher` |
| PostgreSQL 发件箱 | `backend/internal/redcart/infrastructure/postgres/outbox_repository.go` | pending、published、failed |
| 轮询与发布编排 | `backend/internal/event/outbox/publisher.go` | `tick` 的五个步骤 |
| Kafka JSON 信封 | `backend/internal/event/kafka/codec.go` | transport message 与领域 Event 的转换 |
| Kafka broker 适配 | `backend/internal/event/kafka/publisher.go` | record key、同步 broker ack |
| 启动装配 | `backend/cmd/api/main.go` | `startOutboxPublisher` |

依赖方向是：`application -> event contract <- kafka adapter`。应用层不能导入 `franz-go`。

## Topic 与消息格式

所有事件进入一个物理 topic，默认名为 `redcart.events`。`order.created`、`order.paid` 等是信封中的逻辑 `event_topic`，不是一批需要预先维护的 Kafka topic。

```json
{
  "event_id": 42,
  "event_type": "ORDER_CREATED",
  "event_topic": "order.created",
  "correlation_id": "order-1001",
  "payload": {"order_id": 1001},
  "occurred_at": "2026-08-02T12:00:00Z"
}
```

- record key：优先使用 `correlation_id`，缺失时使用 `event_id`。
- headers：重复携带 `event_id`、`event_type`、`event_topic`，便于消费者快速检查。
- value：使用 JSON，牺牲一部分体积，换取本地调试和跨语言消费的可读性。

## 可靠性语义

1. 业务数据与 outbox 记录在同一个 PostgreSQL 事务中写入。
2. relay 用 `FOR UPDATE SKIP LOCKED` 领取批次，避免多个 relay 同时处理同一行。
3. Kafka producer 等待 broker acknowledgement；成功后才更新 `published_at`。
4. 发布失败会增加 `retry_count`；达到上限后移入 `outbox_dead_letter`。
5. franz-go 默认启用幂等 producer，可减少同一 producer 会话内的网络重试重复。

仍然存在一个不可消除的窗口：Kafka 已确认、但 PostgreSQL 标记提交前进程崩溃。重启后该 outbox 事件会再次发布。因此整体语义是 **at-least-once**，消费者必须用稳定的 `event_id` 做持久化去重。Kafka producer 幂等不等于业务端 exactly-once。

当前 relay 在数据库事务内等待网络 ack，代码直观且能阻止并发 relay 重复领取，但会延长行锁和连接占用。流量增长后应演进为“短事务租约 claim → 事务外 publish → 短事务完成标记”，并为租约超时补恢复测试。

## 配置与本地运行

- `KAFKA_BROKERS`：逗号分隔的 broker 地址；为空时禁用 outbox relay。
- `KAFKA_TOPIC`：物理 topic，默认 `redcart.events`。
- Docker Compose：单节点 Kafka/KRaft，监听 `127.0.0.1:19092`。

本地单节点只用于开发和测试，不具备副本容灾能力。生产环境应预创建 topic，配置多 broker、副本因子、`min.insync.replicas`、认证加密和容量告警。

## 验证证据

- `backend/internal/event/kafka/publisher_test.go`：校验 topic、key、信封和 broker error 传播。
- `backend/internal/event/outbox/publisher_test.go`：校验 relay 成功、失败和并发领取。
- `BenchmarkKafkaPublish`：真实 Kafka producer 路径。
- `BenchmarkPostgresKafkaOutboxRelay`：真实 PostgreSQL outbox 到 Kafka 的完整 relay 路径。

## 非目标

- 不立即拆分订单、库存、通知和分析服务。
- 不引入 Saga、TCC 或 Kafka transaction 来替代 PostgreSQL 强事务。
- 不在没有真实消费者的情况下声称已实现 Inbox、消费重试或 DLQ。
- 不把 Kafka record、client 或错误类型暴露到应用层和领域层。
