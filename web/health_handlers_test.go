package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/adapter"
)

func TestHandleHealthReady_OK(t *testing.T) {
	orch := adapter.NewEngineOrchestrator()
	s := &Server{
		orchestrator: orch,
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	s.handleHealthReady(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", resp["status"])
	}
	if _, ok := resp["checks"]; !ok {
		t.Fatal("expected checks in response")
	}
}

func TestHandleHealthReady_NoOrchestrator(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	s.handleHealthReady(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	checks := resp["checks"].(map[string]interface{})
	if checks["engines"] != "not initialized" {
		t.Fatalf("expected 'not initialized', got %v", checks["engines"])
	}
}

func TestHandleHealthLive_OK(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	s.handleHealthLive(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", resp["status"])
	}
}

func TestLivenessCheck(t *testing.T) {
	// 正常状态应返回 true
	if !livenessCheck(context.Background()) {
		t.Fatal("expected liveness check to return true")
	}
}
