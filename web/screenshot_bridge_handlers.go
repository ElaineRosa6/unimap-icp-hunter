package web

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/unimap-icp-hunter/project/internal/screenshot"
	"golang.org/x/image/webp"
)

var screenshotFilenameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func (s *Server) handleScreenshotBridgeHealth(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	snap := s.buildBridgeDiagnosticSnapshot()
	snap["success"] = true
	snap["message"] = "bridge diagnostic ready"
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleScreenshotBridgeStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	snap := s.buildBridgeDiagnosticSnapshot()
	snap["success"] = true
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleScreenshotBridgePair(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	if !isLoopbackRequest(r) {
		s.setBridgeLastError("forbidden_origin: bridge pairing is restricted to loopback requests")
		writeAPIError(w, http.StatusForbidden, "forbidden_origin", "bridge pairing is restricted to loopback requests", nil)
		return
	}

	var req struct {
		ClientID string `json:"client_id"`
		PairCode string `json:"pair_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.setBridgeLastError("invalid_pair_request: invalid pair request")
		writeAPIError(w, http.StatusBadRequest, "invalid_pair_request", "invalid pair request", nil)
		return
	}
	if strings.TrimSpace(req.ClientID) == "" || strings.TrimSpace(req.PairCode) == "" {
		s.setBridgeLastError("invalid_pair_request: client_id and pair_code are required")
		writeAPIError(w, http.StatusBadRequest, "invalid_pair_request", "client_id and pair_code are required", nil)
		return
	}

	ttl := 600
	if s.config != nil && s.config.Screenshot.Extension.TokenTTLSeconds > 0 {
		ttl = s.config.Screenshot.Extension.TokenTTLSeconds
	}
	token, expireAt, err := s.issueBridgeToken(ttl)
	if err != nil {
		s.setBridgeLastError("bridge_internal_error: failed to issue bridge token")
		writeAPIError(w, http.StatusInternalServerError, "bridge_internal_error", "failed to issue bridge token", err.Error())
		return
	}
	s.clearBridgeLastError()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"token":      token,
		"expires_in": ttl,
		"expire_at":  expireAt,
	})
}

func (s *Server) handleScreenshotBridgeRotateToken(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !isLoopbackRequest(r) {
		s.setBridgeLastError("forbidden_origin: bridge token rotate is restricted to loopback requests")
		writeAPIError(w, http.StatusForbidden, "forbidden_origin", "bridge token rotate is restricted to loopback requests", nil)
		return
	}

	oldToken, ok := s.validateBridgeAuthIfRequired(w, r)
	if !ok {
		return
	}
	s.touchBridgeToken(oldToken)

	var req struct {
		RevokeOld bool `json:"revoke_old"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		s.setBridgeLastError("invalid_rotate_request: invalid rotate request payload")
		writeAPIError(w, http.StatusBadRequest, "invalid_rotate_request", "invalid rotate request payload", nil)
		return
	}

	ttl := 600
	if s.config != nil && s.config.Screenshot.Extension.TokenTTLSeconds > 0 {
		ttl = s.config.Screenshot.Extension.TokenTTLSeconds
	}
	newToken, expireAt, err := s.issueBridgeToken(ttl)
	if err != nil {
		s.setBridgeLastError("bridge_internal_error: failed to rotate bridge token")
		writeAPIError(w, http.StatusInternalServerError, "bridge_internal_error", "failed to rotate bridge token", err.Error())
		return
	}

	revoked := false
	if req.RevokeOld || strings.TrimSpace(oldToken) != "" {
		revoked = s.revokeBridgeToken(oldToken)
	}
	s.clearBridgeLastError()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":           true,
		"token":             newToken,
		"expires_in":        ttl,
		"expire_at":         expireAt,
		"revoked_old_token": revoked,
	})
}

