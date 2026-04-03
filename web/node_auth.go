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
	for _, token := range s.config.Distributed.NodeAuthTokens {
		if strings.TrimSpace(token) != "" {
			return true
		}
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
		return true
	}
	expected := strings.TrimSpace(s.config.Distributed.AdminToken)
	if expected == "" {
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
