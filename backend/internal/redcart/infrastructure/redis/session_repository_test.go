package redis

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	postgresrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type redisPostgresFixture struct {
	repo   *postgresrepo.Repository
	client *goredis.Client
}

func newRedisPostgresFixture(t *testing.T) redisPostgresFixture {
	t.Helper()
	if os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("RUN_POSTGRES_INTEGRATION is not set")
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN is not set")
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR is not set")
	}

	repo, err := postgresrepo.NewRepository(dsn)
	if err != nil {
		t.Fatalf("new postgres repository: %v", err)
	}
	client, err := NewClient(addr)
	if err != nil {
		_ = repo.Close()
		t.Fatalf("new redis client: %v", err)
	}
	t.Cleanup(func() {
		_ = client.FlushDB(context.Background()).Err()
		_ = client.Close()
		_ = repo.Close()
	})
	return redisPostgresFixture{repo: repo, client: client}
}

func TestSessionRepositoryRoundTrip(t *testing.T) {
	fixture := newRedisPostgresFixture(t)
	repo := NewSessionRepository(fixture.repo, fixture.client, time.Hour, 24*time.Hour)
	service := application.NewService(repo, backendai.MockProvider{})

	phone := uniqueRedisTestPhone()
	password := "consumer-pass"
	if _, err := createRedisTestUser(fixture.repo, phone, password, domain.RoleConsumer); err != nil {
		t.Fatalf("create user: %v", err)
	}

	session, err := service.Login(t.Context(), application.LoginInput{
		Phone:    phone,
		Password: password,
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	user, tokenType, ok := repo.GetUserByToken(session.Token)
	if !ok {
		t.Fatal("expected redis-backed session lookup")
	}
	if user.ID != session.User.ID || user.Phone != session.User.Phone || user.Role != session.User.Role {
		t.Fatalf("unexpected session user: %+v", user)
	}
	if tokenType != application.TokenTypeAccess {
		t.Fatalf("expected access token type, got %s", tokenType)
	}
}

func TestSessionRepositoryDeleteInvalidatesTokens(t *testing.T) {
	fixture := newRedisPostgresFixture(t)
	repo := NewSessionRepository(fixture.repo, fixture.client, time.Hour, 24*time.Hour)
	service := application.NewService(repo, backendai.MockProvider{})

	phone := uniqueRedisTestPhone()
	password := "consumer-pass"
	if _, err := createRedisTestUser(fixture.repo, phone, password, domain.RoleConsumer); err != nil {
		t.Fatalf("create user: %v", err)
	}

	session, err := service.Login(t.Context(), application.LoginInput{
		Phone:    phone,
		Password: password,
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	repo.DeleteSession(session.Token)
	if _, _, ok := repo.GetUserByToken(session.Token); ok {
		t.Fatal("expected access token invalidated after delete")
	}
}

func TestSessionTTLFromEnv(t *testing.T) {
	ttl, err := SessionTTLFromEnv("")
	if err != nil {
		t.Fatalf("default ttl: %v", err)
	}
	if ttl != defaultSessionTTL {
		t.Fatalf("expected default ttl %s, got %s", defaultSessionTTL, ttl)
	}

	ttl, err = SessionTTLFromEnv("90m")
	if err != nil {
		t.Fatalf("custom ttl: %v", err)
	}
	if ttl != 90*time.Minute {
		t.Fatalf("expected 90m ttl, got %s", ttl)
	}

	if _, err := SessionTTLFromEnv("bad"); err == nil {
		t.Fatal("expected parse error")
	}
	if _, err := SessionTTLFromEnv("0s"); err == nil {
		t.Fatal("expected positive ttl error")
	}
}

func TestAccessTokenTTLFromEnv(t *testing.T) {
	ttl, err := AccessTokenTTLFromEnv("")
	if err != nil {
		t.Fatalf("default access ttl: %v", err)
	}
	if ttl != defaultAccessTokenTTL {
		t.Fatalf("expected default access ttl %s, got %s", defaultAccessTokenTTL, ttl)
	}

	ttl, err = AccessTokenTTLFromEnv("30m")
	if err != nil {
		t.Fatalf("custom access ttl: %v", err)
	}
	if ttl != 30*time.Minute {
		t.Fatalf("expected 30m access ttl, got %s", ttl)
	}

	if _, err := AccessTokenTTLFromEnv("bad"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRefreshTokenTTLFromEnv(t *testing.T) {
	ttl, err := RefreshTokenTTLFromEnv("")
	if err != nil {
		t.Fatalf("default refresh ttl: %v", err)
	}
	if ttl != defaultRefreshTokenTTL {
		t.Fatalf("expected default refresh ttl %s, got %s", defaultRefreshTokenTTL, ttl)
	}

	ttl, err = RefreshTokenTTLFromEnv("336h")
	if err != nil {
		t.Fatalf("custom refresh ttl: %v", err)
	}
	if ttl != 336*time.Hour {
		t.Fatalf("expected 336h refresh ttl, got %s", ttl)
	}

	if _, err := RefreshTokenTTLFromEnv("0s"); err == nil {
		t.Fatal("expected positive ttl error")
	}
}

func createRedisTestUser(repo *postgresrepo.Repository, phone, password, role string) (domain.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	return repo.CreateUser(domain.User{
		Nickname:     "Redis Test User",
		Phone:        phone,
		PasswordHash: string(hash),
		Role:         role,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	})
}

func uniqueRedisTestPhone() string {
	return fmt.Sprintf("137%08d", time.Now().UnixNano()%100000000)
}
