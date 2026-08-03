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

// defaultPyroscopeApplicationName 是未显式配置 PYROSCOPE_APPLICATION_NAME
// 时使用的默认应用名。
const defaultPyroscopeApplicationName = "redcart.backend"

// runningProfiler 抽象一个已启动的 profiler，便于测试注入替身。
type runningProfiler interface {
	Stop() error
}

// profilerStarter 抽象 profiler 启动函数，便于测试替换真实的 pyroscope 启动。
type profilerStarter func(pyroscope.Config) (runningProfiler, error)

// profilerRuntimeSettings 记录需要临时调整的 Go 运行时采样配置；
// 停止 profiler 时需要恢复原值。
type profilerRuntimeSettings struct {
	mutexProfileFraction int
	blockProfileRate     int
}

// startProfilerFromEnv 根据环境变量决定是否启动 pyroscope 采样：
// 未配置 PYROSCOPE_SERVER_ADDRESS 时返回空操作；启动失败会先恢复运行时
// 采样配置再返回错误。返回的闭包负责停止 profiler 并恢复采样配置。
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

// loadProfilerConfigFromEnv 从环境变量组装 pyroscope.Config；
// 返回的 enabled 为 false 表示未配置服务地址（profiler 不启用）。
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

// loadProfilerRuntimeSettingsFromEnv 读取互斥锁/阻塞采样率配置，
// 非法值（非正整数）返回错误。
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

// parsePositiveProfilerInt 解析正整数环境变量；空串视为 0（表示不启用该采样）。
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

// applyProfilerRuntimeSettings 应用运行时采样配置，返回恢复函数；
// 恢复时互斥锁采样恢复原值，阻塞采样率归零（关闭）。
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

// pyroscopeStart 是 profilerStarter 的真实实现，直接调用 pyroscope.Start。
func pyroscopeStart(cfg pyroscope.Config) (runningProfiler, error) {
	return pyroscope.Start(cfg)
}
