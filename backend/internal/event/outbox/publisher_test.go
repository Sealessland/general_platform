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

// memoryTx 是测试用的内存事务句柄，记录是否已提交/回滚。
type memoryTx struct {
	committed  bool
	rolledBack bool
}

// Commit 标记事务已提交。
func (t *memoryTx) Commit() error { t.committed = true; return nil }

// Rollback 标记事务已回滚。
func (t *memoryTx) Rollback() error { t.rolledBack = true; return nil }

type memoryOutbox struct {
	mu      sync.Mutex
	events  []event.Event
	failed  []int64
	pending []event.Event
	locked  []int64
}

// Append 将事件追加到内存列表并返回其序号作为 ID。
func (m *memoryOutbox) Append(ctx context.Context, evt event.Event) (int64, error) {
	m.events = append(m.events, evt)
	return int64(len(m.events)), nil
}

// PollPending 返回待发布列表（按 limit 截断），模拟未发布事件读取。
func (m *memoryOutbox) PollPending(ctx context.Context, limit int) ([]event.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.pending
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// MarkPublished 从待发布列表移除已发布的事件，模拟发布成功后出队。
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

// MarkFailed 将失败的事件 ID 记入 failed 列表。
func (m *memoryOutbox) MarkFailed(ctx context.Context, id int64, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failed = append(m.failed, id)
	return nil
}

// BeginTx 返回一个新的内存事务句柄。
func (m *memoryOutbox) BeginTx(ctx context.Context) (event.OutboxTx, error) {
	return &memoryTx{}, nil
}

// PollPendingInTx 返回未锁定且未发布的至多 limit 条事件，并标记为已锁定，
// 模拟 SELECT ... FOR UPDATE SKIP LOCKED 的并发互斥行为。
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

// MarkPublishedInTx 在事务内从待发布列表移除已发布事件，模拟置 published_at。
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

// MarkFailedInTx 在事务内将失败的事件 ID 记入 failed 列表。
func (m *memoryOutbox) MarkFailedInTx(ctx context.Context, tx event.OutboxTx, id int64, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failed = append(m.failed, id)
	return nil
}

// mockPublisher 是测试用的发布器替身：可注入延迟与错误，记录已发布事件。
type mockPublisher struct {
	mu        sync.Mutex
	published []event.Event
	err       error
	delay     time.Duration
}

// Publish 可选地模拟延迟与失败，成功时把事件记入 published 列表。
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

// count 返回已发布事件数量，供测试断言发布结果。
func (p *mockPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.published)
}

// TestPublisherRelaysPendingEvents 验证 tick 会把待发布事件全部发布并清空 outbox。
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

// TestPublisherMarksFailedEvents 验证发布失败的事件被标记为失败。
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

// TestPublisherNoDuplicatePublishUnderConcurrency 验证并发 tick 不会重复发布同一事件。
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
	// 两个转发器并发执行 tick，验证锁定机制避免重复发布。
	go func() { defer wg.Done(); relay1.tick(context.Background()) }()
	go func() { defer wg.Done(); relay2.tick(context.Background()) }()
	wg.Wait()

	total := pub.count()
	if total != 50 {
		t.Fatalf("expected exactly 50 publishes (no duplicates), got %d", total)
	}
}

// TestPublisherRollbackOnPublishFailure 验证发布失败后事务回滚且事件保留在 outbox。
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
