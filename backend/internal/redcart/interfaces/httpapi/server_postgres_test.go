package httpapi

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

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
