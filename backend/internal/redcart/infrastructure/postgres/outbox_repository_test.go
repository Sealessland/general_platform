package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
)

func TestOutboxAppendAndPoll(t *testing.T) {
	repo := newPostgresRepo(t)
	ctx := context.Background()

	evt := event.Event{
		Type:       event.TypeOrderCreated,
		Topic:      event.TypeOrderCreated.Topic(),
		Payload:    json.RawMessage(`{"order_id":42}`),
		OccurredAt: time.Now().UTC(),
	}
	id, err := repo.Outbox.Append(ctx, evt)
	if err != nil {
		t.Fatalf("append outbox: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero outbox id")
	}

	pending, err := repo.Outbox.PollPending(ctx, 10)
	if err != nil {
		t.Fatalf("poll pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending event, got %d", len(pending))
	}
	if pending[0].Type != event.TypeOrderCreated {
		t.Fatalf("expected %s, got %s", event.TypeOrderCreated, pending[0].Type)
	}

	if err := repo.Outbox.MarkPublished(ctx, []int64{pending[0].ID}); err != nil {
		t.Fatalf("mark published: %v", err)
	}
	pending, err = repo.Outbox.PollPending(ctx, 10)
	if err != nil {
		t.Fatalf("poll after publish: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending events, got %d", len(pending))
	}
}

func TestOutboxMarkFailedMovesToDeadLetter(t *testing.T) {
	repo := newPostgresRepo(t)
	ctx := context.Background()

	evt := event.Event{
		Type:       event.TypeOrderPaid,
		Topic:      event.TypeOrderPaid.Topic(),
		Payload:    json.RawMessage(`{}`),
		OccurredAt: time.Now().UTC(),
	}
	id, err := repo.Outbox.Append(ctx, evt)
	if err != nil {
		t.Fatalf("append outbox: %v", err)
	}

	for i := 0; i < 6; i++ {
		if err := repo.Outbox.MarkFailed(ctx, id, "broker down"); err != nil {
			t.Fatalf("mark failed %d: %v", i, err)
		}
	}

	pending, err := repo.Outbox.PollPending(ctx, 10)
	if err != nil {
		t.Fatalf("poll pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected event to leave pending after max retries, got %d", len(pending))
	}
}
