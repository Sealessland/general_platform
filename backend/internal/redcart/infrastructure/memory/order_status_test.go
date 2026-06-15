package memory

import (
	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"testing"
	"time"
)

func TestUpdateOrderStatusWithSideEffect(t *testing.T) {
	repo := NewRepository()
	now := time.Now().UTC()

	sku, ok := repo.GetSKU(1)
	if !ok {
		t.Fatal("expected seeded sku")
	}

	order, err := repo.SaveOrderWithInventoryLocks(domain.Order{
		OrderNo:         "MEM-ROLLBACK",
		UserID:          1,
		MerchantID:      1,
		Status:          orderdomain.StatusCreated,
		TotalAmountCent: sku.PriceCent,
		PayAmountCent:   sku.PriceCent,
		IdempotencyKey:  "mem-rollback",
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
		CreatedAt:       now,
		UpdatedAt:       now,
		Items: []domain.OrderItem{{
			ProductID:            1,
			SKUID:                sku.ID,
			ProductTitleSnapshot: "Product",
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
		t.Fatalf("create order: %v", err)
	}

	skuBeforePay, _ := repo.GetSKU(sku.ID)
	var eventRecorded bool
	saved, err := repo.UpdateOrderStatus(order.ID, string(orderdomain.StatusCreated), string(orderdomain.StatusPaid), func(o *domain.Order) error {
		o.PaidAt = &now
		return nil
	}, func(tx application.OrderTx, o domain.Order) error {
		locks := tx.ListInventoryLocksByOrder(o.ID)
		if len(locks) != 1 {
			t.Fatalf("expected one lock, got %d", len(locks))
		}
		lock := locks[0]
		sku, ok := tx.GetSKU(lock.SKUID)
		if !ok {
			t.Fatal("expected sku in side effect")
		}
		sku.Stock -= lock.Quantity
		sku.LockedStock -= lock.Quantity
		if _, err := tx.SaveSKU(sku); err != nil {
			return err
		}
		lock.Status = domain.InventoryLockStatusConfirmed
		lock.ConfirmedAt = &now
		if err := tx.UpdateInventoryLock(lock); err != nil {
			return err
		}
		_, _ = tx.AppendOrderEvent(domain.OrderEvent{
			OrderID:    o.ID,
			FromStatus: string(orderdomain.StatusCreated),
			ToStatus:   string(orderdomain.StatusPaid),
			EventType:  "ORDER_PAID",
			CreatedAt:  now,
		})
		eventRecorded = true
		return nil
	})
	if err != nil {
		t.Fatalf("update order status: %v", err)
	}
	if saved.Status != orderdomain.StatusPaid {
		t.Fatalf("expected PAID, got %s", saved.Status)
	}
	if !eventRecorded {
		t.Fatal("expected side effect to run")
	}

	updatedSKU, _ := repo.GetSKU(sku.ID)
	if updatedSKU.Stock != skuBeforePay.Stock-1 || updatedSKU.LockedStock != skuBeforePay.LockedStock-1 {
		t.Fatalf("expected stock decrement, got stock=%d locked_stock=%d", updatedSKU.Stock, updatedSKU.LockedStock)
	}

	// Side effect failure must roll back status change.
	_, err = repo.UpdateOrderStatus(order.ID, string(orderdomain.StatusPaid), string(orderdomain.StatusCancelled), func(o *domain.Order) error {
		o.CancelledAt = &now
		return nil
	}, func(tx application.OrderTx, o domain.Order) error {
		return application.ErrInsufficientStock
	})
	if err == nil {
		t.Fatal("expected side effect failure to rollback")
	}
	rolledBack, _ := repo.GetOrder(order.ID)
	if rolledBack.Status != orderdomain.StatusPaid {
		t.Fatalf("expected status rollback to PAID, got %s", rolledBack.Status)
	}
}
