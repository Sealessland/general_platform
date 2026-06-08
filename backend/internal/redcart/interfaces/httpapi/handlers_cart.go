package httpapi

import (
	"net/http"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
)

func (s *Server) handleCart(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	result, err := s.service.GetCart(r.Context(), actor)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCartItems(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var input application.CartItemInput
	if err := decodeJSON(r, &input); err != nil {
		writeBadRequest(w, err)
		return
	}
	result, err := s.service.AddCartItem(r.Context(), actor, input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleCartItemByID(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	id, err := parseIDFromPath(r.URL.Path, "/api/cart/items/")
	if err != nil {
		writeBadRequest(w, err)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var input application.CartItemUpdateInput
		if err := decodeJSON(r, &input); err != nil {
			writeBadRequest(w, err)
			return
		}
		result, svcErr := s.service.UpdateCartItem(r.Context(), actor, id, input)
		if svcErr != nil {
			writeAppError(w, svcErr)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodDelete:
		if svcErr := s.service.DeleteCartItem(r.Context(), actor, id); svcErr != nil {
			writeAppError(w, svcErr)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
	default:
		writeMethodNotAllowed(w)
	}
}
