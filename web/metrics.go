package web

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/unimap/project/internal/metrics"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		state := &metricRouteState{path: "unmatched"}
		r = r.WithContext(context.WithValue(r.Context(), metricRouteKey{}, state))
		metrics.IncHTTPInFlight()
		defer metrics.DecHTTPInFlight()

		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r)
		metrics.ObserveHTTPRequest(state.path, metricMethod(r.Method), recorder.statusCode, time.Since(start))
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", map[string]string{"expected": http.MethodGet})
		return
	}
	if cfg := s.currentConfig(); cfg != nil && cfg.Web.Auth.Enabled {
		token := extractBearerToken(r.Header.Get("Authorization"))
		if token == "" {
			token = r.Header.Get("X-Admin-Token")
		}
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.adminToken())) != 1 {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "admin token required for metrics", nil)
			return
		}
	} else if bindAddr := s.bindAddr(); bindAddr != "127.0.0.1" && bindAddr != "localhost" {
		writeAPIError(w, http.StatusForbidden, "forbidden", "metrics disabled on non-loopback without auth", nil)
		return
	}
	promhttp.Handler().ServeHTTP(w, r)
}

// The mutable route slot survives WithContext request copies in authentication
// and request-ID middleware. It is written synchronously by the mux wrapper.
type metricRouteKey struct{}
type metricRouteState struct{ path string }

func metricRoutePath(r *http.Request) string {
	pattern := r.Pattern
	if _, path, ok := strings.Cut(pattern, " "); ok {
		pattern = path
	}
	if pattern == "" {
		return "unmatched"
	}
	return pattern
}

func captureMetricRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if state, ok := r.Context().Value(metricRouteKey{}).(*metricRouteState); ok {
			state.path = metricRoutePath(r)
		}
	})
}

func metricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodConnect, http.MethodTrace:
		return method
	default:
		return "OTHER"
	}
}
