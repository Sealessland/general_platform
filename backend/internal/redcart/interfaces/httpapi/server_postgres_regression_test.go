package httpapi

import (
	"net/http"
	"testing"
)

func TestPostgresHTTPAuthCatalogAndCartRegression(t *testing.T) {
	handler, cleanup := newPostgresTestHandler(t)
	defer cleanup()

	suffix := uniqueSuffix()
	consumerPhone := "139" + suffix[len(suffix)-8:]
	consumerPassword := "pass-" + suffix
	consumerSession := requestJSON(t, handler, http.MethodPost, "/api/auth/register", "", map[string]any{
		"nickname": "pg-auth-cart-" + suffix,
		"phone":    consumerPhone,
		"password": consumerPassword,
		"role":     "consumer",
	}, http.StatusCreated)
	consumerToken := consumerSession["token"].(string)
	merchantToken := registerAndGetToken(t, handler, "pg-auth-cart-merchant-"+suffix, "merchant")
	productID, skuID := createOnlineProductAndSKU(t, handler, merchantToken, suffix, 5)

	me := requestJSON(t, handler, http.MethodGet, "/api/auth/me", consumerToken, nil, http.StatusOK)
	if me["role"].(string) != "consumer" {
		t.Fatalf("expected consumer me response, got %+v", me)
	}
	login := requestJSON(t, handler, http.MethodPost, "/api/auth/login", "", map[string]any{
		"phone":    consumerPhone,
		"password": consumerPassword,
	}, http.StatusOK)
	refreshed := requestJSON(t, handler, http.MethodPost, "/api/auth/refresh", "", map[string]any{
		"refresh_token": login["refresh_token"].(string),
	}, http.StatusOK)
	if refreshed["token"].(string) == "" || refreshed["refresh_token"].(string) == "" {
		t.Fatalf("expected refreshed tokens, got %+v", refreshed)
	}
	_ = requestJSON(t, handler, http.MethodPost, "/api/auth/logout", consumerToken, nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodGet, "/api/auth/me", consumerToken, nil, http.StatusUnauthorized)
	_ = requestJSON(t, handler, http.MethodPost, "/api/auth/login", "", map[string]any{
		"phone":    "13800000001",
		"password": "wrong",
	}, http.StatusUnauthorized)

	notes := requestJSON(t, handler, http.MethodGet, "/api/notes", "", nil, http.StatusOK)
	firstNote := firstItem(t, notes)
	note := requestJSON(t, handler, http.MethodGet, pathf("/api/notes/%d", int64Field(t, firstNote, "id")), refreshed["token"].(string), nil, http.StatusOK)
	if int64Field(t, note, "view_count") <= int64Field(t, firstNote, "view_count") {
		t.Fatalf("expected note view count to increase, before=%+v after=%+v", firstNote, note)
	}
	products := requestJSON(t, handler, http.MethodGet, "/api/products", "", nil, http.StatusOK)
	if len(products["items"].([]any)) == 0 {
		t.Fatalf("expected product list, got %+v", products)
	}
	product := requestJSON(t, handler, http.MethodGet, pathf("/api/products/%d", productID), refreshed["token"].(string), nil, http.StatusOK)
	if int64Field(t, product, "id") != productID {
		t.Fatalf("expected product %d, got %+v", productID, product)
	}
	_ = requestJSON(t, handler, http.MethodGet, "/api/products/999999999/skus", "", nil, http.StatusNotFound)

	item := requestJSON(t, handler, http.MethodPost, "/api/cart/items", refreshed["token"].(string), map[string]any{
		"sku_id":   skuID,
		"quantity": 2,
	}, http.StatusCreated)
	itemID := int64Field(t, item, "id")
	selected := false
	updated := requestJSON(t, handler, http.MethodPut, pathf("/api/cart/items/%d", itemID), refreshed["token"].(string), map[string]any{
		"quantity": 1,
		"selected": selected,
	}, http.StatusOK)
	if updated["selected"].(bool) {
		t.Fatalf("expected cart item unselected, got %+v", updated)
	}
	cart := requestJSON(t, handler, http.MethodGet, "/api/cart", refreshed["token"].(string), nil, http.StatusOK)
	if int64Field(t, cart, "selected_item_count") != 0 {
		t.Fatalf("expected no selected cart items, got %+v", cart)
	}
	_ = requestJSON(t, handler, http.MethodDelete, pathf("/api/cart/items/%d", itemID), refreshed["token"].(string), nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodDelete, pathf("/api/cart/items/%d", itemID), refreshed["token"].(string), nil, http.StatusNotFound)
}

