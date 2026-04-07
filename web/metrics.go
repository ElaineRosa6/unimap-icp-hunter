package web

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/unimap-icp-hunter/project/internal/metrics"
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
		metrics.IncHTTPInFlight()
		defer metrics.DecHTTPInFlight()

		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r)
		metrics.ObserveHTTPRequest(r.URL.Path, r.Method, recorder.statusCode, time.Since(start))
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", map[string]string{"expected": http.MethodGet})
		return
	}
	promhttp.Handler().ServeHTTP(w, r)
}
