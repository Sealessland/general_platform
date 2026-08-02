package redis

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/ratelimit"
)

func TestRateLimiterRejectsInvalidPolicy(t *testing.T) {
	limiter := NewRateLimiter(nil)
	_, err := limiter.Allow(context.Background(), "key", ratelimit.Policy{})
	if err == nil {
		t.Fatal("expected invalid policy error")
	}
}

func TestRateLimiterTokenBucketIsAtomic(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR is not set")
	}
	client, err := NewClient(addr)
	if err != nil {
		t.Fatalf("connect Redis: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	limiter := NewRateLimiter(client)
	key := fmt.Sprintf("redcart:test:rate:%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = client.Del(context.Background(), key).Err() })

	const requests = 20
	var allowed atomic.Int64
	var wait sync.WaitGroup
	wait.Add(requests)
	for range requests {
		go func() {
			defer wait.Done()
			decision, err := limiter.Allow(context.Background(), key, ratelimit.Policy{RatePerSecond: 0.01, Burst: 5})
			if err != nil {
				t.Errorf("allow: %v", err)
				return
			}
			if decision.Allowed {
				allowed.Add(1)
			}
		}()
	}
	wait.Wait()
	if got := allowed.Load(); got != 5 {
		t.Fatalf("allowed requests = %d, want exactly burst size 5", got)
	}

	decision, err := limiter.Allow(context.Background(), key, ratelimit.Policy{RatePerSecond: 0.01, Burst: 5})
	if err != nil {
		t.Fatalf("final allow: %v", err)
	}
	if decision.Allowed || decision.RetryAfter <= 0 {
		t.Fatalf("final decision = %+v, want rejection with retry delay", decision)
	}
}
