package postgres

import (
	"database/sql"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

// CreateUser 创建用户并返回带数据库生成 ID 与时间戳的记录。
func (r *Repository) CreateUser(user domain.User) (domain.User, error) {
	query := `
INSERT INTO users (nickname, phone, password_hash, role, created_at, updated_at)
VALUES ($1, $2, $3, $4, COALESCE($5, CURRENT_TIMESTAMP), COALESCE($6, CURRENT_TIMESTAMP))
RETURNING id, created_at, updated_at`
	if err := r.db.QueryRow(
		query,
		user.Nickname,
		user.Phone,
		user.PasswordHash,
		user.Role,
		timeToSQL(user.CreatedAt),
		timeToSQL(user.UpdatedAt),
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

// FindUserByPhone 按手机号查询用户；不存在返回 (零值, false)。
func (r *Repository) FindUserByPhone(phone string) (domain.User, bool) {
	user, err := r.queryUser(`SELECT id, nickname, phone, password_hash, role, created_at, updated_at FROM users WHERE phone = $1`, phone)
	if err == sql.ErrNoRows {
		return domain.User{}, false
	}
	return user, err == nil
}

// GetUser 按 ID 查询用户；不存在返回 (零值, false)。
func (r *Repository) GetUser(id int64) (domain.User, bool) {
	user, err := r.queryUser(`SELECT id, nickname, phone, password_hash, role, created_at, updated_at FROM users WHERE id = $1`, id)
	if err == sql.ErrNoRows {
		return domain.User{}, false
	}
	return user, err == nil
}

// SaveSession / GetUserByToken / DeleteSession 维护进程内的 token -> 用户
// 内存会话表，仅用于单实例演示；多实例或正式部署应替换为 Redis 等共享
// 会话存储（见 infrastructure/redis 下的 SessionRepository）。
func (r *Repository) SaveSession(accessToken, refreshToken string, userID int64) error {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	r.sessions[accessToken] = sessionEntry{userID: userID, tokenType: application.TokenTypeAccess}
	if refreshToken != "" {
		r.sessions[refreshToken] = sessionEntry{userID: userID, tokenType: application.TokenTypeRefresh}
	}
	return nil
}

// GetUserByToken 按 token 查询对应用户与 token 类型；未命中返回 (零值, "", false)。
func (r *Repository) GetUserByToken(token string) (domain.User, application.TokenType, bool) {
	r.sessionMu.RLock()
	entry, ok := r.sessions[token]
	r.sessionMu.RUnlock()
	if !ok {
		return domain.User{}, "", false
	}
	user, found := r.GetUser(entry.userID)
	if !found {
		return domain.User{}, "", false
	}
	return user, entry.tokenType, true
}

// DeleteSession 注销时删除该用户的全部 token（一处退出、所有会话失效）。
func (r *Repository) DeleteSession(token string) {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	entry, ok := r.sessions[token]
	if !ok {
		return
	}
	for t, e := range r.sessions {
		if e.userID == entry.userID {
			delete(r.sessions, t)
		}
	}
}
