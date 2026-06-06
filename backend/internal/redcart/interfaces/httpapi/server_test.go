package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
	postgresrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres"
)

var testUniqueCounter atomic.Int64

func TestOrderFlowAndMerchantFlow(t *testing.T) {
	handler := newTestHandler()

	consumerToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000001",
		"password": "consumer-demo",
	})
	merchantToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000002",
		"password": "merchant-demo",
	})

	preview := requestJSON(t, handler, http.MethodPost, "/api/orders/preview", consumerToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": 1, "quantity": 1},
		},
	}, http.StatusOK)
	if int(preview["pay_amount_cent"].(float64)) != 12900 {
		t.Fatalf("expected preview pay amount 12900, got %v", preview["pay_amount_cent"])
	}

	order := requestJSON(t, handler, http.MethodPost, "/api/orders", consumerToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": 1, "quantity": 1},
		},
		"receiver_name":    "Alice",
		"receiver_phone":   "13800000001",
		"receiver_address": "Shanghai",
	}, http.StatusCreated, headerKV{"Idempotency-Key", "order-key-001"})
	orderID := int(order["id"].(float64))

	orderRepeat := requestJSON(t, handler, http.MethodPost, "/api/orders", consumerToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": 1, "quantity": 1},
		},
		"receiver_name":    "Alice",
		"receiver_phone":   "13800000001",
		"receiver_address": "Shanghai",
	}, http.StatusCreated, headerKV{"Idempotency-Key", "order-key-001"})
	if int(orderRepeat["id"].(float64)) != orderID {
		t.Fatalf("expected idempotent order id %d, got %v", orderID, orderRepeat["id"])
	}

	paid := requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/pay", orderID), consumerToken, nil, http.StatusOK)
	if paid["status"].(string) != "PAID" {
		t.Fatalf("expected PAID, got %v", paid["status"])
	}

	shipped := requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/orders/%d/ship", orderID), merchantToken, map[string]any{
		"logistics_no": "SF123456",
	}, http.StatusOK)
	if shipped["status"].(string) != "SHIPPED" {
		t.Fatalf("expected SHIPPED, got %v", shipped["status"])
	}

	finished := requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/finish", orderID), consumerToken, nil, http.StatusOK)
	if finished["status"].(string) != "FINISHED" {
		t.Fatalf("expected FINISHED, got %v", finished["status"])
	}

	merchantOrders := requestJSON(t, handler, http.MethodGet, "/api/merchant/orders", merchantToken, nil, http.StatusOK)
	if len(merchantOrders["items"].([]any)) == 0 {
		t.Fatal("expected merchant orders")
	}
}

func TestRefundFlowAndAIGeneration(t *testing.T) {
	handler := newTestHandler()
	consumerToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000001",
		"password": "consumer-demo",
	})
	merchantToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000002",
		"password": "merchant-demo",
	})

	order := requestJSON(t, handler, http.MethodPost, "/api/orders", consumerToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": 3, "quantity": 1},
		},
		"receiver_name":    "Alice",
		"receiver_phone":   "13800000001",
		"receiver_address": "Hangzhou",
	}, http.StatusCreated, headerKV{"Idempotency-Key", "order-key-002"})
	orderID := int(order["id"].(float64))

	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/pay", orderID), consumerToken, nil, http.StatusOK)
	refunding := requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/refund", orderID), consumerToken, map[string]any{
		"reason": "size mismatch",
	}, http.StatusAccepted)
	if refunding["status"].(string) != "REFUNDING" {
		t.Fatalf("expected REFUNDING, got %v", refunding["status"])
	}

	refunded := requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/orders/%d/refund/approve", orderID), merchantToken, nil, http.StatusOK)
	if refunded["status"].(string) != "REFUNDED" {
		t.Fatalf("expected REFUNDED, got %v", refunded["status"])
	}

	aiTask := requestJSON(t, handler, http.MethodPost, "/api/ai/product-selling-points", merchantToken, map[string]any{
		"product_name": "Travel Makeup Organizer",
		"attributes":   []string{"portable", "multi-layer"},
		"target_users": "dorm users",
		"price_cent":   8900,
	}, http.StatusOK)
	if aiTask["status"].(string) != "completed" {
		t.Fatalf("expected ai task completed, got %v", aiTask["status"])
	}
	if aiTask["task_type"].(string) != "product_selling_points" {
		t.Fatalf("unexpected ai task type %v", aiTask["task_type"])
	}
}

