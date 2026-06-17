package application_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
	"golang.org/x/crypto/bcrypt"
)

// latencyRepo wraps a Repository and injects a fixed delay after every order
// save to simulate synchronous downstream side effects (notification,
// analytics, search index) happening inside the request path.
type latencyRepo struct {
	application.Repository
	latency time.Duration
}

func (r *latencyRepo) SaveOrderWithInventoryLocks(order domain.Order, locks []domain.InventoryLock) (domain.Order, error) {
	order, err := r.Repository.SaveOrderWithInventoryLocks(order, locks)
	if err != nil {
		return order, err
	}
	// Simulate three synchronous downstream calls (notification + analytics +
	// search index) that block the response.
	time.Sleep(r.latency * 3)
	return order, nil
}

func seededPasswordHash(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(fmt.Sprintf("seed password hash: %v", err))
	}
	return string(hash)
}

func setupOrderBenchmark(b *testing.B) (*application.Service, application.Actor, int64) {
	b.Helper()
	base := memory.NewRepository()

	merchantUser, _ := base.CreateUser(domain.User{
		Nickname:     "Merchant Bench",
		Phone:        "13900000001",
		PasswordHash: seededPasswordHash("merchant-pass"),
		Role:         domain.RoleMerchant,
	})
	consumerUser, _ := base.CreateUser(domain.User{
		Nickname:     "Consumer Bench",
		Phone:        "13900000002",
		PasswordHash: seededPasswordHash("consumer-pass"),
		Role:         domain.RoleConsumer,
	})

	merchant, _ := base.CreateMerchant(domain.Merchant{
		UserID: merchantUser.ID,
		Name:   "Bench Merchant",
		Status: "active",
	})

	product, _ := base.SaveProduct(domain.Product{
		MerchantID: merchant.ID,
		Title:      "Bench Product",
		CategoryID: 1,
		Status:     domain.ProductStatusOnline,
	})
	sku, _ := base.SaveSKU(domain.SKU{
		ProductID: product.ID,
		SKUName:   "Bench SKU",
		SKUAttrs:  map[string]string{"size": "M"},
		PriceCent: 10000,
		Stock:     b.N + 100,
		Status:    domain.SKUStatusActive,
	})

	svc := application.NewService(base, backendai.MockProvider{})
	consumer := application.Actor{UserID: consumerUser.ID, Role: domain.RoleConsumer}
	return svc, consumer, sku.ID
}

// BenchmarkCreateOrderOutbox measures order creation when downstream work is
// deferred through the transactional outbox. The request path only pays for
// the outbox Append call.
func BenchmarkCreateOrderOutbox(b *testing.B) {
	svc, consumer, skuID := setupOrderBenchmark(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := svc.CreateOrder(ctx, consumer, benchKey(i), application.CheckoutInput{
			Items: []application.OrderLineInput{
				{SKUID: skuID, Quantity: 1},
			},
			ReceiverName:    "Bench",
			ReceiverPhone:   "13900000003",
			ReceiverAddress: "Shanghai",
		})
		if err != nil {
			b.Fatalf("create order: %v", err)
		}
	}
}

// BenchmarkCreateOrderSyncSideEffects measures the same order creation flow
// when the request path blocks on simulated downstream side effects. The
// latency is chosen to model three light downstream RPCs (~0.5ms each).
func BenchmarkCreateOrderSyncSideEffects(b *testing.B) {
	base := memory.NewRepository()
	repo := &latencyRepo{Repository: base, latency: 500 * time.Microsecond}
	svc := application.NewService(repo, backendai.MockProvider{})

	merchantUser, _ := base.CreateUser(domain.User{
		Nickname:     "Merchant Bench",
		Phone:        "13900000001",
		PasswordHash: seededPasswordHash("merchant-pass"),
		Role:         domain.RoleMerchant,
	})
	consumerUser, _ := base.CreateUser(domain.User{
		Nickname:     "Consumer Bench",
		Phone:        "13900000002",
		PasswordHash: seededPasswordHash("consumer-pass"),
		Role:         domain.RoleConsumer,
	})
	merchant, _ := base.CreateMerchant(domain.Merchant{
		UserID: merchantUser.ID,
		Name:   "Bench Merchant",
		Status: "active",
	})
	product, _ := base.SaveProduct(domain.Product{
		MerchantID: merchant.ID,
		Title:      "Bench Product",
		CategoryID: 1,
		Status:     domain.ProductStatusOnline,
	})
	sku, _ := base.SaveSKU(domain.SKU{
		ProductID: product.ID,
		SKUName:   "Bench SKU",
		SKUAttrs:  map[string]string{"size": "M"},
		PriceCent: 10000,
		Stock:     b.N + 100,
		Status:    domain.SKUStatusActive,
	})
	consumer := application.Actor{UserID: consumerUser.ID, Role: domain.RoleConsumer}

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := svc.CreateOrder(ctx, consumer, benchKey(i), application.CheckoutInput{
			Items: []application.OrderLineInput{
				{SKUID: sku.ID, Quantity: 1},
			},
			ReceiverName:    "Bench",
			ReceiverPhone:   "13900000003",
			ReceiverAddress: "Shanghai",
		})
		if err != nil {
			b.Fatalf("create order: %v", err)
		}
	}
}

func benchKey(i int) string {
	return fmt.Sprintf("bench-order-%d", i)
}
