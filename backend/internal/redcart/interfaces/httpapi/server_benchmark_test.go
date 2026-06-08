package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
)

func BenchmarkHTTPNotes(b *testing.B) {
	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
	handler := NewServer(service).Handler()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/notes", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d", rec.Code)
		}
	}
}

func BenchmarkHTTPPostgresOrderPreview(b *testing.B) {
	handler, cleanup := newPostgresTestHandler(b)
	defer cleanup()

	suffix := uniqueSuffix()
	consumerToken := registerAndGetToken(b, handler, "pg-bench-preview-consumer-"+suffix, "consumer")
	merchantToken := registerAndGetToken(b, handler, "pg-bench-preview-merchant-"+suffix, "merchant")
	_, skuID := createOnlineProductAndSKU(b, handler, merchantToken, suffix, b.N+10)
	body := mustJSON(b, map[string]any{
		"items": []map[string]any{
			{"sku_id": skuID, "quantity": 1},
		},
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/orders/preview", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+consumerToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
	}
}

func BenchmarkHTTPPostgresCreateOrder(b *testing.B) {
	handler, cleanup := newPostgresTestHandler(b)
	defer cleanup()

	suffix := uniqueSuffix()
	consumerToken := registerAndGetToken(b, handler, "pg-bench-order-consumer-"+suffix, "consumer")
	merchantToken := registerAndGetToken(b, handler, "pg-bench-order-merchant-"+suffix, "merchant")
	_, skuID := createOnlineProductAndSKU(b, handler, merchantToken, suffix, b.N+10)
	body := mustJSON(b, map[string]any{
		"items": []map[string]any{
			{"sku_id": skuID, "quantity": 1},
		},
		"receiver_name":    "Benchmark Consumer",
		"receiver_phone":   "13900000002",
		"receiver_address": "Shanghai",
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/orders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+consumerToken)
		req.Header.Set("Idempotency-Key", fmt.Sprintf("pg-bench-order-%s-%d", suffix, i))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			b.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
		}
	}
}

func mustJSON(t testing.TB, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return payload
}

func BenchmarkHTTPOrderPreview(b *testing.B) {
	repo := memory.NewRepository()
	service := application.NewService(repo, backendai.MockProvider{})
	handler := NewServer(service).Handler()

	loginBody, err := json.Marshal(map[string]any{
		"phone":    "13800000001",
		"password": "consumer-demo",
	})
	if err != nil {
		b.Fatalf("marshal login body: %v", err)
	}
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		b.Fatalf("login failed, status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(loginRec.Body.Bytes(), &payload); err != nil {
		b.Fatalf("decode login response: %v", err)
	}
	token, ok := payload["token"].(string)
	if !ok || token == "" {
		b.Fatal("missing token in login response")
	}

	orderBody, err := json.Marshal(map[string]any{})
	if err != nil {
		b.Fatalf("marshal preview body: %v", err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/orders/preview", bytes.NewReader(orderBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d", rec.Code)
		}
	}
}
