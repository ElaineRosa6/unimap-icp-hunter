package web

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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

	"github.com/unimap/project/internal/logger"
	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/screenshot"
	"golang.org/x/image/webp"
)

var screenshotFilenameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func (s *Server) handleScreenshotBridgeHealth(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	// FINDING-002: health/status expose internal diagnostics (engine, paired
	// clients, queue depth, errors). Restrict to loopback like other bridge
	// endpoints; remote callers get a minimal ok response only.
	if !isLoopbackRequest(r) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "bridge health ok"})
		return
	}
	snap := s.buildBridgeDiagnosticSnapshot()
	snap.Success = true
	snap.Message = "bridge diagnostic ready"
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleScreenshotBridgeStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if !isLoopbackRequest(r) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "bridge status ok"})
		return
	}
	snap := s.buildBridgeDiagnosticSnapshot()
	snap.Success = true
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
	if !decodeJSONBody(w, r, &req) {
		s.setBridgeLastError("invalid_pair_request: invalid pair request")
		return
	}
	if strings.TrimSpace(req.ClientID) == "" || strings.TrimSpace(req.PairCode) == "" {
		s.setBridgeLastError("invalid_pair_request: client_id and pair_code are required")
		writeAPIError(w, http.StatusBadRequest, "invalid_pair_request", "client_id and pair_code are required", nil)
		return
	}

	cfg := s.currentConfig()
	if cfg != nil && cfg.Screenshot.Extension.PairCode != "" {
		if subtle.ConstantTimeCompare([]byte(cfg.Screenshot.Extension.PairCode), []byte(req.PairCode)) != 1 {
			s.setBridgeLastError("invalid_pair_code: pair_code mismatch")
			writeAPIError(w, http.StatusForbidden, "invalid_pair_code", "pair_code mismatch", nil)
			return
		}
	}

	ttl := 600
	if cfg != nil && cfg.Screenshot.Extension.TokenTTLSeconds > 0 {
		ttl = cfg.Screenshot.Extension.TokenTTLSeconds
	}
	token, expireAt, err := s.issueBridgeToken(ttl)
	if err != nil {
		s.setBridgeLastError("bridge_internal_error: failed to issue bridge token")
		writeAPIError(w, http.StatusInternalServerError, "bridge_internal_error", "failed to issue bridge token", sanitizeError(err.Error()))
		return
	}
	s.clearBridgeLastError()
	s.bridge.mu.Lock()
	s.bridge.LastPairAt = time.Now().Unix()
	s.bridge.mu.Unlock()

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
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil && err != io.EOF {
		s.setBridgeLastError("invalid_rotate_request: invalid rotate request payload")
		writeAPIError(w, http.StatusBadRequest, "invalid_rotate_request", "invalid rotate request payload", nil)
		return
	}

	ttl := 600
	if cfg := s.currentConfig(); cfg != nil && cfg.Screenshot.Extension.TokenTTLSeconds > 0 {
		ttl = cfg.Screenshot.Extension.TokenTTLSeconds
	}
	newToken, expireAt, err := s.issueBridgeToken(ttl)
	if err != nil {
		s.setBridgeLastError("bridge_internal_error: failed to rotate bridge token")
		writeAPIError(w, http.StatusInternalServerError, "bridge_internal_error", "failed to rotate bridge token", sanitizeError(err.Error()))
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

	rawBody, err := s.readAndValidateBridgeBody(w, r)
	if err != nil {
		return
	}
	if token != "" {
		if err := s.validateBridgeCallbackSignatureIfRequired(r, rawBody, token); err != nil {
			s.setBridgeLastError("unauthorized_bridge: invalid callback signature")
			writeAPIError(w, http.StatusUnauthorized, "unauthorized_bridge", "invalid callback signature", nil)
			return
		}
		s.touchBridgeToken(token)
	}

	var req struct {
		RequestID               string                 `json:"request_id"`
		Success                 bool                   `json:"success"`
		ImagePath               string                 `json:"image_path"`
		ImageData               string                 `json:"image_data"`
		BatchID                 string                 `json:"batch_id"`
		URL                     string                 `json:"url"`
		FinalURL                string                 `json:"final_url"`
		CollectedData           string                 `json:"collected_data"`
		StructuredCollectedData map[string]interface{} `json:"structured_collected_data"`
		Error                   string                 `json:"error"`
		ErrorCode               string                 `json:"error_code"`
		DurationMs              int64                  `json:"duration_ms"`
	}
	if err := json.Unmarshal(rawBody, &req); err != nil {
		s.setBridgeLastError("invalid_bridge_result: invalid bridge result payload")
		writeAPIError(w, http.StatusBadRequest, "invalid_bridge_result", "invalid bridge result payload", nil)
		return
	}
	if strings.TrimSpace(req.RequestID) == "" {
		s.setBridgeLastError("invalid_bridge_result: request_id is required")
		writeAPIError(w, http.StatusBadRequest, "invalid_bridge_result", "request_id is required", nil)
		return
	}

	// Convert raw map to typed struct
	var structuredData *model.BridgeCollectedData
	if req.StructuredCollectedData != nil {
		raw, err := json.Marshal(req.StructuredCollectedData)
		if err == nil {
			var cd model.BridgeCollectedData
			_ = json.Unmarshal(raw, &cd)
			structuredData = &cd
		}
	}

	logBridgeCollectedData(req.RequestID, structuredData)

	resolvedPath, pathOK := s.resolveBridgeImagePath(w, &req)
	if !pathOK {
		return
	}

	s.bridge.Mock.PushResult(screenshot.BridgeResult{
		RequestID:               strings.TrimSpace(req.RequestID),
		Success:                 req.Success,
		ImagePath:               resolvedPath,
		FinalURL:                strings.TrimSpace(req.FinalURL),
		CollectedData:           strings.TrimSpace(req.CollectedData),
		StructuredCollectedData: structuredData,
		ErrorCode:               strings.TrimSpace(req.ErrorCode),
		Error:                   strings.TrimSpace(req.Error),
		DurationMS:              1,
	})
	s.clearBridgeLastError()
	s.bridge.mu.Lock()
	s.bridge.LastCallbackAt = time.Now().Unix()
	s.bridge.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"accepted":      true,
		"request_id":    strings.TrimSpace(req.RequestID),
		"image_path":    resolvedPath,
		"received_at":   time.Now().Unix(),
		"result_source": "mock",
	})
}

