package application_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	application "github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	postgresrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres"
)

func TestPostgresApplicationAuthCartCheckoutAndOrderLifecycle(t *testing.T) {
	repo, service := newPostgresApplicationService(t)
	ctx := context.Background()
	consumer := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}
	_, skuID := createPostgresApplicationProductAndSKU(t, repo, 10)
	if err := repo.DeleteSelectedCartItems(consumer.UserID); err != nil {
		t.Fatalf("clear selected cart items: %v", err)
	}

	session, err := service.Login(ctx, application.LoginInput{Phone: "13800000001", Password: "consumer-demo"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := service.Me(ctx, session.Token); err != nil {
		t.Fatalf("me: %v", err)
	}

	cartItem, err := service.AddCartItem(ctx, consumer, application.CartItemInput{SKUID: skuID, Quantity: 2})
	if err != nil {
		t.Fatalf("add cart item: %v", err)
	}
	selected := false
	if _, err := service.UpdateCartItem(ctx, consumer, cartItem.ID, application.CartItemUpdateInput{Selected: &selected}); err != nil {
		t.Fatalf("unselect cart item: %v", err)
	}
	selected = true
	if _, err := service.UpdateCartItem(ctx, consumer, cartItem.ID, application.CartItemUpdateInput{Quantity: 1, Selected: &selected}); err != nil {
		t.Fatalf("select cart item: %v", err)
	}
	cart, err := service.GetCart(ctx, consumer)
	if err != nil {
		t.Fatalf("get cart: %v", err)
	}
	if cart.SelectedItemCount == 0 || cart.SelectedQuantity != 1 {
		t.Fatalf("expected selected cart item, got %+v", cart)
	}

	preview, err := service.PreviewOrder(ctx, consumer, application.CheckoutInput{})
	if err != nil {
		t.Fatalf("preview order: %v", err)
	}
	if !preview.StockOK || len(preview.Items) == 0 {
		t.Fatalf("expected stock-ok preview from selected cart, got %+v", preview)
	}
	if len(preview.Items) != 1 || preview.Items[0].SKUID != skuID {
		t.Fatalf("expected preview from isolated selected cart item, got %+v", preview.Items)
	}

	order, err := service.CreateOrder(ctx, consumer, fmt.Sprintf("app-pg-lifecycle-%d", time.Now().UnixNano()), application.CheckoutInput{
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Status != string(orderdomain.StatusCreated) || len(order.InventoryLocks) == 0 || len(order.Events) == 0 {
		t.Fatalf("expected created order with locks and events, got %+v", order)
	}

	paid, err := service.PayOrder(ctx, consumer, order.ID)
	if err != nil {
		t.Fatalf("pay order: %v", err)
	}
	if paid.Status != string(orderdomain.StatusPaid) {
		t.Fatalf("expected paid order, got %+v", paid)
	}
	shipped, err := service.MerchantShipOrder(ctx, merchant, order.ID, application.MerchantOrderShipInput{LogisticsNo: "SF123"})
	if err != nil {
		t.Fatalf("ship order: %v", err)
	}
	if shipped.Status != string(orderdomain.StatusShipped) {
		t.Fatalf("expected shipped order, got %+v", shipped)
	}
	finished, err := service.FinishOrder(ctx, consumer, order.ID)
	if err != nil {
		t.Fatalf("finish order: %v", err)
	}
	if finished.Status != string(orderdomain.StatusFinished) {
		t.Fatalf("expected finished order, got %+v", finished)
	}
}

func TestPostgresApplicationRefundReturnsInventory(t *testing.T) {
	repo, service := newPostgresApplicationService(t)
	ctx := context.Background()
	consumer := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}
	_, skuID := createPostgresApplicationProductAndSKU(t, repo, 10)
	before, ok := repo.GetSKU(skuID)
	if !ok {
		t.Fatal("expected sku before order")
	}

	order, err := service.CreateOrder(ctx, consumer, fmt.Sprintf("app-pg-refund-%d", time.Now().UnixNano()), application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: skuID, Quantity: 1}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Hangzhou",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if _, err := service.PayOrder(ctx, consumer, order.ID); err != nil {
		t.Fatalf("pay order: %v", err)
	}
	if _, err := service.RequestRefund(ctx, consumer, order.ID, application.RefundRequestInput{Reason: "no longer needed"}); err != nil {
		t.Fatalf("request refund: %v", err)
	}
	refunded, err := service.MerchantApproveRefund(ctx, merchant, order.ID)
	if err != nil {
		t.Fatalf("approve refund: %v", err)
	}
	if refunded.Status != string(orderdomain.StatusRefunded) {
		t.Fatalf("expected refunded status, got %s", refunded.Status)
	}

	after, ok := repo.GetSKU(skuID)
	if !ok {
		t.Fatal("expected sku after refund")
	}
	if after.Stock != before.Stock || after.LockedStock != before.LockedStock {
		t.Fatalf("expected inventory restored, before=%+v after=%+v", before, after)
	}
}

func TestPostgresApplicationMerchantDashboardAndAI(t *testing.T) {
	repo, service := newPostgresApplicationService(t)
	ctx := context.Background()
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}
	consumer := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	productID, skuID := createPostgresApplicationProductAndSKU(t, repo, 10)

	if _, err := service.GetProduct(ctx, productID, &consumer); err != nil {
		t.Fatalf("get product: %v", err)
	}
	if _, err := service.AddCartItem(ctx, consumer, application.CartItemInput{SKUID: skuID, Quantity: 1}); err != nil {
		t.Fatalf("add cart item: %v", err)
	}
	order, err := service.CreateOrder(ctx, consumer, fmt.Sprintf("app-pg-dashboard-%d", time.Now().UnixNano()), application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: skuID, Quantity: 1}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if _, err := service.PayOrder(ctx, consumer, order.ID); err != nil {
		t.Fatalf("pay order: %v", err)
	}

	funnel, err := service.DashboardFunnel(ctx, merchant)
	if err != nil {
		t.Fatalf("dashboard funnel: %v", err)
	}
	if funnel.ProductClicks == 0 || funnel.AddToCart == 0 || funnel.OrderCreate == 0 || funnel.OrderPay == 0 {
		t.Fatalf("expected dashboard events, got %+v", funnel)
	}
	summary, err := service.DashboardSummary(ctx, merchant)
	if err != nil {
		t.Fatalf("dashboard summary: %v", err)
	}
	if summary.ProductCount == 0 || summary.PaidOrderCount == 0 {
		t.Fatalf("expected dashboard summary, got %+v", summary)
	}
	products, err := service.DashboardProducts(ctx, merchant)
	if err != nil {
		t.Fatalf("dashboard products: %v", err)
	}
	if len(products) == 0 {
		t.Fatal("expected dashboard product stats")
	}

	task, err := service.GenerateSellingPoints(ctx, merchant, application.SellingPointInput{
		ProductName: "Travel Makeup Organizer",
		Attributes:  []string{"portable"},
		TargetUsers: "dorm users",
		PriceCent:   8900,
	})
	if err != nil {
		t.Fatalf("generate selling points: %v", err)
	}
	if task.Status != domain.AITaskStatusCompleted || len(task.Output) == 0 {
		t.Fatalf("expected completed ai task, got %+v", task)
	}
	if _, err := service.GetAITask(ctx, application.Actor{UserID: 99, Role: domain.RoleConsumer}, task.ID); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected not found for cross-user ai task read, got %v", err)
	}
}

