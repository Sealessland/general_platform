package redis

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
	goredis "github.com/redis/go-redis/v9"
)

type countingRepo struct {
	application.Repository
	productReads int
	skuReads     int
	skuListReads int
}

func (r *countingRepo) GetProduct(id int64) (domain.Product, bool) {
	r.productReads++
	return r.Repository.GetProduct(id)
}

func (r *countingRepo) GetSKU(id int64) (domain.SKU, bool) {
	r.skuReads++
	return r.Repository.GetSKU(id)
}

func (r *countingRepo) ListSKUsByProduct(productID int64) []domain.SKU {
	r.skuListReads++
	return r.Repository.ListSKUsByProduct(productID)
}

func TestCatalogCacheRepositoryUsesLocalCacheAfterFirstRead(t *testing.T) {
	base := &countingRepo{Repository: memory.NewRepository()}
	repo := NewCatalogCacheRepository(base, nil, time.Hour)

	if _, ok := repo.GetProduct(1); !ok {
		t.Fatal("expected seeded product")
	}
	if _, ok := repo.GetProduct(1); !ok {
		t.Fatal("expected cached product")
	}
	if base.productReads != 1 {
		t.Fatalf("expected one product read, got %d", base.productReads)
	}

	if _, ok := repo.GetSKU(1); !ok {
		t.Fatal("expected seeded sku")
	}
	if _, ok := repo.GetSKU(1); !ok {
		t.Fatal("expected cached sku")
	}
	if base.skuReads != 1 {
		t.Fatalf("expected one sku read, got %d", base.skuReads)
	}

	if skus := repo.ListSKUsByProduct(1); len(skus) == 0 {
		t.Fatal("expected seeded skus")
	}
	if skus := repo.ListSKUsByProduct(1); len(skus) == 0 {
		t.Fatal("expected cached sku list")
	}
	if base.skuListReads != 1 {
		t.Fatalf("expected one sku list read, got %d", base.skuListReads)
	}
}

func TestCatalogCacheRepositoryReadsFromRedisAcrossInstances(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	baseOne := &countingRepo{Repository: memory.NewRepository()}
	repoOne := NewCatalogCacheRepository(baseOne, client, time.Hour)
	product, ok := repoOne.GetProduct(1)
	if !ok {
		t.Fatal("expected seeded product")
	}

	baseTwo := &countingRepo{Repository: memory.NewRepository()}
	repoTwo := NewCatalogCacheRepository(baseTwo, client, time.Hour)
	loaded, ok := repoTwo.GetProduct(product.ID)
	if !ok {
		t.Fatal("expected redis-backed product")
	}
	if loaded.ID != product.ID {
		t.Fatalf("expected product %d, got %d", product.ID, loaded.ID)
	}
	if baseTwo.productReads != 0 {
		t.Fatalf("expected zero DB product reads, got %d", baseTwo.productReads)
	}
}

func TestCatalogCacheRepositoryInvalidatesSKUListOnSaveSKU(t *testing.T) {
	base := &countingRepo{Repository: memory.NewRepository()}
	repo := NewCatalogCacheRepository(base, nil, time.Hour)

	skus := repo.ListSKUsByProduct(1)
	if len(skus) == 0 {
		t.Fatal("expected seeded skus")
	}
	updated := skus[0]
	updated.Stock++
	if _, err := repo.SaveSKU(updated); err != nil {
		t.Fatalf("save sku: %v", err)
	}
	_ = repo.ListSKUsByProduct(updated.ProductID)
	if base.skuListReads != 2 {
		t.Fatalf("expected second sku list read after invalidation, got %d", base.skuListReads)
	}
}

func TestCatalogCacheRepositoryInvalidatesOrderSKUsAfterSaveOrderWithInventoryLocks(t *testing.T) {
	base := &countingRepo{Repository: memory.NewRepository()}
	repo := NewCatalogCacheRepository(base, nil, time.Hour)

	sku, ok := repo.GetSKU(1)
	if !ok {
		t.Fatal("expected seeded sku")
	}
	order, err := repo.SaveOrderWithInventoryLocks(domain.Order{
		UserID:             1,
		MerchantID:         1,
		IdempotencyKey:     "cache-invalidate-order",
		OrderNo:            "CACHE-ORDER-1",
		Status:             "CREATED",
		TotalAmountCent:    sku.PriceCent,
		PayAmountCent:      sku.PriceCent,
		DiscountAmountCent: 0,
		ReceiverName:       "Alice",
		ReceiverPhone:      "13800000001",
		ReceiverAddress:    "Shanghai",
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
		Items: []domain.OrderItem{{
			ProductID:            sku.ProductID,
			SKUID:                sku.ID,
			ProductTitleSnapshot: "seed",
			SKUNameSnapshot:      sku.SKUName,
			PriceCentSnapshot:    sku.PriceCent,
			Quantity:             1,
			TotalAmountCent:      sku.PriceCent,
			CreatedAt:            time.Now().UTC(),
			UpdatedAt:            time.Now().UTC(),
		}},
	}, []domain.InventoryLock{{
		SKUID:     sku.ID,
		Quantity:  1,
		Status:    domain.InventoryLockStatusLocked,
		LockedAt:  time.Now().UTC(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}})
	if err != nil {
		t.Fatalf("save order with locks: %v", err)
	}
	if order.ID == 0 {
		t.Fatal("expected saved order id")
	}

	loaded, ok := repo.GetSKU(sku.ID)
	if !ok {
		t.Fatal("expected sku after order")
	}
	if loaded.LockedStock != sku.LockedStock+1 {
		t.Fatalf("expected locked stock %d, got %d", sku.LockedStock+1, loaded.LockedStock)
	}
}
