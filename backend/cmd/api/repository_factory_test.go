package main

import (
	"context"
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

// TestWrapRepositoryWithRedisSessionMissingAddr 验证缺少 REDIS_ADDR 时报错且返回空资源。
func TestWrapRepositoryWithRedisSessionMissingAddr(t *testing.T) {
	base := newRepositoryFactoryPostgresRepo(t)
	t.Setenv("REDIS_ADDR", "")

	_, limiter, cleanup, err := wrapRepositoryWithRedisSession(base, log.Default())
	if limiter != nil {
		t.Fatalf("expected nil limiter, got %T", limiter)
	}
	if cleanup == nil {
		t.Fatal("expected non-nil cleanup")
	}
	cleanup()
	if err == nil {
		t.Fatal("expected error when REDIS_ADDR is missing")
	}
}

// TestWrapRepositoryWithRedisSessionEnabled 验证 Redis 会话/目录缓存包装链与 TTL 生效。
func TestWrapRepositoryWithRedisSessionEnabled(t *testing.T) {
	base := newRepositoryFactoryPostgresRepo(t)
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR is not set")
	}
	t.Setenv("REDIS_ADDR", addr)
	t.Setenv("REDIS_SESSION_TTL", "45m")
	t.Setenv("REDIS_CATALOG_TTL", "2m")

	repo, limiter, cleanup, err := wrapRepositoryWithRedisSession(base, log.Default())
	if err != nil {
		t.Fatalf("wrap repository: %v", err)
	}
	if limiter == nil {
		t.Fatal("expected non-nil limiter")
	}
	t.Cleanup(cleanup)

	sessionRepo, ok := repo.(*redisrepo.SessionRepository)
	if !ok {
		t.Fatalf("expected redis session repository, got %T", repo)
	}

	catalogRepo, ok := sessionRepo.Repository.(*redisrepo.CatalogCacheRepository)
	if !ok {
		t.Fatalf("expected catalog cache repository under session repository, got %T", sessionRepo.Repository)
	}
	if catalogRepo.Repository != base {
		t.Fatal("expected catalog cache repository to wrap base repository")
	}

	user := createRepositoryFactoryUser(t, base)
	sessionRepo.SaveSession("wrapped-token", "wrapped-refresh", user.ID)

	saved, _, ok := sessionRepo.GetUserByToken("wrapped-token")
	if !ok || saved.ID != user.ID {
		t.Fatalf("expected redis-backed token lookup, got %+v ok=%v", saved, ok)
	}
	client, err := redisrepo.NewClient(addr)
	if err != nil {
		t.Fatalf("new redis client: %v", err)
	}
	t.Cleanup(func() {
		_ = client.FlushDB(context.Background()).Err()
		_ = client.Close()
	})
	ttl := client.TTL(context.Background(), "redcart:session:wrapped-token").Val()
	if ttl < 45*time.Minute || ttl > 56*time.Minute {
		t.Fatalf("expected ttl around 45m with jitter, got %s", ttl)
	}
}

// newRepositoryFactoryPostgresRepo 创建真实 Postgres 仓储；未开启集成测试
// 或缺少 DSN 时跳过，并在清理阶段关闭连接。
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

// createRepositoryFactoryUser 在给定仓储中创建一个测试用户并返回其领域模型。
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

var _ application.Repository = (*redisrepo.SessionRepository)(nil)
