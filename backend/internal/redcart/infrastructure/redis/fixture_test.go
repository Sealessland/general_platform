package redis

import (
	"context"
	"os"
	"testing"

	postgresrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres"
	goredis "github.com/redis/go-redis/v9"
)

type redisPostgresFixture struct {
	repo   *postgresrepo.Repository
	client *goredis.Client
}

func newRedisPostgresFixture(t *testing.T) redisPostgresFixture {
	t.Helper()
	if os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("RUN_POSTGRES_INTEGRATION is not set")
	}
	dsn := os.Getenv("POSTGRES_DSN")
	addr := os.Getenv("REDIS_ADDR")
	if dsn == "" || addr == "" {
		t.Skip("POSTGRES_DSN and REDIS_ADDR are required")
	}
	repo, err := postgresrepo.NewRepository(dsn)
	if err != nil {
		t.Fatalf("new postgres repository: %v", err)
	}
	client, err := NewClient(addr)
	if err != nil {
		_ = repo.Close()
		t.Fatalf("new redis client: %v", err)
	}
	t.Cleanup(func() {
		_ = client.FlushDB(context.Background()).Err()
		_ = client.Close()
		_ = repo.Close()
	})
	return redisPostgresFixture{repo: repo, client: client}
}
