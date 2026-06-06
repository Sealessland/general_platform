package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	application "github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
)

func TestCreateOrderIsIdempotent(t *testing.T) {
	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
	actor := application.Actor{UserID: 1, Role: "consumer"}

	input := application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: 1, Quantity: 1}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
	}

	first, err := service.CreateOrder(context.Background(), actor, "dup-key", input)
	if err != nil {
		t.Fatalf("create first order: %v", err)
	}
	second, err := service.CreateOrder(context.Background(), actor, "dup-key", input)
	if err != nil {
		t.Fatalf("create second order: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected idempotent order id %d, got %d", first.ID, second.ID)
	}
}

func TestRefundReturnsInventory(t *testing.T) {
	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
	consumer := application.Actor{UserID: 1, Role: "consumer"}
	merchant := application.Actor{UserID: 2, Role: "merchant", MerchantID: 1}

	before, ok := repo.GetSKU(3)
	if !ok {
		t.Fatal("expected seeded sku")
	}

	order, err := service.CreateOrder(context.Background(), consumer, "refund-key", application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: 3, Quantity: 1}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Hangzhou",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	if _, err := service.PayOrder(context.Background(), consumer, order.ID); err != nil {
		t.Fatalf("pay order: %v", err)
	}
	if _, err := service.RequestRefund(context.Background(), consumer, order.ID, application.RefundRequestInput{Reason: "no longer needed"}); err != nil {
		t.Fatalf("request refund: %v", err)
	}
	refunded, err := service.MerchantApproveRefund(context.Background(), merchant, order.ID)
	if err != nil {
		t.Fatalf("approve refund: %v", err)
	}
	if refunded.Status != string(orderdomain.StatusRefunded) {
		t.Fatalf("expected refunded status, got %s", refunded.Status)
	}

	after, ok := repo.GetSKU(3)
	if !ok {
		t.Fatal("expected seeded sku after refund")
	}
	if before.Stock != after.Stock {
		t.Fatalf("expected stock restored to %d, got %d", before.Stock, after.Stock)
	}
	if after.LockedStock != 0 {
		t.Fatalf("expected locked stock reset, got %d", after.LockedStock)
	}
}

func TestConsumerCannotGenerateAISellingPoints(t *testing.T) {
	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
	consumer := application.Actor{UserID: 1, Role: domain.RoleConsumer}

	_, err := service.GenerateSellingPoints(context.Background(), consumer, application.SellingPointInput{
		ProductName: "Travel Makeup Organizer",
		Attributes:  []string{"portable"},
		TargetUsers: "dorm users",
		PriceCent:   8900,
	})
	if !isAppError(err, application.ErrorForbidden) {
		t.Fatalf("expected forbidden for consumer AI generation, got %v", err)
	}
}

func TestConsumerCannotReadAnotherConsumersLegacyAITask(t *testing.T) {
	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
	now := time.Now().UTC()
	task, err := repo.CreateAITask(domain.AIGenerationTask{
		UserID:     1,
		MerchantID: 0,
		TaskType:   domain.TaskTypeSellingPoints,
		Input:      map[string]any{"product_name": "legacy task"},
		Output:     map[string]any{"core_points": []string{"legacy"}},
		Status:     domain.AITaskStatusCompleted,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("create legacy ai task: %v", err)
	}

	_, err = service.GetAITask(context.Background(), application.Actor{UserID: 99, Role: domain.RoleConsumer}, task.ID)
	if !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected not found for cross-consumer ai task read, got %v", err)
	}
}

func isAppError(err error, kind application.ErrorKind) bool {
	var appErr *application.AppError
	return errors.As(err, &appErr) && appErr.Kind == kind
}
