package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestShouldAudit_Skips(t *testing.T) {
	skips := []string{
		"/health",
		"/health/ready",
		"/health/live",
		"/metrics",
		"/static/css/style.css",
		"/favicon.ico",
	}
	for _, path := range skips {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if shouldAudit(req) {
			t.Fatalf("expected %s to be skipped", path)
		}
	}
}

func TestShouldAudit_Allows(t *testing.T) {
	allows := []string{
		"/api/query",
		"/api/screenshot",
		"/api/nodes/register",
		"/query",
		"/results",
	}
	for _, path := range allows {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if !shouldAudit(req) {
			t.Fatalf("expected %s to be audited", path)
		}
	}
}

func TestAuditMiddleware_SkipsHealth(t *testing.T) {
	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	auditHandler := auditMiddleware(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	auditHandler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Fatal("expected handler to be called")
	}
}

func TestAuditMiddleware_RecordsAPI(t *testing.T) {
	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusCreated)
	})

	auditHandler := auditMiddleware(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/query", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("User-Agent", "test-agent")
	auditHandler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Fatal("expected handler to be called")
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestAuditResponseWriter_CaptureStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &auditResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	w.WriteHeader(http.StatusNotFound)
	if w.statusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.statusCode)
	}

	// 多次 WriteHeader 不应改变状态码
	w.WriteHeader(http.StatusInternalServerError)
	if w.statusCode != http.StatusNotFound {
		t.Fatalf("expected still 404, got %d", w.statusCode)
	}
}
