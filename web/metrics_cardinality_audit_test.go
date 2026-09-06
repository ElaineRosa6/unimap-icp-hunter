package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestAuditRejectedPathCardinality(t *testing.T) {
	getGlobalLimiter()
	limiter := NewRateLimiter(1, time.Hour)
	t.Cleanup(limiter.Stop)
	limiter.Allow("192.0.2.10")
	globalLimiterMu.Lock()
	old := globalLimiter
	globalLimiter = limiter
	globalLimiterMu.Unlock()
	enabled := rateLimitEnabled.Swap(true)
	t.Cleanup(func() {
		globalLimiterMu.Lock()
		globalLimiter = old
		globalLimiterMu.Unlock()
		rateLimitEnabled.Store(enabled)
	})
	prefix := fmt.Sprintf("/api/v1/audit-cardinality-%d/", time.Now().UnixNano())
	handler := metricsMiddleware(rateLimitMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("rejected request reached handler") })))
	for i := 0; i < 64; i++ {
		request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s%d", prefix, i), nil)
		request.RemoteAddr = "192.0.2.10:12345"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusTooManyRequests {
			t.Fatalf("status=%d", response.Code)
		}
	}
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		switch family.GetName() {
		case "unimap_http_requests_total", "unimap_http_request_duration_seconds", "unimap_rate_limit_rejected_total":
			count := 0
			for _, metric := range family.Metric {
				for _, label := range metric.Label {
					if label.GetName() == "path" && strings.HasPrefix(label.GetValue(), prefix) {
						count++
					}
				}
			}
			t.Logf("%s: raw-path label sets=%d for 64 rejected requests", family.GetName(), count)
			if count > 1 {
				t.Errorf("unbounded raw request paths retained by %s", family.GetName())
			}
		}
	}
}
