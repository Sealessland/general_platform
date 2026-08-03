package main

import "testing"

// TestEnvOrDefault 验证空值与有值环境变量分别回退与直取。
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

// TestInitRepositoryRequiresPostgresDSN 验证缺少 POSTGRES_DSN 时初始化失败且返回空资源。
func TestInitRepositoryRequiresPostgresDSN(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "")
	repo, limiter, cleanup, err := initRepository(nil)
	if err == nil {
		t.Fatal("expected missing POSTGRES_DSN error")
	}
	if repo != nil {
		t.Fatalf("expected nil repository, got %T", repo)
	}
	if limiter != nil {
		t.Fatalf("expected nil limiter, got %T", limiter)
	}
	cleanup()
}
