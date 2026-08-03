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

// 商品目录缓存的 Redis 键前缀，统一挂在 redcart:cache: 命名空间下。
const (
	productKeyPrefix     = "redcart:cache:product:"
	skuKeyPrefix         = "redcart:cache:sku:"
	productSKUsKeyPrefix = "redcart:cache:product_skus:"
)

// CatalogCacheRepository 商品目录缓存仓库：以装饰器方式包装底层
// application.Repository，叠加进程内缓存与 Redis 两级缓存；
// 读取按"本地缓存 → Redis → 底层仓库"逐级回退并回填，写入后同步失效。
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

// productCacheEntry 商品本地缓存条目：缓存值与过期时间。
type productCacheEntry struct {
	value     domain.Product
	expiresAt time.Time
}

// skuCacheEntry SKU 本地缓存条目。
type skuCacheEntry struct {
	value     domain.SKU
	expiresAt time.Time
}

// skuListCacheEntry 商品下 SKU 列表的本地缓存条目。
type skuListCacheEntry struct {
	value     []domain.SKU
	expiresAt time.Time
}

// NewCatalogCacheRepository 构造缓存仓库；ttl<=0 时回退到默认目录缓存 TTL。
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

// GetProduct 按三级缓存读取商品：本地缓存命中直接返回；
// Redis 命中回填本地；都未命中则回源底层仓库并回填两级缓存。
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

// SaveProduct 保存商品到底层仓库后回填两级缓存。
func (r *CatalogCacheRepository) SaveProduct(product domain.Product) (domain.Product, error) {
	saved, err := r.Repository.SaveProduct(product)
	if err != nil {
		return domain.Product{}, err
	}
	r.saveProductCache(saved)
	r.saveProductRedis(saved)
	return saved, nil
}

// GetSKU 按三级缓存读取 SKU，策略同 GetProduct。
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

// SaveSKU 保存 SKU 后回填缓存，并失效该商品下的 SKU 列表缓存，
// 避免后续 ListSKUsByProduct 返回过期列表。
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

// Append 透传事件追加到底层 outbox（若底层仓库实现了 event.Outbox），
// 否则静默忽略——缓存装饰器不强制要求 outbox 能力。
func (r *CatalogCacheRepository) Append(ctx context.Context, evt event.Event) (int64, error) {
	if outbox, ok := r.Repository.(event.Outbox); ok {
		return outbox.Append(ctx, evt)
	}
	return 0, nil
}

// SaveOrderWithInventoryLocks 委托底层保存订单与库存锁，
// 完成后失效订单涉及 SKU 的缓存，保证下单后库存读数一致。
func (r *CatalogCacheRepository) SaveOrderWithInventoryLocks(order domain.Order, locks []domain.InventoryLock) (domain.Order, error) {
	saved, err := r.Repository.SaveOrderWithInventoryLocks(order, locks)
	if err != nil {
		return domain.Order{}, err
	}
	r.invalidateOrderItems(order.Items)
	return saved, nil
}

// ListSKUsByProduct 按三级缓存读取商品 SKU 列表，并顺带回填单个 SKU 缓存。
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

// loadProductRedis 从 Redis 读商品；超时/反序列化失败一律视为未命中，
// 交由上层回退到底层仓库。
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

// saveProductRedis 写商品到 Redis；缓存写入尽力而为，忽略错误。
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

// loadSKURedis 从 Redis 读 SKU，失败视为未命中。
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

// saveSKURedis 写 SKU 到 Redis，忽略错误。
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

// loadSKUListRedis 从 Redis 读商品 SKU 列表，失败视为未命中。
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

// saveSKUListRedis 写商品 SKU 列表到 Redis，忽略错误。
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

// invalidateSKUList 失效某商品下的 SKU 列表缓存：同时删除本地条目与 Redis 键。
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

// invalidateOrderItems 订单落库后批量失效订单涉及的 SKU 与列表缓存。
func (r *CatalogCacheRepository) invalidateOrderItems(items []domain.OrderItem) {
	for _, item := range items {
		r.invalidateSKU(item.SKUID)
		r.invalidateSKUList(item.ProductID)
	}
}

// loadProductCache 读商品本地缓存，过期条目视为未命中。
func (r *CatalogCacheRepository) loadProductCache(id int64) (domain.Product, bool) {
	r.productMu.RLock()
	entry, ok := r.productCache[id]
	r.productMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return domain.Product{}, false
	}
	return cloneProduct(entry.value), true
}

// saveProductCache 写商品本地缓存并记录过期时间。
func (r *CatalogCacheRepository) saveProductCache(product domain.Product) {
	r.productMu.Lock()
	r.productCache[product.ID] = productCacheEntry{value: cloneProduct(product), expiresAt: time.Now().Add(r.ttl)}
	r.productMu.Unlock()
}

// loadSKUCache 读 SKU 本地缓存，过期条目视为未命中。
func (r *CatalogCacheRepository) loadSKUCache(id int64) (domain.SKU, bool) {
	r.skuMu.RLock()
	entry, ok := r.skuCache[id]
	r.skuMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return domain.SKU{}, false
	}
	return cloneSKU(entry.value), true
}

// saveSKUCache 写 SKU 本地缓存。
func (r *CatalogCacheRepository) saveSKUCache(sku domain.SKU) {
	r.skuMu.Lock()
	r.skuCache[sku.ID] = skuCacheEntry{value: cloneSKU(sku), expiresAt: time.Now().Add(r.ttl)}
	r.skuMu.Unlock()
}

// invalidateSKU 失效单个 SKU 的本地与 Redis 缓存。
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

// loadSKUListCache 读 SKU 列表本地缓存。
func (r *CatalogCacheRepository) loadSKUListCache(productID int64) ([]domain.SKU, bool) {
	r.listMu.RLock()
	entry, ok := r.listCache[productID]
	r.listMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return cloneSKUs(entry.value), true
}

// saveSKUListCache 写 SKU 列表本地缓存。
func (r *CatalogCacheRepository) saveSKUListCache(productID int64, skus []domain.SKU) {
	r.listMu.Lock()
	r.listCache[productID] = skuListCacheEntry{value: cloneSKUs(skus), expiresAt: time.Now().Add(r.ttl)}
	r.listMu.Unlock()
}

// productKey/skuKey/productSKUsKey 分别拼接三类缓存的 Redis 键（前缀 + 主键）。
func productKey(id int64) string {
	return fmt.Sprintf("%s%d", productKeyPrefix, id)
}

func skuKey(id int64) string {
	return fmt.Sprintf("%s%d", skuKeyPrefix, id)
}

func productSKUsKey(productID int64) string {
	return fmt.Sprintf("%s%d", productSKUsKeyPrefix, productID)
}

// cloneProduct/cloneSKU/cloneSKUs 深拷贝缓存值，避免返回的切片/映射与内部
// 缓存共享底层数组，防止调用方修改或并发读写导致数据错乱。
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
