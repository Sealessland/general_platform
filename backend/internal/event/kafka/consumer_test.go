package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
	kafkago "github.com/segmentio/kafka-go"
)

type mockHandler struct {
	called int
	events []event.Event
	err    error
}

func (h *mockHandler) Handle(_ context.Context, evt event.Event) error {
	h.called++
	h.events = append(h.events, evt)
	return h.err
}

type countingDedup struct {
	seen      map[int64]bool
	checkErr  error
	markErr   error
	markCalls int
}

func (d *countingDedup) IsDuplicate(_ context.Context, eventID int64) (bool, error) {
	if d.checkErr != nil {
		return false, d.checkErr
	}
	return d.seen[eventID], nil
}

func (d *countingDedup) MarkProcessed(_ context.Context, eventID int64) error {
	d.markCalls++
	if d.markErr != nil {
		return d.markErr
	}
	d.seen[eventID] = true
	return nil
}

type recordingWriter struct {
	messages []kafkago.Message
	err      error
	closed   bool
}

func (w *recordingWriter) WriteMessages(_ context.Context, msgs ...kafkago.Message) error {
	if w.err != nil {
		return w.err
	}
	w.messages = append(w.messages, msgs...)
	return nil
}

func (w *recordingWriter) Close() error {
	w.closed = true
	return nil
}

func encodeMessage(t *testing.T, evt event.Event) []byte {
	t.Helper()
	body, err := encodeEvent(evt)
	if err != nil {
		t.Fatalf("encode event: %v", err)
	}
	return body
}

func discardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

func newTestConsumer(h Handler, d Deduplicator, writer *recordingWriter) *Consumer {
	return &Consumer{
		handler:   h,
		dedup:     d,
		dlqWriter: writer,
		dlqTopic:  "redcart.events.dlq",
		logger:    discardLogger(),
	}
}

func TestProcessMessageSuccessCommitsAndMarksProcessed(t *testing.T) {
	handler := &mockHandler{}
	dedup := &countingDedup{seen: make(map[int64]bool)}
	writer := &recordingWriter{}
	consumer := newTestConsumer(handler, dedup, writer)
	evt := event.Event{ID: 42, Type: event.TypeOrderPaid, Topic: event.TypeOrderPaid.Topic(), Payload: json.RawMessage(`{"order_id":42}`), OccurredAt: time.Now().UTC()}

	commit, err := consumer.processMessage(context.Background(), kafkago.Message{Topic: "redcart.events.order.paid", Value: encodeMessage(t, evt)})
	if err != nil {
		t.Fatalf("process message: %v", err)
	}
	if !commit {
		t.Fatal("expected source offset commit")
	}
	if handler.called != 1 || handler.events[0].ID != 42 {
		t.Fatalf("handler not called with event: calls=%d events=%v", handler.called, handler.events)
	}
	if !dedup.seen[42] || dedup.markCalls != 1 {
		t.Fatalf("expected event marked processed once, seen=%v markCalls=%d", dedup.seen, dedup.markCalls)
	}
	if len(writer.messages) != 0 {
		t.Fatalf("expected no DLQ messages, got %d", len(writer.messages))
	}
}

func TestProcessMessageHandlerFailureWritesDLQAndCommits(t *testing.T) {
	handler := &mockHandler{err: errors.New("boom")}
	dedup := &countingDedup{seen: make(map[int64]bool)}
	writer := &recordingWriter{}
	consumer := newTestConsumer(handler, dedup, writer)
	evt := event.Event{ID: 7, Type: event.TypeOrderCancelled, Topic: event.TypeOrderCancelled.Topic(), Payload: json.RawMessage(`{"order_id":7}`), OccurredAt: time.Now().UTC()}

	commit, err := consumer.processMessage(context.Background(), kafkago.Message{Topic: "redcart.events.order.cancelled", Partition: 1, Offset: 9, Value: encodeMessage(t, evt)})
	if err != nil {
		t.Fatalf("process message: %v", err)
	}
	if !commit {
		t.Fatal("expected failed source message to be committed after DLQ write")
	}
	if len(writer.messages) != 1 {
		t.Fatalf("expected one DLQ message, got %d", len(writer.messages))
	}
	if got := writer.messages[0].Topic; got != "redcart.events.dlq" {
		t.Fatalf("expected DLQ topic, got %q", got)
	}
	if dedup.markCalls != 0 {
		t.Fatalf("expected no mark on handler failure, got %d", dedup.markCalls)
	}
}

