package memory

import (
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"sort"
	"time"
)

func (r *Repository) ListSKUsByProduct(productID int64) []domain.SKU {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.SKU, 0)
	for _, sku := range r.skus {
		if sku.ProductID == productID {
			out = append(out, cloneSKU(sku))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Repository) GetSKU(id int64) (domain.SKU, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.getSKULocked(id)
}

func (r *Repository) getSKULocked(id int64) (domain.SKU, bool) {
	sku, ok := r.skus[id]
	return cloneSKU(sku), ok
}

func (r *Repository) SaveSKU(sku domain.SKU) (domain.SKU, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveSKULocked(sku)
}

func (r *Repository) saveSKULocked(sku domain.SKU) (domain.SKU, error) {
	if sku.ID == 0 {
		sku.ID = r.nextID(&r.nextSKUID)
		if sku.CreatedAt.IsZero() {
			sku.CreatedAt = time.Now().UTC()
		}
	}
	sku.UpdatedAt = time.Now().UTC()
	r.skus[sku.ID] = cloneSKU(sku)
	return cloneSKU(sku), nil
}
