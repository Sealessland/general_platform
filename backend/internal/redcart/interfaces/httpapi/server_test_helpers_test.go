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
	postgresrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/postgres"
	redisrepo "github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/redis"
)

var testUniqueCounter atomic.Int64

// newPostgresTestHandler 构造接入真实 PostgreSQL 与 Redis 的 HTTP 处理器，
// 未设置 RUN_POSTGRES_INTEGRATION 等环境变量时跳过测试。
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
	wrapped, redisCleanup := wrapPostgresRepoWithRedisForTest(t, repo)
	service := application.NewService(wrapped, backendai.MockProvider{})
	return NewServer(service).Handler(), func() {
		redisCleanup()
		if err := repo.Close(); err != nil {
			t.Fatalf("close postgres repository: %v", err)
		}
	}
}

// wrapPostgresRepoWithRedisForTest 用 Redis 会话仓库包装基础仓库，返回关闭 Redis 的清理函数。
func wrapPostgresRepoWithRedisForTest(t testing.TB, base application.Repository) (application.Repository, func()) {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Fatalf("REDIS_ADDR is required for postgres integration tests")
	}

	client, err := redisrepo.NewClient(addr)
	if err != nil {
		t.Fatalf("new redis client: %v", err)
	}
	accessTTL, err := redisrepo.AccessTokenTTLFromEnv(os.Getenv("REDIS_ACCESS_TOKEN_TTL"))
	if err != nil {
		_ = client.Close()
		t.Fatalf("parse redis access token ttl: %v", err)
	}
	refreshTTL, err := redisrepo.RefreshTokenTTLFromEnv(os.Getenv("REDIS_REFRESH_TOKEN_TTL"))
	if err != nil {
		_ = client.Close()
		t.Fatalf("parse redis refresh token ttl: %v", err)
	}
	return redisrepo.NewSessionRepository(base, client, accessTTL, refreshTTL), func() {
		if err := client.Close(); err != nil {
			t.Fatalf("close redis client: %v", err)
		}
	}
}

type headerKV struct {
	key   string
	value string
}

// requestJSON 发送 JSON 请求并断言期望状态码，返回解析后的响应体。
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

// postJSONStatus 发送请求并仅返回状态码，用于只关心成功/失败的断言。
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

// loginAndGetToken 登录并返回 access token。
func loginAndGetToken(t testing.TB, handler http.Handler, body map[string]any) string {
	t.Helper()
	resp := requestJSON(t, handler, http.MethodPost, "/api/auth/login", "", body, http.StatusOK)
	token, ok := resp["token"].(string)
	if !ok || token == "" {
		t.Fatalf("missing token in response: %+v", resp)
	}
	return token
}

// registerAndGetToken 注册指定角色账号并返回其 access token。
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

// createOnlineProductAndSKU 创建上架商品及其 SKU，返回两者 ID。
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

// createOrder 以指定幂等键创建一个待支付订单，返回订单响应体。
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

// assertSKUStock 断言商品 SKU 的库存与锁定库存。
func assertSKUStock(t testing.TB, handler http.Handler, productID, skuID int64, wantStock, wantLockedStock int64) {
	t.Helper()
	skus := requestJSON(t, handler, http.MethodGet, pathf("/api/products/%d/skus", productID), "", nil, http.StatusOK)
	sku := findSKU(t, skus, skuID)
	if int64Field(t, sku, "stock") != wantStock || int64Field(t, sku, "locked_stock") != wantLockedStock {
		t.Fatalf("expected sku %d stock=%d locked_stock=%d, got %+v", skuID, wantStock, wantLockedStock, sku)
	}
}

// pathf 格式化拼接路径，避免在调用处重复内联 fmt.Sprintf。
func pathf(format string, values ...any) string {
	return fmt.Sprintf(format, values...)
}

// productStatusFromList 从商品列表响应中提取指定商品的 status。
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

// findSKU 从 SKU 列表响应中按 ID 查找 SKU。
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

// int64Field 从 JSON 响应中读取数字字段并转为 int64。
func int64Field(t testing.TB, payload map[string]any, field string) int64 {
	t.Helper()
	value, ok := payload[field].(float64)
	if !ok {
		t.Fatalf("missing numeric field %s in response: %+v", field, payload)
	}
	return int64(value)
}

// uniqueSuffix 生成基于时间戳与计数器的唯一后缀，避免测试数据相互冲突。
func uniqueSuffix() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), testUniqueCounter.Add(1))
}
