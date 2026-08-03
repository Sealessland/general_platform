package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
)

// 验证 outbox 事件的追加、轮询与发布后移除。
func TestOutboxAppendAndPoll(t *testing.T) {
	repo := newPostgresRepo(t)
	clearOutboxForTest(t, repo)
	ctx := context.Background()

	evt := event.Event{
		Type:          event.TypeOrderCreated,
		Topic:         event.TypeOrderCreated.Topic(),
		CorrelationID: "outbox-test-created",
		Payload:       json.RawMessage(`{"order_id":42}`),
		OccurredAt:    time.Now().UTC(),
	}
	id, err := repo.Outbox.Append(ctx, evt)
	if err != nil {
		t.Fatalf("append outbox: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero outbox id")
	}

	pending, err := repo.Outbox.PollPending(ctx, 100)
	if err != nil {
		t.Fatalf("poll pending: %v", err)
	}
	polled, ok := findOutboxEvent(pending, id)
	if !ok {
		t.Fatalf("expected pending event %d in %+v", id, pending)
	}
	if polled.Type != event.TypeOrderCreated {
		t.Fatalf("expected %s, got %s", event.TypeOrderCreated, polled.Type)
	}

	if err := repo.Outbox.MarkPublished(ctx, []int64{id}); err != nil {
		t.Fatalf("mark published: %v", err)
	}
	pending, err = repo.Outbox.PollPending(ctx, 100)
	if err != nil {
		t.Fatalf("poll after publish: %v", err)
	}
	if _, ok := findOutboxEvent(pending, id); ok {
		t.Fatalf("expected event %d to be removed after publish", id)
	}
}

// 验证失败重试达到上限后事件离开待发布队列（迁移死信）。
func TestOutboxMarkFailedMovesToDeadLetter(t *testing.T) {
	repo := newPostgresRepo(t)
	clearOutboxForTest(t, repo)
	ctx := context.Background()

	evt := event.Event{
		Type:          event.TypeOrderPaid,
		Topic:         event.TypeOrderPaid.Topic(),
		CorrelationID: "outbox-test-paid",
		Payload:       json.RawMessage(`{}`),
		OccurredAt:    time.Now().UTC(),
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

	pending, err := repo.Outbox.PollPending(ctx, 100)
	if err != nil {
		t.Fatalf("poll pending: %v", err)
	}
	if _, ok := findOutboxEvent(pending, id); ok {
		t.Fatalf("expected event %d to leave pending after max retries", id)
	}
}

// 清空 outbox 与死信表，保证测试从干净状态开始。
func clearOutboxForTest(t *testing.T, repo *Repository) {
	t.Helper()
	if _, err := repo.db.Exec(`DELETE FROM outbox_dead_letter`); err != nil {
		t.Fatalf("clear outbox dead letter: %v", err)
	}
	if _, err := repo.db.Exec(`DELETE FROM outbox`); err != nil {
		t.Fatalf("clear outbox: %v", err)
	}
}

// 在事件列表中按 ID 查找目标事件。
func findOutboxEvent(events []event.Event, id int64) (event.Event, bool) {
	for _, evt := range events {
		if evt.ID == id {
			return evt, true
		}
	}
	return event.Event{}, false
}
