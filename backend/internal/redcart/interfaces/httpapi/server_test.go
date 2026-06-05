package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
)

func TestOrderFlowAndMerchantFlow(t *testing.T) {
	server := newTestServer()

	consumerToken := loginAndGetToken(t, server, map[string]any{
		"phone":    "13800000001",
		"password": "consumer-demo",
	})
	merchantToken := loginAndGetToken(t, server, map[string]any{
		"phone":    "13800000002",
		"password": "merchant-demo",
	})

	preview := requestJSON(t, server, http.MethodPost, "/api/orders/preview", consumerToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": 1, "quantity": 1},
		},
	}, http.StatusOK)
	if int(preview["pay_amount_cent"].(float64)) != 12900 {
		t.Fatalf("expected preview pay amount 12900, got %v", preview["pay_amount_cent"])
	}

	order := requestJSON(t, server, http.MethodPost, "/api/orders", consumerToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": 1, "quantity": 1},
		},
		"receiver_name":    "Alice",
		"receiver_phone":   "13800000001",
		"receiver_address": "Shanghai",
	}, http.StatusCreated, headerKV{"Idempotency-Key", "order-key-001"})
	orderID := int(order["id"].(float64))

	orderRepeat := requestJSON(t, server, http.MethodPost, "/api/orders", consumerToken, map[string]any{
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

	paid := requestJSON(t, server, http.MethodPost, pathf("/api/orders/%d/pay", orderID), consumerToken, nil, http.StatusOK)
	if paid["status"].(string) != "PAID" {
		t.Fatalf("expected PAID, got %v", paid["status"])
	}

	shipped := requestJSON(t, server, http.MethodPost, pathf("/api/merchant/orders/%d/ship", orderID), merchantToken, map[string]any{
		"logistics_no": "SF123456",
	}, http.StatusOK)
	if shipped["status"].(string) != "SHIPPED" {
		t.Fatalf("expected SHIPPED, got %v", shipped["status"])
	}

	finished := requestJSON(t, server, http.MethodPost, pathf("/api/orders/%d/finish", orderID), consumerToken, nil, http.StatusOK)
	if finished["status"].(string) != "FINISHED" {
		t.Fatalf("expected FINISHED, got %v", finished["status"])
	}

	merchantOrders := requestJSON(t, server, http.MethodGet, "/api/merchant/orders", merchantToken, nil, http.StatusOK)
	if len(merchantOrders["items"].([]any)) == 0 {
		t.Fatal("expected merchant orders")
	}
}

func TestRefundFlowAndAIGeneration(t *testing.T) {
	server := newTestServer()
	consumerToken := loginAndGetToken(t, server, map[string]any{
		"phone":    "13800000001",
		"password": "consumer-demo",
	})
	merchantToken := loginAndGetToken(t, server, map[string]any{
		"phone":    "13800000002",
		"password": "merchant-demo",
	})

	order := requestJSON(t, server, http.MethodPost, "/api/orders", consumerToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": 3, "quantity": 1},
		},
		"receiver_name":    "Alice",
		"receiver_phone":   "13800000001",
		"receiver_address": "Hangzhou",
	}, http.StatusCreated, headerKV{"Idempotency-Key", "order-key-002"})
	orderID := int(order["id"].(float64))

	_ = requestJSON(t, server, http.MethodPost, pathf("/api/orders/%d/pay", orderID), consumerToken, nil, http.StatusOK)
	refunding := requestJSON(t, server, http.MethodPost, pathf("/api/orders/%d/refund", orderID), consumerToken, map[string]any{
		"reason": "size mismatch",
	}, http.StatusAccepted)
	if refunding["status"].(string) != "REFUNDING" {
		t.Fatalf("expected REFUNDING, got %v", refunding["status"])
	}

	refunded := requestJSON(t, server, http.MethodPost, pathf("/api/merchant/orders/%d/refund/approve", orderID), merchantToken, nil, http.StatusOK)
	if refunded["status"].(string) != "REFUNDED" {
		t.Fatalf("expected REFUNDED, got %v", refunded["status"])
	}

	aiTask := requestJSON(t, server, http.MethodPost, "/api/ai/product-selling-points", merchantToken, map[string]any{
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

func TestUnauthorizedRequestsRejected(t *testing.T) {
	server := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/cart", nil)
	rec := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func newTestServer() *httptest.Server {
	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
	server := httptest.NewServer(NewServer(service).Handler())
	return server
}

type headerKV struct {
	key   string
	value string
}

func requestJSON(t *testing.T, server *httptest.Server, method, path, token string, body any, wantStatus int, extraHeaders ...headerKV) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(server.URL, "http://")
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
	server.Config.Handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s expected status %d, got %d body=%s", method, path, wantStatus, rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return payload
}

func loginAndGetToken(t *testing.T, server *httptest.Server, body map[string]any) string {
	t.Helper()
	resp := requestJSON(t, server, http.MethodPost, "/api/auth/login", "", body, http.StatusOK)
	token, ok := resp["token"].(string)
	if !ok || token == "" {
		t.Fatalf("missing token in response: %+v", resp)
	}
	return token
}

func pathf(format string, values ...any) string {
	return fmt.Sprintf(format, values...)
}
