package memory

import (
	"fmt"
	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"sort"
	"time"
)

func (r *Repository) FindOrderByUserAndIdempotency(userID int64, idempotencyKey string) (domain.Order, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.orderByIdempotency[fmt.Sprintf("%d:%s", userID, idempotencyKey)]
	if !ok {
		return domain.Order{}, false
	}
	order, ok := r.orders[id]
	return cloneOrder(order), ok
}

func (r *Repository) ListOrdersByUser(userID int64) []domain.Order {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Order, 0)
	for _, order := range r.orders {
		if order.UserID == userID {
			out = append(out, cloneOrder(order))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Repository) ListOrdersByMerchant(merchantID int64) []domain.Order {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Order, 0)
	for _, order := range r.orders {
		if order.MerchantID == merchantID {
			out = append(out, cloneOrder(order))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Repository) GetOrder(id int64) (domain.Order, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	order, ok := r.orders[id]
	return cloneOrder(order), ok
}

func (r *Repository) SaveOrder(order domain.Order) (domain.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveOrderLocked(order), nil
}

func (r *Repository) saveOrderLocked(order domain.Order) domain.Order {
	if order.ID == 0 {
		order.ID = r.nextID(&r.nextOrderID)
		if order.CreatedAt.IsZero() {
			order.CreatedAt = time.Now().UTC()
		}
		for i := range order.Items {
			order.Items[i].ID = r.nextID(&r.nextOrderItemID)
			order.Items[i].OrderID = order.ID
			if order.Items[i].CreatedAt.IsZero() {
				order.Items[i].CreatedAt = order.CreatedAt
			}
			order.Items[i].UpdatedAt = time.Now().UTC()
		}
	}
	order.UpdatedAt = time.Now().UTC()
	r.orders[order.ID] = cloneOrder(order)
	if order.IdempotencyKey != "" {
		r.orderByIdempotency[fmt.Sprintf("%d:%s", order.UserID, order.IdempotencyKey)] = order.ID
	}
	return cloneOrder(order)
}

type memOrderTx struct {
	r *Repository
}

func (t *memOrderTx) GetSKU(id int64) (domain.SKU, bool) {
	return t.r.getSKULocked(id)
}

func (t *memOrderTx) SaveSKU(sku domain.SKU) (domain.SKU, error) {
	return t.r.saveSKULocked(sku)
}

func (t *memOrderTx) ListInventoryLocksByOrder(orderID int64) []domain.InventoryLock {
	return t.r.listInventoryLocksByOrderLocked(orderID)
}

func (t *memOrderTx) UpdateInventoryLock(lock domain.InventoryLock) error {
	return t.r.updateInventoryLockLocked(lock)
}

func (t *memOrderTx) AppendOrderEvent(event domain.OrderEvent) (domain.OrderEvent, error) {
	return t.r.appendOrderEventLocked(event)
}

func (r *Repository) UpdateOrderStatus(orderID int64, fromStatus, toStatus string, mutator func(*domain.Order) error, sideEffect func(application.OrderTx, domain.Order) error) (domain.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	order, ok := r.orders[orderID]
	if !ok {
		return domain.Order{}, fmt.Errorf("order %d not found", orderID)
	}
	if order.Status != orderdomain.OrderStatus(fromStatus) {
		return domain.Order{}, fmt.Errorf("order %d status is %s, expected %s", orderID, order.Status, fromStatus)
	}
	if mutator != nil {
		if err := mutator(&order); err != nil {
			return domain.Order{}, err
		}
	}
	order.Status = orderdomain.OrderStatus(toStatus)
	order.UpdatedAt = time.Now().UTC()
	r.orders[orderID] = cloneOrder(order)
	if sideEffect != nil {
		if err := sideEffect(&memOrderTx{r: r}, order); err != nil {
			// Rollback the status change on side effect failure to keep the
			// in-memory contract consistent with the PostgreSQL transaction.
			order.Status = orderdomain.OrderStatus(fromStatus)
			r.orders[orderID] = cloneOrder(order)
			return domain.Order{}, err
		}
	}
	return cloneOrder(order), nil
}

func (r *Repository) SaveOrderWithInventoryLocks(order domain.Order, locks []domain.InventoryLock) (domain.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, lock := range locks {
		sku, ok := r.skus[lock.SKUID]
		if !ok {
			return domain.Order{}, fmt.Errorf("sku not found")
		}
		if sku.Stock-sku.LockedStock < lock.Quantity {
			return domain.Order{}, application.ErrInsufficientStock
		}
	}
	saved := r.saveOrderLocked(order)
	for i := range locks {
		lock := &locks[i]
		sku := r.skus[lock.SKUID]
		sku.LockedStock += lock.Quantity
		sku.UpdatedAt = time.Now().UTC()
		r.skus[sku.ID] = cloneSKU(sku)

		lock.ID = r.nextID(&r.nextInventoryLockID)
		lock.OrderID = saved.ID
		if lock.CreatedAt.IsZero() {
			lock.CreatedAt = time.Now().UTC()
		}
		lock.UpdatedAt = time.Now().UTC()
		r.locksByOrder[saved.ID] = append(r.locksByOrder[saved.ID], cloneInventoryLock(*lock))
	}
	return cloneOrder(saved), nil
}
