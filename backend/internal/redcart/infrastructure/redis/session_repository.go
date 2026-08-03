package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	goredis "github.com/redis/go-redis/v9"
)

// 会话缓存的 Redis 键前缀（redcart:session: + token）。
const sessionKeyPrefix = "redcart:session:"

// SessionRepository 会话仓库：在底层仓库之上以 Redis + 进程内缓存存储
// token → 用户 会话（access/refresh 双 token），并附带商户信息缓存与
// 按用户粒度的回源互斥，避免并发登录/查询时重复访问底层仓库。
type SessionRepository struct {
	application.Repository
	client     goredis.UniversalClient
	accessTTL  time.Duration
	refreshTTL time.Duration

	cacheMu sync.RWMutex
	cache   map[string]sessionCacheEntry

	merchantMu    sync.RWMutex
	merchantCache map[int64]merchantState
	merchantLocks sync.Map
}

// sessionRecord 是写入 Redis 的会话记录，同时携带 access/refresh 两个
// token，便于删除任一侧时级联清除另一侧。
type sessionRecord struct {
	ID           int64                 `json:"id"`
	Nickname     string                `json:"nickname"`
	Phone        string                `json:"phone"`
	Role         string                `json:"role"`
	Merchant     *merchantWire         `json:"merchant,omitempty"`
	TokenType    application.TokenType `json:"token_type"`
	AccessToken  string                `json:"access_token,omitempty"`
	RefreshToken string                `json:"refresh_token,omitempty"`
}

// merchantWire 是 sessionRecord 中商户快照的 JSON 结构。
type merchantWire struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// merchantState 商户本地缓存条目：known 标记该用户是否已查询过商户
// （用于区分"无商户"与"未查询"，避免对无商户用户反复回源）。
type merchantState struct {
	known    bool
	merchant domain.Merchant
}

// sessionCacheEntry 本地会话缓存条目；found=false 表示负缓存（已确认未命中）。
type sessionCacheEntry struct {
	user      domain.User
	tokenType application.TokenType
	expiresAt time.Time
	found     bool
}

// NewSessionRepository 构造会话仓库；TTL<=0 时回退到默认值。
func NewSessionRepository(base application.Repository, client goredis.UniversalClient, accessTTL, refreshTTL time.Duration) *SessionRepository {
	if accessTTL <= 0 {
		accessTTL = defaultAccessTokenTTL
	}
	if refreshTTL <= 0 {
		refreshTTL = defaultRefreshTokenTTL
	}
	return &SessionRepository{
		Repository:    base,
		client:        client,
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
		cache:         make(map[string]sessionCacheEntry),
		merchantCache: make(map[int64]merchantState),
	}
}

// Append 透传事件到底层 outbox（若底层实现 event.Outbox），否则忽略。
func (r *SessionRepository) Append(ctx context.Context, evt event.Event) (int64, error) {
	if outbox, ok := r.Repository.(event.Outbox); ok {
		return outbox.Append(ctx, evt)
	}
	return 0, nil
}

// SaveSession 登录成功后落库会话：将 access/refresh 两个 token 分别写入
// Redis（记录内嵌完整会话与商户快照），并回填本地缓存。
func (r *SessionRepository) SaveSession(accessToken, refreshToken string, userID int64) error {
	if r.client == nil {
		return r.Repository.SaveSession(accessToken, refreshToken, userID)
	}
	if accessToken == "" || userID == 0 {
		return nil
	}

	user, ok := r.Repository.GetUser(userID)
	if !ok {
		return nil
	}

	base := sessionRecord{
		ID:           user.ID,
		Nickname:     user.Nickname,
		Phone:        user.Phone,
		Role:         user.Role,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}
	if merchant, ok := r.Repository.GetMerchantByUserID(userID); ok {
		base.Merchant = &merchantWire{
			ID:          merchant.ID,
			Name:        merchant.Name,
			Description: merchant.Description,
			Status:      merchant.Status,
		}
		r.saveMerchantCache(userID, merchantState{known: true, merchant: merchant})
	} else {
		r.saveMerchantCache(userID, merchantState{known: true})
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultWriteTimeout)
	defer cancel()

	accessRecord := base
	accessRecord.TokenType = application.TokenTypeAccess
	accessPayload, err := json.Marshal(accessRecord)
	if err != nil {
		return fmt.Errorf("marshal access session: %w", err)
	}
	accessTTL := ttlWithJitter(r.accessTTL)
	_ = r.client.Set(ctx, sessionKey(accessToken), accessPayload, accessTTL).Err()
	r.saveCacheWithTTL(accessToken, user, application.TokenTypeAccess, accessTTL)

	if refreshToken != "" {
		refreshRecord := base
		refreshRecord.TokenType = application.TokenTypeRefresh
		refreshPayload, err := json.Marshal(refreshRecord)
		if err != nil {
			return fmt.Errorf("marshal refresh session: %w", err)
		}
		refreshTTL := ttlWithJitter(r.refreshTTL)
		_ = r.client.Set(ctx, sessionKey(refreshToken), refreshPayload, refreshTTL).Err()
		r.saveCacheWithTTL(refreshToken, user, application.TokenTypeRefresh, refreshTTL)
	}
	return nil
}

