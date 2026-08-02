package postgres

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func TestConcurrentRepositoryStartup(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" || os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("POSTGRES_DSN not set")
	}
	start := make(chan struct{})
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			repo, err := NewRepository(dsn)
			if err == nil {
				err = repo.Close()
			}
			errors <- err
		}()
	}
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent repository startup: %v", err)
		}
	}
}

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

	products := repo.ListProducts(0, 0)
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
