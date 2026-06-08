package httpapi

import (
	"net/http"
	"testing"
)

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
func TestAIHTTPBusinessReviewAndTaskBoundaries(t *testing.T) {
	handler := newTestHandler()
	consumerToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000001",
		"password": "consumer-demo",
	})
	merchantToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000002",
		"password": "merchant-demo",
	})

	_ = requestJSON(t, handler, http.MethodPost, "/api/ai/business-review", consumerToken, map[string]any{
		"window_days": 7,
	}, http.StatusForbidden)
	_ = requestJSON(t, handler, http.MethodPost, "/api/ai/business-review", merchantToken, map[string]any{
		"window_days": 7,
	}, http.StatusOK)
	task := requestJSON(t, handler, http.MethodPost, "/api/ai/product-selling-points", merchantToken, map[string]any{
		"product_name": "AI HTTP Product",
		"attributes":   []string{"compact"},
		"target_users": "commuters",
		"price_cent":   19900,
	}, http.StatusOK)
	taskID := int64Field(t, task, "id")
	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/ai/tasks/%d", taskID), consumerToken, nil, http.StatusNotFound)
	fetched := requestJSON(t, handler, http.MethodGet, pathf("/api/ai/tasks/%d", taskID), merchantToken, nil, http.StatusOK)
	if fetched["status"].(string) != "completed" {
		t.Fatalf("expected completed task, got %+v", fetched)
	}
}
