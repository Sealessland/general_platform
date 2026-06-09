package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	defaultDialTimeout    = 3 * time.Second
	defaultReadTimeout    = 1 * time.Second
	defaultWriteTimeout   = 1 * time.Second
	defaultConnectTimeout = 5 * time.Second
	defaultSessionTTL     = 24 * time.Hour
	defaultCatalogTTL     = 5 * time.Minute
)

func NewClient(addr string) (*goredis.Client, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, fmt.Errorf("REDIS_ADDR is required")
	}

	client := goredis.NewClient(&goredis.Options{
		Addr:         addr,
		DialTimeout:  defaultDialTimeout,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), defaultConnectTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}

func SessionTTLFromEnv(raw string) (time.Duration, error) {
	return ttlFromEnv(raw, defaultSessionTTL, "REDIS_SESSION_TTL")
}

func CatalogTTLFromEnv(raw string) (time.Duration, error) {
	return ttlFromEnv(raw, defaultCatalogTTL, "REDIS_CATALOG_TTL")
}

func ttlFromEnv(raw string, fallback time.Duration, envName string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", envName, err)
	}
	if ttl <= 0 {
		return 0, fmt.Errorf("%s must be positive", envName)
	}
	return ttl, nil
}
