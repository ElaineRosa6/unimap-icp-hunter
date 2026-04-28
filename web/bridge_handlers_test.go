package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newBridgeState() *BridgeState {
	return &BridgeState{
		Tokens:         make(map[string]int64),
		LastSeen:       make(map[string]int64),
		CallbackNonces: make(map[string]int64),
	}
}

// ============================================================
// handleScreenshotBridgeHealth tests
// ============================================================

func TestHandleScreenshotBridgeHealth_Success(t *testing.T) {
	s := &Server{bridge: newBridgeState()}
	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/bridge/health", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotBridgeHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"success":true`) {
		t.Fatalf("expected success:true, got %q", body)
	}
}

func TestHandleScreenshotBridgeHealth_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/bridge/health", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotBridgeHealth(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// ============================================================
// handleScreenshotBridgeStatus tests
// ============================================================

func TestHandleScreenshotBridgeStatus_Success(t *testing.T) {
	s := &Server{bridge: newBridgeState()}
	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/bridge/status", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotBridgeStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"success":true`) {
		t.Fatalf("expected success:true, got %q", body)
	}
}

func TestHandleScreenshotBridgeStatus_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/bridge/status", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotBridgeStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// ============================================================
// handleScreenshotBridgePair tests
// ============================================================

func TestHandleScreenshotBridgePair_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/bridge/pair", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotBridgePair(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleScreenshotBridgePair_NonLoopback(t *testing.T) {
	s := &Server{bridge: newBridgeState()}
	body := strings.NewReader(`{"client_id":"test","pair_code":"123456"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/bridge/pair", body)
	req.RemoteAddr = "1.2.3.4:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	w := httptest.NewRecorder()
	s.handleScreenshotBridgePair(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "forbidden_origin") {
		t.Fatalf("expected 'forbidden_origin' in body, got %q", w.Body.String())
	}
}

func TestHandleScreenshotBridgePair_MissingFields(t *testing.T) {
	s := &Server{bridge: newBridgeState()}
	body := strings.NewReader(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/bridge/pair", body)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	s.handleScreenshotBridgePair(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid_pair_request") {
		t.Fatalf("expected 'invalid_pair_request' in body, got %q", w.Body.String())
	}
}

func TestHandleScreenshotBridgePair_InvalidJSON(t *testing.T) {
	s := &Server{bridge: newBridgeState()}
	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/bridge/pair", body)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	s.handleScreenshotBridgePair(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ============================================================
// handleScreenshotBridgeRotateToken tests
// ============================================================

func TestHandleScreenshotBridgeRotateToken_WrongMethod(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/screenshot/bridge/rotate-token", nil)
	w := httptest.NewRecorder()
	s.handleScreenshotBridgeRotateToken(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleScreenshotBridgeRotateToken_NonLoopback(t *testing.T) {
	s := &Server{bridge: newBridgeState()}
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/bridge/rotate-token", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	w := httptest.NewRecorder()
	s.handleScreenshotBridgeRotateToken(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

// ============================================================
// buildBridgeDiagnosticSnapshot tests
// ============================================================

func TestBuildBridgeDiagnosticSnapshot_NilDeps(t *testing.T) {
	s := &Server{bridge: newBridgeState()}
	snap := s.buildBridgeDiagnosticSnapshot()

	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if _, ok := snap["last_error"]; !ok {
		t.Fatal("expected 'last_error' key in snapshot")
	}
	if _, ok := snap["engine"]; !ok {
		t.Fatal("expected 'engine' key in snapshot")
	}
	if _, ok := snap["ready"]; !ok {
		t.Fatal("expected 'ready' key in snapshot")
	}
}

// ============================================================
// isLoopbackRequest tests
// ============================================================

func TestIsLoopbackRequest_DirectLocalhost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	req.Host = "localhost:8080"
	if !isLoopbackRequest(req) {
		t.Fatal("expected direct 127.0.0.1 to be loopback")
	}
}

func TestIsLoopbackRequest_ForwardedNonLoopback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	if isLoopbackRequest(req) {
		t.Fatal("expected forwarded 1.2.3.4 to NOT be loopback")
	}
}

func TestIsLoopbackRequest_ForwardedLoopback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	// X-Forwarded-For means it's NOT loopback per the implementation
	if isLoopbackRequest(req) {
		t.Fatal("expected forwarded 127.0.0.1 via X-Forwarded-For to NOT be loopback")
	}
}

func TestIsLoopbackRequest_localhostHostname(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	req.Host = "localhost:8080"
	if !isLoopbackRequest(req) {
		t.Fatal("expected localhost hostname to be loopback")
	}
}

func TestIsLoopbackRequest_PrivateIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:8080"
	req.Host = "192.168.1.1:8080"
	// Private IPs are NOT considered loopback by the implementation
	if isLoopbackRequest(req) {
		t.Fatal("expected private IP to NOT be loopback")
	}
}

// ============================================================
// setBridgeLastError / clearBridgeLastError / bridge.LastErr tests
// ============================================================

func TestBridgeLastError(t *testing.T) {
	s := &Server{bridge: newBridgeState()}

	s.setBridgeLastError("test error")
	if s.bridge.LastErr != "test error" {
		t.Fatalf("expected 'test error', got %q", s.bridge.LastErr)
	}

	s.clearBridgeLastError()
	if s.bridge.LastErr != "" {
		t.Fatalf("expected empty after clear, got %q", s.bridge.LastErr)
	}
}

// ============================================================
// issueBridgeToken / validateBridgeAuthIfRequired tests
// ============================================================

func TestIssueBridgeToken(t *testing.T) {
	s := &Server{bridge: newBridgeState()}
	token, expireAt, err := s.issueBridgeToken(600)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if expireAt <= 0 {
		t.Fatalf("expected positive expireAt, got %d", expireAt)
	}
}

func TestIssueBridgeToken_ZeroTTL(t *testing.T) {
	s := &Server{bridge: newBridgeState()}
	token, _, err := s.issueBridgeToken(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token with zero TTL")
	}
}