func (s *Server) handleScreenshotBridgeMockResult(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !isLoopbackRequest(r) {
		s.setBridgeLastError("forbidden_origin: mock bridge callback is restricted to loopback requests")
		writeAPIError(w, http.StatusForbidden, "forbidden_origin", "mock bridge callback is restricted to loopback requests", nil)
		return
	}
	if s.bridge.Mock == nil {
		s.setBridgeLastError("bridge_unavailable: bridge mock client not initialized")
		writeAPIError(w, http.StatusServiceUnavailable, "bridge_unavailable", "bridge mock client not initialized", nil)
		return
	}
	token, ok := s.validateBridgeAuthIfRequired(w, r)
	if !ok {
		return
	}

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		s.setBridgeLastError("invalid_bridge_result: failed to read bridge result payload")
		writeAPIError(w, http.StatusBadRequest, "invalid_bridge_result", "failed to read bridge result payload", nil)
		return
	}
	if err := s.validateBridgeCallbackSignatureIfRequired(r, rawBody, token); err != nil {
		s.setBridgeLastError("unauthorized_bridge: invalid callback signature")
		writeAPIError(w, http.StatusUnauthorized, "unauthorized_bridge", "invalid callback signature", err.Error())
		return
	}
	s.touchBridgeToken(token)

	var req struct {
		RequestID string `json:"request_id"`
		Success   bool   `json:"success"`
		ImagePath string `json:"image_path"`
		ImageData string `json:"image_data"`
		BatchID   string `json:"batch_id"`
		URL       string `json:"url"`
		ErrorCode string `json:"error_code"`
		Error     string `json:"error"`
	}
	if err := json.NewDecoder(bytes.NewReader(rawBody)).Decode(&req); err != nil {
		s.setBridgeLastError("invalid_bridge_result: invalid bridge result payload")
		writeAPIError(w, http.StatusBadRequest, "invalid_bridge_result", "invalid bridge result payload", nil)
		return
	}
	if strings.TrimSpace(req.RequestID) == "" {
		s.setBridgeLastError("invalid_bridge_result: request_id is required")
		writeAPIError(w, http.StatusBadRequest, "invalid_bridge_result", "request_id is required", nil)
		return
	}

	resolvedPath := strings.TrimSpace(req.ImagePath)
	if resolvedPath == "" && strings.TrimSpace(req.ImageData) != "" {
		taskMeta, _ := s.bridge.Mock.TaskForRequest(strings.TrimSpace(req.RequestID))
		batchID := strings.TrimSpace(req.BatchID)
		if batchID == "" {
			batchID = strings.TrimSpace(taskMeta.BatchID)
		}
		targetURL := strings.TrimSpace(req.URL)
		if targetURL == "" {
			targetURL = strings.TrimSpace(taskMeta.URL)
		}
		savedPath, saveErr := s.persistBridgeImageData(strings.TrimSpace(req.ImageData), strings.TrimSpace(req.RequestID), batchID, targetURL)
		if saveErr != nil {
			s.setBridgeLastError("invalid_bridge_result: failed to persist image_data")
			writeAPIError(w, http.StatusBadRequest, "invalid_bridge_result", "failed to persist image_data", saveErr.Error())
			return
		}
		resolvedPath = savedPath
	}

	s.bridge.Mock.PushResult(screenshot.BridgeResult{
		RequestID:  strings.TrimSpace(req.RequestID),
		Success:    req.Success,
		ImagePath:  resolvedPath,
		ErrorCode:  strings.TrimSpace(req.ErrorCode),
		Error:      strings.TrimSpace(req.Error),
		DurationMS: 1,
	})
	s.clearBridgeLastError()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"accepted":      true,
		"request_id":    strings.TrimSpace(req.RequestID),
		"image_path":    resolvedPath,
		"received_at":   time.Now().Unix(),
		"result_source": "mock",
	})
}

