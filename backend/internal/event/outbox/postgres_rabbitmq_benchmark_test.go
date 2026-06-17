package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
	rabbitmqevent "github.com/example/redcart-copilot/backend/internal/event/rabbitmq"
	postgresrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func BenchmarkPostgresRabbitMQOutboxRelay(b *testing.B) {
	if os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		b.Skip("RUN_POSTGRES_INTEGRATION is not set")
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		b.Skip("POSTGRES_DSN is not set")
	}
	addr := os.Getenv("RABBITMQ_ADDR")
	if addr == "" {
		b.Skip("RABBITMQ_ADDR is not set")
	}

	repo, err := postgresrepo.NewRepository(dsn)
	if err != nil {
		b.Fatalf("new postgres repository: %v", err)
	}
	defer repo.Close()
	publisher, err := rabbitmqevent.NewPublisher(addr, os.Getenv("RABBITMQ_EXCHANGE"))
	if err != nil {
		b.Fatalf("new rabbitmq publisher: %v", err)
	}
	defer publisher.Close()

	ctx := context.Background()
	rawDB, err := sql.Open("pgx", dsn)
	if err != nil {
		b.Fatalf("open raw postgres conn: %v", err)
	}
	defer rawDB.Close()
	if _, err := rawDB.ExecContext(ctx, `DELETE FROM outbox_dead_letter`); err != nil {
		b.Fatalf("clear dead letter outbox: %v", err)
	}
	if _, err := rawDB.ExecContext(ctx, `DELETE FROM outbox`); err != nil {
		b.Fatalf("clear outbox: %v", err)
	}

	payload := json.RawMessage(`{"order_id":42,"status":"PAID"}`)
	now := time.Now().UTC()
	relay := NewPublisher(repo.Outbox, publisher, Config{
		Interval:  time.Hour,
		BatchSize: 1,
		Logger:    log.New(ioDiscard{}, "", 0),
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := repo.Outbox.Append(ctx, event.Event{
			Type:          event.TypeOrderPaid,
			Topic:         event.TypeOrderPaid.Topic(),
			CorrelationID: fmt.Sprintf("relay-bench-%d", i),
			Payload:       payload,
			OccurredAt:    now,
		}); err != nil {
			b.Fatalf("append outbox: %v", err)
		}
		relay.tick(ctx)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
