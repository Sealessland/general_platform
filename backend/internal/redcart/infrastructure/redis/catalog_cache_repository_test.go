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

// countingRepo 包装底层仓库并统计各类读取次数，用于断言缓存确实命中而非回源。
type countingRepo struct {
	application.Repository
	productReads int
	skuReads     int
	skuListReads int
}

// GetProduct 统计商品读取次数后透传到底层仓库，供断言缓存命中而非回源使用。
func (r *countingRepo) GetProduct(id int64) (domain.Product, bool) {
	r.productReads++
	return r.Repository.GetProduct(id)
}

// GetSKU 统计 SKU 读取次数后透传到底层仓库，供断言缓存命中而非回源使用。
func (r *countingRepo) GetSKU(id int64) (domain.SKU, bool) {
	r.skuReads++
	return r.Repository.GetSKU(id)
}

// ListSKUsByProduct 统计 SKU 列表读取次数后透传到底层仓库，供断言缓存命中而非回源使用。
func (r *countingRepo) ListSKUsByProduct(productID int64) []domain.SKU {
	r.skuListReads++
	return r.Repository.ListSKUsByProduct(productID)
}

// newCatalogCacheRepo 构造 TTL 为 1 小时的缓存仓库，测试用。
func newCatalogCacheRepo(t *testing.T, base application.Repository, client goredis.UniversalClient) *CatalogCacheRepository {
	t.Helper()
	return NewCatalogCacheRepository(base, client, time.Hour)
}

// TestCatalogCacheRepositoryUsesLocalCacheAfterFirstRead 验证首次回源后，
// 二次读取命中本地缓存（底层仓库读取次数保持为 1）。
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

// TestCatalogCacheRepositoryReadsFromRedisAcrossInstances 验证两个仓库实例
// 共享同一 Redis 时，第二个实例能命中第一个实例写入的缓存（零回源）。
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

// TestCatalogCacheRepositoryInvalidatesSKUListOnSaveSKU 验证保存 SKU 后
// 该商品 SKU 列表缓存被失效，再次读取会重新回源。
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

// TestCatalogCacheRepositoryInvalidatesOrderSKUsAfterSaveOrderWithInventoryLocks
// 验证下单（携带库存锁）后相关 SKU 缓存被失效，重新读取可见锁定库存。
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

// createRedisCatalogFixture 创建商品与 SKU 的集成测试夹具（标题带时间戳避免冲突）。
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
