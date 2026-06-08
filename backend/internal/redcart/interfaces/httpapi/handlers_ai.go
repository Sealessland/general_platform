package httpapi

import (
	"net/http"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
)

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

func (s *Server) handleAITaskByID(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	id, err := parseIDFromPath(r.URL.Path, "/api/ai/tasks/")
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
