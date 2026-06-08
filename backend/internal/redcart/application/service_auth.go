package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (s *Service) Register(ctx context.Context, input RegisterInput) (*AuthSession, error) {
	_ = ctx
	if strings.TrimSpace(input.Nickname) == "" || strings.TrimSpace(input.Phone) == "" || input.Password == "" {
		return nil, newError(ErrorInvalidArgument, "nickname, phone, and password are required")
	}
	if input.Role != domain.RoleConsumer && input.Role != domain.RoleMerchant {
		return nil, newError(ErrorInvalidArgument, "role must be consumer or merchant")
	}
	now := s.now()
	user, err := s.repo.CreateUser(domain.User{
		Nickname:     strings.TrimSpace(input.Nickname),
		Phone:        strings.TrimSpace(input.Phone),
		PasswordHash: hashPassword(input.Password),
		Role:         input.Role,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	if user.Role == domain.RoleMerchant {
		_, err = s.repo.CreateMerchant(domain.Merchant{
			UserID:      user.ID,
			Name:        fmt.Sprintf("%s 的店铺", user.Nickname),
			Description: "merchant workspace created from registration",
			Status:      "active",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if err != nil {
			return nil, err
		}
	}
	return s.issueSession(user)
}

func (s *Service) Login(ctx context.Context, input LoginInput) (*AuthSession, error) {
	_ = ctx
	user, ok := s.repo.FindUserByPhone(strings.TrimSpace(input.Phone))
	if !ok {
		return nil, newError(ErrorUnauthorized, "invalid phone or password")
	}
	if user.PasswordHash != hashPassword(input.Password) {
		return nil, newError(ErrorUnauthorized, "invalid phone or password")
	}
	return s.issueSession(user)
}

func (s *Service) Me(ctx context.Context, token string) (*UserView, error) {
	_ = ctx
	user, ok := s.repo.GetUserByToken(token)
	if !ok {
		return nil, newError(ErrorUnauthorized, "invalid token")
	}
	view := s.toUserView(user)
	return &view, nil
}

func (s *Service) Authenticate(token string) (*Actor, error) {
	user, ok := s.repo.GetUserByToken(token)
	if !ok {
		return nil, newError(ErrorUnauthorized, "missing or invalid token")
	}
	actor := &Actor{
		UserID:   user.ID,
		Role:     user.Role,
		Nickname: user.Nickname,
	}
	if merchant, ok := s.repo.GetMerchantByUserID(user.ID); ok {
		actor.MerchantID = merchant.ID
	}
	return actor, nil
}

func (s *Service) issueSession(user domain.User) (*AuthSession, error) {
	token := fmt.Sprintf("redcart-%d-%d", user.ID, s.now().UnixNano())
	s.repo.SaveSession(token, user.ID)
	view := s.toUserView(user)
	return &AuthSession{Token: token, User: view}, nil
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}
