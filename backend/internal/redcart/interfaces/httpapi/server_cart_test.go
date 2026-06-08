package httpapi

import (
	"net/http"
	"testing"
)

func TestCartHTTPFlowAndValidation(t *testing.T) {
	handler := newTestHandler()
	consumerToken := loginAndGetToken(t, handler, map[string]any{
		"phone":    "13800000001",
		"password": "consumer-demo",
	})

	_ = requestJSON(t, handler, http.MethodPost, "/api/cart/items", consumerToken, map[string]any{
		"sku_id":   1,
		"quantity": 0,
	}, http.StatusBadRequest)
	item := requestJSON(t, handler, http.MethodPost, "/api/cart/items", consumerToken, map[string]any{
		"sku_id":   1,
		"quantity": 1,
	}, http.StatusCreated)
	itemID := int64Field(t, item, "id")

	updated := requestJSON(t, handler, http.MethodPut, pathf("/api/cart/items/%d", itemID), consumerToken, map[string]any{
		"quantity": 2,
		"selected": false,
	}, http.StatusOK)
	if int64Field(t, updated, "quantity") != 2 || updated["selected"].(bool) {
		t.Fatalf("expected updated quantity and selection, got %+v", updated)
	}
	cart := requestJSON(t, handler, http.MethodGet, "/api/cart", consumerToken, nil, http.StatusOK)
	if int64Field(t, cart, "selected_item_count") != 0 || int64Field(t, cart, "selected_amount_cent") != 0 {
		t.Fatalf("expected no selected totals, got %+v", cart)
	}

	_ = requestJSON(t, handler, http.MethodDelete, pathf("/api/cart/items/%d", itemID), consumerToken, nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodDelete, pathf("/api/cart/items/%d", itemID), consumerToken, nil, http.StatusNotFound)
}
