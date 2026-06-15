package postgres

import (
	"context"
	"database/sql"
	"fmt"
	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"os"
	"testing"
	"time"
)

func skipIfNoPostgres(t *testing.T) (string, bool) {
	t.Helper()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" || os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("POSTGRES_DSN not set or RUN_POSTGRES_INTEGRATION != 1")
	}
	return dsn, true
}

func newPostgresRepo(t *testing.T) *Repository {
	t.Helper()
	dsn, _ := skipIfNoPostgres(t)
	repo, err := NewRepository(dsn)
	if err != nil {
		t.Fatalf("new postgres repository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func newPostgresService(t *testing.T) (*Repository, *application.Service) {
	t.Helper()
	repo := newPostgresRepo(t)
	return repo, application.NewService(repo, backendai.MockProvider{})
}

func openRawConn(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open raw postgres conn: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping raw postgres conn: %v", err)
	}
	return db
}

func createStabilityProductAndSKU(t *testing.T, repo *Repository, stock int) domain.SKU {
	t.Helper()
	now := time.Now().UTC()
	product, err := repo.SaveProduct(domain.Product{
		MerchantID:    1,
		Title:         fmt.Sprintf("Stability Product %d", now.UnixNano()),
		Description:   "created for stability test",
		CategoryID:    999,
		Status:        domain.ProductStatusOnline,
		SellingPoints: []string{"stability"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	sku, err := repo.SaveSKU(domain.SKU{
		ProductID:   product.ID,
		SKUName:     fmt.Sprintf("Stability SKU %d", now.UnixNano()),
		SKUAttrs:    map[string]string{"batch": fmt.Sprintf("%d", now.UnixNano())},
		PriceCent:   100,
		Stock:       stock,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create sku: %v", err)
	}
	return sku
}

func createStabilityOrder(t *testing.T, service *application.Service, sku domain.SKU, quantity int) *application.OrderView {
	t.Helper()
	view, err := service.CreateOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, fmt.Sprintf("stability-%d-%d", sku.ID, time.Now().UnixNano()), application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: sku.ID, Quantity: quantity}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	return view
}

// TestReadCommittedNoDirtyRead verifies that uncommitted writes are invisible
// to other transactions under PostgreSQL default READ COMMITTED isolation.
