package redis

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
	goredis "github.com/redis/go-redis/v9"
)

func TestSessionRepositoryRoundTrip(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	base := memory.NewRepository()
	repo := NewSessionRepository(base, client, time.Hour)
	service := application.NewService(repo, backendai.MockProvider{})

	session, err := service.Login(t.Context(), application.LoginInput{
		Phone:    "13800000001",
		Password: "consumer-demo",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	server.FastForward(10 * time.Minute)
	user, ok := repo.GetUserByToken(session.Token)
	if !ok {
		t.Fatal("expected redis-backed session lookup")
	}
	if user.ID != session.User.ID || user.Phone != session.User.Phone || user.Role != session.User.Role {
		t.Fatalf("unexpected session user: %+v", user)
	}
}

func TestSessionRepositoryDeleteInvalidatesTokens(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	base := memory.NewRepository()
	repo := NewSessionRepository(base, client, time.Hour)
	service := application.NewService(repo, backendai.MockProvider{})

	session, err := service.Login(t.Context(), application.LoginInput{
		Phone:    "13800000001",
		Password: "consumer-demo",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	repo.DeleteSession(session.Token)
	if _, ok := repo.GetUserByToken(session.Token); ok {
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
