package main

import (
	"fmt"
	"log"
	"os"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	postgresrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres"
	redisrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/redis"
)

func initRepository(logger *log.Logger) (application.Repository, func(), error) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		return nil, func() {}, fmt.Errorf("POSTGRES_DSN is required")
	}

	repo, err := postgresrepo.NewRepository(dsn)
	if err != nil {
		return nil, func() {}, fmt.Errorf("initialize postgres repository: %w", err)
	}
	if logger != nil {
		logger.Printf("postgres repository connected")
	}

	cleanup := func() {
		if err := repo.Close(); err != nil && logger != nil {
			logger.Printf("close postgres repository: %v", err)
		}
	}

	wrapped, extraCleanup, err := wrapRepositoryWithRedisSession(repo, logger)
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return wrapped, func() {
		extraCleanup()
		cleanup()
	}, nil
}

func wrapRepositoryWithRedisSession(base application.Repository, logger *log.Logger) (application.Repository, func(), error) {
	addr := envOrDefault("REDIS_ADDR", "")
	if addr == "" {
		return nil, func() {}, fmt.Errorf("REDIS_ADDR is required")
	}

	client, err := redisrepo.NewClient(addr)
	if err != nil {
		return nil, func() {}, fmt.Errorf("initialize redis session store: %w", err)
	}
	accessTTL, err := redisrepo.AccessTokenTTLFromEnv(os.Getenv("REDIS_ACCESS_TOKEN_TTL"))
	if err != nil {
		_ = client.Close()
		return nil, func() {}, err
	}
	refreshTTL, err := redisrepo.RefreshTokenTTLFromEnv(os.Getenv("REDIS_REFRESH_TOKEN_TTL"))
	if err != nil {
		_ = client.Close()
		return nil, func() {}, err
	}
	catalogTTL, err := redisrepo.CatalogTTLFromEnv(os.Getenv("REDIS_CATALOG_TTL"))
	if err != nil {
		_ = client.Close()
		return nil, func() {}, err
	}
	if logger != nil {
		logger.Printf("redis repository wrapped on %s with access_ttl=%s refresh_ttl=%s catalog_ttl=%s", addr, accessTTL, refreshTTL, catalogTTL)
	}
	withCatalog := redisrepo.NewCatalogCacheRepository(base, client, catalogTTL)
	withSession := redisrepo.NewSessionRepository(withCatalog, client, accessTTL, refreshTTL)
	return withSession, func() {
		if err := client.Close(); err != nil && logger != nil {
			logger.Printf("close redis client: %v", err)
		}
	}, nil
}
