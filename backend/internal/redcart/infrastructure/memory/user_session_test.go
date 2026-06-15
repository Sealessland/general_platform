package memory

import (
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"testing"
	"time"
)

func TestRepositoryUserSessionAndMerchantFlow(t *testing.T) {
	repo := NewRepository()
	now := time.Now().UTC()

	user, err := repo.CreateUser(domain.User{
		Nickname:     "Repository User",
		Phone:        "13920000001",
		PasswordHash: "hashed",
		Role:         domain.RoleMerchant,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := repo.CreateUser(domain.User{Phone: "13920000001"}); err == nil {
		t.Fatal("expected duplicate phone error")
	}
	if fetched, ok := repo.FindUserByPhone(user.Phone); !ok || fetched.ID != user.ID {
		t.Fatalf("expected user by phone, got %+v ok=%v", fetched, ok)
	}
	repo.SaveSession("repo-token", "repo-refresh", user.ID)
	if fetched, ok := repo.GetUserByToken("repo-token"); !ok || fetched.ID != user.ID {
		t.Fatalf("expected user by token, got %+v ok=%v", fetched, ok)
	}

	merchant, err := repo.CreateMerchant(domain.Merchant{
		UserID:      user.ID,
		Name:        "Repository Shop",
		Description: "test shop",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	if fetched, ok := repo.GetMerchantByUserID(user.ID); !ok || fetched.ID != merchant.ID {
		t.Fatalf("expected merchant by user id, got %+v ok=%v", fetched, ok)
	}
}
