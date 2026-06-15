package postgres

import (
	"fmt"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"os"
	"testing"
	"time"
)

func TestRepositoryAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" || os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
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

	phone := fmt.Sprintf("138%08d", time.Now().UnixNano()%100000000)
	user, err := repo.CreateUser(domain.User{
		Nickname:     "PG User",
		Phone:        phone,
		PasswordHash: "hashed",
		Role:         domain.RoleConsumer,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if fetched, ok := repo.FindUserByPhone(user.Phone); !ok || fetched.ID != user.ID {
		t.Fatal("expected user by phone")
	}

	repo.SaveSession("pg-token", "pg-refresh", user.ID)
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
