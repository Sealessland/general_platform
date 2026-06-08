package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
)

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	result, err := s.service.ListNotes(r.Context())
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

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

func (s *Server) handleProducts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	result, err := s.service.ListProducts(r.Context())
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (s *Server) handleProductRoutes(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/skus"):
		s.handleProductSKUs(w, r)
	default:
		s.handleProductByID(w, r)
	}
}

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
