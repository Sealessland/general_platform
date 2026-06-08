package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
)

func (s *Server) handleMerchantProducts(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	switch r.Method {
	case http.MethodGet:
		result, err := s.service.MerchantListProducts(r.Context(), actor)
		if err != nil {
			writeAppError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": result})
	case http.MethodPost:
		var input application.MerchantProductInput
		if err := decodeJSON(r, &input); err != nil {
			writeBadRequest(w, err)
			return
		}
		result, err := s.service.MerchantCreateProduct(r.Context(), actor, input)
		if err != nil {
			writeAppError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, result)
	default:
		writeMethodNotAllowed(w)
	}
}

func (s *Server) handleMerchantProductByID(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	path := strings.TrimPrefix(r.URL.Path, "/api/merchant/products/")
	switch {
	case strings.HasSuffix(path, "/skus"):
		handleMerchantCreateSKU(w, r, actor, path, s)
	case strings.HasSuffix(path, "/online"):
		handleMerchantSetProductStatus(w, r, actor, path, s, "online")
	case strings.HasSuffix(path, "/offline"):
		handleMerchantSetProductStatus(w, r, actor, path, s, "offline")
	default:
		id, err := strconv.ParseInt(strings.Trim(path, "/"), 10, 64)
		if err != nil {
			writeBadRequest(w, fmt.Errorf("invalid product id"))
			return
		}
		if r.Method != http.MethodPut {
			writeMethodNotAllowed(w)
			return
		}
		var input application.MerchantProductInput
		if err := decodeJSON(r, &input); err != nil {
			writeBadRequest(w, err)
			return
		}
		result, svcErr := s.service.MerchantUpdateProduct(r.Context(), actor, id, input)
		if svcErr != nil {
			writeAppError(w, svcErr)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func handleMerchantCreateSKU(w http.ResponseWriter, r *http.Request, actor application.Actor, path string, s *Server) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	id, err := parseSuffixID(path, "/skus")
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	var input application.MerchantSKUInput
	if err := decodeJSON(r, &input); err != nil {
		writeBadRequest(w, err)
		return
	}
	result, svcErr := s.service.MerchantCreateSKU(r.Context(), actor, id, input)
	if svcErr != nil {
		writeAppError(w, svcErr)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func handleMerchantSetProductStatus(w http.ResponseWriter, r *http.Request, actor application.Actor, path string, s *Server, status string) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	id, err := parseSuffixID(path, "/"+status)
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	result, svcErr := s.service.MerchantSetProductStatus(r.Context(), actor, id, status)
	if svcErr != nil {
		writeAppError(w, svcErr)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMerchantSKUByID(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodPut {
		writeMethodNotAllowed(w)
		return
	}
	id, err := parseIDFromPath(r.URL.Path, "/api/merchant/skus/")
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	var input application.MerchantSKUInput
	if err := decodeJSON(r, &input); err != nil {
		writeBadRequest(w, err)
		return
	}
	result, svcErr := s.service.MerchantUpdateSKU(r.Context(), actor, id, input)
	if svcErr != nil {
		writeAppError(w, svcErr)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMerchantOrders(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	result, err := s.service.MerchantListOrders(r.Context(), actor)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}
