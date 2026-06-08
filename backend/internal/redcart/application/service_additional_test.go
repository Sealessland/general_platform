package application_test

import (
	"context"
	"errors"
	"testing"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	application "github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
)

func TestRegisterValidationAndMerchantSession(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})

	cases := []struct {
		name  string
		input application.RegisterInput
	}{
		{name: "missing nickname", input: application.RegisterInput{Phone: "13900001000", Password: "secret", Role: domain.RoleConsumer}},
		{name: "missing phone", input: application.RegisterInput{Nickname: "Alice", Password: "secret", Role: domain.RoleConsumer}},
		{name: "missing password", input: application.RegisterInput{Nickname: "Alice", Phone: "13900001000", Role: domain.RoleConsumer}},
		{name: "invalid role", input: application.RegisterInput{Nickname: "Alice", Phone: "13900001000", Password: "secret", Role: "admin"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.Register(context.Background(), tc.input)
			if !isAppError(err, application.ErrorInvalidArgument) {
				t.Fatalf("expected invalid argument, got %v", err)
			}
		})
	}

	session, err := service.Register(context.Background(), application.RegisterInput{
		Nickname: "Merchant A",
		Phone:    "13900001001",
		Password: "secret",
		Role:     domain.RoleMerchant,
	})
	if err != nil {
		t.Fatalf("register merchant: %v", err)
	}
	if session.Token == "" || session.User.MerchantID == 0 || session.User.Merchant == nil {
		t.Fatalf("expected merchant session with token and merchant view, got %+v", session)
	}

	_, err = service.Register(context.Background(), application.RegisterInput{
		Nickname: "Merchant A Duplicate",
		Phone:    "13900001001",
		Password: "secret",
		Role:     domain.RoleMerchant,
	})
	if !isAppError(err, application.ErrorConflict) {
		t.Fatalf("expected duplicate phone conflict, got %v", err)
	}
}

func TestLoginMeAndAuthenticateRejectInvalidCredentials(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})

	if _, err := service.Login(context.Background(), application.LoginInput{Phone: "13800000001", Password: "wrong"}); !isAppError(err, application.ErrorUnauthorized) {
		t.Fatalf("expected unauthorized login, got %v", err)
	}
	if _, err := service.Me(context.Background(), "missing-token"); !isAppError(err, application.ErrorUnauthorized) {
		t.Fatalf("expected unauthorized me, got %v", err)
	}
	if _, err := service.Authenticate("missing-token"); !isAppError(err, application.ErrorUnauthorized) {
		t.Fatalf("expected unauthorized authenticate, got %v", err)
	}

	session, err := service.Login(context.Background(), application.LoginInput{Phone: "13800000002", Password: "merchant-demo"})
	if err != nil {
		t.Fatalf("login merchant: %v", err)
	}
	actor, err := service.Authenticate(session.Token)
	if err != nil {
		t.Fatalf("authenticate merchant: %v", err)
	}
	if actor.Role != domain.RoleMerchant || actor.MerchantID == 0 {
		t.Fatalf("expected merchant actor, got %+v", actor)
	}
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

func TestCheckoutValidationRejectsInvalidItems(t *testing.T) {
	t.Parallel()

	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
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

func TestBusinessReviewGenerationAndTaskRead(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})
	consumer := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}

	if _, err := service.GenerateBusinessReview(context.Background(), consumer, application.BusinessReviewInput{WindowDays: 7}); !isAppError(err, application.ErrorForbidden) {
		t.Fatalf("expected consumer business review forbidden, got %v", err)
	}
	task, err := service.GenerateBusinessReview(context.Background(), merchant, application.BusinessReviewInput{WindowDays: 7})
	if err != nil {
		t.Fatalf("generate business review: %v", err)
	}
	if task.Status != domain.AITaskStatusCompleted || task.Output["diagnosis"] == nil {
		t.Fatalf("expected completed business review, got %+v", task)
	}
	fetched, err := service.GetAITask(context.Background(), merchant, task.ID)
	if err != nil {
		t.Fatalf("get ai task: %v", err)
	}
	if fetched.ID != task.ID || fetched.TaskType != domain.TaskTypeBusinessReview {
		t.Fatalf("expected fetched business review task, got %+v", fetched)
	}
}

