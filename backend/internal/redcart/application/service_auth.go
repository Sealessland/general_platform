package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"golang.org/x/crypto/bcrypt"
)

func (s *Service) Register(ctx context.Context, input RegisterInput) (*AuthSession, error) {
	_ = ctx
	if strings.TrimSpace(input.Nickname) == "" || strings.TrimSpace(input.Phone) == "" || input.Password == "" {
		return nil, newError(ErrorInvalidArgument, "nickname, phone, and password are required")
	}
	if input.Role != domain.RoleConsumer && input.Role != domain.RoleMerchant {
		return nil, newError(ErrorInvalidArgument, "role must be consumer or merchant")
	}
	hash, err := hashPassword(input.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	now := s.now()
	user, err := s.repo.CreateUser(domain.User{
		Nickname:     strings.TrimSpace(input.Nickname),
		Phone:        strings.TrimSpace(input.Phone),
		PasswordHash: hash,
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
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return nil, newError(ErrorUnauthorized, "invalid phone or password")
	}
	return s.issueSession(user)
}

func (s *Service) Logout(ctx context.Context, token string) error {
	_ = ctx
	s.repo.DeleteSession(strings.TrimPrefix(strings.TrimSpace(token), "Bearer "))
	return nil
}

func (s *Service) RefreshSession(ctx context.Context, refreshToken string) (*AuthSession, error) {
	_ = ctx
	user, ok := s.repo.GetUserByToken(refreshToken)
	if !ok {
		return nil, newError(ErrorUnauthorized, "invalid refresh token")
	}
	s.repo.DeleteSession(refreshToken)
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
	accessToken, err := generateOpaqueToken()
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}
	refreshToken, err := generateOpaqueToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	s.repo.SaveSession(accessToken, refreshToken, user.ID)
	view := s.toUserView(user)
	return &AuthSession{
		Token:        accessToken,
		RefreshToken: refreshToken,
		User:         view,
	}, nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func generateOpaqueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
