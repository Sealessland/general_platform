package application_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	application "github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func TestPostgresApplicationAuthSessionAndCatalogRegression(t *testing.T) {
	_, service := newPostgresApplicationService(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	if _, err := service.Register(ctx, application.RegisterInput{Phone: "139" + suffix[len(suffix)-8:], Password: "secret", Role: domain.RoleConsumer}); !isAppError(err, application.ErrorInvalidArgument) {
		t.Fatalf("expected missing nickname invalid argument, got %v", err)
	}
	if _, err := service.Register(ctx, application.RegisterInput{Nickname: "Bad Role", Phone: "137" + suffix[len(suffix)-8:], Password: "secret", Role: "admin"}); !isAppError(err, application.ErrorInvalidArgument) {
		t.Fatalf("expected invalid role, got %v", err)
	}

	phone := "136" + suffix[len(suffix)-8:]
	session, err := service.Register(ctx, application.RegisterInput{
		Nickname: "PG Regression Merchant",
		Phone:    phone,
		Password: "secret",
		Role:     domain.RoleMerchant,
	})
	if err != nil {
		t.Fatalf("register merchant: %v", err)
	}
	if session.Token == "" || session.RefreshToken == "" || session.User.MerchantID == 0 || session.User.Merchant == nil {
		t.Fatalf("expected merchant session with merchant view, got %+v", session)
	}
	if _, err := service.Register(ctx, application.RegisterInput{Nickname: "Duplicate", Phone: phone, Password: "secret", Role: domain.RoleMerchant}); !isAppError(err, application.ErrorConflict) {
		t.Fatalf("expected duplicate phone conflict, got %v", err)
	}
	if _, err := service.Login(ctx, application.LoginInput{Phone: phone, Password: "wrong"}); !isAppError(err, application.ErrorUnauthorized) {
		t.Fatalf("expected wrong password unauthorized, got %v", err)
	}

	actor, err := service.Authenticate(session.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if actor.Role != domain.RoleMerchant || actor.MerchantID != session.User.MerchantID {
		t.Fatalf("expected merchant actor, got %+v", actor)
	}
	refreshed, err := service.RefreshSession(ctx, session.RefreshToken)
	if err != nil {
		t.Fatalf("refresh session: %v", err)
	}
	if refreshed.Token == session.Token || refreshed.RefreshToken == session.RefreshToken {
		t.Fatal("expected refresh to rotate both tokens")
	}
	if _, err := service.Authenticate(session.Token); !isAppError(err, application.ErrorUnauthorized) {
		t.Fatalf("expected old token unauthorized, got %v", err)
	}
	if err := service.Logout(ctx, refreshed.Token); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := service.Me(ctx, refreshed.Token); !isAppError(err, application.ErrorUnauthorized) {
		t.Fatalf("expected logged out token unauthorized, got %v", err)
	}

	notes, err := service.ListNotes(ctx)
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if len(notes) == 0 || len(notes[0].LinkedProducts) == 0 {
		t.Fatalf("expected seeded notes with linked products, got %+v", notes)
	}
	consumer := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	note, err := service.GetNote(ctx, notes[0].ID, &consumer)
	if err != nil {
		t.Fatalf("get note: %v", err)
	}
	if note.ViewCount <= notes[0].ViewCount {
		t.Fatalf("expected note view count to increase, before=%d after=%d", notes[0].ViewCount, note.ViewCount)
	}
	products, err := service.ListProducts(ctx)
	if err != nil {
		t.Fatalf("list products: %v", err)
	}
	if len(products) == 0 {
		t.Fatal("expected online products")
	}
	if _, err := service.ListProductSKUs(ctx, products[0].ID); err != nil {
		t.Fatalf("list product skus: %v", err)
	}
	if _, err := service.ListProductSKUs(ctx, 999999999); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected missing product not found, got %v", err)
	}
}