// GetUserByToken 解析 token 对应的用户：本地缓存 → Redis → 底层仓库；
// Redis 明确未命中（Nil）时写入短期负缓存，防止不存在 token 击穿后端。
func (r *SessionRepository) GetUserByToken(token string) (domain.User, application.TokenType, bool) {
	if token == "" {
		return domain.User{}, "", false
	}
	if user, tokenType, found, ok := r.loadCache(token); ok {
		return user, tokenType, found
	}
	if r.client == nil {
		return r.Repository.GetUserByToken(token)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultReadTimeout)
	defer cancel()
	payload, err := r.client.Get(ctx, sessionKey(token)).Bytes()
	if err == nil {
		user, state, tokenType, ok := decodeSessionUser(payload)
		if ok {
			r.saveCache(token, user, tokenType)
			r.saveMerchantCache(user.ID, state)
			return user, tokenType, true
		}
	}
	if err == goredis.Nil {
		r.saveNegativeCache(token)
		return domain.User{}, "", false
	}
	return domain.User{}, "", false
}

// DeleteSession 删除会话：任一 token 键的值中都存有双 token，因此按记录
// 级联删除 access/refresh 两个键，并清理本地缓存。
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

// decodeSessionRecord 解析并校验会话记录（ID 与 Role 必须非空）。
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

// decodeSessionUser 将会话记录还原为 domain.User 与商户缓存状态。
func decodeSessionUser(payload []byte) (domain.User, merchantState, application.TokenType, bool) {
	var record sessionRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return domain.User{}, merchantState{}, "", false
	}
	if record.ID == 0 || record.Role == "" {
		return domain.User{}, merchantState{}, "", false
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
	}, state, record.TokenType, true
}

// sessionKey 拼接会话键（前缀 + token）。
func sessionKey(token string) string {
	return fmt.Sprintf("%s%s", sessionKeyPrefix, token)
}

// saveCacheWithTTL 写本地会话缓存并记录指定 TTL 的过期时间。
func (r *SessionRepository) saveCacheWithTTL(token string, user domain.User, tokenType application.TokenType, ttl time.Duration) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	r.cache[token] = sessionCacheEntry{
		user:      user,
		tokenType: tokenType,
		expiresAt: time.Now().Add(ttl),
		found:     true,
	}
}

// saveCache 写本地会话缓存，TTL 按 token 类型（access/refresh）选取。
func (r *SessionRepository) saveCache(token string, user domain.User, tokenType application.TokenType) {
	ttl := r.accessTTL
	if tokenType == application.TokenTypeRefresh {
		ttl = r.refreshTTL
	}
	r.saveCacheWithTTL(token, user, tokenType, ttlWithJitter(ttl))
}

// saveNegativeCache 写负缓存条目：标记该 token 已确认无效，TTL 取较短值。
func (r *SessionRepository) saveNegativeCache(token string) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	r.cache[token] = sessionCacheEntry{
		expiresAt: time.Now().Add(negativeCacheTTL(r.accessTTL)),
		found:     false,
	}
}

// loadCache 读本地会话缓存；过期条目会被清除并视为未命中。
func (r *SessionRepository) loadCache(token string) (domain.User, application.TokenType, bool, bool) {
	r.cacheMu.RLock()
	session, ok := r.cache[token]
	r.cacheMu.RUnlock()
	if !ok || time.Now().After(session.expiresAt) {
		if ok {
			r.cacheMu.Lock()
			delete(r.cache, token)
			r.cacheMu.Unlock()
		}
		return domain.User{}, "", false, false
	}
	return session.user, session.tokenType, session.found, true
}

// invalidateCache 删除本地缓存条目。
func (r *SessionRepository) invalidateCache(token string) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	delete(r.cache, token)
}

// ttlWithJitter 在基础 TTL 上叠加 0~25% 的随机抖动，错开大批会话的
// 统一过期时间，避免缓存雪崩。
func ttlWithJitter(base time.Duration) time.Duration {
	if base <= 0 {
		return base
	}
	jitter := time.Duration(rand.Int63n(int64(base) / 4))
	return base + jitter
}

// negativeCacheTTL 负缓存有效期：基准 TTL 的 1/10，并夹在 5s 到 1min 之间。
func negativeCacheTTL(base time.Duration) time.Duration {
	ttl := base / 10
	if ttl < 5*time.Second {
		ttl = 5 * time.Second
	}
	if ttl > time.Minute {
		ttl = time.Minute
	}
	return ttl
}

// GetMerchantByUserID 读取用户商户信息：本地缓存优先；未命中时按用户
// 粒度加锁回源，避免并发请求同时穿透到底层仓库。
func (r *SessionRepository) GetMerchantByUserID(userID int64) (domain.Merchant, bool) {
	if state, ok := r.loadMerchantCache(userID); ok {
		if state.merchant.ID == 0 {
			return domain.Merchant{}, false
		}
		return state.merchant, true
	}

	mu, _ := r.merchantLocks.LoadOrStore(userID, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	if state, ok := r.loadMerchantCache(userID); ok {
		if state.merchant.ID == 0 {
			return domain.Merchant{}, false
		}
		return state.merchant, true
	}
	merchant, ok := r.Repository.GetMerchantByUserID(userID)
	r.saveMerchantCache(userID, merchantState{known: true, merchant: merchant})
	return merchant, ok
}

// loadMerchantCache 读商户本地缓存。
func (r *SessionRepository) loadMerchantCache(userID int64) (merchantState, bool) {
	r.merchantMu.RLock()
	state, ok := r.merchantCache[userID]
	r.merchantMu.RUnlock()
	if !ok {
		return merchantState{}, false
	}
	return state, true
}

// saveMerchantCache 写商户本地缓存。
func (r *SessionRepository) saveMerchantCache(userID int64, state merchantState) {
	r.merchantMu.Lock()
	defer r.merchantMu.Unlock()
	r.merchantCache[userID] = state
}
