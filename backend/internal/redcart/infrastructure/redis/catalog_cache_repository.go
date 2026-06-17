package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/example/redcart-copilot/backend/internal/event"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	goredis "github.com/redis/go-redis/v9"
)

const (
	productKeyPrefix     = "redcart:cache:product:"
	skuKeyPrefix         = "redcart:cache:sku:"
	productSKUsKeyPrefix = "redcart:cache:product_skus:"
)

type CatalogCacheRepository struct {
	application.Repository
	client goredis.UniversalClient
	ttl    time.Duration

	productMu    sync.RWMutex
	productCache map[int64]productCacheEntry

	skuMu    sync.RWMutex
	skuCache map[int64]skuCacheEntry

	listMu    sync.RWMutex
	listCache map[int64]skuListCacheEntry
}

type productCacheEntry struct {
	value     domain.Product
	expiresAt time.Time
}

type skuCacheEntry struct {
	value     domain.SKU
	expiresAt time.Time
}

type skuListCacheEntry struct {
	value     []domain.SKU
	expiresAt time.Time
}

func NewCatalogCacheRepository(base application.Repository, client goredis.UniversalClient, ttl time.Duration) *CatalogCacheRepository {
	if ttl <= 0 {
		ttl = defaultCatalogTTL
	}
	return &CatalogCacheRepository{
		Repository:   base,
		client:       client,
		ttl:          ttl,
		productCache: make(map[int64]productCacheEntry),
		skuCache:     make(map[int64]skuCacheEntry),
		listCache:    make(map[int64]skuListCacheEntry),
	}
}

func (r *CatalogCacheRepository) GetProduct(id int64) (domain.Product, bool) {
	if product, ok := r.loadProductCache(id); ok {
		return product, true
	}
	if product, ok := r.loadProductRedis(id); ok {
		r.saveProductCache(product)
		return product, true
	}
	product, ok := r.Repository.GetProduct(id)
	if ok {
		r.saveProductCache(product)
		r.saveProductRedis(product)
	}
	return product, ok
}

func (r *CatalogCacheRepository) SaveProduct(product domain.Product) (domain.Product, error) {
	saved, err := r.Repository.SaveProduct(product)
	if err != nil {
		return domain.Product{}, err
	}
	r.saveProductCache(saved)
	r.saveProductRedis(saved)
	return saved, nil
}

func (r *CatalogCacheRepository) GetSKU(id int64) (domain.SKU, bool) {
	if sku, ok := r.loadSKUCache(id); ok {
		return sku, true
	}
	if sku, ok := r.loadSKURedis(id); ok {
		r.saveSKUCache(sku)
		return sku, true
	}
	sku, ok := r.Repository.GetSKU(id)
	if ok {
		r.saveSKUCache(sku)
		r.saveSKURedis(sku)
	}
	return sku, ok
}

func (r *CatalogCacheRepository) SaveSKU(sku domain.SKU) (domain.SKU, error) {
	saved, err := r.Repository.SaveSKU(sku)
	if err != nil {
		return domain.SKU{}, err
	}
	r.saveSKUCache(saved)
	r.saveSKURedis(saved)
	r.invalidateSKUList(saved.ProductID)
	return saved, nil
}

func (r *CatalogCacheRepository) Append(ctx context.Context, evt event.Event) (int64, error) {
	if outbox, ok := r.Repository.(event.Outbox); ok {
		return outbox.Append(ctx, evt)
	}
	return 0, nil
}

func (r *CatalogCacheRepository) SaveOrderWithInventoryLocks(order domain.Order, locks []domain.InventoryLock) (domain.Order, error) {
	saved, err := r.Repository.SaveOrderWithInventoryLocks(order, locks)
	if err != nil {
		return domain.Order{}, err
	}
	r.invalidateOrderItems(order.Items)
	return saved, nil
}

func (r *CatalogCacheRepository) ListSKUsByProduct(productID int64) []domain.SKU {
	if skus, ok := r.loadSKUListCache(productID); ok {
		return skus
	}
	if skus, ok := r.loadSKUListRedis(productID); ok {
		r.saveSKUListCache(productID, skus)
		return skus
	}
	skus := r.Repository.ListSKUsByProduct(productID)
	r.saveSKUListCache(productID, skus)
	r.saveSKUListRedis(productID, skus)
	for _, sku := range skus {
		r.saveSKUCache(sku)
	}
	return cloneSKUs(skus)
}

func (r *CatalogCacheRepository) loadProductRedis(id int64) (domain.Product, bool) {
	if r.client == nil {
		return domain.Product{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultReadTimeout)
	defer cancel()
	payload, err := r.client.Get(ctx, productKey(id)).Bytes()
	if err != nil {
		return domain.Product{}, false
	}
	var product domain.Product
	if err := json.Unmarshal(payload, &product); err != nil {
		return domain.Product{}, false
	}
	return cloneProduct(product), true
}

func (r *CatalogCacheRepository) saveProductRedis(product domain.Product) {
	if r.client == nil {
		return
	}
	payload, err := json.Marshal(product)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultWriteTimeout)
	defer cancel()
	_ = r.client.Set(ctx, productKey(product.ID), payload, r.ttl).Err()
}

