package redis

import "testing"

// TestRateLimitPolicyFromEnv 验证限流策略环境变量解析：
// 空串回退默认值，合法值覆盖默认值，非法值返回带变量名的错误。
func TestRateLimitPolicyFromEnv(t *testing.T) {
	fallback := Policy{RatePerSecond: DefaultWriteRate, Burst: DefaultWriteBurst}

	// 全部为空串：回退默认值
	policy, err := RateLimitPolicyFromEnv("RATE_LIMIT_WRITE", "", "", fallback)
	if err != nil {
		t.Fatalf("default policy: %v", err)
	}
	if policy != fallback {
		t.Fatalf("expected default policy %+v, got %+v", fallback, policy)
	}

	// 合法值：覆盖默认值
	policy, err = RateLimitPolicyFromEnv("RATE_LIMIT_WRITE", "10", "20", fallback)
	if err != nil {
		t.Fatalf("custom policy: %v", err)
	}
	if policy.RatePerSecond != 10 || policy.Burst != 20 {
		t.Fatalf("expected rate=10 burst=20, got %+v", policy)
	}

	// 只覆盖 rate：burst 保留默认值
	policy, err = RateLimitPolicyFromEnv("RATE_LIMIT_WRITE", "8", "", fallback)
	if err != nil {
		t.Fatalf("rate-only policy: %v", err)
	}
	if policy.RatePerSecond != 8 || policy.Burst != DefaultWriteBurst {
		t.Fatalf("expected rate=8 burst=%d, got %+v", DefaultWriteBurst, policy)
	}

	// 只覆盖 burst：rate 保留默认值
	policy, err = RateLimitPolicyFromEnv("RATE_LIMIT_WRITE", "", "30", fallback)
	if err != nil {
		t.Fatalf("burst-only policy: %v", err)
	}
	if policy.RatePerSecond != DefaultWriteRate || policy.Burst != 30 {
		t.Fatalf("expected rate=%d burst=30, got %+v", DefaultWriteRate, policy)
	}

	// 非法 rate：非数字、零、负数、NaN、Inf
	for _, raw := range []string{"abc", "0", "-1", "NaN", "Inf"} {
		if _, err := RateLimitPolicyFromEnv("RATE_LIMIT_WRITE", raw, "", fallback); err == nil {
			t.Fatalf("expected rate error for %q", raw)
		}
	}

	// 非法 burst：非数字、零、负数
	for _, raw := range []string{"abc", "0", "-3", "1.5"} {
		if _, err := RateLimitPolicyFromEnv("RATE_LIMIT_WRITE", "", raw, fallback); err == nil {
			t.Fatalf("expected burst error for %q", raw)
		}
	}
}

// TestRateLimitKey 验证限流 key 结构为 redcart:ratelimit:<class>:<identity>。
func TestRateLimitKey(t *testing.T) {
	if got := RateLimitKey("write", "user:42"); got != "redcart:ratelimit:write:user:42" {
		t.Fatalf("unexpected key %q", got)
	}
	if got := RateLimitKey("ai", "ip:203.0.113.7"); got != "redcart:ratelimit:ai:ip:203.0.113.7" {
		t.Fatalf("unexpected key %q", got)
	}
}
