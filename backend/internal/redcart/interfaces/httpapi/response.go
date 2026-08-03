package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
)

// decodeJSON 严格解码请求体（拒绝未知字段），空请求体统一返回 errEmptyBody。
func decodeJSON(r *http.Request, out any) error {
	if r.Body == nil {
		return errEmptyBody
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		if err.Error() == "EOF" {
			return errEmptyBody
		}
		return err
	}
	return nil
}

var errEmptyBody = fmt.Errorf("empty request body")

// parsePathID 从路径中解析数字 ID：依次剥去可选前缀 prefix 与后缀 suffix，
// 再去掉两端斜杠后解析。三种用法：
//   - 仅前缀：/api/cart/items/42 → parsePathID(path, "/api/cart/items/", "") → 42；
//   - 仅后缀：/api/orders/42/pay → parsePathID(path, "", "/pay") → 42；
//   - 前缀+后缀：/api/products/42/skus → parsePathID(path, "/api/products/", "/skus") → 42。
//
// 剥离后仍含路径分隔符（非法路径）或非数字内容时返回错误。
func parsePathID(path, prefix, suffix string) (int64, error) {
	idStr := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	idStr = strings.Trim(idStr, "/")
	if strings.Contains(idStr, "/") {
		return 0, fmt.Errorf("invalid path")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

// writeJSON 以 JSON 写出响应体与状态码。
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// setCORSHeaders 为跨域请求设置允许的来源、请求头与方法。
func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
}

// writeBadRequest 写出 400 响应，错误信息直接透传请求体解析错误。
func writeBadRequest(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": map[string]any{
			"kind":    "bad_request",
			"message": err.Error(),
		},
	})
}

// writeMethodNotAllowed 写出 405 响应。
func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
		"error": map[string]any{
			"kind":    "method_not_allowed",
			"message": "method not allowed",
		},
	})
}

const (
	defaultLimit = 20
	maxLimit     = 100
)

// parsePagination 解析 limit/offset 查询参数，limit 默认为 20 且上限 100。
func parsePagination(r *http.Request) (int, int) {
	limit := defaultLimit
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
			if limit > maxLimit {
				limit = maxLimit
			}
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

// writeAppError 将应用层错误映射为 HTTP 状态码：未授权/禁止/未找到/冲突分别
// 对应 401/403/404/409，其余按 400；非 AppError 一律视为 500 内部错误。
func writeAppError(w http.ResponseWriter, err error) {
	appErr, ok := err.(*application.AppError)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]any{
				"kind":    "internal",
				"message": err.Error(),
			},
		})
		return
	}
	status := http.StatusBadRequest
	switch appErr.Kind {
	case application.ErrorUnauthorized:
		status = http.StatusUnauthorized
	case application.ErrorForbidden:
		status = http.StatusForbidden
	case application.ErrorNotFound:
		status = http.StatusNotFound
	case application.ErrorConflict:
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"kind":    string(appErr.Kind),
			"message": appErr.Message,
		},
	})
}
