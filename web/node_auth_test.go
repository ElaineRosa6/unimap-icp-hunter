package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/config"
)

func TestIsNodeAuthRequired_NilServer(t *testing.T) {
	var s *Server
	if s.isNodeAuthRequired() {
		t.Fatal("expected false for nil server")
	}
}

func TestIsNodeAuthRequired_NilConfig(t *testing.T) {
	s := &Server{config: nil}
	if s.isNodeAuthRequired() {
		t.Fatal("expected false for nil config")
	}
}

func TestIsNodeAuthRequired_TokenConfigured(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.NodeAuthTokens = map[string]string{"node1": "token1"}
	if !s.isNodeAuthRequired() {
		t.Fatal("expected true when node tokens are configured")
	}
}

func TestIsNodeAuthRequired_DistributedEnabledNoTokens(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.Enabled = true
	s.config.Distributed.NodeAuthTokens = map[string]string{}
	if !s.isNodeAuthRequired() {
		t.Fatal("expected true when distributed is enabled but no tokens")
	}
}

func TestIsNodeAuthRequired_DistributedDisabledNoTokens(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.Enabled = false
	s.config.Distributed.NodeAuthTokens = map[string]string{}
	if s.isNodeAuthRequired() {
		t.Fatal("expected false when distributed is disabled and no tokens")
	}
}

func TestRequireNodeToken_AuthNotRequired(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.Enabled = false
	s.config.Distributed.NodeAuthTokens = map[string]string{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	if !s.requireNodeToken(rec, req, "node1") {
		t.Fatal("expected true when auth not required")
	}
}

func TestRequireNodeToken_EmptyNodeID(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.NodeAuthTokens = map[string]string{"node1": "token1"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	if s.requireNodeToken(rec, req, "") {
		t.Fatal("expected false for empty node ID")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRequireNodeToken_TokenNotConfigured(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.NodeAuthTokens = map[string]string{"node1": "token1"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	if s.requireNodeToken(rec, req, "node2") {
		t.Fatal("expected false when token not configured for node")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireNodeToken_MissingToken(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.NodeAuthTokens = map[string]string{"node1": "token1"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	if s.requireNodeToken(rec, req, "node1") {
		t.Fatal("expected false when no token provided")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireNodeToken_ValidBearer(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.NodeAuthTokens = map[string]string{"node1": "token1"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	req.Header.Set("Authorization", "Bearer token1")
	if !s.requireNodeToken(rec, req, "node1") {
		t.Fatal("expected true for valid bearer token")
	}
}

func TestRequireNodeToken_ValidXNodeToken(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.NodeAuthTokens = map[string]string{"node1": "token1"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	req.Header.Set("X-Node-Token", "token1")
	if !s.requireNodeToken(rec, req, "node1") {
		t.Fatal("expected true for valid X-Node-Token header")
	}
}

func TestRequireNodeToken_InvalidToken(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.NodeAuthTokens = map[string]string{"node1": "token1"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	if s.requireNodeToken(rec, req, "node1") {
		t.Fatal("expected false for wrong token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireDistributedAdminToken_NilConfig(t *testing.T) {
	s := &Server{config: nil}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	if s.requireDistributedAdminToken(rec, req) {
		t.Fatal("expected false for nil config")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestRequireDistributedAdminToken_AdminTokenEmpty_DistributedEnabled(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.Enabled = true
	s.config.Distributed.AdminToken = ""

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	if s.requireDistributedAdminToken(rec, req) {
		t.Fatal("expected false when admin token empty and distributed enabled")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestRequireDistributedAdminToken_AdminTokenEmpty_DistributedDisabled(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.Enabled = false
	s.config.Distributed.AdminToken = ""

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	if !s.requireDistributedAdminToken(rec, req) {
		t.Fatal("expected true when admin token empty and distributed disabled")
	}
}

func TestRequireDistributedAdminToken_ValidBearer(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.Enabled = true
	s.config.Distributed.AdminToken = "admin-secret"

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	if !s.requireDistributedAdminToken(rec, req) {
		t.Fatal("expected true for valid admin bearer token")
	}
}

func TestRequireDistributedAdminToken_ValidXAdminToken(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.Enabled = true
	s.config.Distributed.AdminToken = "admin-secret"

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	req.Header.Set("X-Admin-Token", "admin-secret")
	if !s.requireDistributedAdminToken(rec, req) {
		t.Fatal("expected true for valid X-Admin-Token header")
	}
}

func TestRequireDistributedAdminToken_InvalidToken(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.Enabled = true
	s.config.Distributed.AdminToken = "admin-secret"

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	if s.requireDistributedAdminToken(rec, req) {
		t.Fatal("expected false for wrong admin token")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHasValidDistributedAdminToken_NilServer(t *testing.T) {
	var s *Server
	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	if s.hasValidDistributedAdminToken(req) {
		t.Fatal("expected false for nil server")
	}
}

func TestHasValidDistributedAdminToken_ValidBearer(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.AdminToken = "admin-secret"

	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	if !s.hasValidDistributedAdminToken(req) {
		t.Fatal("expected true for valid admin token")
	}
}

func TestHasValidDistributedAdminToken_NoToken(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.AdminToken = "admin-secret"

	req := httptest.NewRequest(http.MethodGet, "/api/admin", nil)
	if s.hasValidDistributedAdminToken(req) {
		t.Fatal("expected false when no token provided")
	}
}

func TestRequireNodeOrAdminToken_AdminPasses(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.AdminToken = "admin-secret"

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	req.Header.Set("Authorization", "Bearer admin-secret")
	if !s.requireNodeOrDistributedAdminToken(rec, req, "node1") {
		t.Fatal("expected true for valid admin token")
	}
}

func TestRequireNodeOrAdminToken_NodePasses(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Distributed.NodeAuthTokens = map[string]string{"node1": "token1"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	req.Header.Set("X-Node-Token", "token1")
	if !s.requireNodeOrDistributedAdminToken(rec, req, "node1") {
		t.Fatal("expected true for valid node token")
	}
}