func TestStateChangingRoutesRejectGET(t *testing.T) {
	handler := newTestHandler()
	consumerToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000001",
		"password": "consumer-demo",
	})
	merchantToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000002",
		"password": "merchant-demo",
	})

	order := requestJSON(t, handler, http.MethodPost, "/api/orders", consumerToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": 1, "quantity": 1},
		},
		"receiver_name":    "Alice",
		"receiver_phone":   "13800000001",
		"receiver_address": "Shanghai",
	}, http.StatusCreated, headerKV{"Idempotency-Key", "method-order-001"})
	orderID := int(order["id"].(float64))

	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/orders/%d/pay", orderID), consumerToken, nil, http.StatusMethodNotAllowed)
	fetched := requestJSON(t, handler, http.MethodGet, pathf("/api/orders/%d", orderID), consumerToken, nil, http.StatusOK)
	if fetched["status"].(string) != "CREATED" {
		t.Fatalf("GET pay changed order status to %s", fetched["status"])
	}

	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/orders/%d/cancel", orderID), consumerToken, nil, http.StatusMethodNotAllowed)
	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/orders/%d/refund", orderID), consumerToken, nil, http.StatusMethodNotAllowed)
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/pay", orderID), consumerToken, nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/merchant/orders/%d/ship", orderID), merchantToken, nil, http.StatusMethodNotAllowed)
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/orders/%d/ship", orderID), merchantToken, map[string]any{
		"logistics_no": "SF654321",
	}, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/orders/%d/finish", orderID), consumerToken, nil, http.StatusMethodNotAllowed)

	refundOrder := requestJSON(t, handler, http.MethodPost, "/api/orders", consumerToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": 3, "quantity": 1},
		},
		"receiver_name":    "Alice",
		"receiver_phone":   "13800000001",
		"receiver_address": "Hangzhou",
	}, http.StatusCreated, headerKV{"Idempotency-Key", "method-order-002"})
	refundOrderID := int(refundOrder["id"].(float64))
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/pay", refundOrderID), consumerToken, nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/refund", refundOrderID), consumerToken, map[string]any{
		"reason": "size mismatch",
	}, http.StatusAccepted)
	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/merchant/orders/%d/refund/approve", refundOrderID), merchantToken, nil, http.StatusMethodNotAllowed)

	product := requestJSON(t, handler, http.MethodPost, "/api/merchant/products", merchantToken, map[string]any{
		"title":          "Method Gate Product",
		"description":    "created for method gate test",
		"category_id":    109,
		"selling_points": []string{"method safe"},
	}, http.StatusCreated)
	productID := int(product["id"].(float64))
	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/merchant/products/%d/online", productID), merchantToken, nil, http.StatusMethodNotAllowed)
	merchantProducts := requestJSON(t, handler, http.MethodGet, "/api/merchant/products", merchantToken, nil, http.StatusOK)
	if status := productStatusFromList(t, merchantProducts, productID); status != "draft" {
		t.Fatalf("GET online changed product status to %s", status)
	}
	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/merchant/products/%d/offline", productID), merchantToken, nil, http.StatusMethodNotAllowed)
}

