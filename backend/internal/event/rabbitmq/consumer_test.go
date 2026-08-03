package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"testing"

	"github.com/example/redcart-copilot/backend/internal/event"
)

type mockAcker struct {
	acked     bool
	nacked    bool
	requeue   bool
	callCount int
}

// Ack 记录一次 ack 调用并返回 nil，供测试断言消费成功路径。
func (m *mockAcker) Ack(multiple bool) error {
	m.callCount++
	m.acked = true
	return nil
}

// Nack 记录一次 nack 调用（含是否重新入队），供测试断言失败路径。
func (m *mockAcker) Nack(multiple, requeue bool) error {
	m.callCount++
	m.nacked = true
	m.requeue = requeue
	return nil
}

type mockHandler struct {
	err   error
	calls []event.Event
}

// Handle 记录调用的事件并返回预设错误，供测试断言业务处理结果。
func (h *mockHandler) Handle(_ context.Context, evt event.Event) error {
	h.calls = append(h.calls, evt)
	return h.err
}

type countingDedup struct {
	duplicates map[int64]bool
	processed  []int64
	checkErr   error
}

// IsDuplicate 查询事件 ID 是否已处理；checkErr 非空时直接返回错误模拟故障。
func (d *countingDedup) IsDuplicate(_ context.Context, eventID int64) (bool, error) {
	if d.checkErr != nil {
		return false, d.checkErr
	}
	return d.duplicates[eventID], nil
}

// MarkProcessed 记录已处理事件 ID 并写回去重集合。
func (d *countingDedup) MarkProcessed(_ context.Context, eventID int64) error {
	d.processed = append(d.processed, eventID)
	d.duplicates[eventID] = true
	return nil
}

// encodeMessage 按发布端 envelope 结构把事件序列化为消息体，供消费测试使用。
func encodeMessage(t *testing.T, evt event.Event) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"event_id":       evt.ID,
		"event_type":     string(evt.Type),
		"topic":          evt.Topic,
		"correlation_id": evt.CorrelationID,
		"occurred_at":    evt.OccurredAt,
		"payload":        evt.Payload,
	})
	if err != nil {
		t.Fatalf("encode message: %v", err)
	}
	return body
}

// newTestConsumer 构造一个不连接 AMQP 的 Consumer，便于直接驱动 processDelivery 测试。
func newTestConsumer(h Handler, d Deduplicator) *Consumer {
	return &Consumer{
		handler: h,
		dedup:   d,
		logger:  log.Default(),
	}
}

// TestProcessDeliverySuccessAcksAndMarksProcessed 验证成功路径：ack 且标记已处理。
func TestProcessDeliverySuccessAcksAndMarksProcessed(t *testing.T) {
	evt := event.Event{ID: 1, Type: event.TypeOrderCreated, Topic: "order.created", Payload: json.RawMessage(`{}`)}
	body := encodeMessage(t, evt)

	h := &mockHandler{}
	dedup := &countingDedup{duplicates: map[int64]bool{}}
	c := newTestConsumer(h, dedup)
	acker := &mockAcker{}

	c.processDelivery(context.Background(), body, acker)

	if !acker.acked || acker.nacked {
		t.Fatalf("expected ack, got acked=%v nacked=%v", acker.acked, acker.nacked)
	}
	if len(h.calls) != 1 {
		t.Fatalf("expected 1 handler call, got %d", len(h.calls))
	}
	if len(dedup.processed) != 1 || dedup.processed[0] != 1 {
		t.Fatalf("expected event 1 marked processed, got %v", dedup.processed)
	}
}

// TestProcessDeliveryHandlerFailureNacksToDLX 验证业务失败时 nack 到死信队列且不标记已处理。
func TestProcessDeliveryHandlerFailureNacksToDLX(t *testing.T) {
	evt := event.Event{ID: 2, Type: event.TypeOrderPaid, Topic: "order.paid", Payload: json.RawMessage(`{}`)}
	body := encodeMessage(t, evt)

	h := &mockHandler{err: errors.New("downstream timeout")}
	dedup := &countingDedup{duplicates: map[int64]bool{}}
	c := newTestConsumer(h, dedup)
	acker := &mockAcker{}

	c.processDelivery(context.Background(), body, acker)

	if acker.acked || !acker.nacked {
		t.Fatalf("expected nack, got acked=%v nacked=%v", acker.acked, acker.nacked)
	}
	if acker.requeue {
		t.Fatalf("expected requeue=false (send to DLX), got requeue=true")
	}
	if len(dedup.processed) != 0 {
		t.Fatalf("expected no mark-processed on failure, got %v", dedup.processed)
	}
}

// TestProcessDeliveryDuplicateSkipsHandler 验证重复事件被 ack 且跳过业务处理。
func TestProcessDeliveryDuplicateSkipsHandler(t *testing.T) {
	evt := event.Event{ID: 3, Type: event.TypeOrderCreated, Topic: "order.created", Payload: json.RawMessage(`{}`)}
	body := encodeMessage(t, evt)

	h := &mockHandler{}
	dedup := &countingDedup{duplicates: map[int64]bool{3: true}} // already processed
	c := newTestConsumer(h, dedup)
	acker := &mockAcker{}

	c.processDelivery(context.Background(), body, acker)

	if !acker.acked || acker.nacked {
		t.Fatalf("expected ack for duplicate, got acked=%v nacked=%v", acker.acked, acker.nacked)
	}
	if len(h.calls) != 0 {
		t.Fatalf("expected 0 handler calls for duplicate, got %d", len(h.calls))
	}
}

// TestProcessDeliveryDecodeErrorNacksToDLX 验证解码失败时 nack 到死信队列。
func TestProcessDeliveryDecodeErrorNacksToDLX(t *testing.T) {
	h := &mockHandler{}
	dedup := &countingDedup{duplicates: map[int64]bool{}}
	c := newTestConsumer(h, dedup)
	acker := &mockAcker{}

	c.processDelivery(context.Background(), []byte("not-json"), acker)

	if acker.acked || !acker.nacked {
		t.Fatalf("expected nack for decode error, got acked=%v nacked=%v", acker.acked, acker.nacked)
	}
	if acker.requeue {
		t.Fatalf("expected requeue=false for decode error, got requeue=true")
	}
	if len(h.calls) != 0 {
		t.Fatalf("expected 0 handler calls, got %d", len(h.calls))
	}
}

// TestProcessDeliveryDedupErrorRequeues 验证去重检查失败时重新入队。
func TestProcessDeliveryDedupErrorRequeues(t *testing.T) {
	evt := event.Event{ID: 4, Type: event.TypeOrderCreated, Topic: "order.created", Payload: json.RawMessage(`{}`)}
	body := encodeMessage(t, evt)

	h := &mockHandler{}
	dedup := &countingDedup{
		duplicates: map[int64]bool{},
		checkErr:   errors.New("redis unavailable"),
	}
	c := newTestConsumer(h, dedup)
	acker := &mockAcker{}

	c.processDelivery(context.Background(), body, acker)

	if acker.acked || !acker.nacked {
		t.Fatalf("expected nack for dedup error, got acked=%v nacked=%v", acker.acked, acker.nacked)
	}
	if !acker.requeue {
		t.Fatalf("expected requeue=true for transient dedup error, got requeue=false")
	}
	if len(h.calls) != 0 {
		t.Fatalf("expected 0 handler calls when dedup fails, got %d", len(h.calls))
	}
}
