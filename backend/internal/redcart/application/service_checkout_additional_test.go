package application_test

import (
	"context"
	"testing"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	application "github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
)

type createOrderProbeRepo struct {
	application.Repository
	listOrderEventsCalls    int
	listInventoryLocksCalls int
}

func (r *createOrderProbeRepo) ListOrderEvents(orderID int64) []domain.OrderEvent {
	r.listOrderEventsCalls++
	return r.Repository.ListOrderEvents(orderID)
}

func (r *createOrderProbeRepo) ListInventoryLocksByOrder(orderID int64) []domain.InventoryLock {
	r.listInventoryLocksCalls++
	return r.Repository.ListInventoryLocksByOrder(orderID)
}

func TestCartSelectionAndCheckoutFromSelectedItems(t *testing.T) {
	t.Parallel()

	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
	actor := application.Actor{UserID: 1, Role: domain.RoleConsumer}

	first, err := service.AddCartItem(context.Background(), actor, application.CartItemInput{SKUID: 1, Quantity: 1})
	if err != nil {
		t.Fatalf("add first cart item: %v", err)
	}
	second, err := service.AddCartItem(context.Background(), actor, application.CartItemInput{SKUID: 3, Quantity: 2})
	if err != nil {
		t.Fatalf("add second cart item: %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("expected distinct cart items")
	}

	selected := false
	if _, err := service.UpdateCartItem(context.Background(), actor, second.ID, application.CartItemUpdateInput{Selected: &selected}); err != nil {
		t.Fatalf("unselect second item: %v", err)
	}
	if _, err := service.UpdateCartItem(context.Background(), actor, first.ID, application.CartItemUpdateInput{Quantity: 3}); err != nil {
		t.Fatalf("update first quantity: %v", err)
	}

	cart, err := service.GetCart(context.Background(), actor)
	if err != nil {
		t.Fatalf("get cart: %v", err)
	}
	if cart.SelectedItemCount != 1 || cart.SelectedQuantity != 3 || cart.SelectedAmountCent != 38700 {
		t.Fatalf("unexpected selected cart totals: %+v", cart)
	}

	preview, err := service.PreviewOrder(context.Background(), actor, application.CheckoutInput{})
	if err != nil {
		t.Fatalf("preview selected cart: %v", err)
	}
	if len(preview.Items) != 1 || preview.Items[0].SKUID != first.SKUID || preview.PayAmountCent != 38700 {
		t.Fatalf("expected preview from selected first item only, got %+v", preview)
	}

	order, err := service.CreateOrder(context.Background(), actor, "selected-cart-checkout", application.CheckoutInput{
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
	})
	if err != nil {
		t.Fatalf("create order from selected cart: %v", err)
	}
	if len(order.Items) != 1 || order.Items[0].SKUID != first.SKUID {
		t.Fatalf("expected selected cart order item, got %+v", order.Items)
	}
	remaining, err := service.GetCart(context.Background(), actor)
	if err != nil {
		t.Fatalf("get remaining cart: %v", err)
	}
	if len(remaining.Items) != 1 || remaining.Items[0].SKUID != second.SKUID {
		t.Fatalf("expected unselected cart item to remain, got %+v", remaining.Items)
	}
	if err := service.DeleteCartItem(context.Background(), actor, remaining.Items[0].ID); err != nil {
		t.Fatalf("delete remaining cart item: %v", err)
	}
	if err := service.DeleteCartItem(context.Background(), actor, remaining.Items[0].ID); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected missing cart item not found, got %v", err)
	}
}

