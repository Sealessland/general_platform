package httpapi

import (
	"fmt"
	"net/http"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
)

// handleAISellingPoints 处理商家提交商品信息、生成卖点提炼的 POST 请求。
func (s *Server) handleAISellingPoints(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var input application.SellingPointInput
	if err := decodeJSON(r, &input); err != nil {
		writeBadRequest(w, err)
		return
	}
	result, err := s.service.GenerateSellingPoints(r.Context(), actor, input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleAIBusinessReview 处理商家提交窗口参数、生成经营复盘的 POST 请求。
func (s *Server) handleAIBusinessReview(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var input application.BusinessReviewInput
	if err := decodeJSON(r, &input); err != nil {
		writeBadRequest(w, err)
		return
	}
	result, err := s.service.GenerateBusinessReview(r.Context(), actor, input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleAIA2UISurface 生成 A2UI 交互界面，surface_id 与 user_intent 为必填。
func (s *Server) handleAIA2UISurface(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var input application.A2UISurfaceInput
	if err := decodeJSON(r, &input); err != nil {
		writeBadRequest(w, err)
		return
	}
	if input.SurfaceID == "" {
		writeBadRequest(w, fmt.Errorf("surface_id is required"))
		return
	}
	if input.UserIntent == "" {
		writeBadRequest(w, fmt.Errorf("user_intent is required"))
		return
	}
	result, err := s.service.GenerateA2UISurface(r.Context(), actor, input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleAITaskByID 按 ID 查询 AI 生成任务的执行结果。
func (s *Server) handleAITaskByID(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	id, err := parsePathID(r.URL.Path, "/api/ai/tasks/", "")
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	result, svcErr := s.service.GetAITask(r.Context(), actor, id)
	if svcErr != nil {
		writeAppError(w, svcErr)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
