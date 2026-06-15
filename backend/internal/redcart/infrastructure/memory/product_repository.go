package memory

import (
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"sort"
	"time"
)

func (r *Repository) ListProducts() []domain.Product {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Product, 0, len(r.products))
	for _, product := range r.products {
		out = append(out, cloneProduct(product))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Repository) GetProduct(id int64) (domain.Product, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	product, ok := r.products[id]
	return cloneProduct(product), ok
}

func (r *Repository) SaveProduct(product domain.Product) (domain.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if product.ID == 0 {
		product.ID = r.nextID(&r.nextProductID)
		if product.CreatedAt.IsZero() {
			product.CreatedAt = time.Now().UTC()
		}
	}
	product.UpdatedAt = time.Now().UTC()
	r.products[product.ID] = cloneProduct(product)
	return cloneProduct(product), nil
}
