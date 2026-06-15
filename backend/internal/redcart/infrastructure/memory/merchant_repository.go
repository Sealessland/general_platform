package memory

import (
	"fmt"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"time"
)

func (r *Repository) CreateMerchant(merchant domain.Merchant) (domain.Merchant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.merchantsByUserID[merchant.UserID]; exists {
		return domain.Merchant{}, fmt.Errorf("merchant already exists for user")
	}
	merchant.ID = r.nextID(&r.nextMerchantID)
	if merchant.CreatedAt.IsZero() {
		merchant.CreatedAt = time.Now().UTC()
	}
	if merchant.UpdatedAt.IsZero() {
		merchant.UpdatedAt = merchant.CreatedAt
	}
	r.merchants[merchant.ID] = cloneMerchant(merchant)
	r.merchantsByUserID[merchant.UserID] = merchant.ID
	return cloneMerchant(merchant), nil
}

func (r *Repository) GetMerchant(id int64) (domain.Merchant, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	merchant, ok := r.merchants[id]
	return cloneMerchant(merchant), ok
}

func (r *Repository) GetMerchantByUserID(userID int64) (domain.Merchant, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.merchantsByUserID[userID]
	if !ok {
		return domain.Merchant{}, false
	}
	merchant, ok := r.merchants[id]
	return cloneMerchant(merchant), ok
}
