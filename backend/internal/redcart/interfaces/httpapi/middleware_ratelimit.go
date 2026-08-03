// 限流中间件：按类别对 authedHandler 链执行 Redis 令牌桶限流，
// 并暴露 Prometheus 指标（放行/拒绝/后端错误，按 class 标签）。
package httpapi

import (
	"context"
	"log"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	redisrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/redis"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// 限流指标按 class 标签统计放行、拒绝与后端（Redis）错误。使用 promauto
// 在包级注册（与 middleware_prometheus.go 一致），同一进程重复构造服务
// 也不会触发重复注册 panic。
var (
	rateLimitAllowed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "redcart_rate_limit_allowed_total",
		Help: "Total number of requests admitted by the rate limiter.",
	}, []string{"class"})
	rateLimitRejected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "redcart_rate_limit_rejected_total",
		Help: "Total number of requests rejected by the rate limiter.",
	}, []string{"class"})
	rateLimitBackendErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "redcart_rate_limit_backend_errors_total",
		Help: "Total number of rate limiter backend errors.",
	}, []string{"class"})
)

// 限流类别：write 为订单创建、支付等关键写接口；ai 为 AI 生成接口。
const (
	rateLimitClassWrite = "write"
	rateLimitClassAI    = "ai"
)

// rateLimiter 是令牌桶限流核心的最小契约，便于单元测试用 stub 注入。
type rateLimiter interface {
	Allow(ctx context.Context, key string, policy redisrepo.Policy, now time.Time) (redisrepo.Decision, error)
}

// RateLimitMiddleware 按类别对 authedHandler 链执行令牌桶限流：
// 身份优先取已认证用户 ID，否则回退客户端 IP；Redis 故障时 fail-open 并记日志。
type RateLimitMiddleware struct {
	limiter     rateLimiter
	logger      *log.Logger
	writePolicy redisrepo.Policy
	aiPolicy    redisrepo.Policy
}

// NewRateLimitMiddleware 构造限流中间件：limiter 为限流核心，logger 记录
// 后端故障，writePolicy/aiPolicy 分别是写接口与 AI 接口的令牌桶策略。
func NewRateLimitMiddleware(limiter rateLimiter, logger *log.Logger, writePolicy, aiPolicy redisrepo.Policy) *RateLimitMiddleware {
	if logger == nil {
		logger = log.Default()
	}
	return &RateLimitMiddleware{limiter: limiter, logger: logger, writePolicy: writePolicy, aiPolicy: aiPolicy}
}

// policyFor 返回类别对应的策略；未知类别返回 false，调用方按不限流处理。
func (m *RateLimitMiddleware) policyFor(class string) (redisrepo.Policy, bool) {
	switch class {
	case rateLimitClassWrite:
		return m.writePolicy, true
	case rateLimitClassAI:
		return m.aiPolicy, true
	default:
		return redisrepo.Policy{}, false
	}
}

// limit 返回一个 authedHandler 包装：先按类别评估令牌桶，放行则继续执行
// next；拒绝则写 429 与 Retry-After/RateLimit-Remaining 头；Redis 故障时
// fail-open 放行并记录后端错误指标与日志。
func (m *RateLimitMiddleware) limit(class string, next authedHandler) authedHandler {
	policy, ok := m.policyFor(class)
	return func(w http.ResponseWriter, r *http.Request, actor application.Actor) {
		if !ok {
			next(w, r, actor)
			return
		}
		key := redisrepo.RateLimitKey(class, rateLimitIdentifier(actor, r))
		decision, err := m.limiter.Allow(r.Context(), key, policy, time.Now())
		if err != nil {
			rateLimitBackendErrors.WithLabelValues(class).Inc()
			m.logger.Printf("rate limit backend failed: class=%s key=%s error=%v", class, key, err)
			next(w, r, actor)
			return
		}
		w.Header().Set("RateLimit-Remaining", strconv.Itoa(decision.Remaining))
		if decision.Allowed {
			rateLimitAllowed.WithLabelValues(class).Inc()
			next(w, r, actor)
			return
		}
		rateLimitRejected.WithLabelValues(class).Inc()
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(decision.RetryAfter)))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error": map[string]any{
				"kind":    "rate_limit_exceeded",
				"message": "rate limit exceeded",
			},
		})
	}
}

// rateLimitIdentifier 优先使用已认证用户 ID，否则从 RemoteAddr 提取客户端
// IP（IPv6 冒号替换为下划线，缺失时以 unknown 兜底）。
func rateLimitIdentifier(actor application.Actor, r *http.Request) string {
	if actor.UserID > 0 {
		return "user:" + strconv.FormatInt(actor.UserID, 10)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.ReplaceAll(host, ":", "_")
	if host == "" {
		host = "unknown"
	}
	return "ip:" + host
}

// retryAfterSeconds 把重试时间向上取整为秒且至少为 1 秒，用于 Retry-After 响应头。
func retryAfterSeconds(duration time.Duration) int {
	return max(1, int(math.Ceil(duration.Seconds())))
}
