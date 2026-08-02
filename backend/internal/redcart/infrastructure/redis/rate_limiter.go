package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/example/redcart-copilot/backend/internal/ratelimit"
	goredis "github.com/redis/go-redis/v9"
)

var tokenBucketScript = goredis.NewScript(`
local redis_time = redis.call('TIME')
local now = (tonumber(redis_time[1]) * 1000) + math.floor(tonumber(redis_time[2]) / 1000)
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local current = redis.call('HMGET', KEYS[1], 'tokens', 'updated_at')
local tokens = tonumber(current[1])
local updated_at = tonumber(current[2])

if tokens == nil then tokens = burst end
if updated_at == nil then updated_at = now end
if now < updated_at then now = updated_at end

tokens = math.min(burst, tokens + ((now - updated_at) * rate / 1000))
local allowed = 0
local retry_after = 0
if tokens >= 1 then
  allowed = 1
  tokens = tokens - 1
else
  retry_after = math.ceil((1 - tokens) * 1000 / rate)
end

redis.call('HSET', KEYS[1], 'tokens', tokens, 'updated_at', now)
redis.call('PEXPIRE', KEYS[1], math.max(1000, math.ceil((burst / rate) * 2000)))
return {allowed, retry_after, math.floor(tokens)}
`)

// RateLimiter implements a distributed token bucket. One Lua script performs
// refill, admission, deduction, and expiry atomically on the Redis server.
type RateLimiter struct {
	client goredis.UniversalClient
}

func NewRateLimiter(client goredis.UniversalClient) *RateLimiter {
	return &RateLimiter{client: client}
}

func (l *RateLimiter) Allow(ctx context.Context, key string, policy ratelimit.Policy) (ratelimit.Decision, error) {
	if key == "" || policy.RatePerSecond <= 0 || policy.Burst < 1 {
		return ratelimit.Decision{}, errors.New("invalid rate limit request")
	}
	if l.client == nil {
		return ratelimit.Decision{}, errors.New("Redis rate limiter is not configured")
	}
	result, err := tokenBucketScript.Run(ctx, l.client, []string{key}, policy.RatePerSecond, policy.Burst).Slice()
	if err != nil {
		return ratelimit.Decision{}, fmt.Errorf("evaluate Redis rate limit: %w", err)
	}
	if len(result) != 3 {
		return ratelimit.Decision{}, fmt.Errorf("unexpected Redis rate limit response length %d", len(result))
	}
	allowed, err := redisInteger(result[0])
	if err != nil {
		return ratelimit.Decision{}, err
	}
	retryMilliseconds, err := redisInteger(result[1])
	if err != nil {
		return ratelimit.Decision{}, err
	}
	remaining, err := redisInteger(result[2])
	if err != nil {
		return ratelimit.Decision{}, err
	}
	return ratelimit.Decision{
		Allowed:    allowed == 1,
		Remaining:  int(remaining),
		RetryAfter: time.Duration(retryMilliseconds) * time.Millisecond,
	}, nil
}

func redisInteger(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		var parsed int64
		if _, err := fmt.Sscan(typed, &parsed); err != nil {
			return 0, fmt.Errorf("parse Redis integer %q: %w", typed, err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unexpected Redis integer type %T", value)
	}
}

var _ ratelimit.Limiter = (*RateLimiter)(nil)
