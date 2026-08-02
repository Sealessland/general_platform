package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/golang-jwt/jwt/v5"
	goredis "github.com/redis/go-redis/v9"
)

const (
	refreshKeyPrefix = "redcart:auth:refresh:"
	revokedKeyPrefix = "redcart:auth:revoked:"
	accessTokenUse   = "access"
	refreshTokenUse  = "refresh"
)

var rotateScript = goredis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
  return 0
end
redis.call('DEL', KEYS[1])
redis.call('SET', KEYS[2], '1', 'PX', ARGV[1])
if tonumber(ARGV[2]) > 0 then
  redis.call('SET', KEYS[3], '1', 'PX', ARGV[2])
end
return 1
`)

type Config struct {
	Secret     string
	Issuer     string
	Audience   string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	Now        func() time.Time
}

type JWTManager struct {
	client goredis.UniversalClient
	config Config
}

type tokenClaims struct {
	Role            string `json:"role"`
	Nickname        string `json:"nickname"`
	MerchantID      int64  `json:"merchant_id,omitempty"`
	TokenUse        string `json:"token_use"`
	SessionID       string `json:"sid"`
	AccessJTI       string `json:"access_jti,omitempty"`
	AccessExpiresAt int64  `json:"access_expires_at,omitempty"`
	jwt.RegisteredClaims
}

type signedPair struct {
	pair          application.TokenPair
	accessClaims  tokenClaims
	refreshClaims tokenClaims
}

func NewJWTManager(client goredis.UniversalClient, config Config) (*JWTManager, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if len(config.Secret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must contain at least 32 bytes")
	}
	if config.Issuer == "" || config.Audience == "" {
		return nil, fmt.Errorf("JWT issuer and audience are required")
	}
	if config.AccessTTL <= 0 || config.RefreshTTL <= 0 {
		return nil, fmt.Errorf("JWT token TTLs must be positive")
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	return &JWTManager{client: client, config: config}, nil
}

func (m *JWTManager) Issue(ctx context.Context, principal application.TokenPrincipal) (application.TokenPair, error) {
	signed, err := m.signPair(principal)
	if err != nil {
		return application.TokenPair{}, err
	}
	if err := m.client.Set(ctx, refreshKey(signed.refreshClaims.ID), "1", m.config.RefreshTTL).Err(); err != nil {
		return application.TokenPair{}, fmt.Errorf("store refresh session: %w", err)
	}
	return signed.pair, nil
}

func (m *JWTManager) Authenticate(ctx context.Context, accessToken string) (application.TokenPrincipal, error) {
	claims, err := m.parse(accessToken, accessTokenUse)
	if err != nil {
		return application.TokenPrincipal{}, err
	}
	revoked, err := m.client.Exists(ctx, revokedKey(claims.ID)).Result()
	if err != nil {
		return application.TokenPrincipal{}, fmt.Errorf("check access revocation: %w", err)
	}
	if revoked > 0 {
		return application.TokenPrincipal{}, application.ErrInvalidToken
	}
	return principalFromClaims(claims), nil
}

func (m *JWTManager) Rotate(ctx context.Context, refreshToken string) (application.TokenPair, application.TokenPrincipal, error) {
	oldClaims, err := m.parse(refreshToken, refreshTokenUse)
	if err != nil {
		return application.TokenPair{}, application.TokenPrincipal{}, err
	}
	principal := principalFromClaims(oldClaims)
	signed, err := m.signPair(principal)
	if err != nil {
		return application.TokenPair{}, application.TokenPrincipal{}, err
	}
	oldAccessTTL := time.Until(time.Unix(oldClaims.AccessExpiresAt, 0))
	if m.config.Now != nil {
		oldAccessTTL = time.Unix(oldClaims.AccessExpiresAt, 0).Sub(m.config.Now())
	}
	rotated, err := rotateScript.Run(ctx, m.client, []string{
		refreshKey(oldClaims.ID),
		refreshKey(signed.refreshClaims.ID),
		revokedKey(oldClaims.AccessJTI),
	}, m.config.RefreshTTL.Milliseconds(), max(oldAccessTTL.Milliseconds(), 0)).Int()
	if err != nil {
		return application.TokenPair{}, application.TokenPrincipal{}, fmt.Errorf("rotate refresh session: %w", err)
	}
	if rotated != 1 {
		return application.TokenPair{}, application.TokenPrincipal{}, application.ErrInvalidToken
	}
	return signed.pair, principal, nil
}

func (m *JWTManager) Revoke(ctx context.Context, accessToken string) error {
	claims, err := m.parse(accessToken, accessTokenUse)
	if err != nil {
		return err
	}
	ttl := claims.ExpiresAt.Time.Sub(m.config.Now())
	pipe := m.client.TxPipeline()
	pipe.Del(ctx, refreshKey(claims.SessionID))
	if ttl > 0 {
		pipe.Set(ctx, revokedKey(claims.ID), "1", ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("revoke JWT session: %w", err)
	}
	return nil
}

func (m *JWTManager) signPair(principal application.TokenPrincipal) (signedPair, error) {
	now := m.config.Now()
	accessJTI, err := randomID()
	if err != nil {
		return signedPair{}, fmt.Errorf("generate access jti: %w", err)
	}
	refreshJTI, err := randomID()
	if err != nil {
		return signedPair{}, fmt.Errorf("generate refresh jti: %w", err)
	}
	accessExpiry := now.Add(m.config.AccessTTL)
	accessClaims := m.claims(principal, accessTokenUse, accessJTI, refreshJTI, now, accessExpiry)
	refreshClaims := m.claims(principal, refreshTokenUse, refreshJTI, refreshJTI, now, now.Add(m.config.RefreshTTL))
	refreshClaims.AccessJTI = accessJTI
	refreshClaims.AccessExpiresAt = accessExpiry.Unix()
	accessToken, err := m.sign(accessClaims)
	if err != nil {
		return signedPair{}, err
	}
	refreshToken, err := m.sign(refreshClaims)
	if err != nil {
		return signedPair{}, err
	}
	return signedPair{
		pair:          application.TokenPair{AccessToken: accessToken, RefreshToken: refreshToken},
		accessClaims:  accessClaims,
		refreshClaims: refreshClaims,
	}, nil
}

func (m *JWTManager) claims(principal application.TokenPrincipal, tokenUse, jti, sessionID string, issuedAt, expiresAt time.Time) tokenClaims {
	return tokenClaims{
		Role:       principal.Role,
		Nickname:   principal.Nickname,
		MerchantID: principal.MerchantID,
		TokenUse:   tokenUse,
		SessionID:  sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.config.Issuer,
			Subject:   strconv.FormatInt(principal.UserID, 10),
			Audience:  jwt.ClaimStrings{m.config.Audience},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ID:        jti,
		},
	}
}

func (m *JWTManager) sign(claims tokenClaims) (string, error) {
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(m.config.Secret))
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	return token, nil
}

func (m *JWTManager) parse(raw, tokenUse string) (tokenClaims, error) {
	claims := tokenClaims{}
	token, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (any, error) {
		return []byte(m.config.Secret), nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.config.Issuer),
		jwt.WithAudience(m.config.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(m.config.Now),
	)
	if err != nil || !token.Valid || claims.TokenUse != tokenUse || claims.ID == "" || claims.Subject == "" || claims.SessionID == "" {
		return tokenClaims{}, application.ErrInvalidToken
	}
	if _, err := strconv.ParseInt(claims.Subject, 10, 64); err != nil {
		return tokenClaims{}, application.ErrInvalidToken
	}
	if tokenUse == refreshTokenUse && (claims.AccessJTI == "" || claims.AccessExpiresAt == 0) {
		return tokenClaims{}, application.ErrInvalidToken
	}
	return claims, nil
}

func principalFromClaims(claims tokenClaims) application.TokenPrincipal {
	userID, _ := strconv.ParseInt(claims.Subject, 10, 64)
	return application.TokenPrincipal{
		UserID:     userID,
		Role:       claims.Role,
		Nickname:   claims.Nickname,
		MerchantID: claims.MerchantID,
	}
}

func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func refreshKey(jti string) string { return refreshKeyPrefix + jti }
func revokedKey(jti string) string { return revokedKeyPrefix + jti }

var _ application.TokenManager = (*JWTManager)(nil)