func TestProcessMessageDuplicateSkipsHandler(t *testing.T) {
	handler := &mockHandler{}
	dedup := &countingDedup{seen: map[int64]bool{55: true}}
	writer := &recordingWriter{}
	consumer := newTestConsumer(handler, dedup, writer)
	evt := event.Event{ID: 55, Type: event.TypeOrderFinished, Topic: event.TypeOrderFinished.Topic(), Payload: json.RawMessage(`{"order_id":55}`), OccurredAt: time.Now().UTC()}

	commit, err := consumer.processMessage(context.Background(), kafkago.Message{Value: encodeMessage(t, evt)})
	if err != nil {
		t.Fatalf("process message: %v", err)
	}
	if !commit {
		t.Fatal("expected duplicate source offset commit")
	}
	if handler.called != 0 {
		t.Fatalf("expected duplicate to skip handler, got %d calls", handler.called)
	}
	if len(writer.messages) != 0 {
		t.Fatalf("expected no DLQ messages, got %d", len(writer.messages))
	}
}

func TestProcessMessageDecodeErrorWritesDLQAndCommits(t *testing.T) {
	handler := &mockHandler{}
	dedup := &countingDedup{seen: make(map[int64]bool)}
	writer := &recordingWriter{}
	consumer := newTestConsumer(handler, dedup, writer)

	commit, err := consumer.processMessage(context.Background(), kafkago.Message{Topic: "redcart.events.order.paid", Partition: 2, Offset: 3, Value: []byte(`{bad json`)})
	if err != nil {
		t.Fatalf("process message: %v", err)
	}
	if !commit {
		t.Fatal("expected malformed source offset commit after DLQ write")
	}
	if handler.called != 0 {
		t.Fatalf("expected handler not called, got %d", handler.called)
	}
	if len(writer.messages) != 1 {
		t.Fatalf("expected one DLQ message, got %d", len(writer.messages))
	}
}

func TestProcessMessageDedupErrorLeavesUncommitted(t *testing.T) {
	handler := &mockHandler{}
	dedup := &countingDedup{seen: make(map[int64]bool), checkErr: errors.New("redis down")}
	writer := &recordingWriter{}
	consumer := newTestConsumer(handler, dedup, writer)
	evt := event.Event{ID: 99, Type: event.TypeOrderPaid, Topic: event.TypeOrderPaid.Topic(), Payload: json.RawMessage(`{"order_id":99}`), OccurredAt: time.Now().UTC()}

	commit, err := consumer.processMessage(context.Background(), kafkago.Message{Value: encodeMessage(t, evt)})
	if err == nil {
		t.Fatal("expected dedup error")
	}
	if commit {
		t.Fatal("expected source offset left uncommitted")
	}
	if handler.called != 0 {
		t.Fatalf("expected handler not called, got %d", handler.called)
	}
	if len(writer.messages) != 0 {
		t.Fatalf("expected no DLQ messages, got %d", len(writer.messages))
	}
}

func TestParseBrokersAndTopicPrefix(t *testing.T) {
	brokers := ParseBrokers(" 127.0.0.1:9092, kafka:9092,,")
	if len(brokers) != 2 || brokers[0] != "127.0.0.1:9092" || brokers[1] != "kafka:9092" {
		t.Fatalf("unexpected brokers: %#v", brokers)
	}
	if got := qualifyTopic("redcart.events", "order.paid"); got != "redcart.events.order.paid" {
		t.Fatalf("unexpected qualified topic: %q", got)
	}
	if got := qualifyTopic("redcart.events", "redcart.events.order.paid"); got != "redcart.events.order.paid" {
		t.Fatalf("unexpected already-qualified topic: %q", got)
	}
}