func (s *Server) handleScreenshotBridgeTaskNext(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if !isLoopbackRequest(r) {
		s.setBridgeLastError("forbidden_origin: bridge task pull is restricted to loopback requests")
		writeAPIError(w, http.StatusForbidden, "forbidden_origin", "bridge task pull is restricted to loopback requests", nil)
		return
	}
	if s.bridge.Mock == nil {
		s.setBridgeLastError("bridge_unavailable: bridge mock client not initialized")
		writeAPIError(w, http.StatusServiceUnavailable, "bridge_unavailable", "bridge mock client not initialized", nil)
		return
	}
	if _, ok := s.validateBridgeAuthIfRequired(w, r); !ok {
		return
	}
	token := strings.TrimSpace(extractBearerToken(r.Header.Get("Authorization")))
	if token != "" {
		s.touchBridgeToken(token)
	}

	task, ok := s.bridge.Mock.NextTask()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"task":    nil,
		})
		return
	}

	timeoutMS := int(task.Timeout / time.Millisecond)
	if timeoutMS <= 0 {
		timeoutMS = 15000
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"task": map[string]interface{}{
			"request_id":      task.RequestID,
			"batch_id":        task.BatchID,
			"url":             task.URL,
			"wait_strategy":   task.WaitStrategy,
			"timeout_ms":      timeoutMS,
			"viewport_width":  task.ViewportWidth,
			"viewport_height": task.ViewportHeight,
		},
	})
	s.clearBridgeLastError()
}

func (s *Server) validateBridgeAuthIfRequired(w http.ResponseWriter, r *http.Request) (string, bool) {
	required := true
	if s.config != nil {
		required = s.config.Screenshot.Extension.PairingRequired
	}
	if !required {
		return "", true
	}
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if raw == "" || !strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		s.setBridgeLastError("unauthorized_bridge: missing bridge bearer token")
		writeAPIError(w, http.StatusUnauthorized, "unauthorized_bridge", "missing bridge bearer token", nil)
		return "", false
	}
	token := strings.TrimSpace(raw[7:])
	if token == "" || !s.validateBridgeToken(token) {
		s.setBridgeLastError("unauthorized_bridge: invalid or expired bridge token")
		writeAPIError(w, http.StatusUnauthorized, "unauthorized_bridge", "invalid or expired bridge token", nil)
		return "", false
	}
	return token, true
}

func (s *Server) validateBridgeCallbackSignatureIfRequired(r *http.Request, body []byte, token string) error {
	required := false
	skewSeconds := 300
	nonceTTLSeconds := 600
	if s.config != nil {
		required = s.config.Screenshot.Extension.CallbackSignatureRequired
		if s.config.Screenshot.Extension.CallbackSignatureSkewSeconds > 0 {
			skewSeconds = s.config.Screenshot.Extension.CallbackSignatureSkewSeconds
		}
		if s.config.Screenshot.Extension.CallbackNonceTTLSeconds > 0 {
			nonceTTLSeconds = s.config.Screenshot.Extension.CallbackNonceTTLSeconds
		}
	}
	if !required {
		return nil
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("bridge callback signature requires pairing token")
	}

	timestampRaw := strings.TrimSpace(r.Header.Get("X-Bridge-Timestamp"))
	nonce := strings.TrimSpace(r.Header.Get("X-Bridge-Nonce"))
	signature := strings.TrimSpace(r.Header.Get("X-Bridge-Signature"))
	if timestampRaw == "" || nonce == "" || signature == "" {
		return fmt.Errorf("missing bridge signature headers")
	}

	ts, err := strconv.ParseInt(timestampRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid bridge timestamp")
	}
	now := time.Now().Unix()
	if ts < now-int64(skewSeconds) || ts > now+int64(skewSeconds) {
		return fmt.Errorf("bridge signature timestamp out of allowed skew")
	}

	if len(nonce) < 8 || len(nonce) > 128 {
		return fmt.Errorf("invalid bridge nonce length")
	}
	for _, ch := range nonce {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' {
			continue
		}
		return fmt.Errorf("invalid bridge nonce format")
	}

	lowerSig := strings.ToLower(signature)
	if strings.HasPrefix(lowerSig, "sha256=") {
		signature = strings.TrimSpace(signature[7:])
	}
	provided, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return fmt.Errorf("invalid bridge signature encoding")
	}

	bodyHash := sha256.Sum256(body)
	canonical := fmt.Sprintf("%d\n%s\n%s", ts, nonce, hex.EncodeToString(bodyHash[:]))
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(canonical))
	expected := mac.Sum(nil)
	if !hmac.Equal(provided, expected) {
		return fmt.Errorf("bridge signature mismatch")
	}

	if !s.consumeBridgeCallbackNonce(token, nonce, now+int64(nonceTTLSeconds)) {
		return fmt.Errorf("bridge nonce replay detected")
	}

	return nil
}

