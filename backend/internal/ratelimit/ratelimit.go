// Package ratelimit defines the broker-neutral admission contract used by the
// HTTP layer. Redis is one implementation; callers do not depend on its SDK.
package ratelimit

import (
	"context"
	"math"
	"time"
)

type Policy struct {
	RatePerSecond float64
	Burst         int
}

type Decision struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
}

type Limiter interface {
	Allow(ctx context.Context, key string, policy Policy) (Decision, error)
}

func RetryAfterSeconds(duration time.Duration) int {
	return max(1, int(math.Ceil(duration.Seconds())))
}
