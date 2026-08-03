package rabbitmq

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
)

// skipIfNoRabbitMQ skips the benchmark when RABBITMQ_ADDR is not set.
// skipIfNoRabbitMQ 在未配置 RABBITMQ_ADDR 时跳过基准测试，并返回该地址。
func skipIfNoRabbitMQ(b *testing.B) string {
	b.Helper()
	addr := os.Getenv("RABBITMQ_ADDR")
	if addr == "" {
		b.Skip("RABBITMQ_ADDR not set")
	}
	return addr
}

// BenchmarkRabbitMQPublish 压测 RabbitMQ 发布路径；仅在设置 RABBITMQ_ADDR 时执行。
func BenchmarkRabbitMQPublish(b *testing.B) {
	addr := skipIfNoRabbitMQ(b)
	publisher, err := NewPublisher(addr, os.Getenv("RABBITMQ_EXCHANGE"))
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
