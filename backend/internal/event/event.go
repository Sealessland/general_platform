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
type Outbox interface {
	// Append records an event in the outbox table as part of the current
	// transaction. The returned ID is the outbox primary key.
	Append(ctx context.Context, event Event) (int64, error)
}

// OutboxPoller is used by the background publisher to read pending events.
type OutboxPoller interface {
	// PollPending returns up to limit events that have not been published yet.
	PollPending(ctx context.Context, limit int) ([]Event, error)

	// MarkPublished removes or flags the given outbox records as published.
	MarkPublished(ctx context.Context, ids []int64) error

	// MarkFailed records a failed publish attempt. Implementations may
	// increment a retry counter and move the record to a dead-letter table
	// when retries are exhausted.
	MarkFailed(ctx context.Context, id int64, reason string) error
}

// OutboxStore combines the write and read sides of the outbox.
type OutboxStore interface {
	Outbox
	OutboxPoller
}
