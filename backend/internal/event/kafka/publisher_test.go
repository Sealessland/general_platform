package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
	"github.com/twmb/franz-go/pkg/kgo"
)

type recordingProducer struct {
	records []*kgo.Record
	err     error
}

func (p *recordingProducer) ProduceSync(_ context.Context, records ...*kgo.Record) kgo.ProduceResults {
	p.records = append(p.records, records...)
	results := make(kgo.ProduceResults, 0, len(records))
	for _, record := range records {
		results = append(results, kgo.ProduceResult{Record: record, Err: p.err})
	}
	return results
}

func (p *recordingProducer) Close() {}

func TestPublisherMapsDomainEventToOneKafkaTopic(t *testing.T) {
	producer := &recordingProducer{}
	publisher := newPublisher(producer, "redcart.events")
	occurredAt := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	evt := event.Event{
		ID:            42,
		Type:          event.TypeOrderCreated,
		Topic:         "order.created",
		CorrelationID: "order-1001",
		Payload:       json.RawMessage(`{"order_id":1001}`),
		OccurredAt:    occurredAt,
	}

	if err := publisher.Publish(context.Background(), evt); err != nil {
		t.Fatalf("publish event: %v", err)
	}
	if len(producer.records) != 1 {
		t.Fatalf("expected one Kafka record, got %d", len(producer.records))
	}
	record := producer.records[0]
	if record.Topic != "redcart.events" {
		t.Fatalf("record topic = %q, want redcart.events", record.Topic)
	}
	if string(record.Key) != "order-1001" {
		t.Fatalf("record key = %q, want correlation ID", record.Key)
	}
	if !record.Timestamp.Equal(occurredAt) {
		t.Fatalf("record timestamp = %s, want %s", record.Timestamp, occurredAt)
	}

	decoded, err := decodeEvent(record.Value)
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if decoded.ID != evt.ID || decoded.Type != evt.Type || decoded.Topic != evt.Topic {
		t.Fatalf("decoded event = %+v, want %+v", decoded, evt)
	}
	if string(decoded.Payload) != string(evt.Payload) {
		t.Fatalf("decoded payload = %s, want %s", decoded.Payload, evt.Payload)
	}
}

func TestPublisherFallsBackToEventIDAsKey(t *testing.T) {
	producer := &recordingProducer{}
	publisher := newPublisher(producer, "redcart.events")

	err := publisher.Publish(context.Background(), event.Event{
		ID:         7,
		Type:       event.TypeOrderPaid,
		Topic:      "order.paid",
		Payload:    json.RawMessage(`{}`),
		OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("publish event: %v", err)
	}
	if got := string(producer.records[0].Key); got != "7" {
		t.Fatalf("record key = %q, want event ID", got)
	}
}

func TestPublisherReturnsBrokerAcknowledgementError(t *testing.T) {
	producer := &recordingProducer{err: errors.New("broker unavailable")}
	publisher := newPublisher(producer, "redcart.events")

	err := publisher.Publish(context.Background(), event.Event{
		ID:         8,
		Type:       event.TypeOrderPaid,
		Topic:      "order.paid",
		Payload:    json.RawMessage(`{}`),
		OccurredAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected broker acknowledgement error")
	}
}
