package auth

import (
	"fmt"
	"os"
	"time"
)

const (
	defaultIssuer     = "redcart-copilot"
	defaultAudience   = "redcart-api"
	defaultAccessTTL  = 15 * time.Minute
	defaultRefreshTTL = 7 * 24 * time.Hour
)

func ConfigFromEnv() (Config, error) {
	accessTTL, err := durationFromEnv("JWT_ACCESS_TTL", defaultAccessTTL)
	if err != nil {
		return Config{}, err
	}
	refreshTTL, err := durationFromEnv("JWT_REFRESH_TTL", defaultRefreshTTL)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Secret:     os.Getenv("JWT_SECRET"),
		Issuer:     valueOrDefault(os.Getenv("JWT_ISSUER"), defaultIssuer),
		Audience:   valueOrDefault(os.Getenv("JWT_AUDIENCE"), defaultAudience),
		AccessTTL:  accessTTL,
		RefreshTTL: refreshTTL,
	}, nil
}

func durationFromEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", name)
	}
	return value, nil
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
