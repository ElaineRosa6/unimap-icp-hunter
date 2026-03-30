package web

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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

	engine := "cdp"
	enabled := false
	listenAddr := ""
	if s.config != nil {
		engine = strings.TrimSpace(s.config.Screenshot.Engine)
		if engine == "" {
			engine = "cdp"
		}
		enabled = s.config.Screenshot.Extension.Enabled
		listenAddr = strings.TrimSpace(s.config.Screenshot.Extension.ListenAddr)
	}

	ready := engine == "cdp" || (engine == "extension" && enabled)
	inFlight := 0
	workers := 0
	queueLen := 0
	bridgeConnected := false
	if s.bridgeService != nil {
		inFlight = s.bridgeService.InFlight()
		workers = s.bridgeService.WorkerCount()
		queueLen = s.bridgeService.QueueLen()
		bridgeConnected = true
	}
	pending, _ := 0, 0
	if s.bridgeMock != nil {
		pending, _ = s.bridgeMock.Stats()
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":           true,
		"engine":            engine,
		"extension_enabled": enabled,
		"listen_addr":       listenAddr,
		"ready":             ready,
		"bridge_connected":  bridgeConnected,
		"paired_clients":    0,
		"pending_tasks":     pending,
		"in_flight_tasks":   inFlight,
		"queue_len":         queueLen,
		"worker_count":      workers,
		"message":           "bridge skeleton ready",
	})
}

func (s *Server) handleScreenshotBridgeStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

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
	if s.bridgeService != nil {
		inFlight = s.bridgeService.InFlight()
		workers = s.bridgeService.WorkerCount()
		queueLen = s.bridgeService.QueueLen()
	}
	pending, waiters := 0, 0
	if s.bridgeMock != nil {
		pending, waiters = s.bridgeMock.Stats()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":           true,
		"engine":            engine,
		"extension_enabled": enabled,
		"pairing_required":  pairingRequired,
		"listen_addr":       listenAddr,
		"paired_clients":    0,
		"pending_tasks":     pending,
		"awaiting_results":  waiters,
		"in_flight_tasks":   inFlight,
		"queue_len":         queueLen,
		"worker_count":      workers,
		"last_error":        "",
		"last_error_at":     0,
	})
}

func (s *Server) handleScreenshotBridgePair(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	if !isLoopbackRequest(r) {
		writeAPIError(w, http.StatusForbidden, "forbidden_origin", "bridge pairing is restricted to loopback requests", nil)
		return
	}

	var req struct {
		ClientID string `json:"client_id"`
		PairCode string `json:"pair_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_pair_request", "invalid pair request", nil)
		return
	}
	if strings.TrimSpace(req.ClientID) == "" || strings.TrimSpace(req.PairCode) == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_pair_request", "client_id and pair_code are required", nil)
		return
	}

	ttl := 600
	if s.config != nil && s.config.Screenshot.Extension.TokenTTLSeconds > 0 {
		ttl = s.config.Screenshot.Extension.TokenTTLSeconds
	}
	token, expireAt, err := s.issueBridgeToken(ttl)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "bridge_internal_error", "failed to issue bridge token", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"token":      token,
		"expires_in": ttl,
		"expire_at":  expireAt,
	})
}

func (s *Server) handleScreenshotBridgeMockResult(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !isLoopbackRequest(r) {
		writeAPIError(w, http.StatusForbidden, "forbidden_origin", "mock bridge callback is restricted to loopback requests", nil)
		return
	}
	if s.bridgeMock == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "bridge_unavailable", "bridge mock client not initialized", nil)
		return
	}
	if !s.validateBridgeAuthIfRequired(w, r) {
		return
	}

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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_bridge_result", "invalid bridge result payload", nil)
		return
	}
	if strings.TrimSpace(req.RequestID) == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_bridge_result", "request_id is required", nil)
		return
	}

	resolvedPath := strings.TrimSpace(req.ImagePath)
	if resolvedPath == "" && strings.TrimSpace(req.ImageData) != "" {
		taskMeta, _ := s.bridgeMock.TaskForRequest(strings.TrimSpace(req.RequestID))
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
			writeAPIError(w, http.StatusBadRequest, "invalid_bridge_result", "failed to persist image_data", saveErr.Error())
			return
		}
		resolvedPath = savedPath
	}

	s.bridgeMock.PushResult(screenshot.BridgeResult{
		RequestID:  strings.TrimSpace(req.RequestID),
		Success:    req.Success,
		ImagePath:  resolvedPath,
		ErrorCode:  strings.TrimSpace(req.ErrorCode),
		Error:      strings.TrimSpace(req.Error),
		DurationMS: 1,
	})

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
		writeAPIError(w, http.StatusForbidden, "forbidden_origin", "bridge task pull is restricted to loopback requests", nil)
		return
	}
	if s.bridgeMock == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "bridge_unavailable", "bridge mock client not initialized", nil)
		return
	}
	if !s.validateBridgeAuthIfRequired(w, r) {
		return
	}

	task, ok := s.bridgeMock.NextTask()
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
}

func (s *Server) validateBridgeAuthIfRequired(w http.ResponseWriter, r *http.Request) bool {
	required := true
	if s.config != nil {
		required = s.config.Screenshot.Extension.PairingRequired
	}
	if !required {
		return true
	}
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if raw == "" || !strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized_bridge", "missing bridge bearer token", nil)
		return false
	}
	token := strings.TrimSpace(raw[7:])
	if token == "" || !s.validateBridgeToken(token) {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized_bridge", "invalid or expired bridge token", nil)
		return false
	}
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

	s.configMutex.Lock()
	if s.bridgeTokens == nil {
		s.bridgeTokens = make(map[string]int64)
	}
	s.bridgeTokens[token] = expireAt
	for tk, exp := range s.bridgeTokens {
		if exp <= time.Now().Unix() {
			delete(s.bridgeTokens, tk)
		}
	}
	s.configMutex.Unlock()

	return token, expireAt, nil
}

func (s *Server) validateBridgeToken(token string) bool {
	s.configMutex.Lock()
	defer s.configMutex.Unlock()
	if s.bridgeTokens == nil {
		return false
	}
	expireAt, ok := s.bridgeTokens[token]
	if !ok {
		return false
	}
	if expireAt <= time.Now().Unix() {
		delete(s.bridgeTokens, token)
		return false
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
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
