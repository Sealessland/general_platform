package kafka

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
)

// skipIfNoKafka skips the benchmark when KAFKA_BROKERS is not set.
func skipIfNoKafka(b *testing.B) []string {
	b.Helper()
	brokers := ParseBrokers(os.Getenv("KAFKA_BROKERS"))
	if len(brokers) == 0 {
		b.Skip("KAFKA_BROKERS not set")
	}
	return brokers
}

// BenchmarkKafkaPublish measures the real Kafka publish path.
func BenchmarkKafkaPublish(b *testing.B) {
	brokers := skipIfNoKafka(b)
	publisher, err := NewPublisher(brokers, PublisherConfig{
		TopicPrefix: os.Getenv("KAFKA_TOPIC_PREFIX"),
	})
	if err != nil {
		b.Fatalf("create publisher: %v", err)
	}
	defer publisher.Close()

	evt := event.Event{
		ID:         1,
		Type:       event.TypeOrderPaid,
		Topic:      event.TypeOrderPaid.Topic(),
		Payload:    json.RawMessage(`{"order_id":42}`),
		OccurredAt: time.Now().UTC(),
	}

	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if err := publisher.Publish(ctx, evt); err != nil {
			b.Fatalf("publish: %v", err)
		}
	}
}
