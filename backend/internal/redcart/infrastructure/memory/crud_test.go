package memory

import (
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"testing"
	"time"
)

func TestRepositoryReadListsUpdatesAndCloneBoundaries(t *testing.T) {
	repo := NewRepository()
	now := time.Now().UTC()

	user, err := repo.CreateUser(domain.User{
		Nickname:     "Reader",
		Phone:        "13920000002",
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
	if _, ok := repo.GetUser(999999); ok {
		t.Fatal("expected missing user lookup to fail")
	}

	merchant, err := repo.CreateMerchant(domain.Merchant{
		UserID:      user.ID,
		Name:        "Reader Shop",
		Description: "reader test",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create merchant: %v", err)
	}
	if _, err := repo.CreateMerchant(domain.Merchant{UserID: user.ID}); err == nil {
		t.Fatal("expected duplicate merchant error")
	}
	if fetched, ok := repo.GetMerchant(merchant.ID); !ok || fetched.ID != merchant.ID {
		t.Fatalf("expected merchant by id, got %+v ok=%v", fetched, ok)
	}
	if _, ok := repo.GetMerchant(999999); ok {
		t.Fatal("expected missing merchant lookup to fail")
	}

	note, ok := repo.GetNote(1)
	if !ok {
		t.Fatal("expected seeded note")
	}
	note.ViewCount++
	if err := repo.UpdateNote(note); err != nil {
		t.Fatalf("update note: %v", err)
	}
	updatedNote, _ := repo.GetNote(note.ID)
	if updatedNote.ViewCount != note.ViewCount {
		t.Fatalf("expected updated note view count, got %+v", updatedNote)
	}
	if err := repo.UpdateNote(domain.Note{ID: 999999}); err == nil {
		t.Fatal("expected missing note update error")
	}

	products := repo.ListProducts()
	if len(products) < 2 || products[0].ID > products[1].ID {
		t.Fatalf("expected sorted products, got %+v", products)
	}
	products[0].SellingPoints[0] = "mutated list product"
	refetchedProduct, _ := repo.GetProduct(products[0].ID)
	if refetchedProduct.SellingPoints[0] == "mutated list product" {
		t.Fatal("expected ListProducts to return cloned products")
	}

	skus := repo.ListSKUsByProduct(products[0].ID)
	if len(skus) == 0 {
		t.Fatal("expected skus for seeded product")
	}
	skus[0].SKUAttrs["from_list"] = "mutated"
	refetchedSKU, _ := repo.GetSKU(skus[0].ID)
	if refetchedSKU.SKUAttrs["from_list"] == "mutated" {
		t.Fatal("expected ListSKUsByProduct to return cloned skus")
	}

	selected, err := repo.SaveCartItem(domain.CartItem{UserID: 77, ProductID: 1, SKUID: 1, Quantity: 1, Selected: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatalf("save selected cart item: %v", err)
	}
	unselected, err := repo.SaveCartItem(domain.CartItem{UserID: 77, ProductID: 2, SKUID: 3, Quantity: 1, Selected: false, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatalf("save unselected cart item: %v", err)
	}
	cartItems := repo.ListCartItems(77)
	if len(cartItems) != 2 || cartItems[0].ID != selected.ID || cartItems[1].ID != unselected.ID {
		t.Fatalf("expected sorted cart items, got %+v", cartItems)
	}
	if err := repo.DeleteSelectedCartItems(77); err != nil {
		t.Fatalf("delete selected cart items: %v", err)
	}
	remaining := repo.ListCartItems(77)
	if len(remaining) != 1 || remaining[0].ID != unselected.ID {
		t.Fatalf("expected only unselected cart item to remain, got %+v", remaining)
	}
	if err := repo.DeleteSelectedCartItems(999999); err != nil {
		t.Fatalf("delete selected cart items for missing user: %v", err)
	}

	order, err := repo.SaveOrder(domain.Order{
		UserID:             user.ID,
		MerchantID:         merchant.ID,
		Status:             "CREATED",
		TotalAmountCent:    100,
		PayAmountCent:      100,
		DiscountAmountCent: 0,
		CreatedAt:          now,
		UpdatedAt:          now,
		Items: []domain.OrderItem{{
			ProductID:            products[0].ID,
			SKUID:                skus[0].ID,
			ProductTitleSnapshot: products[0].Title,
			SKUNameSnapshot:      skus[0].SKUName,
			PriceCentSnapshot:    100,
			Quantity:             1,
			TotalAmountCent:      100,
			CreatedAt:            now,
			UpdatedAt:            now,
		}},
	})
	if err != nil {
		t.Fatalf("save order: %v", err)
	}
	fetchedOrder, ok := repo.GetOrder(order.ID)
	if !ok || len(fetchedOrder.Items) != 1 {
		t.Fatalf("expected order by id, got %+v ok=%v", fetchedOrder, ok)
	}
	fetchedOrder.Items[0].ProductTitleSnapshot = "mutated order item"
	refetchedOrder, _ := repo.GetOrder(order.ID)
	if refetchedOrder.Items[0].ProductTitleSnapshot == "mutated order item" {
		t.Fatal("expected GetOrder to return cloned order items")
	}

	orderEvent, err := repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      order.ID,
		ToStatus:     "CREATED",
		EventType:    "ORDER_CREATED",
		OperatorID:   user.ID,
		OperatorRole: domain.RoleConsumer,
		CreatedAt:    now,
	})
	if err != nil {
		t.Fatalf("append order event: %v", err)
	}
	events := repo.ListOrderEvents(order.ID)
	if len(events) != 1 || events[0].ID != orderEvent.ID {
		t.Fatalf("expected order events, got %+v", events)
	}

	behaviorEvent, err := repo.AppendBehaviorEvent(domain.BehaviorEvent{
		UserID:     user.ID,
		EventType:  domain.BehaviorProductClick,
		ProductID:  products[0].ID,
		MerchantID: merchant.ID,
		CreatedAt:  now,
	})
	if err != nil {
		t.Fatalf("append behavior event: %v", err)
	}
	behaviorEvents := repo.ListBehaviorEvents()
	if len(behaviorEvents) == 0 || behaviorEvents[len(behaviorEvents)-1].ID != behaviorEvent.ID {
		t.Fatalf("expected behavior events, got %+v", behaviorEvents)
	}
	behaviorEvents = append(behaviorEvents, domain.BehaviorEvent{ID: 999999})
	if len(repo.ListBehaviorEvents()) == len(behaviorEvents) {
		t.Fatal("expected ListBehaviorEvents to return a copy")
	}
}
