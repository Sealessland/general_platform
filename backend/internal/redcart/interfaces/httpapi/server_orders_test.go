package httpapi

import (
	"net/http"
	"testing"
)

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
func TestOrderHTTPValidationAndStateConflicts(t *testing.T) {
	handler := newTestHandler()
	consumerToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000001",
		"password": "consumer-demo",
	})
	merchantToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000002",
		"password": "merchant-demo",
	})

	_ = requestJSON(t, handler, http.MethodPost, "/api/orders", consumerToken, map[string]any{
		"items": []map[string]any{{"sku_id": 1, "quantity": 1}},
	}, http.StatusBadRequest)
	_ = requestJSON(t, handler, http.MethodPost, "/api/orders/preview", consumerToken, map[string]any{
		"items": []map[string]any{{"sku_id": 1, "quantity": 0}},
	}, http.StatusBadRequest)
	_ = requestJSON(t, handler, http.MethodPost, "/api/orders/preview", consumerToken, map[string]any{
		"items": []map[string]any{{"sku_id": 999999, "quantity": 1}},
	}, http.StatusNotFound)
	_ = requestJSON(t, handler, http.MethodPost, "/api/orders/preview", consumerToken, map[string]any{
		"items": []map[string]any{{"sku_id": 1, "quantity": 999999}},
	}, http.StatusConflict)

	order := requestJSON(t, handler, http.MethodPost, "/api/orders", consumerToken, map[string]any{
		"items":            []map[string]any{{"sku_id": 1, "quantity": 1}},
		"receiver_name":    "Alice",
		"receiver_phone":   "13800000001",
		"receiver_address": "Shanghai",
	}, http.StatusCreated, headerKV{"Idempotency-Key", "http-state-conflicts"})
	orderID := int64Field(t, order, "id")

	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/orders/%d/ship", orderID), merchantToken, map[string]any{
		"logistics_no": "EARLY",
	}, http.StatusConflict)
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/pay", orderID), consumerToken, nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/cancel", orderID), consumerToken, nil, http.StatusConflict)
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/orders/%d/ship", orderID), merchantToken, map[string]any{
		"logistics_no": "SF123",
	}, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/finish", orderID), consumerToken, nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/refund", orderID), consumerToken, map[string]any{
		"reason": "too late",
	}, http.StatusConflict)
}
