package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
)

// handleOrderPreview 下单前预览订单：校验库存并计算金额，不落库。
func (s *Server) handleOrderPreview(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var input application.CheckoutInput
	if err := decodeJSON(r, &input); err != nil && err != errEmptyBody {
		writeBadRequest(w, err)
		return
	}
	result, err := s.service.PreviewOrder(r.Context(), actor, input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleOrders 按方法分发：GET 分页查询订单，POST 创建订单（携带幂等键）。
func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	switch r.Method {
	case http.MethodGet:
		limit, offset := parsePagination(r)
		result, err := s.service.ListOrders(r.Context(), actor, limit, offset)
		if err != nil {
			writeAppError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": result})
	case http.MethodPost:
		var input application.CheckoutInput
		if err := decodeJSON(r, &input); err != nil {
			writeBadRequest(w, err)
			return
		}
		result, err := s.service.CreateOrder(r.Context(), actor, r.Header.Get("Idempotency-Key"), input)
		if err != nil {
			writeAppError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, result)
	default:
		writeMethodNotAllowed(w)
	}
}

// handleOrderByID 按路径后缀分发订单动作：/pay、/cancel、/finish、/refund 为状态
// 流转接口，其余视为按 ID 查询。各动作仅接受 POST，避免 GET 触发状态变更。
func (s *Server) handleOrderByID(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	path := strings.TrimPrefix(r.URL.Path, "/api/orders/")
	switch {
	case strings.HasSuffix(path, "/pay"):
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		id, err := parseSuffixID(path, "/pay")
		if err != nil {
			writeBadRequest(w, err)
			return
		}
		result, svcErr := s.service.PayOrder(r.Context(), actor, id)
		if svcErr != nil {
			writeAppError(w, svcErr)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case strings.HasSuffix(path, "/cancel"):
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		id, err := parseSuffixID(path, "/cancel")
		if err != nil {
			writeBadRequest(w, err)
			return
		}
		result, svcErr := s.service.CancelOrder(r.Context(), actor, id)
		if svcErr != nil {
			writeAppError(w, svcErr)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case strings.HasSuffix(path, "/finish"):
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		id, err := parseSuffixID(path, "/finish")
		if err != nil {
			writeBadRequest(w, err)
			return
		}
		result, svcErr := s.service.FinishOrder(r.Context(), actor, id)
		if svcErr != nil {
			writeAppError(w, svcErr)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case strings.HasSuffix(path, "/refund"):
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		id, err := parseSuffixID(path, "/refund")
		if err != nil {
			writeBadRequest(w, err)
			return
		}
		var input application.RefundRequestInput
		if err := decodeJSON(r, &input); err != nil && err != errEmptyBody {
			writeBadRequest(w, err)
			return
		}
		result, svcErr := s.service.RequestRefund(r.Context(), actor, id, input)
		if svcErr != nil {
			writeAppError(w, svcErr)
			return
		}
		writeJSON(w, http.StatusAccepted, result)
	default:
		id, err := strconv.ParseInt(strings.Trim(path, "/"), 10, 64)
		if err != nil {
			writeBadRequest(w, fmt.Errorf("invalid order id"))
			return
		}
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}
		result, svcErr := s.service.GetOrder(r.Context(), actor, id)
		if svcErr != nil {
			writeAppError(w, svcErr)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}