func TestUnauthorizedRequestsRejected(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/cart", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestPostgresHTTPCriticalPath(t *testing.T) {
	handler, cleanup := newPostgresTestHandler(t)
	defer cleanup()

	suffix := uniqueSuffix()
	consumerToken := registerAndGetToken(t, handler, "pg-consumer-"+suffix, "consumer")
	merchantToken := registerAndGetToken(t, handler, "pg-merchant-"+suffix, "merchant")
	productID, skuID := createOnlineProductAndSKU(t, handler, merchantToken, suffix, 8)

	skus := requestJSON(t, handler, http.MethodGet, pathf("/api/products/%d/skus", productID), "", nil, http.StatusOK)
	if findSKU(t, skus, skuID)["status"].(string) != "active" {
		t.Fatalf("expected active sku %d", skuID)
	}

	preview := requestJSON(t, handler, http.MethodPost, "/api/orders/preview", consumerToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": skuID, "quantity": 1},
		},
	}, http.StatusOK)
	if int64Field(t, preview, "pay_amount_cent") != 12345 {
		t.Fatalf("expected preview pay amount 12345, got %v", preview["pay_amount_cent"])
	}
	if preview["stock_ok"] != true {
		t.Fatalf("expected stock_ok=true, got %v", preview["stock_ok"])
	}

	orderBody := map[string]any{
		"items": []map[string]any{
			{"sku_id": skuID, "quantity": 1},
		},
		"receiver_name":    "Postgres Consumer",
		"receiver_phone":   "13900000000",
		"receiver_address": "Shanghai",
	}
	order := requestJSON(t, handler, http.MethodPost, "/api/orders", consumerToken, orderBody, http.StatusCreated, headerKV{"Idempotency-Key", "pg-critical-" + suffix})
	orderID := int64Field(t, order, "id")
	if order["status"].(string) != "CREATED" {
		t.Fatalf("expected CREATED, got %v", order["status"])
	}
	locks := order["inventory_locks"].([]any)
	if len(locks) != 1 {
		t.Fatalf("expected one inventory lock, got %d", len(locks))
	}
	if locks[0].(map[string]any)["status"].(string) != "locked" {
		t.Fatalf("expected locked inventory, got %+v", locks[0])
	}

	repeated := requestJSON(t, handler, http.MethodPost, "/api/orders", consumerToken, orderBody, http.StatusCreated, headerKV{"Idempotency-Key", "pg-critical-" + suffix})
	if int64Field(t, repeated, "id") != orderID {
		t.Fatalf("expected idempotent order id %d, got %v", orderID, repeated["id"])
	}

	paid := requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/pay", orderID), consumerToken, nil, http.StatusOK)
	if paid["status"].(string) != "PAID" {
		t.Fatalf("expected PAID, got %v", paid["status"])
	}
	paidLocks := paid["inventory_locks"].([]any)
	if paidLocks[0].(map[string]any)["status"].(string) != "confirmed" {
		t.Fatalf("expected confirmed inventory, got %+v", paidLocks[0])
	}

	shipped := requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/orders/%d/ship", orderID), merchantToken, map[string]any{
		"logistics_no": "PG" + suffix,
	}, http.StatusOK)
	if shipped["status"].(string) != "SHIPPED" {
		t.Fatalf("expected SHIPPED, got %v", shipped["status"])
	}
	finished := requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/finish", orderID), consumerToken, nil, http.StatusOK)
	if finished["status"].(string) != "FINISHED" {
		t.Fatalf("expected FINISHED, got %v", finished["status"])
	}

	dashboard := requestJSON(t, handler, http.MethodGet, "/api/merchant/dashboard/summary", merchantToken, nil, http.StatusOK)
	if int64Field(t, dashboard, "order_count") == 0 {
		t.Fatalf("expected dashboard order_count > 0, got %+v", dashboard)
	}

	aiTask := requestJSON(t, handler, http.MethodPost, "/api/ai/product-selling-points", merchantToken, map[string]any{
		"product_name": "PG Critical Product",
		"attributes":   []string{"durable", "compact"},
		"target_users": "commuters",
		"price_cent":   12345,
	}, http.StatusOK)
	taskID := int64Field(t, aiTask, "id")
	fetchedTask := requestJSON(t, handler, http.MethodGet, pathf("/api/ai/tasks/%d", taskID), merchantToken, nil, http.StatusOK)
	if fetchedTask["status"].(string) != "completed" {
		t.Fatalf("expected completed ai task, got %+v", fetchedTask)
	}
}

