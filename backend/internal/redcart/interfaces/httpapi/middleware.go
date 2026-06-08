package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/gin-gonic/gin"
)

type authedHandler func(http.ResponseWriter, *http.Request, application.Actor)

func ginHTTP(next http.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		next(c.Writer, c.Request)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		setCORSHeaders(c.Writer)
		if c.Request.Method == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) withAuth(required bool, next authedHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, err := s.authenticate(c.Request)
		if err != nil {
			if !required {
				next(c.Writer, c.Request, application.Actor{})
				return
			}
			writeAppError(c.Writer, err)
			return
		}
		next(c.Writer, c.Request, *actor)
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

func NewContext(parent context.Context) context.Context {
	if parent != nil {
		return parent
	}
	return context.Background()
}
