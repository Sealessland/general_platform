package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnauthorizedRequestsRejected(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/cart", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
func TestBaseRoutesCatalogAndCORS(t *testing.T) {
	handler := newTestHandler()

	health := requestJSON(t, handler, http.MethodGet, "/healthz", "", nil, http.StatusOK)
	if health["status"].(string) != "ok" {
		t.Fatalf("expected health ok, got %+v", health)
	}

	optionsReq := httptest.NewRequest(http.MethodOptions, "/api/notes", nil)
	optionsRec := httptest.NewRecorder()
	handler.ServeHTTP(optionsRec, optionsReq)
	if optionsRec.Code != http.StatusNoContent {
		t.Fatalf("expected options 204, got %d", optionsRec.Code)
	}
	if optionsRec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("expected cors headers, got %+v", optionsRec.Header())
	}

	_ = requestJSON(t, handler, http.MethodGet, "/api/missing", "", nil, http.StatusNotFound)
	_ = requestJSON(t, handler, http.MethodPost, "/api/auth/login", "", "{", http.StatusBadRequest)

	notes := requestJSON(t, handler, http.MethodGet, "/api/notes", "", nil, http.StatusOK)
	if len(notes["items"].([]any)) == 0 {
		t.Fatalf("expected notes, got %+v", notes)
	}
	note := requestJSON(t, handler, http.MethodGet, "/api/notes/1", "", nil, http.StatusOK)
	if len(note["linked_products"].([]any)) == 0 {
		t.Fatalf("expected linked products, got %+v", note)
	}
	products := requestJSON(t, handler, http.MethodGet, "/api/products", "", nil, http.StatusOK)
	if len(products["items"].([]any)) == 0 {
		t.Fatalf("expected products, got %+v", products)
	}
	product := requestJSON(t, handler, http.MethodGet, "/api/products/1", "", nil, http.StatusOK)
	if len(product["skus"].([]any)) == 0 {
		t.Fatalf("expected product skus, got %+v", product)
	}
	skus := requestJSON(t, handler, http.MethodGet, "/api/products/1/skus", "", nil, http.StatusOK)
	if len(skus["items"].([]any)) == 0 {
		t.Fatalf("expected sku list, got %+v", skus)
	}
}
func TestMeAndListEndpoints(t *testing.T) {
	handler := newTestHandler()
	consumerToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000001",
		"password": "consumer-demo",
	})
	merchantToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000002",
		"password": "merchant-demo",
	})

	me := requestJSON(t, handler, http.MethodGet, "/api/auth/me", consumerToken, nil, http.StatusOK)
	if me["role"].(string) != "consumer" {
		t.Fatalf("expected consumer me, got %+v", me)
	}
	orders := requestJSON(t, handler, http.MethodGet, "/api/orders", consumerToken, nil, http.StatusOK)
	if len(orders["items"].([]any)) == 0 {
		t.Fatalf("expected consumer orders, got %+v", orders)
	}
	merchantOrders := requestJSON(t, handler, http.MethodGet, "/api/merchant/orders", merchantToken, nil, http.StatusOK)
	items := merchantOrders["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("expected merchant orders, got %+v", merchantOrders)
	}
	orderID := int64Field(t, items[0].(map[string]any), "id")
	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/merchant/orders/%d", orderID), merchantToken, nil, http.StatusOK)
}
func TestAuthAndCatalogHTTPValidation(t *testing.T) {
	handler := newTestHandler()

	_ = requestJSON(t, handler, http.MethodPost, "/api/auth/register", "", map[string]any{
		"nickname": "Bad Role",
		"phone":    "13910000001",
		"password": "secret",
		"role":     "admin",
	}, http.StatusBadRequest)
	registered := requestJSON(t, handler, http.MethodPost, "/api/auth/register", "", map[string]any{
		"nickname": "New Merchant",
		"phone":    "13910000002",
		"password": "secret",
		"role":     "merchant",
	}, http.StatusCreated)
	if registered["token"].(string) == "" {
		t.Fatalf("expected token in register response: %+v", registered)
	}
	user := registered["user"].(map[string]any)
	if int64Field(t, user, "merchant_id") == 0 {
		t.Fatalf("expected merchant_id in register response: %+v", user)
	}
	_ = requestJSON(t, handler, http.MethodPost, "/api/auth/login", "", map[string]any{
		"phone":    "13910000002",
		"password": "wrong",
	}, http.StatusUnauthorized)

	_ = requestJSON(t, handler, http.MethodGet, "/api/notes/999999", "", nil, http.StatusNotFound)
	_ = requestJSON(t, handler, http.MethodGet, "/api/products/999999", "", nil, http.StatusNotFound)
	_ = requestJSON(t, handler, http.MethodGet, "/api/products/999999/skus", "", nil, http.StatusNotFound)
}