func TestPostgresHTTPConcurrentOrderReservesStockAtomically(t *testing.T) {
	handler, cleanup := newPostgresTestHandler(t)
	defer cleanup()

	suffix := uniqueSuffix()
	consumerToken := registerAndGetToken(t, handler, "pg-race-consumer-"+suffix, "consumer")
	merchantToken := registerAndGetToken(t, handler, "pg-race-merchant-"+suffix, "merchant")
	productID, skuID := createOnlineProductAndSKU(t, handler, merchantToken, suffix, 1)

	orderBody := map[string]any{
		"items": []map[string]any{
			{"sku_id": skuID, "quantity": 1},
		},
		"receiver_name":    "Race Consumer",
		"receiver_phone":   "13900000001",
		"receiver_address": "Hangzhou",
	}

	const workers = 24
	start := make(chan struct{})
	var wg sync.WaitGroup
	var created atomic.Int64
	var conflicts atomic.Int64
	var other atomic.Int64
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			status := postJSONStatus(handler, http.MethodPost, "/api/orders", consumerToken, orderBody, headerKV{"Idempotency-Key", fmt.Sprintf("pg-race-%s-%d", suffix, i)})
			switch status {
			case http.StatusCreated:
				created.Add(1)
			case http.StatusConflict:
				conflicts.Add(1)
			default:
				other.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if created.Load() != 1 || conflicts.Load() != workers-1 || other.Load() != 0 {
		t.Fatalf("expected created=1 conflicts=%d other=0, got created=%d conflicts=%d other=%d", workers-1, created.Load(), conflicts.Load(), other.Load())
	}
	skus := requestJSON(t, handler, http.MethodGet, pathf("/api/products/%d/skus", productID), "", nil, http.StatusOK)
	sku := findSKU(t, skus, skuID)
	if int64Field(t, sku, "stock") != 1 || int64Field(t, sku, "locked_stock") != 1 {
		t.Fatalf("expected stock=1 locked_stock=1 after concurrent reservation, got %+v", sku)
	}
}

func TestPostgresHTTPInventoryCompensationPaths(t *testing.T) {
	handler, cleanup := newPostgresTestHandler(t)
	defer cleanup()

	suffix := uniqueSuffix()
	consumerToken := registerAndGetToken(t, handler, "pg-comp-consumer-"+suffix, "consumer")
	merchantToken := registerAndGetToken(t, handler, "pg-comp-merchant-"+suffix, "merchant")

	cancelProductID, cancelSKUID := createOnlineProductAndSKU(t, handler, merchantToken, "cancel-"+suffix, 2)
	cancelOrder := createOrder(t, handler, consumerToken, cancelSKUID, "pg-cancel-"+suffix)
	cancelOrderID := int64Field(t, cancelOrder, "id")
	assertSKUStock(t, handler, cancelProductID, cancelSKUID, 2, 1)
	cancelled := requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/cancel", cancelOrderID), consumerToken, nil, http.StatusOK)
	if cancelled["status"].(string) != "CANCELLED" {
		t.Fatalf("expected CANCELLED, got %+v", cancelled)
	}
	assertSKUStock(t, handler, cancelProductID, cancelSKUID, 2, 0)
	cancelLocks := cancelled["inventory_locks"].([]any)
	if cancelLocks[0].(map[string]any)["status"].(string) != "released" {
		t.Fatalf("expected released lock after cancellation, got %+v", cancelLocks[0])
	}

	refundProductID, refundSKUID := createOnlineProductAndSKU(t, handler, merchantToken, "refund-"+suffix, 2)
	refundOrder := createOrder(t, handler, consumerToken, refundSKUID, "pg-refund-"+suffix)
	refundOrderID := int64Field(t, refundOrder, "id")
	paid := requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/pay", refundOrderID), consumerToken, nil, http.StatusOK)
	if paid["status"].(string) != "PAID" {
		t.Fatalf("expected PAID, got %+v", paid)
	}
	assertSKUStock(t, handler, refundProductID, refundSKUID, 1, 0)
	refunding := requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/refund", refundOrderID), consumerToken, map[string]any{
		"reason": "postgres compensation check",
	}, http.StatusAccepted)
	if refunding["status"].(string) != "REFUNDING" {
		t.Fatalf("expected REFUNDING, got %+v", refunding)
	}
	refunded := requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/orders/%d/refund/approve", refundOrderID), merchantToken, nil, http.StatusOK)
	if refunded["status"].(string) != "REFUNDED" {
		t.Fatalf("expected REFUNDED, got %+v", refunded)
	}
	assertSKUStock(t, handler, refundProductID, refundSKUID, 2, 0)
	refundLocks := refunded["inventory_locks"].([]any)
	if refundLocks[0].(map[string]any)["status"].(string) != "released" {
		t.Fatalf("expected released lock after refund, got %+v", refundLocks[0])
	}
}