func (s *Server) consumeBridgeCallbackNonce(token, nonce string, expireAt int64) bool {
	if s == nil {
		return false
	}
	now := time.Now().Unix()
	key := token + ":" + nonce

	s.bridge.mu.Lock()
	if s.bridge.CallbackNonces == nil {
		s.bridge.CallbackNonces = make(map[string]int64)
	}
	for k, exp := range s.bridge.CallbackNonces {
		if exp <= now {
			delete(s.bridge.CallbackNonces, k)
		}
	}
	if _, exists := s.bridge.CallbackNonces[key]; exists {
		s.bridge.mu.Unlock()
		return false
	}
	s.bridge.CallbackNonces[key] = expireAt
	s.bridge.mu.Unlock()

	return true
}

func (s *Server) issueBridgeToken(ttlSeconds int) (string, int64, error) {
	if ttlSeconds <= 0 {
		ttlSeconds = 600
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", 0, err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	expireAt := time.Now().Add(time.Duration(ttlSeconds) * time.Second).Unix()

	s.bridge.mu.Lock()
	if s.bridge.Tokens == nil {
		s.bridge.Tokens = make(map[string]int64)
	}
	if s.bridge.LastSeen == nil {
		s.bridge.LastSeen = make(map[string]int64)
	}
	s.bridge.Tokens[token] = expireAt
	s.bridge.LastSeen[token] = time.Now().Unix()
	for tk, exp := range s.bridge.Tokens {
		if exp <= time.Now().Unix() {
			delete(s.bridge.Tokens, tk)
			delete(s.bridge.LastSeen, tk)
		}
	}
	s.bridge.mu.Unlock()

	return token, expireAt, nil
}

func (s *Server) validateBridgeToken(token string) bool {
	s.bridge.mu.Lock()
	defer s.bridge.mu.Unlock()
	if s.bridge.Tokens == nil {
		return false
	}
	expireAt, ok := s.bridge.Tokens[token]
	if !ok {
		return false
	}
	if expireAt <= time.Now().Unix() {
		delete(s.bridge.Tokens, token)
		delete(s.bridge.LastSeen, token)
		return false
	}
	return true
}

func (s *Server) touchBridgeToken(token string) {
	token = strings.TrimSpace(token)
	if token == "" || s == nil {
		return
	}
	s.bridge.mu.Lock()
	if s.bridge.LastSeen == nil {
		s.bridge.LastSeen = make(map[string]int64)
	}
	if _, ok := s.bridge.Tokens[token]; ok {
		s.bridge.LastSeen[token] = time.Now().Unix()
	}
	s.bridge.mu.Unlock()
}

func (s *Server) revokeBridgeToken(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}

	s.bridge.mu.Lock()
	defer s.bridge.mu.Unlock()
	if s.bridge.Tokens == nil {
		return false
	}
	if _, ok := s.bridge.Tokens[token]; !ok {
		return false
	}
	delete(s.bridge.Tokens, token)
	delete(s.bridge.LastSeen, token)

	if s.bridge.CallbackNonces != nil {
		prefix := token + ":"
		for k := range s.bridge.CallbackNonces {
			if strings.HasPrefix(k, prefix) {
				delete(s.bridge.CallbackNonces, k)
			}
		}
	}

	return true
}