// readAndValidateBridgeBody reads and size-limits the request body.
func (s *Server) readAndValidateBridgeBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	maxBodyBytes := int64(10 * 1024 * 1024)
	if cfg := s.currentConfig(); cfg != nil && cfg.Web.RequestLimits.MaxBodyBytes > 0 {
		maxBodyBytes = cfg.Web.RequestLimits.MaxBodyBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		s.setBridgeLastError("invalid_bridge_result: request body too large")
		writeAPIError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds configured limit", nil)
		return nil, err
	}
	return rawBody, nil
}

// resolveBridgeImagePath resolves the image path from the bridge request,
// persisting image_data if needed. Returns (path, true) on success.
func (s *Server) resolveBridgeImagePath(w http.ResponseWriter, req *struct {
	RequestID               string                 `json:"request_id"`
	Success                 bool                   `json:"success"`
	ImagePath               string                 `json:"image_path"`
	ImageData               string                 `json:"image_data"`
	BatchID                 string                 `json:"batch_id"`
	URL                     string                 `json:"url"`
	FinalURL                string                 `json:"final_url"`
	CollectedData           string                 `json:"collected_data"`
	StructuredCollectedData map[string]interface{} `json:"structured_collected_data"`
	Error                   string                 `json:"error"`
	ErrorCode               string                 `json:"error_code"`
	DurationMs              int64                  `json:"duration_ms"`
}) (string, bool) {
	resolvedPath := strings.TrimSpace(req.ImagePath)
	if resolvedPath != "" || strings.TrimSpace(req.ImageData) == "" {
		return resolvedPath, true
	}
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
		return "", false
	}
	return savedPath, true
}

