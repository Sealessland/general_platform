package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
)

type Server struct {
	service *application.Service
	mux     *http.ServeMux
}

func NewServer(service *application.Service) *Server {
	s := &Server{
		service: service,
		mux:     http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.mux.ServeHTTP(w, r)
	})
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/api/auth/register", s.handleRegister)
	s.mux.HandleFunc("/api/auth/login", s.handleLogin)
	s.mux.HandleFunc("/api/auth/me", s.withAuth(false, s.handleMe))

	s.mux.HandleFunc("/api/notes", s.handleNotes)
	s.mux.HandleFunc("/api/notes/", s.handleNoteByID)
	s.mux.HandleFunc("/api/products", s.handleProducts)
	s.mux.HandleFunc("/api/products/", s.handleProductRoutes)

	s.mux.HandleFunc("/api/cart", s.withAuth(true, s.handleCart))
	s.mux.HandleFunc("/api/cart/items", s.withAuth(true, s.handleCartItems))
	s.mux.HandleFunc("/api/cart/items/", s.withAuth(true, s.handleCartItemByID))

	s.mux.HandleFunc("/api/orders/preview", s.withAuth(true, s.handleOrderPreview))
	s.mux.HandleFunc("/api/orders", s.withAuth(true, s.handleOrders))
	s.mux.HandleFunc("/api/orders/", s.withAuth(true, s.handleOrderByID))

	s.mux.HandleFunc("/api/merchant/products", s.withAuth(true, s.handleMerchantProducts))
	s.mux.HandleFunc("/api/merchant/products/", s.withAuth(true, s.handleMerchantProductByID))
	s.mux.HandleFunc("/api/merchant/skus/", s.withAuth(true, s.handleMerchantSKUByID))
	s.mux.HandleFunc("/api/merchant/orders", s.withAuth(true, s.handleMerchantOrders))
	s.mux.HandleFunc("/api/merchant/orders/", s.withAuth(true, s.handleMerchantOrderByID))
	s.mux.HandleFunc("/api/merchant/dashboard/funnel", s.withAuth(true, s.handleMerchantDashboardFunnel))
	s.mux.HandleFunc("/api/merchant/dashboard/products", s.withAuth(true, s.handleMerchantDashboardProducts))
	s.mux.HandleFunc("/api/merchant/dashboard/summary", s.withAuth(true, s.handleMerchantDashboardSummary))

	s.mux.HandleFunc("/api/ai/product-selling-points", s.withAuth(true, s.handleAISellingPoints))
	s.mux.HandleFunc("/api/ai/business-review", s.withAuth(true, s.handleAIBusinessReview))
	s.mux.HandleFunc("/api/ai/tasks/", s.withAuth(true, s.handleAITaskByID))
}

type authedHandler func(http.ResponseWriter, *http.Request, application.Actor)

func (s *Server) withAuth(required bool, next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, err := s.authenticate(r)
		if err != nil {
			if !required {
				next(w, r, application.Actor{})
				return
			}
			writeAppError(w, err)
			return
		}
		next(w, r, *actor)
	}
}

func (s *Server) authenticate(r *http.Request) (*application.Actor, error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	if authHeader == "" {
		return nil, &application.AppError{Kind: application.ErrorUnauthorized, Message: "missing bearer token"}
	}
	return s.service.Authenticate(authHeader)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var input application.RegisterInput
	if err := decodeJSON(r, &input); err != nil {
		writeBadRequest(w, err)
		return
	}
	result, err := s.service.Register(r.Context(), input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var input application.LoginInput
	if err := decodeJSON(r, &input); err != nil {
		writeBadRequest(w, err)
		return
	}
	result, err := s.service.Login(r.Context(), input)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	_ = actor
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	result, err := s.service.Me(r.Context(), authHeader)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

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

func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	switch r.Method {
	case http.MethodGet:
		result, err := s.service.ListOrders(r.Context(), actor)
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
	case strings.HasSuffix(path, "/online"):
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		id, err := parseSuffixID(path, "/online")
		if err != nil {
			writeBadRequest(w, err)
			return
		}
		result, svcErr := s.service.MerchantSetProductStatus(r.Context(), actor, id, "online")
		if svcErr != nil {
			writeAppError(w, svcErr)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case strings.HasSuffix(path, "/offline"):
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		id, err := parseSuffixID(path, "/offline")
		if err != nil {
			writeBadRequest(w, err)
			return
		}
		result, svcErr := s.service.MerchantSetProductStatus(r.Context(), actor, id, "offline")
		if svcErr != nil {
			writeAppError(w, svcErr)
			return
		}
		writeJSON(w, http.StatusOK, result)
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

func (s *Server) handleMerchantOrderByID(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	path := strings.TrimPrefix(r.URL.Path, "/api/merchant/orders/")
	switch {
	case strings.HasSuffix(path, "/ship"):
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		id, err := parseSuffixID(path, "/ship")
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
		id, err := parseSuffixID(path, "/refund/approve")
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

func parseIDFromPath(path, prefix string) (int64, error) {
	idStr := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if strings.Contains(idStr, "/") {
		return 0, fmt.Errorf("invalid path")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func parseSuffixID(path, suffix string) (int64, error) {
	idStr := strings.TrimSuffix(path, suffix)
	idStr = strings.Trim(idStr, "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
}

func writeBadRequest(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": map[string]any{
			"kind":    "bad_request",
			"message": err.Error(),
		},
	})
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
		"error": map[string]any{
			"kind":    "method_not_allowed",
			"message": "method not allowed",
		},
	})
}

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

func NewContext(parent context.Context) context.Context {
	if parent != nil {
		return parent
	}
	return context.Background()
}
