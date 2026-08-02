package httpapi

import (
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/ratelimit"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type routeLimit struct {
	class    string
	policy   ratelimit.Policy
	failOpen bool
}

var (
	rateLimitAllowed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "redcart_rate_limit_allowed_total",
		Help: "Requests admitted by the Redis rate limiter.",
	}, []string{"class"})
	rateLimitRejected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "redcart_rate_limit_rejected_total",
		Help: "Requests rejected by the Redis rate limiter.",
	}, []string{"class"})
	rateLimitErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "redcart_rate_limit_backend_errors_total",
		Help: "Redis errors encountered while checking rate limits.",
	}, []string{"class"})
)

// rateLimitMiddleware protects only three easy-to-explain route classes:
// authentication writes, costly AI writes, and public catalog reads. Security
// and cost-sensitive writes fail closed; cacheable reads fail open.
func rateLimitMiddleware(limiter ratelimit.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, ok := limitForRequest(c.Request)
		if !ok {
			c.Next()
			return
		}

		key := "redcart:rate:" + limit.class + ":" + requestIdentity(c.Request)
		decision, err := limiter.Allow(c.Request.Context(), key, limit.policy)
		if err != nil {
			rateLimitErrors.WithLabelValues(limit.class).Inc()
			if limit.failOpen {
				c.Next()
				return
			}
			writeJSON(c.Writer, http.StatusServiceUnavailable, map[string]any{
				"error": map[string]any{"kind": "rate_limit_unavailable", "message": "request cannot be admitted"},
			})
			c.Abort()
			return
		}

		c.Header("RateLimit-Remaining", strconv.Itoa(decision.Remaining))
		if !decision.Allowed {
			rateLimitRejected.WithLabelValues(limit.class).Inc()
			c.Header("Retry-After", strconv.Itoa(ratelimit.RetryAfterSeconds(decision.RetryAfter)))
			writeJSON(c.Writer, http.StatusTooManyRequests, map[string]any{
				"error": map[string]any{"kind": "rate_limit_exceeded", "message": "rate limit exceeded"},
			})
			c.Abort()
			return
		}
		rateLimitAllowed.WithLabelValues(limit.class).Inc()
		c.Next()
	}
}

func limitForRequest(r *http.Request) (routeLimit, bool) {
	if r.Method == http.MethodPost {
		switch r.URL.Path {
		case "/api/auth/register", "/api/auth/login", "/api/auth/refresh":
			return routeLimit{class: "auth_write", policy: ratelimit.Policy{RatePerSecond: 5, Burst: 10}}, true
		}
		if strings.HasPrefix(r.URL.Path, "/api/ai/") {
			return routeLimit{class: "ai_write", policy: ratelimit.Policy{RatePerSecond: 2, Burst: 4}}, true
		}
	}
	if r.Method == http.MethodGet && (strings.HasPrefix(r.URL.Path, "/api/products") || strings.HasPrefix(r.URL.Path, "/api/notes")) {
		return routeLimit{class: "catalog_read", policy: ratelimit.Policy{RatePerSecond: 50, Burst: 100}, failOpen: true}, true
	}
	return routeLimit{}, false
}

func requestIdentity(r *http.Request) string {
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(authorization, "Bearer ") {
		token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		if token != "" {
			digest := sha256.Sum256([]byte(token))
			return fmt.Sprintf("token:%x", digest[:12])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	if host == "" {
		host = "unknown"
	}
	return "ip:" + strings.ReplaceAll(host, ":", "_")
}
