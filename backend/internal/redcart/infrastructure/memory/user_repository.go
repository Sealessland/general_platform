package memory

import (
	"fmt"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"time"
)

func (r *Repository) CreateUser(user domain.User) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.usersByPhone[user.Phone]; exists {
		return domain.User{}, fmt.Errorf("user phone already exists")
	}
	user.ID = r.nextID(&r.nextUserID)
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = user.CreatedAt
	}
	r.users[user.ID] = cloneUser(user)
	r.usersByPhone[user.Phone] = user.ID
	return cloneUser(user), nil
}

func (r *Repository) FindUserByPhone(phone string) (domain.User, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.usersByPhone[phone]
	if !ok {
		return domain.User{}, false
	}
	user, ok := r.users[id]
	return cloneUser(user), ok
}

func (r *Repository) GetUser(id int64) (domain.User, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	user, ok := r.users[id]
	return cloneUser(user), ok
}

func (r *Repository) SaveSession(accessToken, refreshToken string, userID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[accessToken] = userID
	if refreshToken != "" {
		r.sessions[refreshToken] = userID
	}
}

func (r *Repository) GetUserByToken(token string) (domain.User, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	userID, ok := r.sessions[token]
	if !ok {
		return domain.User{}, false
	}
	user, ok := r.users[userID]
	return cloneUser(user), ok
}

func (r *Repository) DeleteSession(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
