package application_test

import (
	"context"
	"testing"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	application "github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
)

func TestRegisterValidationAndMerchantSession(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})

	cases := []struct {
		name  string
		input application.RegisterInput
	}{
		{name: "missing nickname", input: application.RegisterInput{Phone: "13900001000", Password: "secret", Role: domain.RoleConsumer}},
		{name: "missing phone", input: application.RegisterInput{Nickname: "Alice", Password: "secret", Role: domain.RoleConsumer}},
		{name: "missing password", input: application.RegisterInput{Nickname: "Alice", Phone: "13900001000", Role: domain.RoleConsumer}},
		{name: "invalid role", input: application.RegisterInput{Nickname: "Alice", Phone: "13900001000", Password: "secret", Role: "admin"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.Register(context.Background(), tc.input)
			if !isAppError(err, application.ErrorInvalidArgument) {
				t.Fatalf("expected invalid argument, got %v", err)
			}
		})
	}

	session, err := service.Register(context.Background(), application.RegisterInput{
		Nickname: "Merchant A",
		Phone:    "13900001001",
		Password: "secret",
		Role:     domain.RoleMerchant,
	})
	if err != nil {
		t.Fatalf("register merchant: %v", err)
	}
	if session.Token == "" || session.User.MerchantID == 0 || session.User.Merchant == nil {
		t.Fatalf("expected merchant session with token and merchant view, got %+v", session)
	}

	_, err = service.Register(context.Background(), application.RegisterInput{
		Nickname: "Merchant A Duplicate",
		Phone:    "13900001001",
		Password: "secret",
		Role:     domain.RoleMerchant,
	})
	if !isAppError(err, application.ErrorConflict) {
		t.Fatalf("expected duplicate phone conflict, got %v", err)
	}
}

func TestLoginMeAndAuthenticateRejectInvalidCredentials(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})

	if _, err := service.Login(context.Background(), application.LoginInput{Phone: "13800000001", Password: "wrong"}); !isAppError(err, application.ErrorUnauthorized) {
		t.Fatalf("expected unauthorized login, got %v", err)
	}
	if _, err := service.Me(context.Background(), "missing-token"); !isAppError(err, application.ErrorUnauthorized) {
		t.Fatalf("expected unauthorized me, got %v", err)
	}
	if _, err := service.Authenticate("missing-token"); !isAppError(err, application.ErrorUnauthorized) {
		t.Fatalf("expected unauthorized authenticate, got %v", err)
	}

	session, err := service.Login(context.Background(), application.LoginInput{Phone: "13800000002", Password: "merchant-demo"})
	if err != nil {
		t.Fatalf("login merchant: %v", err)
	}
	actor, err := service.Authenticate(session.Token)
	if err != nil {
		t.Fatalf("authenticate merchant: %v", err)
	}
	if actor.Role != domain.RoleMerchant || actor.MerchantID == 0 {
		t.Fatalf("expected merchant actor, got %+v", actor)
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})
	session, err := service.Login(context.Background(), application.LoginInput{Phone: "13800000001", Password: "consumer-demo"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := service.Logout(context.Background(), session.Token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := service.Authenticate(session.Token); !isAppError(err, application.ErrorUnauthorized) {
		t.Fatalf("expected session invalidated after logout, got %v", err)
	}
}

func TestRefreshSessionRotatesTokens(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})
	session, err := service.Login(context.Background(), application.LoginInput{Phone: "13800000001", Password: "consumer-demo"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if session.RefreshToken == "" {
		t.Fatal("expected refresh token")
	}

	refreshed, err := service.RefreshSession(context.Background(), session.RefreshToken)
	if err != nil {
		t.Fatalf("refresh session: %v", err)
	}
	if refreshed.Token == session.Token || refreshed.RefreshToken == session.RefreshToken {
		t.Fatal("expected new tokens after refresh")
	}
	if _, err := service.Authenticate(session.Token); !isAppError(err, application.ErrorUnauthorized) {
		t.Fatalf("expected old access token invalidated, got %v", err)
	}
	if _, err := service.RefreshSession(context.Background(), session.RefreshToken); !isAppError(err, application.ErrorUnauthorized) {
		t.Fatalf("expected old refresh token invalidated, got %v", err)
	}
	actor, err := service.Authenticate(refreshed.Token)
	if err != nil {
		t.Fatalf("authenticate refreshed token: %v", err)
	}
	if actor.UserID != session.User.ID {
		t.Fatalf("expected same user after refresh, got %+v", actor)
	}
}
