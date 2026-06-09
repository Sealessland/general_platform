package application_test

import (
	"context"
	"testing"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	application "github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
)

func TestCheckoutValidationRejectsInvalidItems(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})
	actor := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	actorWithoutCart := application.Actor{UserID: 99, Role: domain.RoleConsumer}
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}

	if _, err := service.PreviewOrder(context.Background(), actorWithoutCart, application.CheckoutInput{}); !isAppError(err, application.ErrorInvalidArgument) {
		t.Fatalf("expected empty checkout without selected cart to be invalid, got %v", err)
	}

	cases := []struct {
		name string
		in   application.CheckoutInput
		kind application.ErrorKind
	}{
		{name: "zero quantity", in: application.CheckoutInput{Items: []application.OrderLineInput{{SKUID: 1, Quantity: 0}}}, kind: application.ErrorInvalidArgument},
		{name: "missing sku", in: application.CheckoutInput{Items: []application.OrderLineInput{{SKUID: 999999, Quantity: 1}}}, kind: application.ErrorNotFound},
		{name: "insufficient stock", in: application.CheckoutInput{Items: []application.OrderLineInput{{SKUID: 1, Quantity: 999999}}}, kind: application.ErrorConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.PreviewOrder(context.Background(), actor, tc.in)
			if !isAppError(err, tc.kind) {
				t.Fatalf("expected %s, got %v", tc.kind, err)
			}
		})
	}

	product, err := service.MerchantCreateProduct(context.Background(), merchant, application.MerchantProductInput{
		Title:      "Offline Product",
		CategoryID: 10,
	})
	if err != nil {
		t.Fatalf("create draft product: %v", err)
	}
	sku, err := service.MerchantCreateSKU(context.Background(), merchant, product.ID, application.MerchantSKUInput{
		SKUName:   "Draft SKU",
		PriceCent: 100,
		Stock:     10,
	})
	if err != nil {
		t.Fatalf("create sku: %v", err)
	}
	if _, err := service.PreviewOrder(context.Background(), actor, application.CheckoutInput{Items: []application.OrderLineInput{{SKUID: sku.ID, Quantity: 1}}}); !isAppError(err, application.ErrorConflict) {
		t.Fatalf("expected offline product conflict, got %v", err)
	}

	if _, err := service.MerchantSetProductStatus(context.Background(), merchant, product.ID, domain.ProductStatusOnline); err != nil {
		t.Fatalf("online product: %v", err)
	}
	if _, err := service.MerchantUpdateSKU(context.Background(), merchant, sku.ID, application.MerchantSKUInput{Status: domain.SKUStatusInactive}); err != nil {
		t.Fatalf("inactive sku: %v", err)
	}
	if _, err := service.PreviewOrder(context.Background(), actor, application.CheckoutInput{Items: []application.OrderLineInput{{SKUID: sku.ID, Quantity: 1}}}); !isAppError(err, application.ErrorConflict) {
		t.Fatalf("expected inactive sku conflict, got %v", err)
	}
}

func TestOrderStateAndAuthorizationBoundaries(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})
	owner := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	otherConsumer := application.Actor{UserID: 99, Role: domain.RoleConsumer}
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}
	otherMerchant := application.Actor{UserID: 200, Role: domain.RoleMerchant, MerchantID: 200}

	order, err := service.CreateOrder(context.Background(), owner, "state-boundary", application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: 1, Quantity: 1}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if _, err := service.GetOrder(context.Background(), otherConsumer, order.ID); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected other consumer not found, got %v", err)
	}
	if _, err := service.MerchantShipOrder(context.Background(), otherMerchant, order.ID, application.MerchantOrderShipInput{LogisticsNo: "X"}); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected other merchant not found, got %v", err)
	}
	if _, err := service.MerchantShipOrder(context.Background(), merchant, order.ID, application.MerchantOrderShipInput{LogisticsNo: "EARLY"}); !isAppError(err, application.ErrorConflict) {
		t.Fatalf("expected ship before pay conflict, got %v", err)
	}

	if _, err := service.PayOrder(context.Background(), owner, order.ID); err != nil {
		t.Fatalf("pay order: %v", err)
	}
	if _, err := service.CancelOrder(context.Background(), owner, order.ID); !isAppError(err, application.ErrorConflict) {
		t.Fatalf("expected cancel paid conflict, got %v", err)
	}
	if _, err := service.MerchantShipOrder(context.Background(), merchant, order.ID, application.MerchantOrderShipInput{LogisticsNo: "SF123"}); err != nil {
		t.Fatalf("ship order: %v", err)
	}
	if _, err := service.FinishOrder(context.Background(), otherConsumer, order.ID); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected other consumer finish not found, got %v", err)
	}
	if _, err := service.FinishOrder(context.Background(), owner, order.ID); err != nil {
		t.Fatalf("finish order: %v", err)
	}
	if _, err := service.RequestRefund(context.Background(), owner, order.ID, application.RefundRequestInput{Reason: "too late"}); !isAppError(err, application.ErrorConflict) {
		t.Fatalf("expected refund finished conflict, got %v", err)
	}

	cancelOrder, err := service.CreateOrder(context.Background(), owner, "cancel-success", application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: 3, Quantity: 1}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Hangzhou",
	})
	if err != nil {
		t.Fatalf("create cancel order: %v", err)
	}
	cancelled, err := service.CancelOrder(context.Background(), owner, cancelOrder.ID)
	if err != nil {
		t.Fatalf("cancel order: %v", err)
	}
	if cancelled.Status != "CANCELLED" || len(cancelled.InventoryLocks) == 0 || cancelled.InventoryLocks[0].Status != domain.InventoryLockStatusReleased {
		t.Fatalf("expected cancelled order with released inventory, got %+v", cancelled)
	}
}

