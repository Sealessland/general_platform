package memory

import (
	"fmt"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"time"
)

func (r *Repository) ListInventoryLocksByOrder(orderID int64) []domain.InventoryLock {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listInventoryLocksByOrderLocked(orderID)
}

func (r *Repository) listInventoryLocksByOrderLocked(orderID int64) []domain.InventoryLock {
	locks := r.locksByOrder[orderID]
	out := make([]domain.InventoryLock, len(locks))
	for i, lock := range locks {
		out[i] = cloneInventoryLock(lock)
	}
	return out
}

func (r *Repository) SaveInventoryLock(lock domain.InventoryLock) (domain.InventoryLock, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveInventoryLockLocked(lock)
}

func (r *Repository) saveInventoryLockLocked(lock domain.InventoryLock) (domain.InventoryLock, error) {
	lock.ID = r.nextID(&r.nextInventoryLockID)
	if lock.CreatedAt.IsZero() {
		lock.CreatedAt = time.Now().UTC()
	}
	lock.UpdatedAt = time.Now().UTC()
	r.locksByOrder[lock.OrderID] = append(r.locksByOrder[lock.OrderID], cloneInventoryLock(lock))
	return cloneInventoryLock(lock), nil
}

func (r *Repository) UpdateInventoryLock(lock domain.InventoryLock) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.updateInventoryLockLocked(lock)
}

func (r *Repository) updateInventoryLockLocked(lock domain.InventoryLock) error {
	locks := r.locksByOrder[lock.OrderID]
	for i := range locks {
		if locks[i].ID == lock.ID {
			lock.UpdatedAt = time.Now().UTC()
			locks[i] = cloneInventoryLock(lock)
			r.locksByOrder[lock.OrderID] = locks
			return nil
		}
	}
	return fmt.Errorf("inventory lock not found")
}
