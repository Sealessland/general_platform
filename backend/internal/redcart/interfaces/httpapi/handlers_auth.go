package httpapi

import (
	"net/http"
	"strings"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
)

// handleHealth 健康检查接口，返回服务存活状态。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// handleRegister 处理用户注册，成功即返回会话令牌。
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

// handleLogin 处理账号密码登录，返回会话令牌。
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

// handleMe 以可选认证方式注册，匿名请求也会进入，因此需从请求头重新解析 token
// 后交给服务层查询会话（handleLogout/handleRefresh 采用相同模式）。
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

// handleLogout 使当前会话令牌失效。
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	_ = actor
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	if err := s.service.Logout(r.Context(), authHeader); err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// handleRefresh 使用刷新令牌换取新的会话令牌。
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request, actor application.Actor) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	_ = actor
	var input struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeBadRequest(w, err)
		return
	}
	result, err := s.service.RefreshSession(r.Context(), input.RefreshToken)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
