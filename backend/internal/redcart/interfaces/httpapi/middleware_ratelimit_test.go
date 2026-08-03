package httpapi

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	redisrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/redis"
)

// stubLimiter 是 rateLimiter 接口的测试替身：记录最后一次调用参数并返回预设结果。
type stubLimiter struct {
	mu         sync.Mutex
	decision   redisrepo.Decision
	err        error
	lastKey    string
	lastPolicy redisrepo.Policy
}

// Allow 记录本次调用的 key 与策略并返回预设的决策/错误，供测试断言。
func (s *stubLimiter) Allow(_ context.Context, key string, policy redisrepo.Policy, _ time.Time) (redisrepo.Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastKey = key
	s.lastPolicy = policy
	return s.decision, s.err
}

// snapshot 并发安全地读取最近一次调用的 key 与策略。
func (s *stubLimiter) snapshot() (string, redisrepo.Policy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastKey, s.lastPolicy
}

// newTestRateLimitMiddleware 构造丢弃日志输出的限流中间件，避免测试输出噪音。
func newTestRateLimitMiddleware(limiter rateLimiter) *RateLimitMiddleware {
	return NewRateLimitMiddleware(limiter, log.New(io.Discard, "", 0),
		redisrepo.Policy{RatePerSecond: 5, Burst: 10},
		redisrepo.Policy{RatePerSecond: 1, Burst: 3},
	)
}

// TestRateLimitMiddlewareAllowed 验证放行时调用 next、写入 RateLimit-Remaining
// 头，且 key 使用已认证用户 ID 与对应类别策略。
func TestRateLimitMiddlewareAllowed(t *testing.T) {
	stub := &stubLimiter{decision: redisrepo.Decision{Allowed: true, Remaining: 7}}
	writePolicy := redisrepo.Policy{RatePerSecond: 5, Burst: 10}
	middleware := NewRateLimitMiddleware(stub, log.New(io.Discard, "", 0), writePolicy, redisrepo.Policy{RatePerSecond: 1, Burst: 3})

	nextCalled := false
	handler := middleware.limit(rateLimitClassWrite, func(w http.ResponseWriter, _ *http.Request, _ application.Actor) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/orders", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	rec := httptest.NewRecorder()
	handler(rec, req, application.Actor{UserID: 42, Role: "consumer"})

	if !nextCalled {
		t.Fatal("expected next handler to run on allowed decision")
	}
	if got := rec.Header().Get("RateLimit-Remaining"); got != "7" {
		t.Fatalf("expected RateLimit-Remaining 7, got %q", got)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	key, policy := stub.snapshot()
	if key != "redcart:ratelimit:write:user:42" {
		t.Fatalf("unexpected key %q", key)
	}
	if policy != writePolicy {
		t.Fatalf("unexpected policy %+v", policy)
	}
}

// TestRateLimitMiddlewareRejected 验证拒绝时写 429（既有错误格式）、
// Retry-After / RateLimit-Remaining 头，且不调用 next。
func TestRateLimitMiddlewareRejected(t *testing.T) {
	stub := &stubLimiter{decision: redisrepo.Decision{Allowed: false, RetryAfter: 1200 * time.Millisecond, Remaining: 0}}
	middleware := newTestRateLimitMiddleware(stub)

	nextCalled := false
	handler := middleware.limit(rateLimitClassAI, func(w http.ResponseWriter, _ *http.Request, _ application.Actor) {
		nextCalled = true
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ai/business-review", nil)
	rec := httptest.NewRecorder()
	handler(rec, req, application.Actor{UserID: 7})

	if nextCalled {
		t.Fatal("expected next handler not to run on rejected decision")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("expected Retry-After 2, got %q", got)
	}
	if got := rec.Header().Get("RateLimit-Remaining"); got != "0" {
		t.Fatalf("expected RateLimit-Remaining 0, got %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "rate_limit_exceeded") {
		t.Fatalf("expected error kind rate_limit_exceeded, body=%s", body)
	}
}

// TestRateLimitMiddlewareBackendErrorFailsOpen 验证 Redis 故障时 fail-open：
// 调用 next 且不写任何限流响应头。
func TestRateLimitMiddlewareBackendErrorFailsOpen(t *testing.T) {
	stub := &stubLimiter{err: context.DeadlineExceeded}
	middleware := newTestRateLimitMiddleware(stub)

	nextCalled := false
	handler := middleware.limit(rateLimitClassAI, func(w http.ResponseWriter, _ *http.Request, _ application.Actor) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/ai/a2ui", nil)
	rec := httptest.NewRecorder()
	handler(rec, req, application.Actor{UserID: 9})

	if !nextCalled {
		t.Fatal("expected fail-open: next handler should run on backend error")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on fail-open, got %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "" {
		t.Fatalf("did not expect Retry-After on fail-open, got %q", got)
	}
}

// TestRateLimitIdentifier 验证身份识别：已认证用户优先用户 ID，匿名回退
// RemoteAddr IP（IPv6 冒号替换、无端口兜底）。
func TestRateLimitIdentifier(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	if got := rateLimitIdentifier(application.Actor{UserID: 42}, req); got != "user:42" {
		t.Fatalf("expected user:42, got %q", got)
	}
	if got := rateLimitIdentifier(application.Actor{}, req); got != "ip:203.0.113.7" {
		t.Fatalf("expected ip:203.0.113.7, got %q", got)
	}

	req6 := httptest.NewRequest(http.MethodPost, "/", nil)
	req6.RemoteAddr = "[2001:db8::1]:8080"
	if got := rateLimitIdentifier(application.Actor{}, req6); got != "ip:2001_db8__1" {
		t.Fatalf("expected ip:2001_db8__1, got %q", got)
	}

	reqNoPort := httptest.NewRequest(http.MethodPost, "/", nil)
	reqNoPort.RemoteAddr = "198.51.100.9"
	if got := rateLimitIdentifier(application.Actor{}, reqNoPort); got != "ip:198.51.100.9" {
		t.Fatalf("expected ip:198.51.100.9, got %q", got)
	}
}

// TestServerLimitWithoutMiddleware 验证未配置限流中间件时 limit 原样放行，
// 保证无 Redis 场景下 NewServer 行为不变。
func TestServerLimitWithoutMiddleware(t *testing.T) {
	s := &Server{}
	nextCalled := false
	next := func(w http.ResponseWriter, _ *http.Request, _ application.Actor) {
		nextCalled = true
	}
	wrapped := s.limit(rateLimitClassWrite, next)

	rec := httptest.NewRecorder()
	wrapped(rec, httptest.NewRequest(http.MethodPost, "/api/orders", nil), application.Actor{UserID: 1})

	if !nextCalled {
		t.Fatal("expected next handler to run when rate limit middleware is not configured")
	}
}
