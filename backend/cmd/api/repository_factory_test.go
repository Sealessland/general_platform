package main

import (
	"log"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
	redisrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/redis"
)

func TestWrapRepositoryWithRedisSessionDisabledWithoutAddr(t *testing.T) {
	t.Setenv("REDIS_ADDR", "")
	base := memory.NewRepository()
	repo, cleanup, err := wrapRepositoryWithRedisSession(base, log.Default())
	if err != nil {
		t.Fatalf("wrap repository: %v", err)
	}
	t.Cleanup(cleanup)
	if repo != base {
		t.Fatalf("expected base repository passthrough, got %T", repo)
	}
}

func TestWrapRepositoryWithRedisSessionEnabled(t *testing.T) {
	server := miniredis.RunT(t)
	t.Setenv("REDIS_ADDR", server.Addr())
	t.Setenv("REDIS_SESSION_TTL", "45m")
	t.Setenv("REDIS_CATALOG_TTL", "2m")

	base := memory.NewRepository()
	repo, cleanup, err := wrapRepositoryWithRedisSession(base, log.Default())
	if err != nil {
		t.Fatalf("wrap repository: %v", err)
	}
	t.Cleanup(cleanup)

	sessionRepo, ok := repo.(*redisrepo.SessionRepository)
	if !ok {
		t.Fatalf("expected redis session repository, got %T", repo)
	}

	user, ok := base.FindUserByPhone("13800000001")
	if !ok {
		t.Fatal("expected seeded user")
	}
	sessionRepo.SaveSession("wrapped-token", user.ID)

	saved, ok := sessionRepo.GetUserByToken("wrapped-token")
	if !ok || saved.ID != user.ID {
		t.Fatalf("expected redis-backed token lookup, got %+v ok=%v", saved, ok)
	}
	if ttl := server.TTL("redcart:session:wrapped-token"); ttl != 45*time.Minute {
		t.Fatalf("expected ttl 45m, got %s", ttl)
	}
}

var _ application.Repository = (*redisrepo.SessionRepository)(nil)
