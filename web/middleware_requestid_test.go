package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/requestid"
)

func TestRequestIDMiddleware_GeneratesID(t *testing.T) {
	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	rid := rec.Header().Get(requestid.HeaderName)
	if rid == "" {
		t.Fatal("expected request ID header to be set")
	}
}

func TestRequestIDMiddleware_PreservesExistingID(t *testing.T) {
	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(requestid.HeaderName, "my-custom-id")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	rid := rec.Header().Get(requestid.HeaderName)
	if rid != "my-custom-id" {
		t.Fatalf("expected 'my-custom-id', got %q", rid)
	}
}

func TestRequestIDMiddleware_SetsContext(t *testing.T) {
	var capturedID string
	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = requestid.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedID == "" {
		t.Fatal("expected request ID in context")
	}
	if capturedID != rec.Header().Get(requestid.HeaderName) {
		t.Fatalf("context ID %q != header ID %q", capturedID, rec.Header().Get(requestid.HeaderName))
	}
}
