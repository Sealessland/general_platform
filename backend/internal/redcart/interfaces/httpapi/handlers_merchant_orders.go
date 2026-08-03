package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
)

// handleMerchantOrderByID 按路径后缀分发：/ship 发货、/refund/approve 审核退款，
// 其余视为按 ID 查询订单。
func (s *Server) handleMerchantOrderByID(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	path := strings.TrimPrefix(r.URL.Path, "/api/merchant/orders/")
	switch {
	case strings.HasSuffix(path, "/ship"):
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		id, err := parsePathID(path, "", "/ship")
		if err != nil {
			writeBadRequest(w, err)
			return
		}
		var input application.MerchantOrderShipInput
		if err := decodeJSON(r, &input); err != nil && err != errEmptyBody {
			writeBadRequest(w, err)
			return
		}
		result, svcErr := s.service.MerchantShipOrder(r.Context(), actor, id, input)
		if svcErr != nil {
			writeAppError(w, svcErr)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case strings.HasSuffix(path, "/refund/approve"):
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		id, err := parsePathID(path, "", "/refund/approve")
		if err != nil {
			writeBadRequest(w, err)
			return
		}
		result, svcErr := s.service.MerchantApproveRefund(r.Context(), actor, id)
		if svcErr != nil {
			writeAppError(w, svcErr)
			return
		}
		writeJSON(w, http.StatusOK, result)
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

// handleMerchantDashboardFunnel 返回商家经营漏斗统计（浏览→加购→下单→支付）。
func (s *Server) handleMerchantDashboardFunnel(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	result, err := s.service.DashboardFunnel(r.Context(), actor)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleMerchantDashboardProducts 返回商家商品维度的经营排行。
func (s *Server) handleMerchantDashboardProducts(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	result, err := s.service.DashboardProducts(r.Context(), actor)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

// handleMerchantDashboardSummary 返回商家经营总览统计。
func (s *Server) handleMerchantDashboardSummary(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	result, err := s.service.DashboardSummary(r.Context(), actor)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
