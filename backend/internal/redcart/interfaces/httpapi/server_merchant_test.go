package httpapi

import (
	"net/http"
	"testing"
)

func TestMerchantProductSKUAndDashboardHTTP(t *testing.T) {
	handler := newTestHandler()
	consumerToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000001",
		"password": "consumer-demo",
	})
	merchantToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000002",
		"password": "merchant-demo",
	})

	_ = requestJSON(t, handler, http.MethodPost, "/api/merchant/products", consumerToken, map[string]any{
		"title": "Consumer Product",
	}, http.StatusForbidden)
	_ = requestJSON(t, handler, http.MethodPost, "/api/merchant/products", merchantToken, map[string]any{
		"title": "",
	}, http.StatusBadRequest)
	product := requestJSON(t, handler, http.MethodPost, "/api/merchant/products", merchantToken, map[string]any{
		"title":          "HTTP Product",
		"description":    "created by test",
		"category_id":    88,
		"selling_points": []string{"stable"},
	}, http.StatusCreated)
	productID := int64Field(t, product, "id")
	updated := requestJSON(t, handler, http.MethodPut, pathf("/api/merchant/products/%d", productID), merchantToken, map[string]any{
		"title":          "HTTP Product Updated",
		"description":    "updated by test",
		"category_id":    89,
		"selling_points": []string{"stable", "fast"},
	}, http.StatusOK)
	if updated["title"].(string) != "HTTP Product Updated" {
		t.Fatalf("expected updated product title, got %+v", updated)
	}

	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/products/%d/skus", productID), merchantToken, map[string]any{
		"sku_name":   "Bad SKU",
		"price_cent": 0,
		"stock":      1,
	}, http.StatusBadRequest)
	sku := requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/products/%d/skus", productID), merchantToken, map[string]any{
		"sku_name":   "Standard",
		"sku_attrs":  map[string]string{"color": "red"},
		"price_cent": 9900,
		"stock":      5,
		"status":     "active",
	}, http.StatusCreated)
	skuID := int64Field(t, sku, "id")
	updatedSKU := requestJSON(t, handler, http.MethodPut, pathf("/api/merchant/skus/%d", skuID), merchantToken, map[string]any{
		"sku_name":   "Standard Updated",
		"price_cent": 10900,
		"stock":      4,
		"status":     "inactive",
	}, http.StatusOK)
	if updatedSKU["sku_name"].(string) != "Standard Updated" || int64Field(t, updatedSKU, "stock") != 4 {
		t.Fatalf("expected updated sku, got %+v", updatedSKU)
	}
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/products/%d/online", productID), merchantToken, nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/products/%d/offline", productID), merchantToken, nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodGet, "/api/merchant/dashboard/funnel", consumerToken, nil, http.StatusForbidden)
	_ = requestJSON(t, handler, http.MethodGet, "/api/merchant/dashboard/products", merchantToken, nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodGet, "/api/merchant/dashboard/summary", merchantToken, nil, http.StatusOK)
}

func TestMerchantRefundApproveReturnsCurrentViewOnRepeatSuccess(t *testing.T) {
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
	}, http.StatusCreated, headerKV{"Idempotency-Key", "merchant-refund-repeat-001"})
	orderID := int(order["id"].(float64))
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/pay", orderID), consumerToken, nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/refund", orderID), consumerToken, map[string]any{
		"reason": "size mismatch",
	}, http.StatusAccepted)

	refunded := requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/orders/%d/refund/approve", orderID), merchantToken, nil, http.StatusOK)
	refundedAgain := requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/orders/%d/refund/approve", orderID), merchantToken, nil, http.StatusOK)
	if refundedAgain["status"].(string) != "REFUNDED" || int(refundedAgain["id"].(float64)) != int(refunded["id"].(float64)) {
		t.Fatalf("expected idempotent refunded response, got %+v", refundedAgain)
	}
}
