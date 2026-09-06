package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func metricLabelTotals(t *testing.T) map[string]float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	totals := map[string]float64{}
	for _, family := range families {
		switch family.GetName() {
		case "unimap_http_requests_total", "unimap_http_request_duration_seconds", "unimap_rate_limit_rejected_total":
			for _, metric := range family.Metric {
				labels := map[string]string{}
				for _, label := range metric.Label {
					labels[label.GetName()] = label.GetValue()
				}
				key := family.GetName() + "|" + labels["path"] + "|" + labels["method"] + "|" + labels["status"]
				if metric.Counter != nil {
					totals[key] = metric.Counter.GetValue()
				}
				if metric.Histogram != nil {
					totals[key] = float64(metric.Histogram.GetSampleCount())
				}
			}
		}
	}
	return totals
}

func TestMetricsRouteLabelsAcrossRequestCopies(t *testing.T) {
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
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/metric-items/{id}", rateLimitMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("rate limit bypassed") })))
	mux.HandleFunc("GET /metric-static/{path...}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := metricsMiddleware(requestIDMiddleware(captureMetricRoute(mux)))
	for _, scenario := range []struct {
		name, path, method, label, status string
		code                              int
		count                             int
	}{
		{"before-routing", "/metric-auth/item-", "POST", "unmatched", "401", 401, 64},
		{"dynamic", "/api/v1/metric-items/item-", "GET", "/api/v1/metric-items/{id}", "429", 429, 64},
		{"static", "/metric-static/asset-", "GET", "/metric-static/{path...}", "204", 204, 64},
		{"unknown-method", "/metric-unknown/item-", "CUSTOM", "unmatched", "404", 404, 64},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			before := metricLabelTotals(t)
			for i := 0; i < scenario.count; i++ {
				method := scenario.method
				if method == "CUSTOM" {
					method = fmt.Sprintf("CUSTOM%d", i)
				}
				request := httptest.NewRequest(method, fmt.Sprintf("%s%d", scenario.path, i), nil)
				request.RemoteAddr = "192.0.2.10:12345"
				response := httptest.NewRecorder()
				if scenario.name == "before-routing" {
					metricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })).ServeHTTP(response, request)
				} else {
					handler.ServeHTTP(response, request)
				}
				if response.Code != scenario.code {
					t.Fatalf("status=%d", response.Code)
				}
			}
			after := metricLabelTotals(t)
			method := scenario.method
			if method == "CUSTOM" {
				method = "OTHER"
			}
			for _, family := range []string{"unimap_http_requests_total", "unimap_http_request_duration_seconds"} {
				key := family + "|" + scenario.label + "|" + method + "|" + scenario.status
				if after[key]-before[key] != float64(scenario.count) {
					t.Fatalf("counts lost or labels split: %s delta=%v", key, after[key]-before[key])
				}
			}
			if scenario.code == 429 {
				key := "unimap_rate_limit_rejected_total|" + scenario.label + "||"
				if after[key]-before[key] != 64 {
					t.Fatal("rejection labels did not converge")
				}
			}
		})
	}
}
