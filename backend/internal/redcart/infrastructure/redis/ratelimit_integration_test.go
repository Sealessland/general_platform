package redis

import (
	"context"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// newRedisRateLimitFixture 依赖真实 Redis 的限流集成夹具：
// 未设置 RUN_REDIS_INTEGRATION=1 或 REDIS_ADDR 时跳过，返回限流器与底层客户端。
func newRedisRateLimitFixture(t *testing.T) (*Limiter, goredis.Cmdable) {
	t.Helper()
	if os.Getenv("RUN_REDIS_INTEGRATION") != "1" {
		t.Skip("RUN_REDIS_INTEGRATION is not set")
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR is not set")
	}
	client, err := NewClient(addr)
	if err != nil {
		t.Fatalf("new redis client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return NewLimiter(client), client
}

// TestLimiterTokenBucket 验证 Lua 令牌桶的真实行为：前 burst 次放行且
// 剩余令牌递减、超限拒绝并返回正重试时间、等待补充后可恢复、key 自动过期。
func TestLimiterTokenBucket(t *testing.T) {
	limiter, client := newRedisRateLimitFixture(t)
	ctx := context.Background()
	key := RateLimitKey("write", "integration-user")
	if err := client.Del(ctx, key).Err(); err != nil {
		t.Fatalf("clean key: %v", err)
	}
	t.Cleanup(func() { _ = client.Del(ctx, key).Err() })

	// 速率 10/s、桶容量 2：可连续放行 2 次
	policy := Policy{RatePerSecond: 10, Burst: 2}
	now := time.Now()

	first, err := limiter.Allow(ctx, key, policy, now)
	if err != nil {
		t.Fatalf("first allow: %v", err)
	}
	if !first.Allowed || first.Remaining != 1 {
		t.Fatalf("expected allowed with remaining 1, got %+v", first)
	}

	second, err := limiter.Allow(ctx, key, policy, now)
	if err != nil {
		t.Fatalf("second allow: %v", err)
	}
	if !second.Allowed || second.Remaining != 0 {
		t.Fatalf("expected allowed with remaining 0, got %+v", second)
	}

	third, err := limiter.Allow(ctx, key, policy, now)
	if err != nil {
		t.Fatalf("third allow: %v", err)
	}
	if third.Allowed {
		t.Fatal("expected rejection after burst consumed")
	}
	if third.RetryAfter <= 0 {
		t.Fatalf("expected positive retry after, got %+v", third)
	}
	if third.Remaining != 0 {
		t.Fatalf("expected remaining 0, got %d", third.Remaining)
	}

	// 等待 300ms：10/s 速率应补充至少 1 个令牌（300ms*10=3，截断到 burst），再次放行
	time.Sleep(300 * time.Millisecond)
	refilled, err := limiter.Allow(ctx, key, policy, time.Now())
	if err != nil {
		t.Fatalf("allow after refill: %v", err)
	}
	if !refilled.Allowed {
		t.Fatalf("expected allowed after refill, got %+v", refilled)
	}

	// 脚本应设置 key 过期时间，避免长期堆积
	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("ttl: %v", err)
	}
	if ttl <= 0 {
		t.Fatalf("expected positive ttl, got %s", ttl)
	}
}
