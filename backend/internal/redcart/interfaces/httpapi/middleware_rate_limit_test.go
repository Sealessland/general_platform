package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/ratelimit"
	"github.com/gin-gonic/gin"
)

type fakeLimiter struct {
	decision ratelimit.Decision
	err      error
	keys     []string
}

func (f *fakeLimiter) Allow(_ context.Context, key string, _ ratelimit.Policy) (ratelimit.Decision, error) {
	f.keys = append(f.keys, key)
	return f.decision, f.err
}

func TestRateLimitRejectsSensitiveRequestWithRetryAfter(t *testing.T) {
	limiter := &fakeLimiter{decision: ratelimit.Decision{
		Allowed: false, Remaining: 0, RetryAfter: 1500 * time.Millisecond,
	}}
	response := serveRateLimitedRequest(t, limiter, http.MethodPost, "/api/auth/login", "")

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", response.Code)
	}
	if got := response.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want 2", got)
	}
	if len(limiter.keys) != 1 || limiter.keys[0] != "redcart:rate:auth_write:ip:192.0.2.1" {
		t.Fatalf("rate-limit keys = %#v", limiter.keys)
	}
}

func TestRateLimitUsesHashedBearerIdentity(t *testing.T) {
	limiter := &fakeLimiter{decision: ratelimit.Decision{Allowed: true, Remaining: 3}}
	response := serveRateLimitedRequest(t, limiter, http.MethodPost, "/api/ai/business-review", "Bearer sample")

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
	if len(limiter.keys) != 1 {
		t.Fatalf("rate-limit keys = %#v", limiter.keys)
	}
	if limiter.keys[0] == "redcart:rate:ai_write:token:sample" {
		t.Fatal("raw bearer token must not be stored in the Redis key")
	}
	if got := response.Header().Get("RateLimit-Remaining"); got != "3" {
		t.Fatalf("RateLimit-Remaining = %q, want 3", got)
	}
}

func TestRateLimitFailsClosedForAuthenticationWrites(t *testing.T) {
	limiter := &fakeLimiter{err: errors.New("redis unavailable")}
	response := serveRateLimitedRequest(t, limiter, http.MethodPost, "/api/auth/register", "")

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
}

func TestRateLimitFailsOpenForCatalogReads(t *testing.T) {
	limiter := &fakeLimiter{err: errors.New("redis unavailable")}
	response := serveRateLimitedRequest(t, limiter, http.MethodGet, "/api/products", "")

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}

func TestRateLimitSkipsHealthAndMetrics(t *testing.T) {
	limiter := &fakeLimiter{decision: ratelimit.Decision{Allowed: true}}
	for _, path := range []string{"/healthz", "/metrics"} {
		response := serveRateLimitedRequest(t, limiter, http.MethodGet, path, "")
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want 204", path, response.Code)
		}
	}
	if len(limiter.keys) != 0 {
		t.Fatalf("expected no limiter calls, got %#v", limiter.keys)
	}
}

func serveRateLimitedRequest(t *testing.T, limiter ratelimit.Limiter, method, path, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(rateLimitMiddleware(limiter))
	router.Handle(method, path, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(method, path, nil)
	request.RemoteAddr = "192.0.2.1:12345"
	request.Header.Set("Authorization", authorization)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
