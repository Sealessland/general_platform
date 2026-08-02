package testsupport

import (
	"context"
	"fmt"
	"sync"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
)

// TokenManager is a small in-memory test double. Production wiring must use
// infrastructure/auth.JWTManager so tests do not disguise a fake runtime path.
type TokenManager struct {
	mu      sync.Mutex
	next    int64
	access  map[string]session
	refresh map[string]session
}

type session struct {
	principal application.TokenPrincipal
	access    string
	refresh   string
}

func NewTokenManager() *TokenManager {
	return &TokenManager{
		access:  make(map[string]session),
		refresh: make(map[string]session),
	}
}

func (m *TokenManager) Issue(_ context.Context, principal application.TokenPrincipal) (application.TokenPair, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.issue(principal), nil
}

func (m *TokenManager) Authenticate(_ context.Context, accessToken string) (application.TokenPrincipal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.access[accessToken]
	if !ok {
		return application.TokenPrincipal{}, application.ErrInvalidToken
	}
	return value.principal, nil
}

func (m *TokenManager) Rotate(_ context.Context, refreshToken string) (application.TokenPair, application.TokenPrincipal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.refresh[refreshToken]
	if !ok {
		return application.TokenPair{}, application.TokenPrincipal{}, application.ErrInvalidToken
	}
	delete(m.refresh, value.refresh)
	delete(m.access, value.access)
	return m.issue(value.principal), value.principal, nil
}

func (m *TokenManager) Revoke(_ context.Context, accessToken string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.access[accessToken]
	if !ok {
		return application.ErrInvalidToken
	}
	delete(m.access, value.access)
	delete(m.refresh, value.refresh)
	return nil
}

func (m *TokenManager) issue(principal application.TokenPrincipal) application.TokenPair {
	m.next++
	value := session{
		principal: principal,
		access:    fmt.Sprintf("test-access-%d", m.next),
		refresh:   fmt.Sprintf("test-refresh-%d", m.next),
	}
	m.access[value.access] = value
	m.refresh[value.refresh] = value
	return application.TokenPair{AccessToken: value.access, RefreshToken: value.refresh}
}

var _ application.TokenManager = (*TokenManager)(nil)
