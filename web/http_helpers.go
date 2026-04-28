package web

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

type apiErrorPayload struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type apiErrorResponse struct {
	Success bool            `json:"success"`
	Error   apiErrorPayload `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logger.Errorf("failed to encode JSON response: %v", err)
	}
}

func writeAPIError(w http.ResponseWriter, status int, code, message string, details interface{}) {
	writeJSON(w, status, apiErrorResponse{
		Success: false,
		Error: apiErrorPayload{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", map[string]string{"expected": method})
		return false
	}
	return true
}

func requireTrustedRequest(w http.ResponseWriter, r *http.Request, allowedOrigins []string) bool {
	if !isTrustedRequest(r, allowedOrigins) {
		writeAPIError(w, http.StatusForbidden, "forbidden_origin", "origin not allowed", nil)
		return false
	}
	return true
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			writeAPIError(w, http.StatusBadRequest, "invalid_request_body", "request body is required", nil)
			return false
		}
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			writeAPIError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds configured limit", nil)
			return false
		}
		writeAPIError(w, http.StatusBadRequest, "invalid_request_body", "invalid JSON request body", err.Error())
		return false
	}

	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		writeAPIError(w, http.StatusBadRequest, "invalid_request_body", "request body must contain only one JSON object", nil)
		return false
	}

	return true
}

func isSameHostURL(rawURL string, host string) bool {
	if strings.TrimSpace(rawURL) == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, host)
}

func normalizeOrigin(origin string) string {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.ToLower(strings.TrimRight(u.String(), "/"))
}

func originAllowedByList(origin string, allowedOrigins []string) bool {
	normalized := normalizeOrigin(origin)
	if normalized == "" {
		return false
	}
	for _, allowed := range allowedOrigins {
		allowed = strings.TrimSpace(allowed)
		if allowed == "*" {
			return true
		}
		if normalizeOrigin(allowed) == normalized {
			return true
		}
	}
	return false
}

func isOriginAllowed(origin, host string, allowedOrigins []string) bool {
	if strings.TrimSpace(origin) == "" {
		return true
	}
	if isSameHostURL(origin, host) {
		return true
	}
	return originAllowedByList(origin, allowedOrigins)
}

func isTrustedRequest(r *http.Request, allowedOrigins []string) bool {
	origin := r.Header.Get("Origin")
	referer := r.Header.Get("Referer")

	// 对状态变更操作（POST, PUT, PATCH, DELETE）要求必须有 Origin 或 Referer
	isStateChange := r.Method == http.MethodPost ||
		r.Method == http.MethodPut ||
		r.Method == http.MethodPatch ||
		r.Method == http.MethodDelete

	if isStateChange && strings.TrimSpace(origin) == "" && strings.TrimSpace(referer) == "" {
		return false
	}

	if strings.TrimSpace(origin) == "" && strings.TrimSpace(referer) == "" {
		// Keep compatibility for non-browser clients.
		return true
	}
	if isOriginAllowed(origin, r.Host, allowedOrigins) {
		return true
	}
	return isOriginAllowed(referer, r.Host, allowedOrigins)
}

func requestSizeLimitMiddleware(maxBodyBytes int64) func(http.Handler) http.Handler {
	if maxBodyBytes <= 0 {
		maxBodyBytes = 10 * 1024 * 1024
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			isWebSocket := strings.Contains(r.Header.Get("Connection"), "Upgrade") &&
				strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
			
			if !isWebSocket {
				switch r.Method {
				case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
					if r.ContentLength > maxBodyBytes {
						writeAPIError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds configured limit", map[string]string{"max_body_bytes": strconv.FormatInt(maxBodyBytes, 10)})
						return
					}
					r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isPrivateOrInternalIP 检查主机名是否为私有/回环/内部地址
func isPrivateOrInternalIP(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	// 去除端口
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	// 检查常见主机名
	lower := strings.ToLower(host)
	if lower == "localhost" || lower == "127.0.0.1" || lower == "::1" || lower == "0.0.0.0" {
		return true
	}
	// 解析 IP
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func corsMiddleware(allowedOrigins, allowedMethods, allowedHeaders, exposedHeaders []string, allowCredentials bool, maxAge int) func(http.Handler) http.Handler {
	if len(allowedMethods) == 0 {
		allowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}
	if len(allowedHeaders) == 0 {
		allowedHeaders = []string{"Content-Type", "Authorization", "X-Requested-With", "X-WebSocket-Token", "X-Request-Id"}
	}
	if maxAge < 0 {
		maxAge = 0
	}

	methodHeader := strings.Join(allowedMethods, ", ")
	headerHeader := strings.Join(allowedHeaders, ", ")
	exposedHeader := strings.Join(exposedHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin != "" && isOriginAllowed(origin, r.Host, allowedOrigins) {
				if originAllowedByList(origin, allowedOrigins) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				} else {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
				if allowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				w.Header().Set("Vary", "Origin")
				if exposedHeader != "" {
					w.Header().Set("Access-Control-Expose-Headers", exposedHeader)
				}
			}

			if r.Method == http.MethodOptions {
				if origin == "" || !isOriginAllowed(origin, r.Host, allowedOrigins) {
					writeAPIError(w, http.StatusForbidden, "forbidden_origin", "origin not allowed", nil)
					return
				}
				w.Header().Set("Access-Control-Allow-Methods", methodHeader)
				w.Header().Set("Access-Control-Allow-Headers", headerHeader)
				if maxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", strconv.Itoa(maxAge))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
