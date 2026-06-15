package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	goredis "github.com/redis/go-redis/v9"
)

const sessionKeyPrefix = "redcart:session:"

type SessionRepository struct {
	application.Repository
	client goredis.UniversalClient
	ttl    time.Duration

	cacheMu sync.RWMutex
	cache   map[string]sessionCacheEntry

	merchantMu    sync.RWMutex
	merchantCache map[int64]merchantState
}

type sessionRecord struct {
	ID           int64         `json:"id"`
	Nickname     string        `json:"nickname"`
	Phone        string        `json:"phone"`
	Role         string        `json:"role"`
	Merchant     *merchantWire `json:"merchant,omitempty"`
	AccessToken  string        `json:"access_token,omitempty"`
	RefreshToken string        `json:"refresh_token,omitempty"`
}

type merchantWire struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type merchantState struct {
	known    bool
	merchant domain.Merchant
}

type sessionCacheEntry struct {
	user      domain.User
	expiresAt time.Time
}

func NewSessionRepository(base application.Repository, client goredis.UniversalClient, ttl time.Duration) *SessionRepository {
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	return &SessionRepository{
		Repository:    base,
		client:        client,
		ttl:           ttl,
		cache:         make(map[string]sessionCacheEntry),
		merchantCache: make(map[int64]merchantState),
	}
}

func (r *SessionRepository) SaveSession(accessToken, refreshToken string, userID int64) {
	if r.client == nil {
		r.Repository.SaveSession(accessToken, refreshToken, userID)
		return
	}
	if accessToken == "" || userID == 0 {
		return
	}

	user, ok := r.Repository.GetUser(userID)
	if !ok {
		return
	}

	record := sessionRecord{
		ID:           user.ID,
		Nickname:     user.Nickname,
		Phone:        user.Phone,
		Role:         user.Role,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}
	if merchant, ok := r.Repository.GetMerchantByUserID(userID); ok {
		record.Merchant = &merchantWire{
			ID:          merchant.ID,
			Name:        merchant.Name,
			Description: merchant.Description,
			Status:      merchant.Status,
		}
		r.saveMerchantCache(userID, merchantState{known: true, merchant: merchant})
	} else {
		r.saveMerchantCache(userID, merchantState{known: true})
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultWriteTimeout)
	defer cancel()
	_ = r.client.Set(ctx, sessionKey(accessToken), payload, r.ttl).Err()
	r.saveCache(accessToken, user)
	if refreshToken != "" {
		_ = r.client.Set(ctx, sessionKey(refreshToken), payload, r.ttl).Err()
		r.saveCache(refreshToken, user)
	}
}

func (r *SessionRepository) GetUserByToken(token string) (domain.User, bool) {
	if token == "" {
		return domain.User{}, false
	}
	if user, ok := r.loadCache(token); ok {
		return user, true
	}
	if r.client == nil {
		return r.Repository.GetUserByToken(token)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultReadTimeout)
	defer cancel()
	payload, err := r.client.Get(ctx, sessionKey(token)).Bytes()
	if err == nil {
		user, state, ok := decodeSessionUser(payload)
		if ok {
			r.saveCache(token, user)
			r.saveMerchantCache(user.ID, state)
			return user, true
		}
	}
	if err == goredis.Nil {
		return domain.User{}, false
	}
	return domain.User{}, false
}

func (r *SessionRepository) DeleteSession(token string) {
	if token == "" {
		return
	}
	r.invalidateCache(token)
	if r.client == nil {
		r.Repository.DeleteSession(token)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultWriteTimeout)
	defer cancel()
	payload, err := r.client.Get(ctx, sessionKey(token)).Bytes()
	if err == nil {
		if record, ok := decodeSessionRecord(payload); ok {
			_ = r.client.Del(ctx, sessionKey(record.AccessToken), sessionKey(record.RefreshToken)).Err()
			if record.AccessToken != "" {
				r.invalidateCache(record.AccessToken)
			}
			if record.RefreshToken != "" {
				r.invalidateCache(record.RefreshToken)
			}
			return
		}
	}
	_ = r.client.Del(ctx, sessionKey(token)).Err()
}

func decodeSessionRecord(payload []byte) (sessionRecord, bool) {
	var record sessionRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return sessionRecord{}, false
	}
	if record.ID == 0 || record.Role == "" {
		return sessionRecord{}, false
	}
	return record, true
}

func (r *SessionRepository) GetMerchantByUserID(userID int64) (domain.Merchant, bool) {
	r.merchantMu.RLock()
	state, ok := r.merchantCache[userID]
	r.merchantMu.RUnlock()
	if ok && state.known {
		if state.merchant.ID == 0 {
			return domain.Merchant{}, false
		}
		return state.merchant, true
	}
	return r.Repository.GetMerchantByUserID(userID)
}

func decodeSessionUser(payload []byte) (domain.User, merchantState, bool) {
	var record sessionRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return domain.User{}, merchantState{}, false
	}
	if record.ID == 0 || record.Role == "" {
		return domain.User{}, merchantState{}, false
	}
	state := merchantState{known: true}
	if record.Merchant != nil {
		state.merchant = domain.Merchant{
			ID:          record.Merchant.ID,
			UserID:      record.ID,
			Name:        record.Merchant.Name,
			Description: record.Merchant.Description,
			Status:      record.Merchant.Status,
		}
	}
	return domain.User{
		ID:       record.ID,
		Nickname: record.Nickname,
		Phone:    record.Phone,
		Role:     record.Role,
	}, state, true
}

func sessionKey(token string) string {
	return fmt.Sprintf("%s%s", sessionKeyPrefix, token)
}

func (r *SessionRepository) saveCache(token string, user domain.User) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	r.cache[token] = sessionCacheEntry{
		user:      user,
		expiresAt: time.Now().Add(r.ttl),
	}
}

func (r *SessionRepository) loadCache(token string) (domain.User, bool) {
	r.cacheMu.RLock()
	session, ok := r.cache[token]
	r.cacheMu.RUnlock()
	if !ok || time.Now().After(session.expiresAt) {
		if ok {
			r.cacheMu.Lock()
			delete(r.cache, token)
			r.cacheMu.Unlock()
		}
		return domain.User{}, false
	}
	return session.user, true
}

func (r *SessionRepository) invalidateCache(token string) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	delete(r.cache, token)
}

func (r *SessionRepository) saveMerchantCache(userID int64, state merchantState) {
	r.merchantMu.Lock()
	defer r.merchantMu.Unlock()
	r.merchantCache[userID] = state
}
