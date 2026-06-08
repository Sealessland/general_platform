package main

import (
	"errors"
	"io"
	"log"
	"runtime"
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

	cfg, settings, enabled, err := loadProfilerConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enabled {
		t.Fatal("expected profiler to be disabled without server address")
	}
	if cfg.ApplicationName != "" || cfg.ServerAddress != "" || len(cfg.ProfileTypes) != 0 {
		t.Fatalf("expected empty profiler config when disabled, got %#v", cfg)
	}
	if settings.mutexProfileFraction != 0 || settings.blockProfileRate != 0 {
		t.Fatalf("expected empty runtime settings when disabled, got %#v", settings)
	}
}

func TestLoadProfilerConfigFromEnvUsesDefaults(t *testing.T) {
	t.Setenv("PYROSCOPE_SERVER_ADDRESS", "http://127.0.0.1:4040")
	t.Setenv("PYROSCOPE_APPLICATION_NAME", "")
	t.Setenv("PYROSCOPE_BASIC_AUTH_USER", "user-a")
	t.Setenv("PYROSCOPE_BASIC_AUTH_PASSWORD", "secret")
	t.Setenv("PYROSCOPE_TENANT_ID", "tenant-a")

	cfg, settings, enabled, err := loadProfilerConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Fatal("expected profiler to be enabled")
	}
	if settings.mutexProfileFraction != 0 || settings.blockProfileRate != 0 {
		t.Fatalf("expected default runtime settings, got %#v", settings)
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

func TestLoadProfilerConfigFromEnvAddsMutexAndBlockProfiles(t *testing.T) {
	t.Setenv("PYROSCOPE_SERVER_ADDRESS", "http://127.0.0.1:4040")
	t.Setenv("PYROSCOPE_MUTEX_PROFILE_FRACTION", "5")
	t.Setenv("PYROSCOPE_BLOCK_PROFILE_RATE", "10")

	cfg, settings, enabled, err := loadProfilerConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Fatal("expected profiler to be enabled")
	}
	if settings.mutexProfileFraction != 5 {
		t.Fatalf("expected mutex profile fraction 5, got %d", settings.mutexProfileFraction)
	}
	if settings.blockProfileRate != 10 {
		t.Fatalf("expected block profile rate 10, got %d", settings.blockProfileRate)
	}

	for _, profileType := range []pyroscope.ProfileType{
		pyroscope.ProfileMutexCount,
		pyroscope.ProfileMutexDuration,
		pyroscope.ProfileBlockCount,
		pyroscope.ProfileBlockDuration,
	} {
		if !hasProfileType(cfg.ProfileTypes, profileType) {
			t.Fatalf("expected profile type %s in %v", profileType, cfg.ProfileTypes)
		}
	}
}

func TestLoadProfilerConfigFromEnvRejectsInvalidSamplingConfig(t *testing.T) {
	t.Setenv("PYROSCOPE_SERVER_ADDRESS", "http://127.0.0.1:4040")
	t.Setenv("PYROSCOPE_MUTEX_PROFILE_FRACTION", "not-an-int")

	_, _, _, err := loadProfilerConfigFromEnv()
	if err == nil {
		t.Fatal("expected invalid mutex sampling config error")
	}

	t.Setenv("PYROSCOPE_MUTEX_PROFILE_FRACTION", "")
	t.Setenv("PYROSCOPE_BLOCK_PROFILE_RATE", "-1")

	_, _, _, err = loadProfilerConfigFromEnv()
	if err == nil {
		t.Fatal("expected invalid block sampling config error")
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

func TestStartProfilerFromEnvAppliesAndRestoresRuntimeSettings(t *testing.T) {
	t.Setenv("PYROSCOPE_SERVER_ADDRESS", "http://127.0.0.1:4040")
	t.Setenv("PYROSCOPE_MUTEX_PROFILE_FRACTION", "7")
	t.Setenv("PYROSCOPE_BLOCK_PROFILE_RATE", "11")

	previousMutexFraction := runtime.SetMutexProfileFraction(0)
	t.Cleanup(func() {
		runtime.SetMutexProfileFraction(previousMutexFraction)
		runtime.SetBlockProfileRate(0)
	})

	profiler := &stubProfiler{}
	stop, err := startProfilerFromEnv(func(cfg pyroscope.Config) (runningProfiler, error) {
		return profiler, nil
	}, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := runtime.SetMutexProfileFraction(-1); got != 7 {
		t.Fatalf("expected active mutex profile fraction 7, got %d", got)
	}

	stop()

	if got := runtime.SetMutexProfileFraction(-1); got != 0 {
		t.Fatalf("expected mutex profile fraction restored to 0, got %d", got)
	}
	if profiler.stopCalls != 1 {
		t.Fatalf("expected profiler stop to be called once, got %d", profiler.stopCalls)
	}
}

func TestStartProfilerFromEnvReturnsStartError(t *testing.T) {
	t.Setenv("PYROSCOPE_SERVER_ADDRESS", "http://127.0.0.1:4040")
	t.Setenv("PYROSCOPE_MUTEX_PROFILE_FRACTION", "9")

	previousMutexFraction := runtime.SetMutexProfileFraction(0)
	t.Cleanup(func() {
		runtime.SetMutexProfileFraction(previousMutexFraction)
	})

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
	if got := runtime.SetMutexProfileFraction(-1); got != 0 {
		t.Fatalf("expected mutex profile fraction restored after start error, got %d", got)
	}
}

func hasProfileType(types []pyroscope.ProfileType, want pyroscope.ProfileType) bool {
	for _, profileType := range types {
		if profileType == want {
			return true
		}
	}
	return false
}
