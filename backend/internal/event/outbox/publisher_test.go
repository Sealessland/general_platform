package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
)

type memoryOutbox struct {
	events  []event.Event
	failed  []int64
	pending []event.Event
}

func (m *memoryOutbox) Append(ctx context.Context, evt event.Event) (int64, error) {
	m.events = append(m.events, evt)
	return int64(len(m.events)), nil
}

func (m *memoryOutbox) PollPending(ctx context.Context, limit int) ([]event.Event, error) {
	out := m.pending
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memoryOutbox) MarkPublished(ctx context.Context, ids []int64) error {
	remaining := make([]event.Event, 0, len(m.pending))
	published := make(map[int64]bool)
	for _, id := range ids {
		published[id] = true
	}
	for _, evt := range m.pending {
		if !published[evt.ID] {
			remaining = append(remaining, evt)
		}
	}
	m.pending = remaining
	return nil
}

func (m *memoryOutbox) MarkFailed(ctx context.Context, id int64, reason string) error {
	m.failed = append(m.failed, id)
	return nil
}

type mockPublisher struct {
	published []event.Event
	err       error
}

func (p *mockPublisher) Publish(ctx context.Context, evt event.Event) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, evt)
	return nil
}

func TestPublisherRelaysPendingEvents(t *testing.T) {
	now := time.Now().UTC()
	store := &memoryOutbox{
		pending: []event.Event{
			{ID: 1, Type: event.TypeOrderCreated, Topic: "order.created", Payload: json.RawMessage(`{"order_id":1}`), OccurredAt: now},
			{ID: 2, Type: event.TypeOrderPaid, Topic: "order.paid", Payload: json.RawMessage(`{"order_id":1}`), OccurredAt: now},
		},
	}
	pub := &mockPublisher{}
	relay := NewPublisher(store, pub, Config{Interval: time.Hour, BatchSize: 10, Logger: log.Default()})

	relay.tick(context.Background())

	if len(pub.published) != 2 {
		t.Fatalf("expected 2 published events, got %d", len(pub.published))
	}
	if len(store.pending) != 0 {
		t.Fatalf("expected outbox to be empty, got %d", len(store.pending))
	}
}

func TestPublisherMarksFailedEvents(t *testing.T) {
	now := time.Now().UTC()
	store := &memoryOutbox{
		pending: []event.Event{
			{ID: 1, Type: event.TypeOrderCreated, Topic: "order.created", Payload: json.RawMessage(`{}`), OccurredAt: now},
		},
	}
	pub := &mockPublisher{err: errors.New("broker unavailable")}
	relay := NewPublisher(store, pub, Config{Interval: time.Hour, BatchSize: 10, Logger: log.Default()})

	relay.tick(context.Background())

	if len(pub.published) != 0 {
		t.Fatalf("expected 0 published events, got %d", len(pub.published))
	}
	if len(store.failed) != 1 || store.failed[0] != 1 {
		t.Fatalf("expected event 1 to be marked failed, got %v", store.failed)
	}
}
