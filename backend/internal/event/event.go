// Package event defines the asynchronous event publishing contract used by
// the application layer to decouple domain state changes from downstream
// consumers. Implementations live in the infrastructure layer.
package event

import (
	"context"
	"encoding/json"
	"time"
)

// Type identifies a business event. Values are stable identifiers that can be
// consumed by multiple services and should not be renamed without a migration.
type Type string

const (
	TypeOrderCreated         Type = "ORDER_CREATED"
	TypeOrderPaid            Type = "ORDER_PAID"
	TypeOrderCancelled       Type = "ORDER_CANCELLED"
	TypeOrderShipped         Type = "ORDER_SHIPPED"
	TypeOrderFinished        Type = "ORDER_FINISHED"
	TypeOrderRefundRequested Type = "ORDER_REFUND_REQUESTED"
	TypeOrderRefunded        Type = "ORDER_REFUNDED"

	// TypeBehavior* 系列为行为埋点事件，当前未接线，为预留/演示能力。
	TypeBehaviorNoteView     Type = "BEHAVIOR_NOTE_VIEW"
	TypeBehaviorProductClick Type = "BEHAVIOR_PRODUCT_CLICK"
	TypeBehaviorAddToCart    Type = "BEHAVIOR_ADD_TO_CART"
	TypeBehaviorOrderCreate  Type = "BEHAVIOR_ORDER_CREATE"
	TypeBehaviorOrderPay     Type = "BEHAVIOR_ORDER_PAY"
	TypeBehaviorOrderCancel  Type = "BEHAVIOR_ORDER_CANCEL"
	TypeBehaviorOrderRefund  Type = "BEHAVIOR_ORDER_REFUND"
)

// Topic returns the message queue topic / routing key for an event type.
// The convention is domain.action for order events and behavior.action for
// behavior events, making it easy for consumers to bind to prefixes.
// Topic 返回事件类型对应的消息队列 topic/路由键：订单事件为 domain.action，
// 行为事件为 behavior.action，便于消费者按前缀订阅。
func (t Type) Topic() string {
	switch t {
	case TypeOrderCreated:
		return "order.created"
	case TypeOrderPaid:
		return "order.paid"
	case TypeOrderCancelled:
		return "order.cancelled"
	case TypeOrderShipped:
		return "order.shipped"
	case TypeOrderFinished:
		return "order.finished"
	case TypeOrderRefundRequested:
		return "order.refund_requested"
	case TypeOrderRefunded:
		return "order.refunded"
	case TypeBehaviorNoteView:
		return "behavior.note_view"
	case TypeBehaviorProductClick:
		return "behavior.product_click"
	case TypeBehaviorAddToCart:
		return "behavior.add_to_cart"
	case TypeBehaviorOrderCreate:
		return "behavior.order_create"
	case TypeBehaviorOrderPay:
		return "behavior.order_pay"
	case TypeBehaviorOrderCancel:
		return "behavior.order_cancel"
	case TypeBehaviorOrderRefund:
		return "behavior.order_refund"
	default:
		return "unknown"
	}
}

// Event is the unit published to the message queue. It is intentionally
// serialization-agnostic at this level; the payload is raw JSON bytes.
type Event struct {
	ID            int64
	Type          Type
	Topic         string
	CorrelationID string
	Payload       json.RawMessage
	OccurredAt    time.Time
}

// Publisher sends events to the message broker. The application layer depends
// on this contract, not on any specific broker SDK.
type Publisher interface {
	Publish(ctx context.Context, event Event) error
}

// Outbox stores events that must be published reliably. It is designed to be
// called inside the same database transaction that mutates business state,
// implementing the transactional outbox pattern.
// Outbox 负责可靠存储待发布事件：设计上与业务状态变更处于同一数据库事务中，
// 实现事务性发件箱模式。
type Outbox interface {
	// Append records an event in the outbox table as part of the current
	// transaction. The returned ID is the outbox primary key.
	// Append 在当前事务内把事件写入 outbox 表，返回主键 ID。
	Append(ctx context.Context, event Event) (int64, error)
}

// OutboxPoller is used by the background publisher to read pending events.
// OutboxPoller 供后台转发器读取待发布事件使用。
type OutboxPoller interface {
	// PollPending returns up to limit events that have not been published yet.
	// PollPending 返回最多 limit 条尚未发布的事件。
	PollPending(ctx context.Context, limit int) ([]Event, error)

	// MarkPublished removes or flags the given outbox records as published.
	// MarkPublished 将给定 outbox 记录标记为已发布（删除或打标）。
	MarkPublished(ctx context.Context, ids []int64) error

	// MarkFailed records a failed publish attempt. Implementations may
	// increment a retry counter and move the record to a dead-letter table
	// when retries are exhausted.
	// MarkFailed 记录一次发布失败；实现可递增重试计数，重试耗尽后转入死信表。
	MarkFailed(ctx context.Context, id int64, reason string) error
}

// OutboxRelayStore extends OutboxPoller with transaction-scoped operations for
// the background relay. PollPendingInTx and MarkPublishedInTx run inside a
// caller-managed transaction so the relay can lock rows, publish, and mark in
// one atomic unit — preventing duplicate publishes across concurrent instances.
// OutboxRelayStore 在 OutboxPoller 之上补充事务内操作：锁定、发布、标记在同一个
// 调用方管理的事务里原子完成，避免并发实例重复发布。
type OutboxRelayStore interface {
	OutboxPoller

	// BeginTx starts a transaction for the relay cycle.
	// BeginTx 为一次转发周期开启事务。
	BeginTx(ctx context.Context) (OutboxTx, error)

	// PollPendingInTx locks and returns up to limit unpublished events within
	// the given transaction using SELECT ... FOR UPDATE SKIP LOCKED.
	// PollPendingInTx 在事务内用 SELECT ... FOR UPDATE SKIP LOCKED 锁定并取回
	// 至多 limit 条未发布事件。
	PollPendingInTx(ctx context.Context, tx OutboxTx, limit int) ([]Event, error)

	// MarkPublishedInTx marks the given outbox records as published within the
	// transaction (sets published_at rather than deleting).
	// MarkPublishedInTx 在事务内将给定记录标记为已发布（置 published_at 而非删除）。
	MarkPublishedInTx(ctx context.Context, tx OutboxTx, ids []int64) error

	// MarkFailedInTx records a failed publish attempt within the transaction.
	// MarkFailedInTx 在事务内记录一次失败的发布尝试。
	MarkFailedInTx(ctx context.Context, tx OutboxTx, id int64, reason string) error
}

// OutboxTx is a transaction handle used by the relay store.
// OutboxTx 是转发器使用的事务句柄。
type OutboxTx interface {
	// Commit 提交事务。
	Commit() error
	// Rollback 回滚事务。
	Rollback() error
}

// OutboxStore combines the write and read sides of the outbox.
// OutboxStore 合并 outbox 的写入与读取两侧能力。
type OutboxStore interface {
	Outbox
	OutboxPoller
}
