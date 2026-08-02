package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/golang-jwt/jwt/v5"
	goredis "github.com/redis/go-redis/v9"
)

const testJWTSecret = "0123456789abcdef0123456789abcdef"

func TestJWTManagerIssuesStandardHS256Claims(t *testing.T) {
	manager, _ := newTestJWTManager(t)
	pair, err := manager.Issue(context.Background(), testPrincipal())
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}

	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(pair.AccessToken, claims, func(token *jwt.Token) (any, error) {
		return []byte(testJWTSecret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid {
		t.Fatalf("parse access JWT: token=%v err=%v", token, err)
	}
	for _, name := range []string{"iss", "sub", "aud", "exp", "iat", "jti", "token_use", "role"} {
		if claims[name] == nil || claims[name] == "" {
			t.Fatalf("expected standard/custom claim %q, got %#v", name, claims)
		}
	}
	if claims["token_use"] != "access" {
		t.Fatalf("token_use = %v, want access", claims["token_use"])
	}
}

func TestJWTManagerRefreshIsSingleUseAndRevokesPreviousAccess(t *testing.T) {
	manager, _ := newTestJWTManager(t)
	ctx := context.Background()
	original, err := manager.Issue(ctx, testPrincipal())
	if err != nil {
		t.Fatalf("issue original pair: %v", err)
	}

	rotated, principal, err := manager.Rotate(ctx, original.RefreshToken)
	if err != nil {
		t.Fatalf("rotate refresh token: %v", err)
	}
	if rotated.AccessToken == original.AccessToken || rotated.RefreshToken == original.RefreshToken {
		t.Fatal("expected both tokens to rotate")
	}
	if principal.UserID != testPrincipal().UserID {
		t.Fatalf("rotated principal = %+v", principal)
	}
	if _, err := manager.Authenticate(ctx, original.AccessToken); err == nil {
		t.Fatal("expected previous access token to be revoked")
	}
	if _, _, err := manager.Rotate(ctx, original.RefreshToken); err == nil {
		t.Fatal("expected refresh token replay to be rejected")
	}
	if _, err := manager.Authenticate(ctx, rotated.AccessToken); err != nil {
		t.Fatalf("authenticate rotated access token: %v", err)
	}
}

func TestJWTManagerSharesRevocationAcrossInstances(t *testing.T) {
	first, client := newTestJWTManager(t)
	second, err := NewJWTManager(client, testJWTConfig())
	if err != nil {
		t.Fatalf("new second manager: %v", err)
	}
	ctx := context.Background()
	pair, err := first.Issue(ctx, testPrincipal())
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}
	if _, err := second.Authenticate(ctx, pair.AccessToken); err != nil {
		t.Fatalf("second instance authenticate: %v", err)
	}
	if err := second.Revoke(ctx, pair.AccessToken); err != nil {
		t.Fatalf("second instance revoke: %v", err)
	}
	if _, err := first.Authenticate(ctx, pair.AccessToken); err == nil {
		t.Fatal("expected first instance to observe shared revocation")
	}
}

func TestJWTManagerRejectsWrongAlgorithmAndTokenUse(t *testing.T) {
	manager, _ := newTestJWTManager(t)
	ctx := context.Background()
	pair, err := manager.Issue(ctx, testPrincipal())
	if err != nil {
		t.Fatalf("issue token pair: %v", err)
	}
	if _, err := manager.Authenticate(ctx, pair.RefreshToken); err == nil {
		t.Fatal("expected refresh token rejected as access token")
	}

	claims := jwt.MapClaims{
		"iss": "redcart-copilot", "sub": "42", "aud": "redcart-api",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
		"jti": "wrong-algorithm", "token_use": "access", "role": "consumer",
	}
	wrongAlgorithm, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign HS384 token: %v", err)
	}
	if _, err := manager.Authenticate(ctx, wrongAlgorithm); err == nil {
		t.Fatal("expected non-HS256 JWT to be rejected")
	}
}

func TestJWTManagerRequiresStrongSecret(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	config := testJWTConfig()
	config.Secret = "too-short"
	if _, err := NewJWTManager(client, config); err == nil {
		t.Fatal("expected JWT secret shorter than 32 bytes to be rejected")
	}
}

func TestConfigFromEnvUsesJWTDefaultsAndRejectsBadTTL(t *testing.T) {
	t.Setenv("JWT_SECRET", testJWTSecret)
	t.Setenv("JWT_ISSUER", "")
	t.Setenv("JWT_AUDIENCE", "")
	t.Setenv("JWT_ACCESS_TTL", "")
	t.Setenv("JWT_REFRESH_TTL", "")
	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("config from defaults: %v", err)
	}
	if config.Issuer != "redcart-copilot" || config.Audience != "redcart-api" {
		t.Fatalf("unexpected JWT identity defaults: %+v", config)
	}
	if config.AccessTTL != 15*time.Minute || config.RefreshTTL != 7*24*time.Hour {
		t.Fatalf("unexpected JWT TTL defaults: %+v", config)
	}

	t.Setenv("JWT_ACCESS_TTL", "never")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("expected invalid JWT_ACCESS_TTL to be rejected")
	}
}

func newTestJWTManager(t *testing.T) (*JWTManager, goredis.UniversalClient) {
	t.Helper()
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	manager, err := NewJWTManager(client, testJWTConfig())
	if err != nil {
		t.Fatalf("new JWT manager: %v", err)
	}
	return manager, client
}

func testJWTConfig() Config {
	return Config{
		Secret:     testJWTSecret,
		Issuer:     "redcart-copilot",
		Audience:   "redcart-api",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 7 * 24 * time.Hour,
	}
}

func testPrincipal() application.TokenPrincipal {
	return application.TokenPrincipal{
		UserID:     42,
		Role:       "consumer",
		Nickname:   "JWT User",
		MerchantID: 0,
	}
}