// logBridgeCollectedData logs diagnostic info about structured collect data.
func logBridgeCollectedData(requestID string, data *model.BridgeCollectedData) {
	requestID = strings.TrimSpace(requestID)
	if data == nil {
		logger.Warnf("[bridge-collect] request_id=%s has nil StructuredCollectedData", requestID)
		return
	}
	itemCount := len(data.Items)
	logger.Infof("[bridge-collect] request_id=%s items=%d total=%d engine=%v",
		requestID, itemCount, data.Total, data.Engine)
	if len(data.Items) > 0 {
		for i := 0; i < len(data.Items) && i < 3; i++ {
			item := data.Items[i]
			logger.Infof("[bridge-collect] item[%d]: ip=%v port=%v title=%v", i, item.IP, item.Port, item.Title)
		}
	} else {
		logger.Warnf("[bridge-collect] request_id=%s has structured_collected_data but items is empty or missing", requestID)
	}
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

	// An authenticated empty poll is activity too; idle clients must not age out.
	s.bridge.mu.Lock()
	s.bridge.LastTaskPullAt = time.Now().Unix()
	s.bridge.mu.Unlock()

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
			"query":           task.Query,
			"wait_strategy":   task.WaitStrategy,
			"timeout_ms":      timeoutMS,
			"viewport_width":  task.ViewportWidth,
			"viewport_height": task.ViewportHeight,
			"action":          task.Action,
		},
	})
	s.clearBridgeLastError()
}

func (s *Server) validateBridgeAuthIfRequired(w http.ResponseWriter, r *http.Request) (string, bool) {
	required := true
	if cfg := s.currentConfig(); cfg != nil {
		required = cfg.Screenshot.Extension.PairingRequired
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

	// Accept the admin token as a valid bridge credential for loopback requests.
	// This allows the extension to work after server restart without re-pairing:
	// the admin token is static (from config.yaml) and survives restarts.
	loopback := isLoopbackRequest(r)
	adminTok := s.adminToken()
	if loopback && adminTok != "" && subtle.ConstantTimeCompare([]byte(token), []byte(adminTok)) == 1 {
		logger.Debugf("[bridge-auth] admin token accepted for loopback request")
		return "", true
	}

	if token == "" || !s.validateBridgeToken(token) {
		s.setBridgeLastError("unauthorized_bridge: invalid or expired bridge token")
		writeAPIError(w, http.StatusUnauthorized, "unauthorized_bridge", "invalid or expired bridge token", nil)
		return "", false
	}
	return token, true
}

func (s *Server) validateBridgeCallbackSignatureIfRequired(r *http.Request, body []byte, token string) error {
	required := false
	pairingRequired := true
	skewSeconds := 300
	nonceTTLSeconds := 600
	if cfg := s.currentConfig(); cfg != nil {
		required = cfg.Screenshot.Extension.CallbackSignatureRequired
		pairingRequired = cfg.Screenshot.Extension.PairingRequired
		if cfg.Screenshot.Extension.CallbackSignatureSkewSeconds > 0 {
			skewSeconds = cfg.Screenshot.Extension.CallbackSignatureSkewSeconds
		}
		if cfg.Screenshot.Extension.CallbackNonceTTLSeconds > 0 {
			nonceTTLSeconds = cfg.Screenshot.Extension.CallbackNonceTTLSeconds
		}
	}
	// Callback signature requires a pairing token; skip if pairing is disabled.
	if !required || !pairingRequired {
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
	if _, err := mac.Write([]byte(canonical)); err != nil {
		return fmt.Errorf("bridge HMAC write failed: %w", err)
	}
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
	header, encoded, err := splitDataURL(dataURL)
	if err != nil {
		return "", err
	}
	ext, mime := detectImageFormat(header)

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode image_data: %w", err)
	}
	if len(raw) == 0 {
		return "", fmt.Errorf("decoded image data is empty")
	}

	batchDir, err := s.prepareBridgeBatchDir(batchID)
	if err != nil {
		return "", err
	}

	absPath := buildBridgeFilePath(batchDir, ext, requestID, targetURL)
	return saveImageToFile(absPath, mime, ext, raw)
}

// splitDataURL parses a data URL into its header and encoded payload.
func splitDataURL(dataURL string) (string, string, error) {
	if !strings.HasPrefix(dataURL, "data:image/") {
		return "", "", fmt.Errorf("image_data must be a data URL")
	}
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid data URL format")
	}
	return parts[0], parts[1], nil
}

