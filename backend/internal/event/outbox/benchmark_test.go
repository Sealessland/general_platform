package outbox

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
)

// benchOutbox is an in-memory outbox store prepopulated with pending events.
type benchOutbox struct {
	pending []event.Event
	cursor  int
	sink    []event.Event
}

func (b *benchOutbox) Append(ctx context.Context, evt event.Event) (int64, error) {
	b.sink = append(b.sink, evt)
	return int64(len(b.sink)), nil
}

func (b *benchOutbox) PollPending(ctx context.Context, limit int) ([]event.Event, error) {
	end := b.cursor + limit
	if end > len(b.pending) {
		end = len(b.pending)
	}
	batch := b.pending[b.cursor:end]
	b.cursor = end
	return batch, nil
}

func (b *benchOutbox) MarkPublished(ctx context.Context, ids []int64) error {
	return nil
}

func (b *benchOutbox) MarkFailed(ctx context.Context, id int64, reason string) error {
	return nil
}

type benchPublisher struct{}

func (p *benchPublisher) Publish(ctx context.Context, evt event.Event) error {
	return nil
}

func BenchmarkOutboxRelay(b *testing.B) {
	payload := json.RawMessage(`{"order_id":42,"status":"PAID"}`)
	pending := make([]event.Event, b.N)
	now := time.Now().UTC()
	for i := 0; i < b.N; i++ {
		pending[i] = event.Event{
			ID:         int64(i + 1),
			Type:       event.TypeOrderPaid,
			Topic:      event.TypeOrderPaid.Topic(),
			Payload:    payload,
			OccurredAt: now,
		}
	}

	store := &benchOutbox{pending: pending}
	relay := NewPublisher(store, &benchPublisher{}, Config{
		Interval:  time.Hour,
		BatchSize: b.N,
		Logger:    nil,
	})

	b.ReportAllocs()
	b.ResetTimer()
	relay.tick(context.Background())
}

func BenchmarkOutboxAppend(b *testing.B) {
	payload := json.RawMessage(`{"order_id":42}`)
	now := time.Now().UTC()
	store := &benchOutbox{}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := store.Append(context.Background(), event.Event{
			Type:       event.TypeOrderCreated,
			Topic:      event.TypeOrderCreated.Topic(),
			Payload:    payload,
			OccurredAt: now,
		})
		if err != nil {
			b.Fatalf("append: %v", err)
		}
	}
}
