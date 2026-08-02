package main

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	postgresrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres"
	redisrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/redis"
	"golang.org/x/crypto/bcrypt"
)

func TestWrapRepositoryWithRedisSessionMissingAddr(t *testing.T) {
	base := newRepositoryFactoryPostgresRepo(t)
	t.Setenv("REDIS_ADDR", "")

	_, _, _, cleanup, err := wrapRepositoryWithRedisSession(base, log.Default())
	if cleanup == nil {
		t.Fatal("expected non-nil cleanup")
	}
	cleanup()
	if err == nil {
		t.Fatal("expected error when REDIS_ADDR is missing")
	}
}

func TestWrapRepositoryWithRedisSessionEnabled(t *testing.T) {
	base := newRepositoryFactoryPostgresRepo(t)
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR is not set")
	}
	t.Setenv("REDIS_ADDR", addr)
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("JWT_ACCESS_TTL", "45m")
	t.Setenv("REDIS_CATALOG_TTL", "2m")

	repo, tokens, limiter, cleanup, err := wrapRepositoryWithRedisSession(base, log.Default())
	if err != nil {
		t.Fatalf("wrap repository: %v", err)
	}
	t.Cleanup(cleanup)
	if limiter == nil {
		t.Fatal("expected Redis rate limiter")
	}
	if tokens == nil {
		t.Fatal("expected JWT manager")
	}

	catalogRepo, ok := repo.(*redisrepo.CatalogCacheRepository)
	if !ok {
		t.Fatalf("expected catalog cache repository, got %T", repo)
	}
	if catalogRepo.Repository != base {
		t.Fatal("expected catalog cache repository to wrap base repository")
	}

	user := createRepositoryFactoryUser(t, base)
	pair, err := tokens.Issue(t.Context(), application.TokenPrincipal{UserID: user.ID, Role: user.Role, Nickname: user.Nickname})
	if err != nil {
		t.Fatalf("issue JWT pair: %v", err)
	}
	if _, err := tokens.Authenticate(t.Context(), pair.AccessToken); err != nil {
		t.Fatalf("authenticate issued JWT: %v", err)
	}
}

func newRepositoryFactoryPostgresRepo(t *testing.T) *postgresrepo.Repository {
	t.Helper()
	if os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("RUN_POSTGRES_INTEGRATION is not set")
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN is not set")
	}
	repo, err := postgresrepo.NewRepository(dsn)
	if err != nil {
		t.Fatalf("new postgres repository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func createRepositoryFactoryUser(t *testing.T, repo *postgresrepo.Repository) domain.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("factory-pass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := repo.CreateUser(domain.User{
		Nickname:     "Repository Factory User",
		Phone:        fmt.Sprintf("136%08d", time.Now().UnixNano()%100000000),
		PasswordHash: string(hash),
		Role:         domain.RoleConsumer,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

var _ application.Repository = (*redisrepo.CatalogCacheRepository)(nil)
