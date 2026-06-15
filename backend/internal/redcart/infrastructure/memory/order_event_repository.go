package memory

import (
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"time"
)

func (r *Repository) ListOrderEvents(orderID int64) []domain.OrderEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	events := r.orderEventsByOrder[orderID]
	out := make([]domain.OrderEvent, len(events))
	for i, event := range events {
		out[i] = cloneOrderEvent(event)
	}
	return out
}

func (r *Repository) AppendOrderEvent(event domain.OrderEvent) (domain.OrderEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendOrderEventLocked(event)
}

func (r *Repository) appendOrderEventLocked(event domain.OrderEvent) (domain.OrderEvent, error) {
	event.ID = r.nextID(&r.nextOrderEventID)
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	r.orderEventsByOrder[event.OrderID] = append(r.orderEventsByOrder[event.OrderID], cloneOrderEvent(event))
	return cloneOrderEvent(event), nil
}
