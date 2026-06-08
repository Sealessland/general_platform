package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	pyroscope "github.com/grafana/pyroscope-go"
)

const defaultPyroscopeApplicationName = "redcart.backend"

type runningProfiler interface {
	Stop() error
}

type profilerStarter func(pyroscope.Config) (runningProfiler, error)

func startProfilerFromEnv(start profilerStarter, logger *log.Logger) (func(), error) {
	cfg, enabled := loadProfilerConfigFromEnv()
	if !enabled {
		return func() {}, nil
	}

	profiler, err := start(cfg)
	if err != nil {
		return nil, fmt.Errorf("start pyroscope profiler: %w", err)
	}

	if logger != nil {
		logger.Printf("pyroscope profiling enabled for %s -> %s", cfg.ApplicationName, cfg.ServerAddress)
	}

	return func() {
		if err := profiler.Stop(); err != nil && logger != nil {
			logger.Printf("stop pyroscope profiler: %v", err)
		}
	}, nil
}

func loadProfilerConfigFromEnv() (pyroscope.Config, bool) {
	serverAddress := strings.TrimSpace(os.Getenv("PYROSCOPE_SERVER_ADDRESS"))
	if serverAddress == "" {
		return pyroscope.Config{}, false
	}

	applicationName := strings.TrimSpace(os.Getenv("PYROSCOPE_APPLICATION_NAME"))
	if applicationName == "" {
		applicationName = defaultPyroscopeApplicationName
	}

	return pyroscope.Config{
		ApplicationName:   applicationName,
		ServerAddress:     serverAddress,
		BasicAuthUser:     strings.TrimSpace(os.Getenv("PYROSCOPE_BASIC_AUTH_USER")),
		BasicAuthPassword: os.Getenv("PYROSCOPE_BASIC_AUTH_PASSWORD"),
		TenantID:          strings.TrimSpace(os.Getenv("PYROSCOPE_TENANT_ID")),
		ProfileTypes:      pyroscope.DefaultProfileTypes,
	}, true
}

func pyroscopeStart(cfg pyroscope.Config) (runningProfiler, error) {
	return pyroscope.Start(cfg)
}
