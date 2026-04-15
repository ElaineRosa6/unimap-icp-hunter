package web

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func (s *Server) isNodeAuthRequired() bool {
	if s == nil {
		return false
	}
	if s.config == nil {
		// Default to requiring auth when config is nil for safety
		// But since no tokens are configured, node endpoints should be disabled
		return false
	}
	// 当节点 token 非空时必须鉴权
	for _, token := range s.config.Distributed.NodeAuthTokens {
		if strings.TrimSpace(token) != "" {
			return true
		}
	}
	// 分布式启用但 token 为空时仍要求鉴权（安全默认值）
	if s.config.Distributed.Enabled {
		return true
	}
	return false
}

func (s *Server) requireNodeToken(w http.ResponseWriter, r *http.Request, nodeID string) bool {
	if !s.isNodeAuthRequired() {
		return true
	}

	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		writeAPIError(w, http.StatusBadRequest, "node_auth_failed", "node auth failed", "node_id is required when node auth is enabled")
		return false
	}

	expected := ""
	if s != nil && s.config != nil {
		expected = strings.TrimSpace(s.config.Distributed.NodeAuthTokens[nodeID])
	}
	if expected == "" {
		writeAPIError(w, http.StatusUnauthorized, "node_auth_failed", "node auth failed", "token not configured for node_id")
		return false
	}

	provided := extractBearerToken(r.Header.Get("Authorization"))
	if provided == "" {
		provided = strings.TrimSpace(r.Header.Get("X-Node-Token"))
	}
	if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		writeAPIError(w, http.StatusUnauthorized, "node_auth_failed", "node auth failed", "invalid node token")
		return false
	}

	return true
}

func (s *Server) requireDistributedAdminToken(w http.ResponseWriter, r *http.Request) bool {
	if s == nil || s.config == nil {
		// Reject when config is nil for safety - no admin token to validate against
		writeAPIError(w, http.StatusServiceUnavailable, "admin_auth_failed", "admin auth failed", "server configuration not available")
		return false
	}
	expected := strings.TrimSpace(s.config.Distributed.AdminToken)
	if expected == "" {
		// 分布式启用但 admin_token 为空时拒绝访问，避免生产暴露
		if s.config.Distributed.Enabled {
			writeAPIError(w, http.StatusServiceUnavailable, "admin_auth_failed", "admin auth failed", "distributed admin token not configured -- set distributed.admin_token in config")
			return false
		}
		// 分布式未启用时保持原有行为（允许通过，由 requireDistributedEnabled 控制）
		return true
	}

	provided := extractBearerToken(r.Header.Get("Authorization"))
	if provided == "" {
		provided = strings.TrimSpace(r.Header.Get("X-Admin-Token"))
	}
	if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		writeAPIError(w, http.StatusUnauthorized, "admin_auth_failed", "admin auth failed", "invalid admin token")
		return false
	}

	return true
}

func (s *Server) hasValidDistributedAdminToken(r *http.Request) bool {
	if s == nil || s.config == nil {
		return false
	}
	expected := strings.TrimSpace(s.config.Distributed.AdminToken)
	if expected == "" {
		return false
	}

	provided := extractBearerToken(r.Header.Get("Authorization"))
	if provided == "" {
		provided = strings.TrimSpace(r.Header.Get("X-Admin-Token"))
	}
	if provided == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (s *Server) requireNodeOrDistributedAdminToken(w http.ResponseWriter, r *http.Request, nodeID string) bool {
	if s.hasValidDistributedAdminToken(r) {
		return true
	}
	return s.requireNodeToken(w, r, nodeID)
}

func extractBearerToken(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	parts := strings.SplitN(v, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
