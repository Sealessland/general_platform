package postgres

import (
	"context"
	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"testing"
)

func TestPayOrderInventoryFailureRollsBackStatus(t *testing.T) {
	dsn, _ := skipIfNoPostgres(t)
	db := openRawConn(t, dsn)
	repo, service := newPostgresService(t)
	sku := createStabilityProductAndSKU(t, repo, 10)
	order := createStabilityOrder(t, service, sku, 1)

	// Simulate external corruption: the lock row still claims quantity=1, but the
	// SKU locked_stock has been cleared to 0. PayOrder will try to decrement
	// locked_stock and detect an underflow, causing the side effect to fail.
	_, err := db.Exec(`UPDATE product_skus SET locked_stock = 0 WHERE id = $1`, sku.ID)
	if err != nil {
		t.Fatalf("corrupt sku: %v", err)
	}

	_, err = service.PayOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, order.ID)
	if err == nil {
		t.Fatal("expected PayOrder to fail when inventory underflows")
	}

	current, ok := repo.GetOrder(order.ID)
	if !ok {
		t.Fatal("expected order to still exist")
	}
	if current.Status != orderdomain.StatusCreated {
		t.Fatalf("expected order status to remain CREATED after rollback, got %s", current.Status)
	}

	updated, ok := repo.GetSKU(sku.ID)
	if !ok {
		t.Fatal("expected sku to exist")
	}
	if updated.Stock != 10 || updated.LockedStock != 0 {
		t.Fatalf("expected sku unchanged after rollback, got stock=%d locked_stock=%d", updated.Stock, updated.LockedStock)
	}
}
