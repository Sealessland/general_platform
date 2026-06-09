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
		return base, func() {}, nil
	}

	client, err := redisrepo.NewClient(addr)
	if err != nil {
		return nil, func() {}, fmt.Errorf("initialize redis session store: %w", err)
	}
	ttl, err := redisrepo.SessionTTLFromEnv(os.Getenv("REDIS_SESSION_TTL"))
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
		logger.Printf("redis session repository enabled on %s with session_ttl=%s catalog_ttl=%s", addr, ttl, catalogTTL)
	}
	withCatalog := redisrepo.NewCatalogCacheRepository(base, client, catalogTTL)
	withSession := redisrepo.NewSessionRepository(withCatalog, client, ttl)
	return withSession, func() {
		if err := client.Close(); err != nil && logger != nil {
			logger.Printf("close redis client: %v", err)
		}
	}, nil
}