func TestSellingPointGenerationPersistsReadableCompletedTask(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}

	task, err := service.GenerateSellingPoints(context.Background(), merchant, application.SellingPointInput{
		ProductName: "Travel Makeup Organizer",
		Attributes:  []string{"portable", "washable"},
		TargetUsers: "dorm users",
		PriceCent:   8900,
		Reviews:     []string{"fits my tiny desk"},
	})
	if err != nil {
		t.Fatalf("generate selling points: %v", err)
	}
	if task.Status != domain.AITaskStatusCompleted || task.TaskType != domain.TaskTypeSellingPoints {
		t.Fatalf("expected completed selling point task, got %+v", task)
	}
	if task.Input["product_name"] != "Travel Makeup Organizer" || task.Input["target_users"] != "dorm users" {
		t.Fatalf("expected persisted selling point input, got %+v", task.Input)
	}
	points, ok := task.Output["core_points"].([]string)
	if !ok || len(points) == 0 {
		t.Fatalf("expected core points output, got %+v", task.Output)
	}

	fetched, err := service.GetAITask(context.Background(), merchant, task.ID)
	if err != nil {
		t.Fatalf("get selling point task: %v", err)
	}
	titleSuggest, ok := fetched.Output["detail_title_suggest"].(string)
	if fetched.ID != task.ID || !ok || titleSuggest == "" {
		t.Fatalf("expected readable persisted task output, got %+v", fetched)
	}
}

func TestAIGenerationFailurePersistsFailedTask(t *testing.T) {
	t.Parallel()

	repo := memory.NewRepository()
	service := application.NewService(repo, failingAIProvider{})
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}

	_, err := service.GenerateSellingPoints(context.Background(), merchant, application.SellingPointInput{
		ProductName: "Travel Bag",
		TargetUsers: "commuters",
	})
	if err == nil {
		t.Fatal("expected provider error")
	}

	tasks := []domain.AIGenerationTask{}
	for id := int64(1); id < 20; id++ {
		if task, ok := repo.GetAITask(id); ok {
			tasks = append(tasks, task)
		}
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one failed task, got %+v", tasks)
	}
	if tasks[0].Status != domain.AITaskStatusFailed || tasks[0].ErrorMessage == "" {
		t.Fatalf("expected failed task with error, got %+v", tasks[0])
	}
}

func TestBusinessReviewFailurePersistsFailedTask(t *testing.T) {
	t.Parallel()

	repo := memory.NewRepository()
	service := application.NewService(repo, failingAIProvider{})
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}

	_, err := service.GenerateBusinessReview(context.Background(), merchant, application.BusinessReviewInput{WindowDays: 14})
	if err == nil {
		t.Fatal("expected provider error")
	}

	tasks := []domain.AIGenerationTask{}
	for id := int64(1); id < 20; id++ {
		if task, ok := repo.GetAITask(id); ok {
			tasks = append(tasks, task)
		}
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one failed task, got %+v", tasks)
	}
	if tasks[0].TaskType != domain.TaskTypeBusinessReview || tasks[0].Status != domain.AITaskStatusFailed || tasks[0].ErrorMessage == "" {
		t.Fatalf("expected failed business review task with error, got %+v", tasks[0])
	}
	if tasks[0].Input["window_days"] != 14 {
		t.Fatalf("expected persisted business review input, got %+v", tasks[0].Input)
	}
}

type failingAIProvider struct{}

func (failingAIProvider) GenerateSellingPoints(context.Context, backendai.SellingPointRequest) (*backendai.SellingPointResult, error) {
	return nil, errors.New("provider unavailable")
}

func (failingAIProvider) GenerateBusinessReview(context.Context, backendai.BusinessReviewRequest) (*backendai.BusinessReviewResult, error) {
	return nil, errors.New("provider unavailable")
}
