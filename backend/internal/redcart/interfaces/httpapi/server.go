package httpapi

import (
	"net/http"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/gin-gonic/gin"
)

type Server struct {
	service *application.Service
	router  *gin.Engine
}

func NewServer(service *application.Service) *Server {
	gin.SetMode(gin.ReleaseMode)
	s := &Server{
		service: service,
		router:  gin.New(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) registerRoutes() {
	s.router.HandleMethodNotAllowed = true
	s.router.Use(corsMiddleware())
	s.router.NoMethod(func(c *gin.Context) {
		writeMethodNotAllowed(c.Writer)
	})
	s.router.NoRoute(func(c *gin.Context) {
		writeJSON(c.Writer, http.StatusNotFound, map[string]any{
			"error": map[string]any{
				"kind":    string(application.ErrorNotFound),
				"message": "route not found",
			},
		})
	})

	s.router.GET("/healthz", ginHTTP(s.handleHealth))
	s.router.POST("/api/auth/register", ginHTTP(s.handleRegister))
	s.router.POST("/api/auth/login", ginHTTP(s.handleLogin))
	s.router.GET("/api/auth/me", s.withAuth(false, s.handleMe))

	s.router.GET("/api/notes", ginHTTP(s.handleNotes))
	s.router.GET("/api/notes/:id", ginHTTP(s.handleNoteByID))
	s.router.GET("/api/products", ginHTTP(s.handleProducts))
	s.router.GET("/api/products/:id", ginHTTP(s.handleProductByID))
	s.router.GET("/api/products/:id/skus", ginHTTP(s.handleProductSKUs))

	s.router.GET("/api/cart", s.withAuth(true, s.handleCart))
	s.router.POST("/api/cart/items", s.withAuth(true, s.handleCartItems))
	s.router.PUT("/api/cart/items/:id", s.withAuth(true, s.handleCartItemByID))
	s.router.DELETE("/api/cart/items/:id", s.withAuth(true, s.handleCartItemByID))

	s.router.POST("/api/orders/preview", s.withAuth(true, s.handleOrderPreview))
	s.router.GET("/api/orders", s.withAuth(true, s.handleOrders))
	s.router.POST("/api/orders", s.withAuth(true, s.handleOrders))
	s.router.GET("/api/orders/:id", s.withAuth(true, s.handleOrderByID))
	s.router.POST("/api/orders/:id/pay", s.withAuth(true, s.handleOrderByID))
	s.router.POST("/api/orders/:id/cancel", s.withAuth(true, s.handleOrderByID))
	s.router.POST("/api/orders/:id/finish", s.withAuth(true, s.handleOrderByID))
	s.router.POST("/api/orders/:id/refund", s.withAuth(true, s.handleOrderByID))

	s.router.GET("/api/merchant/products", s.withAuth(true, s.handleMerchantProducts))
	s.router.POST("/api/merchant/products", s.withAuth(true, s.handleMerchantProducts))
	s.router.PUT("/api/merchant/products/:id", s.withAuth(true, s.handleMerchantProductByID))
	s.router.POST("/api/merchant/products/:id/skus", s.withAuth(true, s.handleMerchantProductByID))
	s.router.POST("/api/merchant/products/:id/online", s.withAuth(true, s.handleMerchantProductByID))
	s.router.POST("/api/merchant/products/:id/offline", s.withAuth(true, s.handleMerchantProductByID))
	s.router.PUT("/api/merchant/skus/:id", s.withAuth(true, s.handleMerchantSKUByID))
	s.router.GET("/api/merchant/orders", s.withAuth(true, s.handleMerchantOrders))
	s.router.GET("/api/merchant/orders/:id", s.withAuth(true, s.handleMerchantOrderByID))
	s.router.POST("/api/merchant/orders/:id/ship", s.withAuth(true, s.handleMerchantOrderByID))
	s.router.POST("/api/merchant/orders/:id/refund/approve", s.withAuth(true, s.handleMerchantOrderByID))
	s.router.GET("/api/merchant/dashboard/funnel", s.withAuth(true, s.handleMerchantDashboardFunnel))
	s.router.GET("/api/merchant/dashboard/products", s.withAuth(true, s.handleMerchantDashboardProducts))
	s.router.GET("/api/merchant/dashboard/summary", s.withAuth(true, s.handleMerchantDashboardSummary))

	s.router.POST("/api/ai/product-selling-points", s.withAuth(true, s.handleAISellingPoints))
	s.router.POST("/api/ai/business-review", s.withAuth(true, s.handleAIBusinessReview))
	s.router.GET("/api/ai/tasks/:id", s.withAuth(true, s.handleAITaskByID))
}
