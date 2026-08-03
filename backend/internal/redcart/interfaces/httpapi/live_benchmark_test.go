package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// BenchmarkLiveHTTPHealthz 对真实服务的健康检查接口做基准测试。
func BenchmarkLiveHTTPHealthz(b *testing.B) {
	client, baseURL := newLiveBenchmarkClient(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := liveRequest(b, client, http.MethodGet, baseURL+"/healthz", "", nil, http.StatusOK)
		resp.Body.Close()
	}
}

// BenchmarkLiveHTTPOrderPreview 基准测试真实服务的订单预览接口。
func BenchmarkLiveHTTPOrderPreview(b *testing.B) {
	client, baseURL := newLiveBenchmarkClient(b)
	consumerToken := liveLogin(b, client, baseURL, "13800000001", "consumer-demo")
	merchantToken := liveLogin(b, client, baseURL, "13800000002", "merchant-demo")
	_, skuID := liveCreateOnlineProductAndSKU(b, client, baseURL, merchantToken, b.N+10)
	body := mustLiveJSON(b, map[string]any{
		"items": []map[string]any{
			{"sku_id": skuID, "quantity": 1},
		},
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := liveRequest(b, client, http.MethodPost, baseURL+"/api/orders/preview", consumerToken, body, http.StatusOK)
		resp.Body.Close()
	}
}

// BenchmarkLiveHTTPCreateOrder 基准测试真实服务的下单接口（按幂等键区分请求）。
func BenchmarkLiveHTTPCreateOrder(b *testing.B) {
	client, baseURL := newLiveBenchmarkClient(b)
	consumerToken := liveLogin(b, client, baseURL, "13800000001", "consumer-demo")
	merchantToken := liveLogin(b, client, baseURL, "13800000002", "merchant-demo")
	_, skuID := liveCreateOnlineProductAndSKU(b, client, baseURL, merchantToken, b.N+10)
	body := mustLiveJSON(b, map[string]any{
		"items": []map[string]any{
			{"sku_id": skuID, "quantity": 1},
		},
		"receiver_name":    "Benchmark Consumer",
		"receiver_phone":   "13900000002",
		"receiver_address": "Shanghai",
	})
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := liveRequestWithHeaders(
			b,
			client,
			http.MethodPost,
			baseURL+"/api/orders",
			consumerToken,
			body,
			http.StatusCreated,
			map[string]string{"Idempotency-Key": fmt.Sprintf("live-bench-order-%s-%d", suffix, i)},
		)
		resp.Body.Close()
	}
}

// newLiveBenchmarkClient 读取 LIVE_HTTP_BASE_URL 并校验服务可达，未配置时跳过基准。
func newLiveBenchmarkClient(b *testing.B) (*http.Client, string) {
	b.Helper()
	baseURL := strings.TrimRight(os.Getenv("LIVE_HTTP_BASE_URL"), "/")
	if baseURL == "" {
		b.Skip("LIVE_HTTP_BASE_URL is not set")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp := liveRequest(b, client, http.MethodGet, baseURL+"/healthz", "", nil, http.StatusOK)
	resp.Body.Close()
	return client, baseURL
}

// liveLogin 在真实服务上登录并返回 access token。
func liveLogin(b *testing.B, client *http.Client, baseURL, phone, password string) string {
	b.Helper()
	body := mustLiveJSON(b, map[string]any{
		"phone":    phone,
		"password": password,
	})
	resp := liveRequest(b, client, http.MethodPost, baseURL+"/api/auth/login", "", body, http.StatusOK)
	defer resp.Body.Close()
	payload := liveDecodeJSON(b, resp.Body)
	token, ok := payload["token"].(string)
	if !ok || token == "" {
		b.Fatalf("missing token in login response: %+v", payload)
	}
	return token
}

// liveCreateOnlineProductAndSKU 在真实服务上创建上架商品与 SKU，返回两者 ID。
func liveCreateOnlineProductAndSKU(b *testing.B, client *http.Client, baseURL, merchantToken string, stock int) (int64, int64) {
	b.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	productResp := liveRequest(b, client, http.MethodPost, baseURL+"/api/merchant/products", merchantToken, mustLiveJSON(b, map[string]any{
		"title":          "Live Bench Product " + suffix,
		"description":    "real network benchmark product",
		"category_id":    20260617,
		"selling_points": []string{"live", "benchmark"},
	}), http.StatusCreated)
	product := liveDecodeJSON(b, productResp.Body)
	productResp.Body.Close()
	productID := int64LiveField(b, product, "id")

	skuResp := liveRequest(b, client, http.MethodPost, fmt.Sprintf("%s/api/merchant/products/%d/skus", baseURL, productID), merchantToken, mustLiveJSON(b, map[string]any{
		"sku_name":   "Live Bench SKU " + suffix,
		"sku_attrs":  map[string]string{"size": "standard"},
		"price_cent": 12345,
		"stock":      stock,
		"status":     "active",
	}), http.StatusCreated)
	sku := liveDecodeJSON(b, skuResp.Body)
	skuResp.Body.Close()
	skuID := int64LiveField(b, sku, "id")

	onlineResp := liveRequest(b, client, http.MethodPost, fmt.Sprintf("%s/api/merchant/products/%d/online", baseURL, productID), merchantToken, nil, http.StatusOK)
	onlineResp.Body.Close()
	return productID, skuID
}

// liveRequest 发起真实 HTTP 请求并断言期望状态码。
func liveRequest(b *testing.B, client *http.Client, method, url, token string, body []byte, wantStatus int) *http.Response {
	b.Helper()
	return liveRequestWithHeaders(b, client, method, url, token, body, wantStatus, nil)
}

// liveRequestWithHeaders 附带自定义请求头发起真实请求并断言状态码。
func liveRequestWithHeaders(b *testing.B, client *http.Client, method, url, token string, body []byte, wantStatus int, headers map[string]string) *http.Response {
	b.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		b.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		b.Fatalf("%s %s: %v", method, url, err)
	}
	if resp.StatusCode != wantStatus {
		defer resp.Body.Close()
		payload, _ := io.ReadAll(resp.Body)
		b.Fatalf("%s %s expected status %d, got %d body=%s", method, url, wantStatus, resp.StatusCode, string(payload))
	}
	return resp
}

// mustLiveJSON 将值序列化为 JSON 字节，失败直接终止基准。
func mustLiveJSON(b *testing.B, value any) []byte {
	b.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		b.Fatalf("marshal json: %v", err)
	}
	return payload
}

// liveDecodeJSON 解析 JSON 响应体为 map。
func liveDecodeJSON(b *testing.B, reader io.Reader) map[string]any {
	b.Helper()
	var payload map[string]any
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		b.Fatalf("decode json: %v", err)
	}
	return payload
}

// int64LiveField 从基准响应中读取数字字段并转为 int64。
func int64LiveField(b *testing.B, payload map[string]any, key string) int64 {
	b.Helper()
	value, ok := payload[key].(float64)
	if !ok {
		b.Fatalf("missing numeric field %s in %+v", key, payload)
	}
	return int64(value)
}
