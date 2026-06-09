package main

import "testing"

func TestEnvOrDefault(t *testing.T) {
	t.Setenv("REDCART_TEST_ENV", "")
	if got := envOrDefault("REDCART_TEST_ENV", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}

	t.Setenv("REDCART_TEST_ENV", "configured")
	if got := envOrDefault("REDCART_TEST_ENV", "fallback"); got != "configured" {
		t.Fatalf("expected configured value, got %q", got)
	}
}

func TestInitRepositoryRequiresPostgresDSN(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "")
	repo, cleanup, err := initRepository(nil)
	if err == nil {
		t.Fatal("expected missing POSTGRES_DSN error")
	}
	if repo != nil {
		t.Fatalf("expected nil repository, got %T", repo)
	}
	cleanup()
}