func TestPostgresApplicationMerchantCatalogOrderAndAIRegression(t *testing.T) {
	_, service := newPostgresApplicationService(t)
	ctx := context.Background()
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}
	consumer := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	otherConsumer := application.Actor{UserID: 99, Role: domain.RoleConsumer}
	otherMerchant := application.Actor{UserID: 200, Role: domain.RoleMerchant, MerchantID: 200}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	if _, err := service.MerchantCreateProduct(ctx, consumer, application.MerchantProductInput{Title: "Forbidden"}); !isAppError(err, application.ErrorForbidden) {
		t.Fatalf("expected consumer merchant create forbidden, got %v", err)
	}
	if _, err := service.MerchantCreateProduct(ctx, merchant, application.MerchantProductInput{}); !isAppError(err, application.ErrorInvalidArgument) {
		t.Fatalf("expected missing title invalid argument, got %v", err)
	}
	product, err := service.MerchantCreateProduct(ctx, merchant, application.MerchantProductInput{
		Title:         "PG Regression Product " + suffix,
		Description:   "created by postgres regression test",
		CategoryID:    20260617,
		SellingPoints: []string{"postgres", "regression"},
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	updated, err := service.MerchantUpdateProduct(ctx, merchant, product.ID, application.MerchantProductInput{
		Title:         product.Title + " Updated",
		Description:   "updated",
		CategoryID:    20260618,
		SellingPoints: []string{"updated"},
	})
	if err != nil {
		t.Fatalf("update product: %v", err)
	}
	if !strings.Contains(updated.Title, "Updated") {
		t.Fatalf("expected updated title, got %+v", updated)
	}
	if _, err := service.MerchantCreateSKU(ctx, merchant, product.ID, application.MerchantSKUInput{PriceCent: 0, Stock: 1}); !isAppError(err, application.ErrorInvalidArgument) {
		t.Fatalf("expected invalid sku input, got %v", err)
	}
	sku, err := service.MerchantCreateSKU(ctx, merchant, product.ID, application.MerchantSKUInput{
		SKUName:   "PG Regression SKU",
		SKUAttrs:  map[string]string{"color": "red"},
		PriceCent: 1500,
		Stock:     4,
	})
	if err != nil {
		t.Fatalf("create sku: %v", err)
	}
	sku, err = service.MerchantUpdateSKU(ctx, merchant, sku.ID, application.MerchantSKUInput{
		SKUName:   "PG Regression SKU Updated",
		SKUAttrs:  map[string]string{"color": "blue"},
		PriceCent: 1800,
		Stock:     4,
		Status:    domain.SKUStatusActive,
	})
	if err != nil {
		t.Fatalf("update sku: %v", err)
	}
	if sku.PriceCent != 1800 || sku.SKUAttrs["color"] != "blue" {
		t.Fatalf("expected updated sku, got %+v", sku)
	}
	if _, err := service.MerchantSetProductStatus(ctx, merchant, product.ID, domain.ProductStatusOnline); err != nil {
		t.Fatalf("online product: %v", err)
	}
	merchantProducts, err := service.MerchantListProducts(ctx, merchant)
	if err != nil {
		t.Fatalf("merchant list products: %v", err)
	}
	if len(merchantProducts) == 0 {
		t.Fatal("expected merchant products")
	}

	order, err := service.CreateOrder(ctx, consumer, "pg-regression-order-"+suffix, application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: sku.ID, Quantity: 1}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if _, err := service.GetOrder(ctx, otherConsumer, order.ID); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected other consumer not found, got %v", err)
	}
	if _, err := service.MerchantShipOrder(ctx, otherMerchant, order.ID, application.MerchantOrderShipInput{LogisticsNo: "NOPE"}); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected other merchant not found, got %v", err)
	}
	if _, err := service.MerchantShipOrder(ctx, merchant, order.ID, application.MerchantOrderShipInput{LogisticsNo: "EARLY"}); !isAppError(err, application.ErrorConflict) {
		t.Fatalf("expected ship before pay conflict, got %v", err)
	}
	if _, err := service.PayOrder(ctx, consumer, order.ID); err != nil {
		t.Fatalf("pay order: %v", err)
	}
	paidAgain, err := service.PayOrder(ctx, consumer, order.ID)
	if err != nil {
		t.Fatalf("repeat pay order: %v", err)
	}
	if paidAgain.Status != "PAID" {
		t.Fatalf("expected idempotent paid status, got %+v", paidAgain)
	}
	orders, err := service.ListOrders(ctx, consumer)
	if err != nil {
		t.Fatalf("list orders: %v", err)
	}
	if len(orders) == 0 {
		t.Fatal("expected consumer orders")
	}
	merchantOrders, err := service.MerchantListOrders(ctx, merchant)
	if err != nil {
		t.Fatalf("merchant list orders: %v", err)
	}
	if len(merchantOrders) == 0 {
		t.Fatal("expected merchant orders")
	}

	review, err := service.GenerateBusinessReview(ctx, merchant, application.BusinessReviewInput{WindowDays: 7, ProductID: product.ID})
	if err != nil {
		t.Fatalf("generate business review: %v", err)
	}
	if review.Status != domain.AITaskStatusCompleted || review.Output["diagnosis"] == nil {
		t.Fatalf("expected completed business review, got %+v", review)
	}
	a2ui, err := service.GenerateA2UISurface(ctx, consumer, application.A2UISurfaceInput{
		SurfaceID:   "shopping-guide",
		UserIntent:  "300元通勤收纳",
		ContextJSON: `{"scene":"office_desk"}`,
	})
	if err != nil {
		t.Fatalf("generate a2ui: %v", err)
	}
	if a2ui.SurfaceID == "" || a2ui.A2UIJSON == "" {
		t.Fatalf("expected a2ui payload, got %+v", a2ui)
	}
	if _, err := service.GenerateA2UISurface(ctx, consumer, application.A2UISurfaceInput{ContextJSON: "{"}); !isAppError(err, application.ErrorInvalidArgument) {
		t.Fatalf("expected invalid a2ui context error, got %v", err)
	}
}

