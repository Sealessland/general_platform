package application

import (
	"context"
	"errors"
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
	return s.issueSession(ctx, user)
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
	return s.issueSession(ctx, user)
}

func (s *Service) Logout(ctx context.Context, rawAccessToken string) error {
	credential := strings.TrimPrefix(strings.TrimSpace(rawAccessToken), "Bearer ")
	if err := s.tokens.Revoke(ctx, credential); err != nil {
		return mapTokenError(err, "revoke access token")
	}
	return nil
}

func (s *Service) RefreshSession(ctx context.Context, refreshToken string) (*AuthSession, error) {
	pair, principal, err := s.tokens.Rotate(ctx, refreshToken)
	if err != nil {
		return nil, mapTokenError(err, "rotate refresh token")
	}
	user, ok := s.repo.GetUser(principal.UserID)
	if !ok {
		return nil, newError(ErrorUnauthorized, "invalid refresh token")
	}
	return s.authSession(pair, user), nil
}

func (s *Service) Me(ctx context.Context, token string) (*UserView, error) {
	principal, err := s.tokens.Authenticate(ctx, token)
	if err != nil {
		return nil, mapTokenError(err, "authenticate access token")
	}
	user, ok := s.repo.GetUser(principal.UserID)
	if !ok {
		return nil, newError(ErrorUnauthorized, "invalid token")
	}
	view := s.toUserView(user)
	return &view, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (*Actor, error) {
	principal, err := s.tokens.Authenticate(ctx, token)
	if err != nil {
		return nil, mapTokenError(err, "authenticate access token")
	}
	return &Actor{
		UserID:     principal.UserID,
		Role:       principal.Role,
		Nickname:   principal.Nickname,
		MerchantID: principal.MerchantID,
	}, nil
}

func (s *Service) issueSession(ctx context.Context, user domain.User) (*AuthSession, error) {
	pair, err := s.tokens.Issue(ctx, s.tokenPrincipal(user))
	if err != nil {
		return nil, fmt.Errorf("issue JWT session: %w", err)
	}
	return s.authSession(pair, user), nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func (s *Service) tokenPrincipal(user domain.User) TokenPrincipal {
	principal := TokenPrincipal{UserID: user.ID, Role: user.Role, Nickname: user.Nickname}
	if merchant, ok := s.repo.GetMerchantByUserID(user.ID); ok {
		principal.MerchantID = merchant.ID
	}
	return principal
}

func (s *Service) authSession(pair TokenPair, user domain.User) *AuthSession {
	return &AuthSession{
		Token:        pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		User:         s.toUserView(user),
	}
}

func mapTokenError(err error, operation string) error {
	if errors.Is(err, ErrInvalidToken) {
		return newError(ErrorUnauthorized, "missing or invalid token")
	}
	return fmt.Errorf("%s: %w", operation, err)
}