func (r *CatalogCacheRepository) loadSKURedis(id int64) (domain.SKU, bool) {
	if r.client == nil {
		return domain.SKU{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultReadTimeout)
	defer cancel()
	payload, err := r.client.Get(ctx, skuKey(id)).Bytes()
	if err != nil {
		return domain.SKU{}, false
	}
	var sku domain.SKU
	if err := json.Unmarshal(payload, &sku); err != nil {
		return domain.SKU{}, false
	}
	return cloneSKU(sku), true
}

func (r *CatalogCacheRepository) saveSKURedis(sku domain.SKU) {
	if r.client == nil {
		return
	}
	payload, err := json.Marshal(sku)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultWriteTimeout)
	defer cancel()
	_ = r.client.Set(ctx, skuKey(sku.ID), payload, r.ttl).Err()
}

func (r *CatalogCacheRepository) loadSKUListRedis(productID int64) ([]domain.SKU, bool) {
	if r.client == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultReadTimeout)
	defer cancel()
	payload, err := r.client.Get(ctx, productSKUsKey(productID)).Bytes()
	if err != nil {
		return nil, false
	}
	var skus []domain.SKU
	if err := json.Unmarshal(payload, &skus); err != nil {
		return nil, false
	}
	return cloneSKUs(skus), true
}

func (r *CatalogCacheRepository) saveSKUListRedis(productID int64, skus []domain.SKU) {
	if r.client == nil {
		return
	}
	payload, err := json.Marshal(skus)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultWriteTimeout)
	defer cancel()
	_ = r.client.Set(ctx, productSKUsKey(productID), payload, r.ttl).Err()
}

func (r *CatalogCacheRepository) invalidateSKUList(productID int64) {
	r.listMu.Lock()
	delete(r.listCache, productID)
	r.listMu.Unlock()
	if r.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), defaultWriteTimeout)
		defer cancel()
		_ = r.client.Del(ctx, productSKUsKey(productID)).Err()
	}
}

func (r *CatalogCacheRepository) invalidateOrderItems(items []domain.OrderItem) {
	for _, item := range items {
		r.invalidateSKU(item.SKUID)
		r.invalidateSKUList(item.ProductID)
	}
}

func (r *CatalogCacheRepository) loadProductCache(id int64) (domain.Product, bool) {
	r.productMu.RLock()
	entry, ok := r.productCache[id]
	r.productMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return domain.Product{}, false
	}
	return cloneProduct(entry.value), true
}

func (r *CatalogCacheRepository) saveProductCache(product domain.Product) {
	r.productMu.Lock()
	r.productCache[product.ID] = productCacheEntry{value: cloneProduct(product), expiresAt: time.Now().Add(r.ttl)}
	r.productMu.Unlock()
}

func (r *CatalogCacheRepository) loadSKUCache(id int64) (domain.SKU, bool) {
	r.skuMu.RLock()
	entry, ok := r.skuCache[id]
	r.skuMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return domain.SKU{}, false
	}
	return cloneSKU(entry.value), true
}

func (r *CatalogCacheRepository) saveSKUCache(sku domain.SKU) {
	r.skuMu.Lock()
	r.skuCache[sku.ID] = skuCacheEntry{value: cloneSKU(sku), expiresAt: time.Now().Add(r.ttl)}
	r.skuMu.Unlock()
}

func (r *CatalogCacheRepository) invalidateSKU(id int64) {
	r.skuMu.Lock()
	delete(r.skuCache, id)
	r.skuMu.Unlock()
	if r.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), defaultWriteTimeout)
		defer cancel()
		_ = r.client.Del(ctx, skuKey(id)).Err()
	}
}

func (r *CatalogCacheRepository) loadSKUListCache(productID int64) ([]domain.SKU, bool) {
	r.listMu.RLock()
	entry, ok := r.listCache[productID]
	r.listMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return cloneSKUs(entry.value), true
}

func (r *CatalogCacheRepository) saveSKUListCache(productID int64, skus []domain.SKU) {
	r.listMu.Lock()
	r.listCache[productID] = skuListCacheEntry{value: cloneSKUs(skus), expiresAt: time.Now().Add(r.ttl)}
	r.listMu.Unlock()
}

func productKey(id int64) string {
	return fmt.Sprintf("%s%d", productKeyPrefix, id)
}

func skuKey(id int64) string {
	return fmt.Sprintf("%s%d", skuKeyPrefix, id)
}

func productSKUsKey(productID int64) string {
	return fmt.Sprintf("%s%d", productSKUsKeyPrefix, productID)
}

func cloneProduct(product domain.Product) domain.Product {
	product.SellingPoints = domain.CloneStringSlice(product.SellingPoints)
	return product
}

func cloneSKU(sku domain.SKU) domain.SKU {
	sku.SKUAttrs = domain.CloneMap(sku.SKUAttrs)
	return sku
}

func cloneSKUs(skus []domain.SKU) []domain.SKU {
	out := make([]domain.SKU, 0, len(skus))
	for _, sku := range skus {
		out = append(out, cloneSKU(sku))
	}
	return out
}
