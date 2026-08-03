// 令牌桶限流：基于 Redis Lua 脚本原子执行「读取桶 → 按时间补充 → 扣减
// 或拒绝 → 回写」流程，供 HTTP 中间件按类别（如写接口、AI 接口）限流。
package redis

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// tokenBucketScript 原子执行令牌桶算法：
//   - HMGET 读取 tokens 与 updated_at，首次访问按满桶与当前时间初始化；
//   - 按 (now - updated_at) * rate 补充令牌并截断到 burst；
//   - 有令牌则扣减 1 放行，否则按补齐一个令牌所需时间计算 retry_after；
//   - HSET 回写并 PEXPIRE 设置过期，避免 key 无限堆积。
var tokenBucketScript = goredis.NewScript(`
local now = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
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

// KeyPrefix 是限流 key 的统一前缀：redcart:ratelimit:<class>:<identity>。
const KeyPrefix = "redcart:ratelimit:"

// 各限流类别的默认策略；未设置对应环境变量时作为回退值。
const (
	// DefaultWriteRate / DefaultWriteBurst：订单创建、支付等关键写接口，默认 5 QPS、桶容量 10。
	DefaultWriteRate  = 5
	DefaultWriteBurst = 10
	// DefaultAIRate / DefaultAIBurst：AI 生成接口，默认 1 QPS、桶容量 3。
	DefaultAIRate  = 1
	DefaultAIBurst = 3
)

// Policy 描述令牌桶策略：每秒补充 RatePerSecond 个令牌，桶容量最多 Burst。
type Policy struct {
	RatePerSecond float64
	Burst         int
}

// Decision 是一次限流评估的结果：是否放行、拒绝后建议等待时间与剩余令牌数。
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
	Remaining  int
}

// Limiter 通过 Redis 原子脚本评估并更新令牌桶。
type Limiter struct {
	client goredis.UniversalClient
}

// NewLimiter 创建基于 client 的令牌桶限流器。
func NewLimiter(client goredis.UniversalClient) *Limiter {
	return &Limiter{client: client}
}

// RateLimitKey 构造限流 key：redcart:ratelimit:<class>:<identity>。
func RateLimitKey(class, identity string) string {
	return KeyPrefix + class + ":" + identity
}

// Allow 原子地评估并更新 key 对应的令牌桶：按策略扣减一个令牌并返回决策。
// 参数非法（key 为空、速率非正或桶容量小于 1）或 Redis 调用失败时返回错误。
func (limiter *Limiter) Allow(ctx context.Context, key string, policy Policy, now time.Time) (Decision, error) {
	if key == "" || policy.RatePerSecond <= 0 || policy.Burst < 1 {
		return Decision{}, fmt.Errorf("invalid rate limit request: key=%q rate=%v burst=%d", key, policy.RatePerSecond, policy.Burst)
	}
	result, err := tokenBucketScript.Run(ctx, limiter.client, []string{key},
		now.UnixMilli(), policy.RatePerSecond, policy.Burst).Slice()
	if err != nil {
		return Decision{}, fmt.Errorf("evaluate redis rate limit: %w", err)
	}
	if len(result) != 3 {
		return Decision{}, fmt.Errorf("unexpected redis rate limit response length %d", len(result))
	}
	allowed, err := asInt64(result[0])
	if err != nil {
		return Decision{}, fmt.Errorf("parse allowed flag: %w", err)
	}
	retryMilliseconds, err := asInt64(result[1])
	if err != nil {
		return Decision{}, fmt.Errorf("parse retry_after: %w", err)
	}
	remaining, err := asInt64(result[2])
	if err != nil {
		return Decision{}, fmt.Errorf("parse remaining tokens: %w", err)
	}
	return Decision{
		Allowed:    allowed == 1,
		RetryAfter: time.Duration(retryMilliseconds) * time.Millisecond,
		Remaining:  int(remaining),
	}, nil
}

// RateLimitPolicyFromEnv 解析类别前缀 prefix 对应的限流策略环境变量：
// <prefix>_RATE（每秒补充令牌数，浮点）与 <prefix>_BURST（桶容量，正整数）。
// 任一参数为空串时回退 fallback 对应字段；非法值返回带环境变量名的错误。
func RateLimitPolicyFromEnv(prefix, rateRaw, burstRaw string, fallback Policy) (Policy, error) {
	policy := fallback
	if rateRaw = strings.TrimSpace(rateRaw); rateRaw != "" {
		rate, err := strconv.ParseFloat(rateRaw, 64)
		if err != nil || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			return Policy{}, fmt.Errorf("%s_RATE must be a positive number, got %q", prefix, rateRaw)
		}
		policy.RatePerSecond = rate
	}
	if burstRaw = strings.TrimSpace(burstRaw); burstRaw != "" {
		burst, err := strconv.Atoi(burstRaw)
		if err != nil || burst < 1 {
			return Policy{}, fmt.Errorf("%s_BURST must be a positive integer, got %q", prefix, burstRaw)
		}
		policy.Burst = burst
	}
	return policy, nil
}

// asInt64 把 Redis 脚本返回的整数（int64 或十进制字符串）转为 int64。
func asInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		var parsed int64
		if _, err := fmt.Sscan(typed, &parsed); err != nil {
			return 0, fmt.Errorf("parse redis integer %q: %w", typed, err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unexpected redis integer type %T", value)
	}
}