func (s *Server) persistBridgeImageData(dataURL, requestID, batchID, targetURL string) (string, error) {
	metaPrefix := "data:image/"
	if !strings.HasPrefix(dataURL, metaPrefix) {
		return "", fmt.Errorf("image_data must be a data URL")
	}
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid data URL format")
	}
	header := parts[0]
	encoded := parts[1]

	ext := ".png"
	mime := "image/png"
	switch {
	case strings.Contains(header, "image/jpeg"):
		ext = ".jpg"
		mime = "image/jpeg"
	case strings.Contains(header, "image/webp"):
		ext = ".webp"
		mime = "image/webp"
	case strings.Contains(header, "image/png"):
		ext = ".png"
		mime = "image/png"
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode image_data: %w", err)
	}
	if len(raw) == 0 {
		return "", fmt.Errorf("decoded image data is empty")
	}

	baseDir := s.resolveScreenshotBaseDir()
	if strings.TrimSpace(batchID) == "" {
		batchID = "bridge_" + time.Now().Format("20060102_150405")
	}
	batchID = screenshotFilenameSanitizer.ReplaceAllString(batchID, "_")
	if batchID == "" {
		batchID = "bridge_batch"
	}
	batchDir := filepath.Join(baseDir, batchID)
	if err := os.MkdirAll(batchDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create batch directory: %w", err)
	}

	name := strings.TrimSpace(requestID)
	if name == "" {
		name = fmt.Sprintf("bridge_%d", time.Now().UnixNano())
	}
	if strings.TrimSpace(targetURL) != "" {
		host := strings.TrimSpace(targetURL)
		host = strings.TrimPrefix(host, "https://")
		host = strings.TrimPrefix(host, "http://")
		host = strings.Split(host, "/")[0]
		host = screenshotFilenameSanitizer.ReplaceAllString(host, "_")
		if host != "" {
			name = host + "_" + name
		}
	}
	name = screenshotFilenameSanitizer.ReplaceAllString(name, "_")
	if name == "" {
		name = fmt.Sprintf("bridge_%d", time.Now().UnixNano())
	}
	fileName := name + ext
	absPath := filepath.Join(batchDir, fileName)

	if strings.EqualFold(mime, "image/jpeg") {
		img, _, decErr := image.Decode(bytes.NewReader(raw))
		f, createErr := os.Create(absPath)
		if createErr != nil {
			return "", createErr
		}
		defer f.Close()
		if decErr != nil {
			if _, writeErr := f.Write(raw); writeErr != nil {
				return "", writeErr
			}
		} else {
			if encErr := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); encErr != nil {
				return "", encErr
			}
		}
	} else if strings.EqualFold(mime, "image/webp") {
		img, decErr := webp.Decode(bytes.NewReader(raw))
		absPath = strings.TrimSuffix(absPath, ext) + ".png"
		f, createErr := os.Create(absPath)
		if createErr != nil {
			return "", createErr
		}
		defer f.Close()
		if decErr != nil {
			if _, writeErr := f.Write(raw); writeErr != nil {
				return "", writeErr
			}
		} else {
			if encErr := png.Encode(f, img); encErr != nil {
				return "", encErr
			}
		}
	} else {
		img, _, decErr := image.Decode(bytes.NewReader(raw))
		f, createErr := os.Create(absPath)
		if createErr != nil {
			return "", createErr
		}
		defer f.Close()
		if decErr != nil {
			if _, writeErr := f.Write(raw); writeErr != nil {
				return "", writeErr
			}
		} else {
			if encErr := png.Encode(f, img); encErr != nil {
				return "", encErr
			}
		}
	}

	return absPath, nil
}

func isLoopbackRequest(r *http.Request) bool {
	if r == nil {
		return false
	}

	// Reject forwarded requests for bridge-only local endpoints.
	if strings.TrimSpace(r.Header.Get("X-Forwarded-For")) != "" || strings.TrimSpace(r.Header.Get("X-Real-IP")) != "" {
		return false
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false
	}

	requestHost := strings.TrimSpace(r.Host)
	if requestHost == "" {
		return false
	}
	if h, _, splitErr := net.SplitHostPort(requestHost); splitErr == nil {
		requestHost = strings.TrimSpace(h)
	}
	if strings.EqualFold(requestHost, "localhost") {
		return true
	}
	hostIP := net.ParseIP(requestHost)
	return hostIP != nil && hostIP.IsLoopback()
}

func (s *Server) setBridgeLastError(message string) {
	if s == nil {
		return
	}
	s.bridge.mu.Lock()
	s.bridge.LastErr = strings.TrimSpace(message)
	s.bridge.LastAt = time.Now().Unix()
	s.bridge.mu.Unlock()
}

