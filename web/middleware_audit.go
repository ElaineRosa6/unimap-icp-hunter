package web

import (
	"net/http"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

// 不需要记录审计日志的路径前缀
var auditSkipPrefixes = []string{
	"/health",
	"/metrics",
	"/static/",
	"/favicon.ico",
}

func shouldAudit(r *http.Request) bool {
	path := r.URL.Path
	for _, prefix := range auditSkipPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return false
		}
	}
	return true
}

// auditMiddleware 记录请求时间/方法/路径/IP/UA/响应状态/耗时
func auditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !shouldAudit(r) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		wrapped := &auditResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		elapsed := time.Since(start)
		logger.Infof("AUDIT method=%s path=%s ip=%s ua=%q status=%d elapsed=%s",
			r.Method,
			r.URL.Path,
			getClientIP(r),
			r.UserAgent(),
			wrapped.statusCode,
			elapsed.Round(time.Millisecond),
		)
	})
}

// auditResponseWriter 包装 ResponseWriter 以捕获状态码
type auditResponseWriter struct {
	http.ResponseWriter
	statusCode int
	wroteHeader bool
}

func (w *auditResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.statusCode = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}
