package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// isAllDigits 测试
func TestIsAllDigits(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"", false},
		{"0", true},
		{"12345", true},
		{"9222", true},
		{"12a34", false},
		{"  123", false},
		{"123  ", false},
		{"-123", false},
		{"1.5", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isAllDigits(tt.input)
			if got != tt.want {
				t.Errorf("isAllDigits(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// normalizeCDPBaseURL 测试
func TestNormalizeCDPBaseURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"  ", ""},
		{"9222", "http://127.0.0.1:9222"},
		{"localhost:9222", "http://localhost:9222"},
		{"http://localhost:9222", "http://localhost:9222"},
		{"http://localhost:9222/", "http://localhost:9222"},
		{"http://localhost:9222/devtools/browser/abc", "http://localhost:9222"},
		{"ws://localhost:9222", "http://localhost:9222"},
		{"wss://localhost:9222", "https://localhost:9222"},
		{"127.0.0.1:9222", "http://127.0.0.1:9222"},
		{"http://127.0.0.1:9222/json/version", "http://127.0.0.1:9222/json/version"}, // path not stripped unless /devtools/browser/
		{"http://localhost:9222?token=abc", "http://localhost:9222"},
		{"http://localhost:9222#anchor", "http://localhost:9222"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeCDPBaseURL(tt.input)
			if got != tt.want {
				t.Errorf("normalizeCDPBaseURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// isRemoteDebuggerAvailable 测试
func TestIsRemoteDebuggerAvailable(t *testing.T) {
	// 创建一个模拟的 CDP 服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/version" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"Browser":"Chrome/123.0"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// 测试可用的 URL
	if !isRemoteDebuggerAvailable(ts.URL) {
		t.Error("expected remote debugger to be available")
	}

	// 测试不可用的 URL
	if isRemoteDebuggerAvailable("http://localhost:99999") {
		t.Error("expected remote debugger to be unavailable for invalid URL")
	}

	// 测试非 200 响应
	ts404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts404.Close()

	if isRemoteDebuggerAvailable(ts404.URL) {
		t.Error("expected remote debugger to be unavailable for 404 response")
	}
}

// handleCDPStatus 测试（无 CDP 连接）
func TestHandleCDPStatusOffline(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/cdp/status", nil)
	rec := httptest.NewRecorder()

	s.handleCDPStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// 应该返回 JSON 响应
	if rec.Body.Len() == 0 {
		t.Error("expected non-empty response body")
	}
}