func TestPostgresApplicationCartCancelAndA2UIRegression(t *testing.T) {
	repo, service := newPostgresApplicationService(t)
	ctx := context.Background()
	consumer := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	_, skuID := createPostgresApplicationProductAndSKU(t, repo, 3)
	if err := repo.DeleteSelectedCartItems(consumer.UserID); err != nil {
		t.Fatalf("clear selected cart items: %v", err)
	}

	cartItem, err := service.AddCartItem(ctx, consumer, application.CartItemInput{SKUID: skuID, Quantity: 1})
	if err != nil {
		t.Fatalf("add cart item: %v", err)
	}
	if err := service.DeleteCartItem(ctx, consumer, cartItem.ID); err != nil {
		t.Fatalf("delete cart item: %v", err)
	}
	if err := service.DeleteCartItem(ctx, consumer, cartItem.ID); !isAppError(err, application.ErrorNotFound) {
		t.Fatalf("expected deleted cart item not found, got %v", err)
	}

	order, err := service.CreateOrder(ctx, consumer, "pg-regression-cancel-"+fmt.Sprintf("%d", time.Now().UnixNano()), application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: skuID, Quantity: 1}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	fetched, err := service.GetOrder(ctx, consumer, order.ID)
	if err != nil {
		t.Fatalf("get order: %v", err)
	}
	if fetched.ID != order.ID || len(fetched.InventoryLocks) == 0 {
		t.Fatalf("expected enriched order view, got %+v", fetched)
	}
	cancelled, err := service.CancelOrder(ctx, consumer, order.ID)
	if err != nil {
		t.Fatalf("cancel order: %v", err)
	}
	if cancelled.Status != "CANCELLED" || len(cancelled.InventoryLocks) == 0 || cancelled.InventoryLocks[0].Status != domain.InventoryLockStatusReleased {
		t.Fatalf("expected cancelled order with released lock, got %+v", cancelled)
	}
	cancelledAgain, err := service.CancelOrder(ctx, consumer, order.ID)
	if err != nil {
		t.Fatalf("repeat cancel order: %v", err)
	}
	if cancelledAgain.Status != "CANCELLED" {
		t.Fatalf("expected idempotent cancelled order, got %+v", cancelledAgain)
	}

	for _, intent := range []string{"300元宿舍收纳", "300元出行收纳", "300元普通收纳"} {
		view, err := service.GenerateA2UISurface(ctx, consumer, application.A2UISurfaceInput{
			SurfaceID:  "shopping-guide",
			UserIntent: intent,
		})
		if err != nil {
			t.Fatalf("generate a2ui for %q: %v", intent, err)
		}
		if view.A2UIJSON == "" {
			t.Fatalf("expected a2ui json for %q, got %+v", intent, view)
		}
	}
}
