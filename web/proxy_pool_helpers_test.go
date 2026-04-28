package web

import (
	"testing"
)

func TestSelectRequestProxy_NilServer(t *testing.T) {
	var s *Server
	got := s.selectRequestProxy()
	if got != "" {
		t.Fatalf("expected empty for nil server, got %q", got)
	}
}

func TestSelectRequestProxy_NilProxyPool(t *testing.T) {
	s := &Server{proxyPool: nil}
	got := s.selectRequestProxy()
	if got != "" {
		t.Fatalf("expected empty for nil proxy pool, got %q", got)
	}
}

func TestReportRequestProxy_NilServer(t *testing.T) {
	var s *Server
	s.reportRequestProxy("http://proxy:8080", true)
	// Should not panic
}

func TestReportRequestProxy_NilProxyPool(t *testing.T) {
	s := &Server{proxyPool: nil}
	s.reportRequestProxy("http://proxy:8080", true)
	// Should not panic
}

func TestReportRequestProxy_EmptyProxy(t *testing.T) {
	s := &Server{proxyPool: nil}
	s.reportRequestProxy("  ", true)
	// Should not panic — whitespace-only is trimmed to empty
}
