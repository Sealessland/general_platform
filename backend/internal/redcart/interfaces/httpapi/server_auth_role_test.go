package httpapi

import (
	"net/http"
	"testing"
)

func TestRoleMiddlewareRejectsCrossRoleAccess(t *testing.T) {
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
		"title":       "Forbidden Product",
		"description": "consumer should not create merchant product",
		"category_id": 1,
	}, http.StatusForbidden)

	_ = requestJSON(t, handler, http.MethodPost, "/api/orders", merchantToken, map[string]any{
		"items": []map[string]any{
			{"sku_id": 3, "quantity": 1},
		},
		"receiver_name":    "Merchant Consumer",
		"receiver_phone":   "13900000000",
		"receiver_address": "Shanghai",
	}, http.StatusForbidden, headerKV{"Idempotency-Key", "merchant-order-key"})
}

func TestAuthLogoutAndRefreshEndpoints(t *testing.T) {
	handler := newTestHandler()
	payload := requestJSON(t, handler, http.MethodPost, "/api/auth/login", "", map[string]any{
		"phone":    "13800000001",
		"password": "consumer-demo",
	}, http.StatusOK)
	refreshToken, ok := payload["refresh_token"].(string)
	if !ok || refreshToken == "" {
		t.Fatalf("expected refresh token in login response, got %+v", payload)
	}
	accessToken := payload["token"].(string)

	refreshed := requestJSON(t, handler, http.MethodPost, "/api/auth/refresh", "", map[string]any{
		"refresh_token": refreshToken,
	}, http.StatusOK)
	if refreshed["token"] == accessToken {
		t.Fatal("expected new access token from refresh")
	}

	_ = requestJSON(t, handler, http.MethodPost, "/api/auth/logout", refreshed["token"].(string), nil, http.StatusOK)
	_ = requestJSON(t, handler, http.MethodPost, "/api/auth/refresh", "", map[string]any{
		"refresh_token": refreshed["refresh_token"],
	}, http.StatusUnauthorized)
}
