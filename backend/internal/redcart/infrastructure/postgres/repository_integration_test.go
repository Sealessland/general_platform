package postgres

import (
	"os"
	"testing"

	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func TestRepositoryAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set")
	}

	repo, err := NewRepository(dsn)
	if err != nil {
		t.Fatalf("new postgres repository: %v", err)
	}
	defer repo.Close()

	products := repo.ListProducts()
	if len(products) == 0 {
		t.Fatal("expected seeded products")
	}

	user, err := repo.CreateUser(domain.User{
		Nickname:     "PG User",
		Phone:        "13800100001",
		PasswordHash: "hashed",
		Role:         domain.RoleConsumer,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if fetched, ok := repo.FindUserByPhone(user.Phone); !ok || fetched.ID != user.ID {
		t.Fatal("expected user by phone")
	}

	repo.SaveSession("pg-token", user.ID)
	if fetched, ok := repo.GetUserByToken("pg-token"); !ok || fetched.ID != user.ID {
		t.Fatal("expected user by token")
	}

	note, ok := repo.GetNote(1)
	if !ok || len(note.ProductIDs) == 0 {
		t.Fatal("expected seeded note with product ids")
	}

	product, ok := repo.GetProduct(1)
	if !ok || len(product.SellingPoints) == 0 {
		t.Fatal("expected seeded product")
	}

	skus := repo.ListSKUsByProduct(product.ID)
	if len(skus) == 0 {
		t.Fatal("expected seeded skus")
	}
}
