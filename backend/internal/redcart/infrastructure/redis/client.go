// Package redis 提供 Redis 基础设施适配层：
//   - 会话（access/refresh token）与商品目录缓存的 repository 装饰器实现；
//   - Redis 客户端构造（NewClient）与各类缓存 TTL 的环境变量解析。
package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// 各类缓存的默认过期时间，作为未设置对应环境变量时的回退值。
const (
	defaultDialTimeout     = 3 * time.Second
	defaultReadTimeout     = 1 * time.Second
	defaultWriteTimeout    = 1 * time.Second
	defaultConnectTimeout  = 5 * time.Second
	defaultAccessTokenTTL  = 15 * time.Minute
	defaultRefreshTokenTTL = 168 * time.Hour
	defaultCatalogTTL      = 5 * time.Minute
)

// NewClient 创建 Redis 客户端：连接建立前先 Ping 校验可用性，
// 失败时关闭连接并返回错误；addr 为空视为配置缺失。
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

// AccessTokenTTLFromEnv 解析 REDIS_ACCESS_TOKEN_TTL（访问令牌 TTL）。
func AccessTokenTTLFromEnv(raw string) (time.Duration, error) {
	return ttlFromEnv(raw, defaultAccessTokenTTL, "REDIS_ACCESS_TOKEN_TTL")
}

// RefreshTokenTTLFromEnv 解析 REDIS_REFRESH_TOKEN_TTL（刷新令牌 TTL）。
func RefreshTokenTTLFromEnv(raw string) (time.Duration, error) {
	return ttlFromEnv(raw, defaultRefreshTokenTTL, "REDIS_REFRESH_TOKEN_TTL")
}

// CatalogTTLFromEnv 解析 REDIS_CATALOG_TTL（商品目录缓存 TTL）。
func CatalogTTLFromEnv(raw string) (time.Duration, error) {
	return ttlFromEnv(raw, defaultCatalogTTL, "REDIS_CATALOG_TTL")
}

// ttlFromEnv 是各 TTL 解析的公共实现：空串返回 fallback；
// 解析失败或非正数时返回带环境变量名的错误。
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
