package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
	postgresrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres"
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

func BenchmarkHTTPPostgresCreateOrderWithOutbox(b *testing.B) {
	repo, handler, cleanup := newPostgresTestHandlerWithBaseRepo(b)
	defer cleanup()

	suffix := uniqueSuffix()
	consumerToken := registerAndGetToken(b, handler, "pg-bench-order-outbox-consumer-"+suffix, "consumer")
	merchantToken := registerAndGetToken(b, handler, "pg-bench-order-outbox-merchant-"+suffix, "merchant")
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
		req.Header.Set("Idempotency-Key", fmt.Sprintf("pg-bench-order-outbox-%s-%d", suffix, i))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			b.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
		}
	}
	b.StopTimer()

	// Verify that the outbox captured the expected number of events. This
	// confirms the transactional outbox path is exercised by the benchmark.
	pending, err := repo.Outbox.PollPending(context.Background(), b.N+10)
	if err != nil {
		b.Fatalf("poll pending outbox: %v", err)
	}
	if len(pending) != b.N {
		b.Fatalf("expected %d outbox events, got %d", b.N, len(pending))
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

// newPostgresTestHandlerWithBaseRepo returns the underlying postgres Repository
// so benchmarks can inspect the outbox table directly.
func newPostgresTestHandlerWithBaseRepo(t testing.TB) (*postgresrepo.Repository, http.Handler, func()) {
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
	wrapped, redisCleanup := wrapPostgresRepoWithRedisForTest(t, repo)
	service := application.NewService(wrapped, backendai.MockProvider{})
	return repo, NewServer(service).Handler(), func() {
		redisCleanup()
		if err := repo.Close(); err != nil {
			t.Fatalf("close postgres repository: %v", err)
		}
	}
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
