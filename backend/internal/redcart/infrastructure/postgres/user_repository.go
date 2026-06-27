package postgres

import (
	"database/sql"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

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
		nullTime(user.CreatedAt),
		nullTime(user.UpdatedAt),
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (r *Repository) FindUserByPhone(phone string) (domain.User, bool) {
	user, err := r.queryUser(`SELECT id, nickname, phone, password_hash, role, created_at, updated_at FROM users WHERE phone = $1`, phone)
	if err == sql.ErrNoRows {
		return domain.User{}, false
	}
	return user, err == nil
}

func (r *Repository) GetUser(id int64) (domain.User, bool) {
	user, err := r.queryUser(`SELECT id, nickname, phone, password_hash, role, created_at, updated_at FROM users WHERE id = $1`, id)
	if err == sql.ErrNoRows {
		return domain.User{}, false
	}
	return user, err == nil
}

func (r *Repository) SaveSession(accessToken, refreshToken string, userID int64) error {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	r.sessions[accessToken] = sessionEntry{userID: userID, tokenType: application.TokenTypeAccess}
	if refreshToken != "" {
		r.sessions[refreshToken] = sessionEntry{userID: userID, tokenType: application.TokenTypeRefresh}
	}
	return nil
}

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
