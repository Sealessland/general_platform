package memory

import (
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"time"
)

func (r *Repository) AppendBehaviorEvent(event domain.BehaviorEvent) (domain.BehaviorEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	event.ID = r.nextID(&r.nextBehaviorEventID)
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	r.behaviorEvents = append(r.behaviorEvents, event)
	return event, nil
}

func (r *Repository) ListBehaviorEvents() []domain.BehaviorEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.BehaviorEvent, len(r.behaviorEvents))
	copy(out, r.behaviorEvents)
	return out
}
