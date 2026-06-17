package redis

import (
	"fmt"
	"testing"
	"time"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	postgresrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres"
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

func newCatalogCacheRepo(t *testing.T, base application.Repository, client goredis.UniversalClient) *CatalogCacheRepository {
	t.Helper()
	return NewCatalogCacheRepository(base, client, time.Hour)
}

func TestCatalogCacheRepositoryUsesLocalCacheAfterFirstRead(t *testing.T) {
	fixture := newRedisPostgresFixture(t)
	product, sku := createRedisCatalogFixture(t, fixture.repo, 12)
	base := &countingRepo{Repository: fixture.repo}
	repo := newCatalogCacheRepo(t, base, fixture.client)

	if _, ok := repo.GetProduct(product.ID); !ok {
		t.Fatal("expected postgres product")
	}
	if _, ok := repo.GetProduct(product.ID); !ok {
		t.Fatal("expected cached product")
	}
	if base.productReads != 1 {
		t.Fatalf("expected one product read, got %d", base.productReads)
	}

	if _, ok := repo.GetSKU(sku.ID); !ok {
		t.Fatal("expected postgres sku")
	}
	if _, ok := repo.GetSKU(sku.ID); !ok {
		t.Fatal("expected cached sku")
	}
	if base.skuReads != 1 {
		t.Fatalf("expected one sku read, got %d", base.skuReads)
	}

	if skus := repo.ListSKUsByProduct(product.ID); len(skus) == 0 {
		t.Fatal("expected postgres sku list")
	}
	if skus := repo.ListSKUsByProduct(product.ID); len(skus) == 0 {
		t.Fatal("expected cached sku list")
	}
	if base.skuListReads != 1 {
		t.Fatalf("expected one sku list read, got %d", base.skuListReads)
	}
}

func TestCatalogCacheRepositoryReadsFromRedisAcrossInstances(t *testing.T) {
	fixture := newRedisPostgresFixture(t)
	product, _ := createRedisCatalogFixture(t, fixture.repo, 12)

	baseOne := &countingRepo{Repository: fixture.repo}
	repoOne := newCatalogCacheRepo(t, baseOne, fixture.client)
	loadedByFirst, ok := repoOne.GetProduct(product.ID)
	if !ok {
		t.Fatal("expected postgres product")
	}

	baseTwo := &countingRepo{Repository: fixture.repo}
	repoTwo := newCatalogCacheRepo(t, baseTwo, fixture.client)
	loadedBySecond, ok := repoTwo.GetProduct(product.ID)
	if !ok {
		t.Fatal("expected redis-backed product")
	}
	if loadedBySecond.ID != loadedByFirst.ID {
		t.Fatalf("expected product %d, got %d", loadedByFirst.ID, loadedBySecond.ID)
	}
	if baseTwo.productReads != 0 {
		t.Fatalf("expected zero postgres product reads, got %d", baseTwo.productReads)
	}
}

func TestCatalogCacheRepositoryInvalidatesSKUListOnSaveSKU(t *testing.T) {
	fixture := newRedisPostgresFixture(t)
	product, _ := createRedisCatalogFixture(t, fixture.repo, 12)
	base := &countingRepo{Repository: fixture.repo}
	repo := newCatalogCacheRepo(t, base, fixture.client)

	skus := repo.ListSKUsByProduct(product.ID)
	if len(skus) == 0 {
		t.Fatal("expected postgres skus")
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
	fixture := newRedisPostgresFixture(t)
	product, sku := createRedisCatalogFixture(t, fixture.repo, 12)
	base := &countingRepo{Repository: fixture.repo}
	repo := newCatalogCacheRepo(t, base, fixture.client)

	if _, ok := repo.GetSKU(sku.ID); !ok {
		t.Fatal("expected postgres sku")
	}
	now := time.Now().UTC()
	order, err := repo.SaveOrderWithInventoryLocks(domain.Order{
		UserID:             1,
		MerchantID:         product.MerchantID,
		IdempotencyKey:     fmt.Sprintf("cache-invalidate-order-%d", sku.ID),
		OrderNo:            fmt.Sprintf("CACHE-ORDER-%d", sku.ID),
		Status:             "CREATED",
		TotalAmountCent:    sku.PriceCent,
		PayAmountCent:      sku.PriceCent,
		DiscountAmountCent: 0,
		ReceiverName:       "Alice",
		ReceiverPhone:      "13800000001",
		ReceiverAddress:    "Shanghai",
		CreatedAt:          now,
		UpdatedAt:          now,
		Items: []domain.OrderItem{{
			ProductID:            product.ID,
			SKUID:                sku.ID,
			ProductTitleSnapshot: product.Title,
			SKUNameSnapshot:      sku.SKUName,
			PriceCentSnapshot:    sku.PriceCent,
			Quantity:             1,
			TotalAmountCent:      sku.PriceCent,
			CreatedAt:            now,
			UpdatedAt:            now,
		}},
	}, []domain.InventoryLock{{
		SKUID:     sku.ID,
		Quantity:  1,
		Status:    domain.InventoryLockStatusLocked,
		LockedAt:  now,
		CreatedAt: now,
		UpdatedAt: now,
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

func createRedisCatalogFixture(t *testing.T, repo *postgresrepo.Repository, stock int) (domain.Product, domain.SKU) {
	t.Helper()
	now := time.Now().UTC()
	product, err := repo.SaveProduct(domain.Product{
		MerchantID:    1,
		Title:         fmt.Sprintf("Redis Catalog Product %d", now.UnixNano()),
		Description:   "redis catalog cache integration fixture",
		CategoryID:    20260617,
		Status:        domain.ProductStatusOnline,
		SellingPoints: []string{"redis", "postgres"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("save product: %v", err)
	}
	sku, err := repo.SaveSKU(domain.SKU{
		ProductID:   product.ID,
		SKUName:     fmt.Sprintf("Redis Catalog SKU %d", now.UnixNano()),
		SKUAttrs:    map[string]string{"size": "standard"},
		PriceCent:   12345,
		Stock:       stock,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("save sku: %v", err)
	}
	return product, sku
}