func TestPostgresHTTPRejectsInsufficientStockWithoutSideEffects(t *testing.T) {
	handler, cleanup := newPostgresTestHandler(t)
	defer cleanup()

	suffix := uniqueSuffix()
	consumerToken := registerAndGetToken(t, handler, "pg-stock-consumer-"+suffix, "consumer")
	merchantToken := registerAndGetToken(t, handler, "pg-stock-merchant-"+suffix, "merchant")
	productID, skuID := createOnlineProductAndSKU(t, handler, merchantToken, suffix, 1)

	previewStatus := postJSONStatus(handler, http.MethodPost, "/api/orders/preview", consumerToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": skuID, "quantity": 2},
		},
	})
	if previewStatus != http.StatusConflict {
		t.Fatalf("expected 409 for insufficient stock preview, got %d", previewStatus)
	}

	status := postJSONStatus(handler, http.MethodPost, "/api/orders", consumerToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": skuID, "quantity": 2},
		},
		"receiver_name":    "Insufficient Stock",
		"receiver_phone":   "13900000003",
		"receiver_address": "Shanghai",
	}, headerKV{"Idempotency-Key", "pg-stock-" + suffix})
	if status != http.StatusConflict {
		t.Fatalf("expected 409 for insufficient stock, got %d", status)
	}
	assertSKUStock(t, handler, productID, skuID, 1, 0)
}

func TestPostgresHTTPRejectsGETStateChanges(t *testing.T) {
	handler, cleanup := newPostgresTestHandler(t)
	defer cleanup()

	suffix := uniqueSuffix()
	consumerToken := registerAndGetToken(t, handler, "pg-method-consumer-"+suffix, "consumer")
	merchantToken := registerAndGetToken(t, handler, "pg-method-merchant-"+suffix, "merchant")
	productID, skuID := createOnlineProductAndSKU(t, handler, merchantToken, suffix, 3)
	order := createOrder(t, handler, consumerToken, skuID, "pg-method-"+suffix)
	orderID := int64Field(t, order, "id")

	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/orders/%d/pay", orderID), consumerToken, nil, http.StatusMethodNotAllowed)
	fetched := requestJSON(t, handler, http.MethodGet, pathf("/api/orders/%d", orderID), consumerToken, nil, http.StatusOK)
	if fetched["status"].(string) != "CREATED" {
		t.Fatalf("GET pay changed order status, got %+v", fetched)
	}
	assertSKUStock(t, handler, productID, skuID, 3, 1)

	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/orders/%d/cancel", orderID), consumerToken, nil, http.StatusMethodNotAllowed)
	fetched = requestJSON(t, handler, http.MethodGet, pathf("/api/orders/%d", orderID), consumerToken, nil, http.StatusOK)
	if fetched["status"].(string) != "CREATED" {
		t.Fatalf("GET cancel changed order status, got %+v", fetched)
	}
	assertSKUStock(t, handler, productID, skuID, 3, 1)
}

func TestPostgresHTTPAuthorizationBoundaries(t *testing.T) {
	handler, cleanup := newPostgresTestHandler(t)
	defer cleanup()

	suffix := uniqueSuffix()
	ownerToken := registerAndGetToken(t, handler, "pg-owner-"+suffix, "consumer")
	otherConsumerToken := registerAndGetToken(t, handler, "pg-other-"+suffix, "consumer")
	merchantToken := registerAndGetToken(t, handler, "pg-auth-merchant-"+suffix, "merchant")
	otherMerchantToken := registerAndGetToken(t, handler, "pg-other-merchant-"+suffix, "merchant")
	_, skuID := createOnlineProductAndSKU(t, handler, merchantToken, suffix, 4)
	order := createOrder(t, handler, ownerToken, skuID, "pg-auth-"+suffix)
	orderID := int64Field(t, order, "id")

	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/orders/%d", orderID), otherConsumerToken, nil, http.StatusNotFound)
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/orders/%d/ship", orderID), otherMerchantToken, map[string]any{
		"logistics_no": "UNAUTH" + suffix,
	}, http.StatusNotFound)
	_ = requestJSON(t, handler, http.MethodPost, "/api/merchant/products", ownerToken, map[string]any{
		"title":       "Consumer Product",
		"description": "should be forbidden",
		"category_id": 1,
	}, http.StatusForbidden)
	_ = requestJSON(t, handler, http.MethodPost, "/api/ai/product-selling-points", ownerToken, map[string]any{
		"product_name": "Consumer AI",
		"attributes":   []string{"forbidden"},
		"target_users": "n/a",
		"price_cent":   100,
	}, http.StatusForbidden)

	aiTask := requestJSON(t, handler, http.MethodPost, "/api/ai/product-selling-points", merchantToken, map[string]any{
		"product_name": "Merchant AI",
		"attributes":   []string{"visible"},
		"target_users": "merchant",
		"price_cent":   100,
	}, http.StatusOK)
	taskID := int64Field(t, aiTask, "id")
	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/ai/tasks/%d", taskID), otherMerchantToken, nil, http.StatusNotFound)
}

