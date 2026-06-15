package memory

import (
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"testing"
	"time"
)

func TestRepositoryCartOrderInventoryAndAITaskFlow(t *testing.T) {
	repo := NewRepository()
	now := time.Now().UTC()

	item, err := repo.SaveCartItem(domain.CartItem{
		UserID:    99,
		ProductID: 1,
		SKUID:     1,
		Quantity:  2,
		Selected:  true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("save cart item: %v", err)
	}
	if fetched, ok := repo.GetCartItem(99, item.ID); !ok || fetched.Quantity != 2 {
		t.Fatalf("expected cart item, got %+v ok=%v", fetched, ok)
	}
	if err := repo.DeleteCartItem(99, item.ID); err != nil {
		t.Fatalf("delete cart item: %v", err)
	}
	if err := repo.DeleteCartItem(99, item.ID); err == nil {
		t.Fatal("expected missing cart item delete error")
	}

	order := domain.Order{
		UserID:         1,
		MerchantID:     1,
		Status:         "CREATED",
		IdempotencyKey: "repo-order",
		CreatedAt:      now,
		UpdatedAt:      now,
		Items: []domain.OrderItem{{
			ProductID:            1,
			SKUID:                1,
			ProductTitleSnapshot: "Product",
			SKUNameSnapshot:      "SKU",
			PriceCentSnapshot:    12900,
			Quantity:             1,
			TotalAmountCent:      12900,
		}},
	}
	saved, err := repo.SaveOrderWithInventoryLocks(order, []domain.InventoryLock{{
		SKUID:     1,
		Quantity:  1,
		Status:    domain.InventoryLockStatusLocked,
		LockedAt:  now,
		CreatedAt: now,
		UpdatedAt: now,
	}})
	if err != nil {
		t.Fatalf("save order with inventory locks: %v", err)
	}
	if existing, ok := repo.FindOrderByUserAndIdempotency(1, "repo-order"); !ok || existing.ID != saved.ID {
		t.Fatalf("expected idempotent order lookup, got %+v ok=%v", existing, ok)
	}
	if len(repo.ListOrdersByUser(1)) == 0 || len(repo.ListOrdersByMerchant(1)) == 0 {
		t.Fatal("expected order in user and merchant lists")
	}
	locks := repo.ListInventoryLocksByOrder(saved.ID)
	if len(locks) != 1 || locks[0].Status != domain.InventoryLockStatusLocked {
		t.Fatalf("expected locked inventory, got %+v", locks)
	}
	locks[0].Status = domain.InventoryLockStatusReleased
	if err := repo.UpdateInventoryLock(locks[0]); err != nil {
		t.Fatalf("update inventory lock: %v", err)
	}
	if _, err := repo.SaveOrderWithInventoryLocks(order, []domain.InventoryLock{{SKUID: 1, Quantity: 999999}}); err != application.ErrInsufficientStock {
		t.Fatalf("expected insufficient stock, got %v", err)
	}

	task, err := repo.CreateAITask(domain.AIGenerationTask{
		UserID:    1,
		TaskType:  domain.TaskTypeSellingPoints,
		Input:     map[string]any{"product_name": "Repository Product"},
		Output:    map[string]any{"core_points": []string{"one"}},
		Status:    domain.AITaskStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("create ai task: %v", err)
	}
	task.Input["product_name"] = "mutated"
	persistedTask, ok := repo.GetAITask(task.ID)
	if !ok || persistedTask.Input["product_name"] != "Repository Product" {
		t.Fatalf("expected created ai task input to be cloned, got %+v ok=%v", persistedTask, ok)
	}
	task.Status = domain.AITaskStatusCompleted
	if err := repo.UpdateAITask(task); err != nil {
		t.Fatalf("update ai task: %v", err)
	}
	if fetched, ok := repo.GetAITask(task.ID); !ok || fetched.Status != domain.AITaskStatusCompleted {
		t.Fatalf("expected completed ai task, got %+v ok=%v", fetched, ok)
	} else {
		fetched.Input["product_name"] = "mutated again"
		fetched.Output["core_points"] = []string{"mutated"}
		refetched, _ := repo.GetAITask(task.ID)
		corePoints, ok := refetched.Output["core_points"].([]string)
		if refetched.Input["product_name"] == "mutated again" || !ok || len(corePoints) != 1 || corePoints[0] != "one" {
			t.Fatalf("expected fetched ai task maps to be cloned, got %+v", refetched)
		}
	}
	if err := repo.UpdateAITask(domain.AIGenerationTask{ID: 999999}); err == nil {
		t.Fatal("expected missing ai task update error")
	}
}