// detectImageFormat returns the file extension and MIME type from a data URL header.
func detectImageFormat(header string) (ext, mime string) {
	ext = ".png"
	mime = "image/png"
	switch {
	case strings.Contains(header, "image/jpeg"):
		return ".jpg", "image/jpeg"
	case strings.Contains(header, "image/webp"):
		return ".webp", "image/webp"
	}
	return ext, mime
}

// prepareBridgeBatchDir creates and returns the batch directory for saving images.
func (s *Server) prepareBridgeBatchDir(batchID string) (string, error) {
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
	return batchDir, nil
}

// buildBridgeFilePath constructs the full file path for a bridge image.
func buildBridgeFilePath(batchDir, ext, requestID, targetURL string) string {
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
	return filepath.Join(batchDir, name+ext)
}

// saveImageToFile decodes raw image bytes and writes them to absPath.
// For JPEG it re-encodes at quality 90; for WebP/PNG it converts to PNG.
func saveImageToFile(absPath, mime, ext string, raw []byte) (string, error) {
	if strings.EqualFold(mime, "image/jpeg") {
		return absPath, encodeOrWriteRaw(absPath, raw, func() error {
			img, _, decErr := image.Decode(bytes.NewReader(raw))
			if decErr != nil {
				return writeRawToFile(absPath, raw)
			}
			return encodeJPEG(absPath, img)
		})
	}

	outPath := strings.TrimSuffix(absPath, ext) + ".png"
	if strings.EqualFold(mime, "image/webp") {
		return outPath, encodeOrWriteRaw(outPath, raw, func() error {
			img, decErr := webp.Decode(bytes.NewReader(raw))
			if decErr != nil {
				return writeRawToFile(outPath, raw)
			}
			return encodePNG(outPath, img)
		})
	}

	return outPath, encodeOrWriteRaw(outPath, raw, func() error {
		img, _, decErr := image.Decode(bytes.NewReader(raw))
		if decErr != nil {
			return writeRawToFile(outPath, raw)
		}
		return encodePNG(outPath, img)
	})
}

// encodeOrWriteRaw runs the encodeFn and cleans up on failure.
func encodeOrWriteRaw(path string, raw []byte, encodeFn func() error) error {
	f, createErr := os.Create(path)
	if createErr != nil {
		return createErr
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	if err := encodeFn(); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

func writeRawToFile(path string, raw []byte) error {
	return os.WriteFile(path, raw, 0644)
}

func encodeJPEG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
}

func encodePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
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
	return activeBridgeLiveTokenCount(s.bridge)
}

func activeBridgeLiveTokenCount(bridge *BridgeState) int {
	if bridge == nil {
		return 0
	}
	now := time.Now().Unix()
	const liveWindowSeconds = 60
	count := 0
	bridge.mu.Lock()
	for token, expireAt := range bridge.Tokens {
		if expireAt <= now {
			delete(bridge.Tokens, token)
			delete(bridge.LastSeen, token)
			continue
		}
		lastSeen := bridge.LastSeen[token]
		if lastSeen <= 0 || now-lastSeen > liveWindowSeconds {
			continue
		}
		count++
	}
	bridge.mu.Unlock()
	return count
}

