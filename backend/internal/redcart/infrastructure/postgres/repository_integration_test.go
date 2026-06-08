package postgres

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func TestRepositoryAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" || os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("POSTGRES_DSN not set")
	}

	repo, err := NewRepository(dsn)
	if err != nil {
		t.Fatalf("new postgres repository: %v", err)
	}
	defer repo.Close()

	products := repo.ListProducts()
	if len(products) == 0 {
		t.Fatal("expected seeded products")
	}

	phone := fmt.Sprintf("138%08d", time.Now().UnixNano()%100000000)
	user, err := repo.CreateUser(domain.User{
		Nickname:     "PG User",
		Phone:        phone,
		PasswordHash: "hashed",
		Role:         domain.RoleConsumer,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if fetched, ok := repo.FindUserByPhone(user.Phone); !ok || fetched.ID != user.ID {
		t.Fatal("expected user by phone")
	}

	repo.SaveSession("pg-token", user.ID)
	if fetched, ok := repo.GetUserByToken("pg-token"); !ok || fetched.ID != user.ID {
		t.Fatal("expected user by token")
	}

	note, ok := repo.GetNote(1)
	if !ok || len(note.ProductIDs) == 0 {
		t.Fatal("expected seeded note with product ids")
	}

	product, ok := repo.GetProduct(1)
	if !ok || len(product.SellingPoints) == 0 {
		t.Fatal("expected seeded product")
	}

	skus := repo.ListSKUsByProduct(product.ID)
	if len(skus) == 0 {
		t.Fatal("expected seeded skus")
	}
}

func TestRepositoryPostgresCRUDCoverage(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" || os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("POSTGRES_DSN not set")
	}

	repo, err := NewRepository(dsn)
	if err != nil {
		t.Fatalf("new postgres repository: %v", err)
	}
	defer repo.Close()

	now := time.Now().UTC().Truncate(time.Microsecond)
	suffix := fmt.Sprintf("%d", now.UnixNano())
	user, err := repo.CreateUser(domain.User{
		Nickname:     "PG Coverage User",
		Phone:        "137" + suffix[len(suffix)-8:],
		PasswordHash: "hashed",
		Role:         domain.RoleMerchant,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if fetched, ok := repo.GetUser(user.ID); !ok || fetched.ID != user.ID {
		t.Fatalf("expected user by id, got %+v ok=%v", fetched, ok)
	}

	merchant, err := repo.CreateMerchant(domain.Merchant{
		UserID:      user.ID,
		Name:        "PG Coverage Shop " + suffix,
		Description: "postgres repository coverage",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	if fetched, ok := repo.GetMerchant(merchant.ID); !ok || fetched.ID != merchant.ID {
		t.Fatalf("expected merchant by id, got %+v ok=%v", fetched, ok)
	}
	if fetched, ok := repo.GetMerchantByUserID(user.ID); !ok || fetched.ID != merchant.ID {
		t.Fatalf("expected merchant by user id, got %+v ok=%v", fetched, ok)
	}

	notes := repo.ListNotes()
	if len(notes) == 0 {
		t.Fatal("expected seeded notes")
	}
	note := notes[0]
	note.ViewCount++
	note.LikeCount++
	if err := repo.UpdateNote(note); err != nil {
		t.Fatalf("update note: %v", err)
	}
	updatedNote, ok := repo.GetNote(note.ID)
	if !ok || updatedNote.ViewCount != note.ViewCount || updatedNote.LikeCount != note.LikeCount {
		t.Fatalf("expected updated note, got %+v ok=%v", updatedNote, ok)
	}

	product, err := repo.SaveProduct(domain.Product{
		MerchantID:    merchant.ID,
		Title:         "PG Coverage Product " + suffix,
		Description:   "created by repository coverage test",
		CoverURL:      "https://images.example.com/pg-coverage.jpg",
		CategoryID:    700,
		Status:        domain.ProductStatusDraft,
		SellingPoints: []string{"coverage", "postgres"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("save product insert: %v", err)
	}
	product.Status = domain.ProductStatusOnline
	product.Title = "PG Coverage Product Updated " + suffix
	product.SellingPoints = []string{"updated", "postgres"}
	product, err = repo.SaveProduct(product)
	if err != nil {
		t.Fatalf("save product update: %v", err)
	}
	if product.Status != domain.ProductStatusOnline || len(product.SellingPoints) != 2 {
		t.Fatalf("expected updated product, got %+v", product)
	}

	sku, err := repo.SaveSKU(domain.SKU{
		ProductID:   product.ID,
		SKUName:     "Coverage SKU",
		SKUAttrs:    map[string]string{"color": "green"},
		PriceCent:   12345,
		Stock:       12,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("save sku insert: %v", err)
	}
	sku.SKUName = "Coverage SKU Updated"
	sku.Stock = 10
	sku.SKUAttrs = map[string]string{"color": "blue"}
	sku, err = repo.SaveSKU(sku)
	if err != nil {
		t.Fatalf("save sku update: %v", err)
	}
	if sku.SKUName != "Coverage SKU Updated" || sku.Stock != 10 || sku.SKUAttrs["color"] != "blue" {
		t.Fatalf("expected updated sku, got %+v", sku)
	}

	selectedCartItem, err := repo.SaveCartItem(domain.CartItem{
		UserID:    user.ID,
		ProductID: product.ID,
		SKUID:     sku.ID,
		Quantity:  1,
		Selected:  true,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("save selected cart item: %v", err)
	}
	selectedCartItem.Quantity = 2
	selectedCartItem.Selected = false
	selectedCartItem, err = repo.SaveCartItem(selectedCartItem)
	if err != nil {
		t.Fatalf("update cart item: %v", err)
	}
	if selectedCartItem.Quantity != 2 || selectedCartItem.Selected {
		t.Fatalf("expected updated cart item, got %+v", selectedCartItem)
	}
	unselectedCartItem, err := repo.SaveCartItem(domain.CartItem{
		UserID:    user.ID,
		ProductID: product.ID,
		SKUID:     sku.ID,
		Quantity:  1,
		Selected:  false,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("save unselected cart item: %v", err)
	}
	cartItems := repo.ListCartItems(user.ID)
	if len(cartItems) < 2 {
		t.Fatalf("expected cart items, got %+v", cartItems)
	}
	if fetched, ok := repo.GetCartItem(user.ID, selectedCartItem.ID); !ok || fetched.Quantity != 2 {
		t.Fatalf("expected cart item by id, got %+v ok=%v", fetched, ok)
	}
	if err := repo.DeleteCartItem(user.ID, selectedCartItem.ID); err != nil {
		t.Fatalf("delete cart item: %v", err)
	}
	if err := repo.DeleteCartItem(user.ID, selectedCartItem.ID); err == nil {
		t.Fatal("expected deleting missing cart item to fail")
	}
	if err := repo.DeleteSelectedCartItems(user.ID); err != nil {
		t.Fatalf("delete selected cart items: %v", err)
	}
	if fetched, ok := repo.GetCartItem(user.ID, unselectedCartItem.ID); !ok || fetched.ID != unselectedCartItem.ID {
		t.Fatalf("expected unselected cart item to remain, got %+v ok=%v", fetched, ok)
	}

	order, err := repo.SaveOrder(domain.Order{
		OrderNo:            "RCCOV" + suffix,
		UserID:             user.ID,
		MerchantID:         merchant.ID,
		Status:             orderdomain.StatusCreated,
		TotalAmountCent:    sku.PriceCent,
		PayAmountCent:      sku.PriceCent,
		DiscountAmountCent: 0,
		IdempotencyKey:     "pg-coverage-" + suffix,
		ReceiverName:       "PG User",
		ReceiverPhone:      user.Phone,
		ReceiverAddress:    "Shanghai",
		CreatedAt:          now,
		UpdatedAt:          now,
		Items: []domain.OrderItem{{
			ProductID:            product.ID,
			SKUID:                sku.ID,
			ProductTitleSnapshot: product.Title,
			SKUNameSnapshot:      sku.SKUName,
			PriceCentSnapshot:    sku.PriceCent,
			Quantity:             1,
			TotalAmountCent:      sku.PriceCent,
			CreatedAt:            now,
			UpdatedAt:            now,
		}},
	})
	if err != nil {
		t.Fatalf("save order insert: %v", err)
	}
	if len(order.Items) != 1 {
		t.Fatalf("expected saved order items, got %+v", order)
	}
	if existing, ok := repo.FindOrderByUserAndIdempotency(user.ID, order.IdempotencyKey); !ok || existing.ID != order.ID {
		t.Fatalf("expected idempotent order lookup, got %+v ok=%v", existing, ok)
	}
	if orders := repo.ListOrdersByUser(user.ID); len(orders) == 0 {
		t.Fatal("expected orders by user")
	}
	if orders := repo.ListOrdersByMerchant(merchant.ID); len(orders) == 0 {
		t.Fatal("expected orders by merchant")
	}
	paidAt := now.Add(time.Minute)
	order.Status = orderdomain.StatusPaid
	order.PaidAt = &paidAt
	order, err = repo.SaveOrder(order)
	if err != nil {
		t.Fatalf("save order update: %v", err)
	}
	if order.Status != orderdomain.StatusPaid || order.PaidAt == nil {
		t.Fatalf("expected updated order, got %+v", order)
	}

	orderEvent, err := repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      order.ID,
		FromStatus:   string(orderdomain.StatusCreated),
		ToStatus:     string(orderdomain.StatusPaid),
		EventType:    "ORDER_PAID",
		OperatorID:   user.ID,
		OperatorRole: domain.RoleConsumer,
		Remark:       "coverage event",
		CreatedAt:    now,
	})
	if err != nil {
		t.Fatalf("append order event: %v", err)
	}
	orderEvents := repo.ListOrderEvents(order.ID)
	if len(orderEvents) == 0 || orderEvents[len(orderEvents)-1].ID != orderEvent.ID {
		t.Fatalf("expected order events, got %+v", orderEvents)
	}

	inventoryLock, err := repo.SaveInventoryLock(domain.InventoryLock{
		OrderID:   order.ID,
		SKUID:     sku.ID,
		Quantity:  1,
		Status:    domain.InventoryLockStatusLocked,
		LockedAt:  now,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("save inventory lock: %v", err)
	}
	releasedAt := now.Add(2 * time.Minute)
	inventoryLock.Status = domain.InventoryLockStatusReleased
	inventoryLock.ReleasedAt = &releasedAt
	if err := repo.UpdateInventoryLock(inventoryLock); err != nil {
		t.Fatalf("update inventory lock: %v", err)
	}
	locks := repo.ListInventoryLocksByOrder(order.ID)
	if len(locks) == 0 || locks[len(locks)-1].Status != domain.InventoryLockStatusReleased {
		t.Fatalf("expected updated inventory lock, got %+v", locks)
	}

	behavior, err := repo.AppendBehaviorEvent(domain.BehaviorEvent{
		UserID:     user.ID,
		EventType:  domain.BehaviorOrderPay,
		ProductID:  product.ID,
		SKUID:      sku.ID,
		OrderID:    order.ID,
		MerchantID: merchant.ID,
		CreatedAt:  now,
	})
	if err != nil {
		t.Fatalf("append behavior event: %v", err)
	}
	behaviorEvents := repo.ListBehaviorEvents()
	if len(behaviorEvents) == 0 || behaviorEvents[len(behaviorEvents)-1].ID != behavior.ID {
		t.Fatalf("expected behavior events, got %+v", behaviorEvents)
	}

	task, err := repo.CreateAITask(domain.AIGenerationTask{
		UserID:     user.ID,
		MerchantID: merchant.ID,
		TaskType:   domain.TaskTypeBusinessReview,
		Input:      map[string]any{"window_days": 7},
		Output:     map[string]any{"diagnosis": "initial"},
		Status:     domain.AITaskStatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("create ai task: %v", err)
	}
	task.Status = domain.AITaskStatusCompleted
	task.Output = map[string]any{"diagnosis": "updated"}
	if err := repo.UpdateAITask(task); err != nil {
		t.Fatalf("update ai task: %v", err)
	}
	fetchedTask, ok := repo.GetAITask(task.ID)
	if !ok || fetchedTask.Status != domain.AITaskStatusCompleted || fetchedTask.Output["diagnosis"] != "updated" {
		t.Fatalf("expected updated ai task, got %+v ok=%v", fetchedTask, ok)
	}
}

func TestConcurrentCreateOrderReservesStockAtomically(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" || os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("POSTGRES_DSN not set")
	}

	repo, err := NewRepository(dsn)
	if err != nil {
		t.Fatalf("new postgres repository: %v", err)
	}
	defer repo.Close()
	service := application.NewService(repo, backendai.MockProvider{})

	now := time.Now().UTC()
	product, err := repo.SaveProduct(domain.Product{
		MerchantID:    1,
		Title:         fmt.Sprintf("Atomic Stock Product %d", now.UnixNano()),
		Description:   "created for concurrent stock test",
		CategoryID:    999,
		Status:        domain.ProductStatusOnline,
		SellingPoints: []string{"stock safe"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	sku, err := repo.SaveSKU(domain.SKU{
		ProductID:   product.ID,
		SKUName:     "Only One",
		SKUAttrs:    map[string]string{"stock": "one"},
		PriceCent:   9900,
		Stock:       1,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create sku: %v", err)
	}

	const workers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	var created atomic.Int64
	var conflicts atomic.Int64
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := service.CreateOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, fmt.Sprintf("atomic-stock-%d-%d", now.UnixNano(), i), application.CheckoutInput{
				Items:           []application.OrderLineInput{{SKUID: sku.ID, Quantity: 1}},
				ReceiverName:    "Alice",
				ReceiverPhone:   "13800000001",
				ReceiverAddress: "Shanghai",
			})
			if err == nil {
				created.Add(1)
				return
			}
			if appErr, ok := err.(*application.AppError); ok && appErr.Kind == application.ErrorConflict {
				conflicts.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if created.Load() != 1 {
		t.Fatalf("expected exactly one order created for stock=1, got %d conflicts=%d", created.Load(), conflicts.Load())
	}
	updated, ok := repo.GetSKU(sku.ID)
	if !ok {
		t.Fatal("expected sku after concurrent order attempts")
	}
	if updated.Stock != 1 || updated.LockedStock != 1 {
		t.Fatalf("expected stock=1 locked_stock=1 after reservation, got stock=%d locked_stock=%d", updated.Stock, updated.LockedStock)
	}
}