func TestHighValueOrderActionsAreIdempotentAfterSuccess(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})
	owner := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}

	order, err := service.CreateOrder(context.Background(), owner, "idempotent-order-actions", application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: 1, Quantity: 1}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	paid, err := service.PayOrder(context.Background(), owner, order.ID)
	if err != nil {
		t.Fatalf("pay order: %v", err)
	}
	paidAgain, err := service.PayOrder(context.Background(), owner, order.ID)
	if err != nil {
		t.Fatalf("repeat pay order: %v", err)
	}
	if paidAgain.Status != "PAID" || paidAgain.ID != paid.ID {
		t.Fatalf("expected idempotent paid order view, got %+v", paidAgain)
	}

	shipped, err := service.MerchantShipOrder(context.Background(), merchant, order.ID, application.MerchantOrderShipInput{LogisticsNo: "SF123"})
	if err != nil {
		t.Fatalf("ship order: %v", err)
	}
	shippedAgain, err := service.MerchantShipOrder(context.Background(), merchant, order.ID, application.MerchantOrderShipInput{LogisticsNo: "SF123"})
	if err != nil {
		t.Fatalf("repeat ship order: %v", err)
	}
	if shippedAgain.Status != "SHIPPED" || shippedAgain.ID != shipped.ID {
		t.Fatalf("expected idempotent shipped order view, got %+v", shippedAgain)
	}

	finished, err := service.FinishOrder(context.Background(), owner, order.ID)
	if err != nil {
		t.Fatalf("finish order: %v", err)
	}
	finishedAgain, err := service.FinishOrder(context.Background(), owner, order.ID)
	if err != nil {
		t.Fatalf("repeat finish order: %v", err)
	}
	if finishedAgain.Status != "FINISHED" || finishedAgain.ID != finished.ID {
		t.Fatalf("expected idempotent finished order view, got %+v", finishedAgain)
	}

	refundOrder, err := service.CreateOrder(context.Background(), owner, "idempotent-refund-actions", application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: 3, Quantity: 1}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Hangzhou",
	})
	if err != nil {
		t.Fatalf("create refund order: %v", err)
	}
	if _, err := service.PayOrder(context.Background(), owner, refundOrder.ID); err != nil {
		t.Fatalf("pay refund order: %v", err)
	}
	refunding, err := service.RequestRefund(context.Background(), owner, refundOrder.ID, application.RefundRequestInput{Reason: "size mismatch"})
	if err != nil {
		t.Fatalf("request refund: %v", err)
	}
	refundingAgain, err := service.RequestRefund(context.Background(), owner, refundOrder.ID, application.RefundRequestInput{Reason: "size mismatch"})
	if err != nil {
		t.Fatalf("repeat refund request: %v", err)
	}
	if refundingAgain.Status != "REFUNDING" || refundingAgain.ID != refunding.ID {
		t.Fatalf("expected idempotent refunding order view, got %+v", refundingAgain)
	}

	refunded, err := service.MerchantApproveRefund(context.Background(), merchant, refundOrder.ID)
	if err != nil {
		t.Fatalf("approve refund: %v", err)
	}
	refundedAgain, err := service.MerchantApproveRefund(context.Background(), merchant, refundOrder.ID)
	if err != nil {
		t.Fatalf("repeat approve refund: %v", err)
	}
	if refundedAgain.Status != "REFUNDED" || refundedAgain.ID != refunded.ID {
		t.Fatalf("expected idempotent refunded order view, got %+v", refundedAgain)
	}
}