// BridgeDiagnosticSnapshot is the typed response for bridge diagnostic status.
type BridgeDiagnosticSnapshot struct {
	Success          bool   `json:"success"`
	Message          string `json:"message,omitempty"`
	Engine           string `json:"engine"`
	ExtensionEnabled bool   `json:"extension_enabled"`
	PairingRequired  bool   `json:"pairing_required"`
	ListenAddr       string `json:"listen_addr"`
	Ready            bool   `json:"ready"`
	BridgeConnected  bool   `json:"bridge_connected"`
	ExtensionOnline  bool   `json:"extension_online"`
	PairedClients    int    `json:"paired_clients"`
	LiveClients      int    `json:"live_clients"`
	PendingTasks     int    `json:"pending_tasks"`
	AwaitingResults  int    `json:"awaiting_results"`
	InFlightTasks    int    `json:"in_flight_tasks"`
	QueueLen         int    `json:"queue_len"`
	WorkerCount      int    `json:"worker_count"`
	LastError        string `json:"last_error,omitempty"`
	LastErrorAt      int64  `json:"last_error_at,omitempty"`
	LastPairAt       int64  `json:"last_pair_at,omitempty"`
	LastTaskPullAt   int64  `json:"last_task_pull_at,omitempty"`
	LastCallbackAt   int64  `json:"last_callback_at,omitempty"`
	RouterMode       string `json:"router_mode"`
	RouterCDPHealthy bool   `json:"router_cdp_healthy"`
	RouterExtHealthy bool   `json:"router_ext_healthy"`
}

func (s *Server) buildBridgeDiagnosticSnapshot() BridgeDiagnosticSnapshot {
	engine := "cdp"
	enabled := false
	pairingRequired := true
	listenAddr := ""
	if cfg := s.currentConfig(); cfg != nil {
		engine = strings.TrimSpace(cfg.Screenshot.Engine)
		if engine == "" {
			engine = "cdp"
		}
		enabled = cfg.Screenshot.Extension.Enabled
		pairingRequired = cfg.Screenshot.Extension.PairingRequired
		listenAddr = strings.TrimSpace(cfg.Screenshot.Extension.ListenAddr)
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

	pairedClients := s.activeBridgeTokens()
	liveClients := s.activeBridgeLiveTokens()
	routerExtHealthy := s.screenshotRouterExtHealthy()
	extensionOnline := enabled && (routerExtHealthy || liveClients > 0)
	ready := engine == "cdp" || (engine == "extension" && extensionOnline)

	s.bridge.mu.Lock()
	lastErr := s.bridge.LastErr
	lastAt := s.bridge.LastAt
	lastPairAt := s.bridge.LastPairAt
	lastTaskPullAt := s.bridge.LastTaskPullAt
	lastCallbackAt := s.bridge.LastCallbackAt
	s.bridge.mu.Unlock()

	return BridgeDiagnosticSnapshot{
		Engine:           engine,
		ExtensionEnabled: enabled,
		PairingRequired:  pairingRequired,
		ListenAddr:       listenAddr,
		Ready:            ready,
		BridgeConnected:  bridgeConnected,
		ExtensionOnline:  extensionOnline,
		PairedClients:    pairedClients,
		LiveClients:      liveClients,
		PendingTasks:     pending,
		AwaitingResults:  waiters,
		InFlightTasks:    inFlight,
		QueueLen:         queueLen,
		WorkerCount:      workers,
		LastError:        lastErr,
		LastErrorAt:      lastAt,
		LastPairAt:       lastPairAt,
		LastTaskPullAt:   lastTaskPullAt,
		LastCallbackAt:   lastCallbackAt,
		RouterMode:       s.screenshotRouterMode(),
		RouterCDPHealthy: s.screenshotRouterCDPHealthy(),
		RouterExtHealthy: routerExtHealthy,
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
