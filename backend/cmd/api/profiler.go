package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"

	pyroscope "github.com/grafana/pyroscope-go"
)

const defaultPyroscopeApplicationName = "redcart.backend"

type runningProfiler interface {
	Stop() error
}

type profilerStarter func(pyroscope.Config) (runningProfiler, error)

type profilerRuntimeSettings struct {
	mutexProfileFraction int
	blockProfileRate     int
}

func startProfilerFromEnv(start profilerStarter, logger *log.Logger) (func(), error) {
	cfg, runtimeSettings, enabled, err := loadProfilerConfigFromEnv()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return func() {}, nil
	}

	restoreRuntimeSettings := applyProfilerRuntimeSettings(runtimeSettings)
	profiler, err := start(cfg)
	if err != nil {
		restoreRuntimeSettings()
		return nil, fmt.Errorf("start pyroscope profiler: %w", err)
	}

	if logger != nil {
		logger.Printf("pyroscope profiling enabled for %s -> %s", cfg.ApplicationName, cfg.ServerAddress)
	}

	return func() {
		defer restoreRuntimeSettings()
		if err := profiler.Stop(); err != nil && logger != nil {
			logger.Printf("stop pyroscope profiler: %v", err)
		}
	}, nil
}

func loadProfilerConfigFromEnv() (pyroscope.Config, profilerRuntimeSettings, bool, error) {
	serverAddress := strings.TrimSpace(os.Getenv("PYROSCOPE_SERVER_ADDRESS"))
	if serverAddress == "" {
		return pyroscope.Config{}, profilerRuntimeSettings{}, false, nil
	}

	applicationName := strings.TrimSpace(os.Getenv("PYROSCOPE_APPLICATION_NAME"))
	if applicationName == "" {
		applicationName = defaultPyroscopeApplicationName
	}
	runtimeSettings, err := loadProfilerRuntimeSettingsFromEnv()
	if err != nil {
		return pyroscope.Config{}, profilerRuntimeSettings{}, false, err
	}

	profileTypes := append([]pyroscope.ProfileType{}, pyroscope.DefaultProfileTypes...)
	if runtimeSettings.mutexProfileFraction > 0 {
		profileTypes = append(profileTypes, pyroscope.ProfileMutexCount, pyroscope.ProfileMutexDuration)
	}
	if runtimeSettings.blockProfileRate > 0 {
		profileTypes = append(profileTypes, pyroscope.ProfileBlockCount, pyroscope.ProfileBlockDuration)
	}

	return pyroscope.Config{
		ApplicationName:   applicationName,
		ServerAddress:     serverAddress,
		BasicAuthUser:     strings.TrimSpace(os.Getenv("PYROSCOPE_BASIC_AUTH_USER")),
		BasicAuthPassword: os.Getenv("PYROSCOPE_BASIC_AUTH_PASSWORD"),
		TenantID:          strings.TrimSpace(os.Getenv("PYROSCOPE_TENANT_ID")),
		ProfileTypes:      profileTypes,
	}, runtimeSettings, true, nil
}

func loadProfilerRuntimeSettingsFromEnv() (profilerRuntimeSettings, error) {
	mutexProfileFraction, err := parsePositiveProfilerInt("PYROSCOPE_MUTEX_PROFILE_FRACTION")
	if err != nil {
		return profilerRuntimeSettings{}, err
	}
	blockProfileRate, err := parsePositiveProfilerInt("PYROSCOPE_BLOCK_PROFILE_RATE")
	if err != nil {
		return profilerRuntimeSettings{}, err
	}
	return profilerRuntimeSettings{
		mutexProfileFraction: mutexProfileFraction,
		blockProfileRate:     blockProfileRate,
	}, nil
}

func parsePositiveProfilerInt(key string) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func applyProfilerRuntimeSettings(settings profilerRuntimeSettings) func() {
	previousMutexFraction := 0
	if settings.mutexProfileFraction > 0 {
		previousMutexFraction = runtime.SetMutexProfileFraction(settings.mutexProfileFraction)
	}
	if settings.blockProfileRate > 0 {
		runtime.SetBlockProfileRate(settings.blockProfileRate)
	}

	return func() {
		if settings.mutexProfileFraction > 0 {
			runtime.SetMutexProfileFraction(previousMutexFraction)
		}
		if settings.blockProfileRate > 0 {
			runtime.SetBlockProfileRate(0)
		}
	}
}

func pyroscopeStart(cfg pyroscope.Config) (runningProfiler, error) {
	return pyroscope.Start(cfg)
}