func newTestHandler() http.Handler {
	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
	return NewServer(service).Handler()
}

func newPostgresTestHandler(t testing.TB) (http.Handler, func()) {
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
	service := application.NewService(repo, backendai.MockProvider{})
	return NewServer(service).Handler(), func() {
		if err := repo.Close(); err != nil {
			t.Fatalf("close postgres repository: %v", err)
		}
	}
}

type headerKV struct {
	key   string
	value string
}

func requestJSON(t testing.TB, handler http.Handler, method, path, token string, body any, wantStatus int, extraHeaders ...headerKV) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for _, header := range extraHeaders {
		req.Header.Set(header.key, header.value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s expected status %d, got %d body=%s", method, path, wantStatus, rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return payload
}

func postJSONStatus(handler http.Handler, method, path, token string, body any, extraHeaders ...headerKV) int {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for _, header := range extraHeaders {
		req.Header.Set(header.key, header.value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code
}

func loginAndGetToken(t testing.TB, handler http.Handler, body map[string]any) string {
	t.Helper()
	resp := requestJSON(t, handler, http.MethodPost, "/api/auth/login", "", body, http.StatusOK)
	token, ok := resp["token"].(string)
	if !ok || token == "" {
		t.Fatalf("missing token in response: %+v", resp)
	}
	return token
}

func registerAndGetToken(t testing.TB, handler http.Handler, nickname, role string) string {
	t.Helper()
	password := "pass-" + uniqueSuffix()
	resp := requestJSON(t, handler, http.MethodPost, "/api/auth/register", "", map[string]any{
		"nickname": nickname,
		"phone":    "139" + uniqueSuffix(),
		"password": password,
		"role":     role,
	}, http.StatusCreated)
	token, ok := resp["token"].(string)
	if !ok || token == "" {
		t.Fatalf("missing token in register response: %+v", resp)
	}
	return token
}

func createOnlineProductAndSKU(t testing.TB, handler http.Handler, merchantToken, suffix string, stock int) (int64, int64) {
	t.Helper()
	product := requestJSON(t, handler, http.MethodPost, "/api/merchant/products", merchantToken, map[string]any{
		"title":          "PG Product " + suffix,
		"description":    "postgres-backed http test product",
		"category_id":    20260606,
		"selling_points": []string{"postgres", "critical path"},
	}, http.StatusCreated)
	productID := int64Field(t, product, "id")
	sku := requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/products/%d/skus", productID), merchantToken, map[string]any{
		"sku_name":   "PG SKU " + suffix,
		"sku_attrs":  map[string]string{"size": "standard"},
		"price_cent": 12345,
		"stock":      stock,
		"status":     "active",
	}, http.StatusCreated)
	skuID := int64Field(t, sku, "id")
	online := requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/products/%d/online", productID), merchantToken, nil, http.StatusOK)
	if online["status"].(string) != "online" {
		t.Fatalf("expected online product, got %+v", online)
	}
	return productID, skuID
}

func createOrder(t testing.TB, handler http.Handler, consumerToken string, skuID int64, idempotencyKey string) map[string]any {
	t.Helper()
	return requestJSON(t, handler, http.MethodPost, "/api/orders", consumerToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": skuID, "quantity": 1},
		},
		"receiver_name":    "Postgres Consumer",
		"receiver_phone":   "13900000000",
		"receiver_address": "Shanghai",
	}, http.StatusCreated, headerKV{"Idempotency-Key", idempotencyKey})
}

func assertSKUStock(t testing.TB, handler http.Handler, productID, skuID int64, wantStock, wantLockedStock int64) {
	t.Helper()
	skus := requestJSON(t, handler, http.MethodGet, pathf("/api/products/%d/skus", productID), "", nil, http.StatusOK)
	sku := findSKU(t, skus, skuID)
	if int64Field(t, sku, "stock") != wantStock || int64Field(t, sku, "locked_stock") != wantLockedStock {
		t.Fatalf("expected sku %d stock=%d locked_stock=%d, got %+v", skuID, wantStock, wantLockedStock, sku)
	}
}

