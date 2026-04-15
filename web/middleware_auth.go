package web

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// adminAuthMiddleware returns a middleware that requires X-Admin-Token header
// (or admin_token query parameter) for all requests except public paths.
func (s *Server) adminAuthMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for public paths
			if s.isPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Extract token from header or query
			token := r.Header.Get("X-Admin-Token")
			if token == "" {
				token = r.URL.Query().Get("admin_token")
			}

			adminToken := s.adminToken()
			if adminToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(adminToken)) != 1 {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "unauthorized: valid admin token required",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isPublicPath returns true for paths that do not require authentication.
func (s *Server) isPublicPath(path string) bool {
	publicPrefixes := []string{
		"/health",
		"/static/",
		"/screenshots/",
	}
	for _, prefix := range publicPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// adminToken returns the configured admin token.
func (s *Server) adminToken() string {
	if s.config != nil && s.config.Web.Auth.Enabled {
		return s.config.Web.Auth.AdminToken
	}
	return ""
}