func TestCreateOrderPersistsCreationSideEffects(t *testing.T) {
	t.Parallel()

	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
	actor := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	beforeSKU, ok := repo.GetSKU(1)
	if !ok {
		t.Fatal("expected seeded sku")
	}

	order, err := service.CreateOrder(context.Background(), actor, "creation-side-effects", application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: 1, Quantity: 2}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	if order.Status != "CREATED" || len(order.Items) != 1 {
		t.Fatalf("expected created order with one item, got %+v", order)
	}
	if len(order.Events) != 1 {
		t.Fatalf("expected order created event, got %+v", order.Events)
	}
	event := order.Events[0]
	if event.EventType != "ORDER_CREATED" || event.FromStatus != "" || event.ToStatus != "CREATED" {
		t.Fatalf("expected order created transition event, got %+v", event)
	}
	if event.OperatorID != actor.UserID || event.OperatorRole != actor.Role || event.Remark != "order created" || event.CreatedAt.IsZero() {
		t.Fatalf("expected order created operator metadata, got %+v", event)
	}
	if len(order.InventoryLocks) != 1 {
		t.Fatalf("expected one inventory lock, got %+v", order.InventoryLocks)
	}
	lock := order.InventoryLocks[0]
	if lock.SKUID != 1 || lock.Quantity != 2 || lock.Status != domain.InventoryLockStatusLocked || lock.LockedAt.IsZero() {
		t.Fatalf("expected locked inventory for created order, got %+v", lock)
	}

	afterSKU, ok := repo.GetSKU(1)
	if !ok {
		t.Fatal("expected sku after create order")
	}
	if afterSKU.Stock != beforeSKU.Stock || afterSKU.LockedStock != beforeSKU.LockedStock+2 {
		t.Fatalf("expected locked stock increased by 2 without reducing stock, before=%+v after=%+v", beforeSKU, afterSKU)
	}

	foundBehaviorEvent := false
	for _, event := range repo.ListBehaviorEvents() {
		if event.EventType == domain.BehaviorOrderCreate && event.OrderID == order.ID && event.UserID == actor.UserID && event.MerchantID == order.MerchantID {
			foundBehaviorEvent = true
			break
		}
	}
	if !foundBehaviorEvent {
		t.Fatalf("expected order create behavior event for order %d", order.ID)
	}
}

func TestCreateOrderReturnsFreshMetadataWithoutRequeryingEventsOrLocks(t *testing.T) {
	t.Parallel()

	repo := &createOrderProbeRepo{Repository: memory.NewRepository()}
	service := application.NewService(repo, backendai.MockProvider{})
	actor := application.Actor{UserID: 1, Role: domain.RoleConsumer}

	order, err := service.CreateOrder(context.Background(), actor, "no-requery-create-order", application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: 1, Quantity: 1}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if len(order.Events) != 1 || order.Events[0].EventType != "ORDER_CREATED" {
		t.Fatalf("expected created order event in response, got %+v", order.Events)
	}
	if len(order.InventoryLocks) != 1 || order.InventoryLocks[0].Status != domain.InventoryLockStatusLocked {
		t.Fatalf("expected locked inventory in response, got %+v", order.InventoryLocks)
	}
	if repo.listOrderEventsCalls != 0 {
		t.Fatalf("expected no ListOrderEvents requery on fresh create response, got %d", repo.listOrderEventsCalls)
	}
	if repo.listInventoryLocksCalls != 0 {
		t.Fatalf("expected no ListInventoryLocksByOrder requery on fresh create response, got %d", repo.listInventoryLocksCalls)
	}
}

func TestCreateOrderWithExplicitItemsKeepsSelectedCartUntouched(t *testing.T) {
	t.Parallel()

	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
	actor := application.Actor{UserID: 1, Role: domain.RoleConsumer}

	before, err := service.GetCart(context.Background(), actor)
	if err != nil {
		t.Fatalf("get cart before create order: %v", err)
	}
	if len(before.Items) == 0 {
		t.Fatal("expected seeded selected cart item")
	}

	_, err = service.CreateOrder(context.Background(), actor, "explicit-items-no-cart-delete", application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: 3, Quantity: 1}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
	})
	if err != nil {
		t.Fatalf("create order with explicit items: %v", err)
	}

	after, err := service.GetCart(context.Background(), actor)
	if err != nil {
		t.Fatalf("get cart after create order: %v", err)
	}
	if len(after.Items) != len(before.Items) || after.Items[0].ID != before.Items[0].ID {
		t.Fatalf("expected explicit-item create order to keep cart unchanged, before=%+v after=%+v", before.Items, after.Items)
	}
}