func pathf(format string, values ...any) string {
	return fmt.Sprintf(format, values...)
}

func productStatusFromList(t testing.TB, payload map[string]any, productID int) string {
	t.Helper()
	items, ok := payload["items"].([]any)
	if !ok {
		t.Fatalf("missing product items in response: %+v", payload)
	}
	for _, item := range items {
		product, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("unexpected product item: %+v", item)
		}
		if int(product["id"].(float64)) == productID {
			return product["status"].(string)
		}
	}
	t.Fatalf("product %d not found in response: %+v", productID, payload)
	return ""
}

func findSKU(t testing.TB, payload map[string]any, skuID int64) map[string]any {
	t.Helper()
	items, ok := payload["items"].([]any)
	if !ok {
		t.Fatalf("missing sku items in response: %+v", payload)
	}
	for _, item := range items {
		sku, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("unexpected sku item: %+v", item)
		}
		if int64Field(t, sku, "id") == skuID {
			return sku
		}
	}
	t.Fatalf("sku %d not found in response: %+v", skuID, payload)
	return nil
}

func int64Field(t testing.TB, payload map[string]any, field string) int64 {
	t.Helper()
	value, ok := payload[field].(float64)
	if !ok {
		t.Fatalf("missing numeric field %s in response: %+v", field, payload)
	}
	return int64(value)
}

func uniqueSuffix() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), testUniqueCounter.Add(1))
}

func BenchmarkHTTPNotes(b *testing.B) {
	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
	handler := NewServer(service).Handler()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/notes", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d", rec.Code)
		}
	}
}

func BenchmarkHTTPPostgresOrderPreview(b *testing.B) {
	handler, cleanup := newPostgresTestHandler(b)
	defer cleanup()

	suffix := uniqueSuffix()
	consumerToken := registerAndGetToken(b, handler, "pg-bench-preview-consumer-"+suffix, "consumer")
	merchantToken := registerAndGetToken(b, handler, "pg-bench-preview-merchant-"+suffix, "merchant")
	_, skuID := createOnlineProductAndSKU(b, handler, merchantToken, suffix, b.N+10)
	body := mustJSON(b, map[string]any{
		"items": []map[string]any{
			{"sku_id": skuID, "quantity": 1},
		},
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/orders/preview", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+consumerToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
	}
}

func BenchmarkHTTPPostgresCreateOrder(b *testing.B) {
	handler, cleanup := newPostgresTestHandler(b)
	defer cleanup()

	suffix := uniqueSuffix()
	consumerToken := registerAndGetToken(b, handler, "pg-bench-order-consumer-"+suffix, "consumer")
	merchantToken := registerAndGetToken(b, handler, "pg-bench-order-merchant-"+suffix, "merchant")
	_, skuID := createOnlineProductAndSKU(b, handler, merchantToken, suffix, b.N+10)
	body := mustJSON(b, map[string]any{
		"items": []map[string]any{
			{"sku_id": skuID, "quantity": 1},
		},
		"receiver_name":    "Benchmark Consumer",
		"receiver_phone":   "13900000002",
		"receiver_address": "Shanghai",
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+consumerToken)
		req.Header.Set("Idempotency-Key", fmt.Sprintf("pg-bench-order-%s-%d", suffix, i))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			b.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
		}
	}
}

func mustJSON(t testing.TB, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return payload
}

func BenchmarkHTTPOrderPreview(b *testing.B) {
	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
	handler := NewServer(service).Handler()

	loginBody, err := json.Marshal(map[string]any{
		"phone":    "13800000001",
		"password": "consumer-demo",
	})
	if err != nil {
		b.Fatalf("marshal login body: %v", err)
	}
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		b.Fatalf("login failed, status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(loginRec.Body.Bytes(), &payload); err != nil {
		b.Fatalf("decode login response: %v", err)
	}
	token, ok := payload["token"].(string)
	if !ok || token == "" {
		b.Fatal("missing token in login response")
	}

	orderBody, err := json.Marshal(map[string]any{})
	if err != nil {
		b.Fatalf("marshal preview body: %v", err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/orders/preview", bytes.NewReader(orderBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d", rec.Code)
		}
	}
}
