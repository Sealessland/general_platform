package main

import (
	"errors"
	"io"
	"log"
	"testing"

	pyroscope "github.com/grafana/pyroscope-go"
)

type stubProfiler struct {
	stopCalls int
	stopErr   error
}

func (p *stubProfiler) Stop() error {
	p.stopCalls++
	return p.stopErr
}

func TestLoadProfilerConfigFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv("PYROSCOPE_SERVER_ADDRESS", "")
	t.Setenv("PYROSCOPE_APPLICATION_NAME", "")

	cfg, enabled := loadProfilerConfigFromEnv()
	if enabled {
		t.Fatal("expected profiler to be disabled without server address")
	}
	if cfg.ApplicationName != "" || cfg.ServerAddress != "" || len(cfg.ProfileTypes) != 0 {
		t.Fatalf("expected empty profiler config when disabled, got %#v", cfg)
	}
}

func TestLoadProfilerConfigFromEnvUsesDefaults(t *testing.T) {
	t.Setenv("PYROSCOPE_SERVER_ADDRESS", "http://127.0.0.1:4040")
	t.Setenv("PYROSCOPE_APPLICATION_NAME", "")
	t.Setenv("PYROSCOPE_BASIC_AUTH_USER", "user-a")
	t.Setenv("PYROSCOPE_BASIC_AUTH_PASSWORD", "secret")
	t.Setenv("PYROSCOPE_TENANT_ID", "tenant-a")

	cfg, enabled := loadProfilerConfigFromEnv()
	if !enabled {
		t.Fatal("expected profiler to be enabled")
	}
	if cfg.ApplicationName != defaultPyroscopeApplicationName {
		t.Fatalf("expected default application name, got %q", cfg.ApplicationName)
	}
	if cfg.ServerAddress != "http://127.0.0.1:4040" {
		t.Fatalf("unexpected server address: %q", cfg.ServerAddress)
	}
	if cfg.BasicAuthUser != "user-a" || cfg.BasicAuthPassword != "secret" {
		t.Fatalf("unexpected basic auth config: %#v", cfg)
	}
	if cfg.TenantID != "tenant-a" {
		t.Fatalf("unexpected tenant id: %q", cfg.TenantID)
	}
	if len(cfg.ProfileTypes) != len(pyroscope.DefaultProfileTypes) {
		t.Fatalf("expected default profile types, got %v", cfg.ProfileTypes)
	}
}

func TestStartProfilerFromEnvNoopWhenDisabled(t *testing.T) {
	t.Setenv("PYROSCOPE_SERVER_ADDRESS", "")

	called := false
	stop, err := startProfilerFromEnv(func(cfg pyroscope.Config) (runningProfiler, error) {
		called = true
		return &stubProfiler{}, nil
	}, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("expected profiler starter not to be called")
	}

	stop()
}

func TestStartProfilerFromEnvStartsAndStopsProfiler(t *testing.T) {
	t.Setenv("PYROSCOPE_SERVER_ADDRESS", "http://127.0.0.1:4040")
	t.Setenv("PYROSCOPE_APPLICATION_NAME", "redcart.backend.dev")

	var captured pyroscope.Config
	profiler := &stubProfiler{}

	stop, err := startProfilerFromEnv(func(cfg pyroscope.Config) (runningProfiler, error) {
		captured = cfg
		return profiler, nil
	}, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.ApplicationName != "redcart.backend.dev" {
		t.Fatalf("unexpected application name: %q", captured.ApplicationName)
	}

	stop()
	if profiler.stopCalls != 1 {
		t.Fatalf("expected profiler stop to be called once, got %d", profiler.stopCalls)
	}
}

func TestStartProfilerFromEnvReturnsStartError(t *testing.T) {
	t.Setenv("PYROSCOPE_SERVER_ADDRESS", "http://127.0.0.1:4040")

	expected := errors.New("dial failed")
	_, err := startProfilerFromEnv(func(cfg pyroscope.Config) (runningProfiler, error) {
		return nil, expected
	}, log.New(io.Discard, "", 0))
	if err == nil {
		t.Fatal("expected start error")
	}
	if !errors.Is(err, expected) {
		t.Fatalf("expected wrapped error %v, got %v", expected, err)
	}
}
