package application_test

import (
	"context"
	"testing"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	application "github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
)

func TestMerchantProductAndDashboardBoundaries(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})
	consumer := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}
	otherMerchant := application.Actor{UserID: 3, Role: domain.RoleMerchant, MerchantID: 99}

	if _, err := service.MerchantCreateProduct(context.Background(), consumer, application.MerchantProductInput{Title: "Nope"}); !isAppError(err, application.ErrorForbidden) {
		t.Fatalf("expected consumer create product forbidden, got %v", err)
	}
	if _, err := service.MerchantCreateProduct(context.Background(), merchant, application.MerchantProductInput{}); !isAppError(err, application.ErrorInvalidArgument) {
		t.Fatalf("expected missing title invalid, got %v", err)
	}

	product, err := service.MerchantCreateProduct(context.Background(), merchant, application.MerchantProductInput{
		Title:         "Dashboard Product",
		Description:   "for dashboard test",
		CategoryID:    42,
		SellingPoints: []string{"fast"},
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	if _, err := service.MerchantUpdateProduct(context.Background(), otherMerchant, product.ID, application.MerchantProductInput{Title: "steal"}); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected other merchant product not found, got %v", err)
	}
	if _, err := service.MerchantCreateSKU(context.Background(), merchant, product.ID, application.MerchantSKUInput{SKUName: "bad", PriceCent: 0, Stock: 1}); !isAppError(err, application.ErrorInvalidArgument) {
		t.Fatalf("expected bad sku invalid, got %v", err)
	}
	sku, err := service.MerchantCreateSKU(context.Background(), merchant, product.ID, application.MerchantSKUInput{SKUName: "Standard", PriceCent: 9900, Stock: 5})
	if err != nil {
		t.Fatalf("create sku: %v", err)
	}
	if _, err := service.MerchantUpdateSKU(context.Background(), otherMerchant, sku.ID, application.MerchantSKUInput{Stock: 99}); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected other merchant sku not found, got %v", err)
	}
	if _, err := service.DashboardSummary(context.Background(), consumer); !isAppError(err, application.ErrorForbidden) {
		t.Fatalf("expected consumer dashboard forbidden, got %v", err)
	}

	summary, err := service.DashboardSummary(context.Background(), merchant)
	if err != nil {
		t.Fatalf("dashboard summary: %v", err)
	}
	if summary.ProductCount == 0 || summary.InventoryWarningSKU == 0 {
		t.Fatalf("expected dashboard product and low-stock sku counts, got %+v", summary)
	}
}

func TestCatalogListOrdersAndDashboardMetrics(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})
	consumer := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}

	notes, err := service.ListNotes(context.Background())
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if len(notes) == 0 || len(notes[0].LinkedProducts) == 0 {
		t.Fatalf("expected notes with linked products, got %+v", notes)
	}
	noteBefore := notes[0].ViewCount
	detail, err := service.GetNote(context.Background(), notes[0].ID, &consumer)
	if err != nil {
		t.Fatalf("get note: %v", err)
	}
	if detail.ViewCount != noteBefore+1 || len(detail.LinkedProducts) == 0 {
		t.Fatalf("expected viewed note with linked products, got %+v", detail)
	}
	if _, err := service.GetNote(context.Background(), 999999, &consumer); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected missing note not found, got %v", err)
	}

	products, err := service.ListProducts(context.Background())
	if err != nil {
		t.Fatalf("list products: %v", err)
	}
	if len(products) == 0 || products[0].MinPriceCent == 0 || products[0].Stock == 0 {
		t.Fatalf("expected online product cards with price and stock, got %+v", products)
	}
	product, err := service.GetProduct(context.Background(), products[0].ID, &consumer)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if len(product.SKUs) == 0 {
		t.Fatalf("expected product skus, got %+v", product)
	}
	skus, err := service.ListProductSKUs(context.Background(), product.ID)
	if err != nil {
		t.Fatalf("list skus: %v", err)
	}
	if len(skus) == 0 {
		t.Fatal("expected skus")
	}
	if _, err := service.GetProduct(context.Background(), 999999, nil); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected missing product not found, got %v", err)
	}
	if _, err := service.ListProductSKUs(context.Background(), 999999); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected missing product skus not found, got %v", err)
	}

	orders, err := service.ListOrders(context.Background(), consumer)
	if err != nil {
		t.Fatalf("list consumer orders: %v", err)
	}
	if len(orders) == 0 {
		t.Fatal("expected seeded consumer orders")
	}
	merchantProducts, err := service.MerchantListProducts(context.Background(), merchant)
	if err != nil {
		t.Fatalf("merchant list products: %v", err)
	}
	if len(merchantProducts) == 0 {
		t.Fatal("expected merchant products")
	}
	merchantOrders, err := service.MerchantListOrders(context.Background(), merchant)
	if err != nil {
		t.Fatalf("merchant list orders: %v", err)
	}
	if len(merchantOrders) == 0 {
		t.Fatal("expected merchant orders")
	}

	funnel, err := service.DashboardFunnel(context.Background(), merchant)
	if err != nil {
		t.Fatalf("dashboard funnel: %v", err)
	}
	if funnel.NoteViews == 0 || funnel.ProductClicks == 0 || funnel.AddToCart == 0 {
		t.Fatalf("expected seeded funnel metrics, got %+v", funnel)
	}
	productStats, err := service.DashboardProducts(context.Background(), merchant)
	if err != nil {
		t.Fatalf("dashboard products: %v", err)
	}
	if len(productStats) == 0 || productStats[0].AvailableStock == 0 {
		t.Fatalf("expected dashboard product stats, got %+v", productStats)
	}
}
