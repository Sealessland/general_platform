package postgres

import (
	"database/sql"
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

func (r *Repository) SaveSession(accessToken, refreshToken string, userID int64) {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	r.sessions[accessToken] = userID
	if refreshToken != "" {
		r.sessions[refreshToken] = userID
	}
}

func (r *Repository) GetUserByToken(token string) (domain.User, bool) {
	r.sessionMu.RLock()
	userID, ok := r.sessions[token]
	r.sessionMu.RUnlock()
	if !ok {
		return domain.User{}, false
	}
	return r.GetUser(userID)
}

func (r *Repository) DeleteSession(token string) {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	userID, ok := r.sessions[token]
	if !ok {
		return
	}
	for t, id := range r.sessions {
		if id == userID {
			delete(r.sessions, t)
		}
	}
}
