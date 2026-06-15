package postgres

import (
	"context"
	"fmt"
	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentCreateOrderReservesStockAtomically(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" || os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("POSTGRES_DSN not set")
	}

	repo, err := NewRepository(dsn)
	if err != nil {
		t.Fatalf("new postgres repository: %v", err)
	}
	defer repo.Close()
	service := application.NewService(repo, backendai.MockProvider{})

	now := time.Now().UTC()
	product, err := repo.SaveProduct(domain.Product{
		MerchantID:    1,
		Title:         fmt.Sprintf("Atomic Stock Product %d", now.UnixNano()),
		Description:   "created for concurrent stock test",
		CategoryID:    999,
		Status:        domain.ProductStatusOnline,
		SellingPoints: []string{"stock safe"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	sku, err := repo.SaveSKU(domain.SKU{
		ProductID:   product.ID,
		SKUName:     "Only One",
		SKUAttrs:    map[string]string{"stock": "one"},
		PriceCent:   9900,
		Stock:       1,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create sku: %v", err)
	}

	const workers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	var created atomic.Int64
	var conflicts atomic.Int64
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := service.CreateOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, fmt.Sprintf("atomic-stock-%d-%d", now.UnixNano(), i), application.CheckoutInput{
				Items:           []application.OrderLineInput{{SKUID: sku.ID, Quantity: 1}},
				ReceiverName:    "Alice",
				ReceiverPhone:   "13800000001",
				ReceiverAddress: "Shanghai",
			})
			if err == nil {
				created.Add(1)
				return
			}
			if appErr, ok := err.(*application.AppError); ok && appErr.Kind == application.ErrorConflict {
				conflicts.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if created.Load() != 1 {
		t.Fatalf("expected exactly one order created for stock=1, got %d conflicts=%d", created.Load(), conflicts.Load())
	}
	updated, ok := repo.GetSKU(sku.ID)
	if !ok {
		t.Fatal("expected sku after concurrent order attempts")
	}
	if updated.Stock != 1 || updated.LockedStock != 1 {
		t.Fatalf("expected stock=1 locked_stock=1 after reservation, got stock=%d locked_stock=%d", updated.Stock, updated.LockedStock)
	}
}
