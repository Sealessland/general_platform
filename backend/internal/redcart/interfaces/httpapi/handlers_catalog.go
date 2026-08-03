package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
)

// handleNotes 分页查询种草笔记列表。
func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	limit, offset := parsePagination(r)
	result, err := s.service.ListNotes(r.Context(), limit, offset)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

// handleNoteByID 详情接口允许匿名访问：尝试认证成功则附带身份（用于浏览计数等），
// 失败则以 nil Actor 继续。
func (s *Server) handleNoteByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	id, err := parseIDFromPath(r.URL.Path, "/api/notes/")
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	var actor *application.Actor
	if authed, authErr := s.authenticate(r); authErr == nil {
		actor = authed
	}
	result, svcErr := s.service.GetNote(r.Context(), id, actor)
	if svcErr != nil {
		writeAppError(w, svcErr)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleProducts 分页查询在售商品列表。
func (s *Server) handleProducts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	limit, offset := parsePagination(r)
	result, err := s.service.ListProducts(r.Context(), limit, offset)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

// handleProductByID 与 handleNoteByID 相同，允许匿名访问，认证成功则附带身份。
func (s *Server) handleProductByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	id, err := parseIDFromPath(r.URL.Path, "/api/products/")
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	var actor *application.Actor
	if authed, authErr := s.authenticate(r); authErr == nil {
		actor = authed
	}
	result, svcErr := s.service.GetProduct(r.Context(), id, actor)
	if svcErr != nil {
		writeAppError(w, svcErr)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleProductSKUs 查询指定商品的全部 SKU。
func (s *Server) handleProductSKUs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	idStr := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/products/"), "/skus")
	id, err := strconv.ParseInt(strings.Trim(idStr, "/"), 10, 64)
	if err != nil {
		writeBadRequest(w, fmt.Errorf("invalid product id"))
		return
	}
	result, svcErr := s.service.ListProductSKUs(r.Context(), id)
	if svcErr != nil {
		writeAppError(w, svcErr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}
