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

func TestSplitCSVRemovesEmptyKafkaBrokers(t *testing.T) {
	got := splitCSV(" kafka-1:9092, ,kafka-2:9092 ")
	if len(got) != 2 || got[0] != "kafka-1:9092" || got[1] != "kafka-2:9092" {
		t.Fatalf("split brokers = %#v", got)
	}
}

// Keep the historical test entrypoint because repository initialization is now
// one explicit part of initDependencies rather than a separate hidden factory.
func TestInitRepositoryRequiresPostgresDSN(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "")
	dependencies, err := initDependencies(nil)
	if err == nil {
		t.Fatal("expected missing POSTGRES_DSN error")
	}
	if dependencies != nil {
		t.Fatalf("expected nil dependencies, got %T", dependencies)
	}
}
