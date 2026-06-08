package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
	postgresrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres"
)

var testUniqueCounter atomic.Int64

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
