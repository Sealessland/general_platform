package memory

import (
	"fmt"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"sort"
	"time"
)

func (r *Repository) ListCartItems(userID int64) []domain.CartItem {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.CartItem, 0)
	for _, item := range r.cartItemsByUser[userID] {
		out = append(out, cloneCartItem(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Repository) GetCartItem(userID, itemID int64) (domain.CartItem, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items, ok := r.cartItemsByUser[userID]
	if !ok {
		return domain.CartItem{}, false
	}
	item, ok := items[itemID]
	return cloneCartItem(item), ok
}

func (r *Repository) SaveCartItem(item domain.CartItem) (domain.CartItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item.ID == 0 {
		item.ID = r.nextID(&r.nextCartItemID)
		if item.CreatedAt.IsZero() {
			item.CreatedAt = time.Now().UTC()
		}
	}
	item.UpdatedAt = time.Now().UTC()
	if _, ok := r.cartItemsByUser[item.UserID]; !ok {
		r.cartItemsByUser[item.UserID] = make(map[int64]domain.CartItem)
	}
	r.cartItemsByUser[item.UserID][item.ID] = cloneCartItem(item)
	return cloneCartItem(item), nil
}

func (r *Repository) DeleteCartItem(userID, itemID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	items, ok := r.cartItemsByUser[userID]
	if !ok {
		return fmt.Errorf("cart not found")
	}
	if _, ok := items[itemID]; !ok {
		return fmt.Errorf("cart item not found")
	}
	delete(items, itemID)
	return nil
}

func (r *Repository) DeleteSelectedCartItems(userID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := r.cartItemsByUser[userID]
	for id, item := range items {
		if item.Selected {
			delete(items, id)
		}
	}
	return nil
}