// newPostgresApplicationService 构造接入真实 Postgres 的应用服务；
// 未设置 RUN_POSTGRES_INTEGRATION 或 POSTGRES_DSN 时跳过（回归测试在同一包内复用）。
func newPostgresApplicationService(t *testing.T) (*postgresrepo.Repository, *application.Service) {
	t.Helper()
	if os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("RUN_POSTGRES_INTEGRATION is not set")
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN is not set")
	}
	repo, err := postgresrepo.NewRepository(dsn)
	if err != nil {
		t.Fatalf("new postgres repository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo, application.NewService(repo, backendai.MockProvider{})
}

// createPostgresApplicationProductAndSKU 造数：创建一个在线商品及其可用库存为 stock 的 SKU。
func createPostgresApplicationProductAndSKU(t *testing.T, repo *postgresrepo.Repository, stock int) (int64, int64) {
	t.Helper()
	now := time.Now().UTC()
	product, err := repo.SaveProduct(domain.Product{
		MerchantID:    1,
		Title:         fmt.Sprintf("Application PG Product %d", now.UnixNano()),
		Description:   "postgres application integration product",
		CategoryID:    20260617,
		Status:        domain.ProductStatusOnline,
		SellingPoints: []string{"postgres", "application"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("save product: %v", err)
	}
	sku, err := repo.SaveSKU(domain.SKU{
		ProductID:   product.ID,
		SKUName:     fmt.Sprintf("Application PG SKU %d", now.UnixNano()),
		SKUAttrs:    map[string]string{"size": "standard"},
		PriceCent:   12345,
		Stock:       stock,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("save sku: %v", err)
	}
	return product.ID, sku.ID
}

func isAppError(err error, kind application.ErrorKind) bool {
	var appErr *application.AppError
	return errors.As(err, &appErr) && appErr.Kind == kind
}
