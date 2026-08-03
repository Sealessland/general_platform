package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
)

type memoryTx struct {
	committed  bool
	rolledBack bool
}

func (t *memoryTx) Commit() error   { t.committed = true; return nil }
func (t *memoryTx) Rollback() error { t.rolledBack = true; return nil }

type memoryOutbox struct {
	mu      sync.Mutex
	events  []event.Event
	failed  []int64
	pending []event.Event
	locked  []int64
}

func (m *memoryOutbox) Append(ctx context.Context, evt event.Event) (int64, error) {
	m.events = append(m.events, evt)
	return int64(len(m.events)), nil
}

func (m *memoryOutbox) PollPending(ctx context.Context, limit int) ([]event.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.pending
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memoryOutbox) MarkPublished(ctx context.Context, ids []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failed = append(m.failed, id)
	return nil
}

func (m *memoryOutbox) BeginTx(ctx context.Context) (event.OutboxTx, error) {
	return &memoryTx{}, nil
}

func (m *memoryOutbox) PollPendingInTx(ctx context.Context, tx event.OutboxTx, limit int) ([]event.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]event.Event, 0, limit)
	for _, evt := range m.pending {
		alreadyLocked := false
		for _, lid := range m.locked {
			if lid == evt.ID {
				alreadyLocked = true
				break
			}
		}
		if alreadyLocked {
			continue
		}
		out = append(out, evt)
		m.locked = append(m.locked, evt.ID)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *memoryOutbox) MarkPublishedInTx(ctx context.Context, tx event.OutboxTx, ids []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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

func (m *memoryOutbox) MarkFailedInTx(ctx context.Context, tx event.OutboxTx, id int64, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failed = append(m.failed, id)
	return nil
}

type mockPublisher struct {
	mu        sync.Mutex
	published []event.Event
	err       error
	delay     time.Duration
}

func (p *mockPublisher) Publish(ctx context.Context, evt event.Event) error {
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	if p.err != nil {
		return p.err
	}
	p.mu.Lock()
	p.published = append(p.published, evt)
	p.mu.Unlock()
	return nil
}

func (p *mockPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.published)
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

	if pub.count() != 2 {
		t.Fatalf("expected 2 published events, got %d", pub.count())
	}
	store.mu.Lock()
	remaining := len(store.pending)
	store.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("expected outbox to be empty, got %d", remaining)
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

	if pub.count() != 0 {
		t.Fatalf("expected 0 published events, got %d", pub.count())
	}
	store.mu.Lock()
	failed := store.failed
	store.mu.Unlock()
	if len(failed) != 1 || failed[0] != 1 {
		t.Fatalf("expected event 1 to be marked failed, got %v", failed)
	}
}

func TestPublisherNoDuplicatePublishUnderConcurrency(t *testing.T) {
	now := time.Now().UTC()
	pending := make([]event.Event, 50)
	for i := range pending {
		id := int64(i + 1)
		pending[i] = event.Event{
			ID:         id,
			Type:       event.TypeOrderCreated,
			Topic:      "order.created",
			Payload:    json.RawMessage(`{"order_id":1}`),
			OccurredAt: now,
		}
	}
	store := &memoryOutbox{pending: pending}
	pub := &mockPublisher{delay: 10 * time.Millisecond}

	relay1 := NewPublisher(store, pub, Config{Interval: time.Hour, BatchSize: 100, Logger: log.Default()})
	relay2 := NewPublisher(store, pub, Config{Interval: time.Hour, BatchSize: 100, Logger: log.Default()})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); relay1.tick(context.Background()) }()
	go func() { defer wg.Done(); relay2.tick(context.Background()) }()
	wg.Wait()

	total := pub.count()
	if total != 50 {
		t.Fatalf("expected exactly 50 publishes (no duplicates), got %d", total)
	}
}

func TestPublisherRollbackOnPublishFailure(t *testing.T) {
	now := time.Now().UTC()
	store := &memoryOutbox{
		pending: []event.Event{
			{ID: 1, Type: event.TypeOrderCreated, Topic: "order.created", Payload: json.RawMessage(`{}`), OccurredAt: now},
		},
	}
	pub := &mockPublisher{err: fmt.Errorf("broker down")}
	relay := NewPublisher(store, pub, Config{Interval: time.Hour, BatchSize: 10, Logger: log.Default()})

	relay.tick(context.Background())

	store.mu.Lock()
	remaining := len(store.pending)
	failed := len(store.failed)
	store.mu.Unlock()
	if remaining != 1 {
		t.Fatalf("expected event to remain in outbox after failure, got %d remaining", remaining)
	}
	if failed != 1 {
		t.Fatalf("expected 1 failed marker, got %d", failed)
	}
}
