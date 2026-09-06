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
	statusCode  int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	// Delegate validation and informational responses to net/http. A 101 is
	// final, unlike other 1xx responses (matching the standard server).
	r.ResponseWriter.WriteHeader(code)
	if code >= 100 && code <= 199 && code != http.StatusSwitchingProtocols {
		return
	}
	r.statusCode = code
	r.wroteHeader = true
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(data)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// FlushError allows ResponseController to flush without bypassing status tracking.
func (r *statusRecorder) FlushError() error {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(r.ResponseWriter).Flush()
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
