package main

import (
	"fmt"
	"log"
	"os"

	"github.com/example/redcart-copilot/backend/internal/event"
	"github.com/example/redcart-copilot/backend/internal/ratelimit"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	authrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/auth"
	postgresrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres"
	redisrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/redis"
)

type runtimeDependencies struct {
	repository   application.Repository
	tokenManager application.TokenManager
	rateLimiter  ratelimit.Limiter
	outbox       event.OutboxRelayStore
	close        func()
}

func initDependencies(logger *log.Logger) (*runtimeDependencies, error) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		return nil, fmt.Errorf("POSTGRES_DSN is required")
	}

	repo, err := postgresrepo.NewRepository(dsn)
	if err != nil {
		return nil, fmt.Errorf("initialize postgres repository: %w", err)
	}
	if logger != nil {
		logger.Printf("postgres repository connected")
	}

	cleanup := func() {
		if err := repo.Close(); err != nil && logger != nil {
			logger.Printf("close postgres repository: %v", err)
		}
	}

	wrapped, tokenManager, limiter, extraCleanup, err := wrapRepositoryWithRedisSession(repo, logger)
	if err != nil {
		cleanup()
		return nil, err
	}
	return &runtimeDependencies{
		repository:   wrapped,
		tokenManager: tokenManager,
		rateLimiter:  limiter,
		outbox:       repo.Outbox,
		close: func() {
			extraCleanup()
			cleanup()
		},
	}, nil
}

func wrapRepositoryWithRedisSession(base application.Repository, logger *log.Logger) (application.Repository, application.TokenManager, ratelimit.Limiter, func(), error) {
	addr := envOrDefault("REDIS_ADDR", "")
	if addr == "" {
		return nil, nil, nil, func() {}, fmt.Errorf("REDIS_ADDR is required")
	}

	client, err := redisrepo.NewClient(addr)
	if err != nil {
		return nil, nil, nil, func() {}, fmt.Errorf("initialize redis client: %w", err)
	}
	jwtConfig, err := authrepo.ConfigFromEnv()
	if err != nil {
		_ = client.Close()
		return nil, nil, nil, func() {}, err
	}
	tokenManager, err := authrepo.NewJWTManager(client, jwtConfig)
	if err != nil {
		_ = client.Close()
		return nil, nil, nil, func() {}, fmt.Errorf("initialize JWT manager: %w", err)
	}
	catalogTTL, err := redisrepo.CatalogTTLFromEnv(os.Getenv("REDIS_CATALOG_TTL"))
	if err != nil {
		_ = client.Close()
		return nil, nil, nil, func() {}, err
	}
	if logger != nil {
		logger.Printf("redis shared state enabled on %s with jwt_access_ttl=%s jwt_refresh_ttl=%s catalog_ttl=%s", addr, jwtConfig.AccessTTL, jwtConfig.RefreshTTL, catalogTTL)
	}
	withCatalog := redisrepo.NewCatalogCacheRepository(base, client, catalogTTL)
	limiter := redisrepo.NewRateLimiter(client)
	return withCatalog, tokenManager, limiter, func() {
		if err := client.Close(); err != nil && logger != nil {
			logger.Printf("close redis client: %v", err)
		}
	}, nil
}
