package kafka

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
)

func BenchmarkKafkaPublish(b *testing.B) {
	brokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")
	if len(brokers) == 1 && strings.TrimSpace(brokers[0]) == "" {
		b.Skip("KAFKA_BROKERS is not set")
	}
	publisher, err := NewPublisher(brokers, envOr("KAFKA_TOPIC", "redcart.events.bench"))
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
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := publisher.Publish(ctx, evt); err != nil {
			b.Fatalf("publish: %v", err)
		}
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
