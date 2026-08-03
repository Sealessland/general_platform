package httpapi

import (
	"net/http"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"github.com/gin-gonic/gin"
)

// authedHandler 是携带已认证身份的处理器签名，供带鉴权中间件的路由使用。
type authedHandler func(http.ResponseWriter, *http.Request, application.Actor)

// ginHTTP 把标准 net/http 处理器适配为 gin 处理器。
func ginHTTP(next http.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		next(c.Writer, c.Request)
	}
}

// corsMiddleware 处理跨域预检请求并附加 CORS 响应头。
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

// withAuth 包装认证逻辑：required=false 时匿名请求以空 Actor 继续进入 handler，
// 供「可选登录」的接口（如获取当前用户、刷新令牌）使用。
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

// authenticate 从 Authorization: Bearer <token> 请求头解析并校验会话身份。
func (s *Server) authenticate(r *http.Request) (*application.Actor, error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	if authHeader == "" {
		return nil, &application.AppError{Kind: application.ErrorUnauthorized, Message: "missing bearer token"}
	}
	return s.service.Authenticate(authHeader)
}

// requireRole 校验当前身份的角色，不匹配时返回 403。
func requireRole(role string, next authedHandler) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, actor application.Actor) {
		if actor.Role != role {
			writeAppError(w, &application.AppError{Kind: application.ErrorForbidden, Message: "role access required"})
			return
		}
		next(w, r, actor)
	}
}

// requireMerchant 限定商家角色可访问。
func requireMerchant(next authedHandler) authedHandler {
	return requireRole(domain.RoleMerchant, next)
}