func TestPostgresHTTPMerchantAndAIRegression(t *testing.T) {
	handler, cleanup := newPostgresTestHandler(t)
	defer cleanup()

	suffix := uniqueSuffix()
	consumerToken := registerAndGetToken(t, handler, "pg-merchant-ai-consumer-"+suffix, "consumer")
	merchantToken := registerAndGetToken(t, handler, "pg-merchant-ai-"+suffix, "merchant")
	_ = requestJSON(t, handler, http.MethodPost, "/api/merchant/products", consumerToken, map[string]any{
		"title":       "Forbidden",
		"description": "consumer cannot create",
		"category_id": 1,
	}, http.StatusForbidden)
	_ = requestJSON(t, handler, http.MethodPost, "/api/merchant/products", merchantToken, map[string]any{
		"description": "missing title",
		"category_id": 1,
	}, http.StatusBadRequest)

	product := requestJSON(t, handler, http.MethodPost, "/api/merchant/products", merchantToken, map[string]any{
		"title":          "PG Merchant AI " + suffix,
		"description":    "merchant regression product",
		"category_id":    20260617,
		"selling_points": []string{"merchant", "ai"},
	}, http.StatusCreated)
	productID := int64Field(t, product, "id")
	updated := requestJSON(t, handler, http.MethodPut, pathf("/api/merchant/products/%d", productID), merchantToken, map[string]any{
		"title":          "PG Merchant AI Updated " + suffix,
		"description":    "updated",
		"category_id":    20260618,
		"selling_points": []string{"updated"},
	}, http.StatusOK)
	if updated["title"].(string) == product["title"].(string) {
		t.Fatalf("expected product title update, got %+v", updated)
	}
	sku := requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/products/%d/skus", productID), merchantToken, map[string]any{
		"sku_name":   "PG Merchant AI SKU",
		"sku_attrs":  map[string]string{"color": "red"},
		"price_cent": 1800,
		"stock":      3,
	}, http.StatusCreated)
	skuID := int64Field(t, sku, "id")
	updatedSKU := requestJSON(t, handler, http.MethodPut, pathf("/api/merchant/skus/%d", skuID), merchantToken, map[string]any{
		"sku_name":   "PG Merchant AI SKU Updated",
		"sku_attrs":  map[string]string{"color": "blue"},
		"price_cent": 1900,
		"stock":      3,
		"status":     "active",
	}, http.StatusOK)
	if int64Field(t, updatedSKU, "price_cent") != 1900 {
		t.Fatalf("expected updated sku price, got %+v", updatedSKU)
	}
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/merchant/products/%d/online", productID), merchantToken, nil, http.StatusOK)
	merchantProducts := requestJSON(t, handler, http.MethodGet, "/api/merchant/products", merchantToken, nil, http.StatusOK)
	if len(merchantProducts["items"].([]any)) == 0 {
		t.Fatalf("expected merchant products, got %+v", merchantProducts)
	}
	order := createOrder(t, handler, consumerToken, skuID, "pg-merchant-ai-order-"+suffix)
	orderID := int64Field(t, order, "id")
	_ = requestJSON(t, handler, http.MethodPost, pathf("/api/orders/%d/pay", orderID), consumerToken, nil, http.StatusOK)
	merchantOrders := requestJSON(t, handler, http.MethodGet, "/api/merchant/orders", merchantToken, nil, http.StatusOK)
	if len(merchantOrders["items"].([]any)) == 0 {
		t.Fatalf("expected merchant orders, got %+v", merchantOrders)
	}
	_ = requestJSON(t, handler, http.MethodGet, pathf("/api/merchant/orders/%d", orderID), merchantToken, nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodGet, "/api/merchant/dashboard/funnel", merchantToken, nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodGet, "/api/merchant/dashboard/products", merchantToken, nil, http.StatusOK)

	review := requestJSON(t, handler, http.MethodPost, "/api/ai/business-review", merchantToken, map[string]any{
		"window_days": 7,
		"product_id":  productID,
	}, http.StatusOK)
	if review["status"].(string) != "completed" {
		t.Fatalf("expected completed review, got %+v", review)
	}
	a2ui := requestJSON(t, handler, http.MethodPost, "/api/ai/a2ui", consumerToken, map[string]any{
		"surface_id":   "shopping-guide",
		"user_intent":  "300元通勤收纳",
		"context_json": `{"scene":"office_desk"}`,
	}, http.StatusOK)
	if a2ui["a2ui_json"].(string) == "" {
		t.Fatalf("expected a2ui json, got %+v", a2ui)
	}
	_ = requestJSON(t, handler, http.MethodPost, "/api/ai/a2ui", consumerToken, map[string]any{
		"user_intent": "missing surface",
	}, http.StatusBadRequest)
}

func firstItem(t testing.TB, payload map[string]any) map[string]any {
	t.Helper()
	items, ok := payload["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("missing items in response: %+v", payload)
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected item shape: %+v", items[0])
	}
	return item
}