func (s *Server) clearBridgeLastError() {
	if s == nil {
		return
	}
	s.bridge.mu.Lock()
	s.bridge.LastErr = ""
	s.bridge.LastAt = 0
	s.bridge.mu.Unlock()
}

func (s *Server) activeBridgeTokens() int {
	if s == nil {
		return 0
	}
	now := time.Now().Unix()
	count := 0
	s.bridge.mu.Lock()
	for token, expireAt := range s.bridge.Tokens {
		if expireAt <= now {
			delete(s.bridge.Tokens, token)
			delete(s.bridge.LastSeen, token)
			continue
		}
		count++
	}
	s.bridge.mu.Unlock()
	return count
}

func (s *Server) activeBridgeLiveTokens() int {
	if s == nil {
		return 0
	}
	now := time.Now().Unix()
	const liveWindowSeconds = 15
	count := 0
	s.bridge.mu.Lock()
	for token, expireAt := range s.bridge.Tokens {
		if expireAt <= now {
			delete(s.bridge.Tokens, token)
			delete(s.bridge.LastSeen, token)
			continue
		}
		lastSeen := s.bridge.LastSeen[token]
		if lastSeen <= 0 || now-lastSeen > liveWindowSeconds {
			continue
		}
		count++
	}
	s.bridge.mu.Unlock()
	return count
}

func (s *Server) buildBridgeDiagnosticSnapshot() map[string]interface{} {
	engine := "cdp"
	enabled := false
	pairingRequired := true
	listenAddr := ""
	if s.config != nil {
		engine = strings.TrimSpace(s.config.Screenshot.Engine)
		if engine == "" {
			engine = "cdp"
		}
		enabled = s.config.Screenshot.Extension.Enabled
		pairingRequired = s.config.Screenshot.Extension.PairingRequired
		listenAddr = strings.TrimSpace(s.config.Screenshot.Extension.ListenAddr)
	}

	inFlight := 0
	workers := 0
	queueLen := 0
	bridgeConnected := false
	if s.bridge.Service != nil {
		inFlight = s.bridge.Service.InFlight()
		workers = s.bridge.Service.WorkerCount()
		queueLen = s.bridge.Service.QueueLen()
		bridgeConnected = true
	}
	pending, waiters := 0, 0
	if s.bridge.Mock != nil {
		pending, waiters = s.bridge.Mock.Stats()
	}

	ready := engine == "cdp" || (engine == "extension" && enabled && bridgeConnected)

	s.bridge.mu.Lock()
	lastErr := s.bridge.LastErr
	lastAt := s.bridge.LastAt
	s.bridge.mu.Unlock()

	return map[string]interface{}{
		"engine":            engine,
		"extension_enabled": enabled,
		"pairing_required":  pairingRequired,
		"listen_addr":       listenAddr,
		"ready":             ready,
		"bridge_connected":  bridgeConnected,
		"paired_clients":    s.activeBridgeTokens(),
		"live_clients":      s.activeBridgeLiveTokens(),
		"pending_tasks":     pending,
		"awaiting_results":  waiters,
		"in_flight_tasks":   inFlight,
		"queue_len":         queueLen,
		"worker_count":      workers,
		"last_error":        lastErr,
		"last_error_at":     lastAt,
		"router_mode":       s.screenshotRouterMode(),
		"router_cdp_healthy": s.screenshotRouterCDPHealthy(),
		"router_ext_healthy": s.screenshotRouterExtHealthy(),
	}
}

func (s *Server) screenshotRouterMode() string {
	if s.screenshotRouter != nil {
		return string(s.screenshotRouter.ActiveMode())
	}
	return ""
}

func (s *Server) screenshotRouterCDPHealthy() bool {
	if s.screenshotRouter != nil {
		cdp, _ := s.screenshotRouter.HealthStatus()
		return cdp
	}
	return false
}

func (s *Server) screenshotRouterExtHealthy() bool {
	if s.screenshotRouter != nil {
		_, ext := s.screenshotRouter.HealthStatus()
		return ext
	}
	return false
}
