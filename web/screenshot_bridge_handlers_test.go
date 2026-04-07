package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unimap-icp-hunter/project/internal/config"
)

func newBridgeTestServer(signatureRequired bool) *Server {
	cfg := &config.Config{}
	cfg.Screenshot.BaseDir = "./screenshots"
	cfg.Screenshot.Extension.PairingRequired = true
	cfg.Screenshot.Extension.CallbackSignatureRequired = signatureRequired
	cfg.Screenshot.Extension.CallbackSignatureSkewSeconds = 300
	cfg.Screenshot.Extension.CallbackNonceTTLSeconds = 600

	return &Server{
		config:               cfg,
		bridgeMock:           newBridgeMockClient(),
		bridgeTokens:         map[string]int64{"tok-test": time.Now().Add(5 * time.Minute).Unix()},
		bridgeCallbackNonces: make(map[string]int64),
	}
}

func signedBridgeHeaders(token string, body []byte, ts int64, nonce string) map[string]string {
	bodyHash := sha256.Sum256(body)
	canonical := fmt.Sprintf("%d\n%s\n%s", ts, nonce, hex.EncodeToString(bodyHash[:]))
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(canonical))
	signature := hex.EncodeToString(mac.Sum(nil))
	return map[string]string{
		"Authorization":      "Bearer " + token,
		"X-Bridge-Timestamp": fmt.Sprintf("%d", ts),
		"X-Bridge-Nonce":     nonce,
		"X-Bridge-Signature": signature,
	}
}

func setLoopbackBridgeRequest(req *http.Request) {
	req.RemoteAddr = "127.0.0.1:12345"
	req.Host = "127.0.0.1:8448"
}

func TestBridgeMockResultRejectsMissingSignatureWhenRequired(t *testing.T) {
	s := newBridgeTestServer(true)
	body := `{"request_id":"req-1","success":true,"image_path":"c:/tmp/x.png"}`
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/bridge/mock/result", strings.NewReader(body))
	setLoopbackBridgeRequest(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok-test")

	w := httptest.NewRecorder()
	s.handleScreenshotBridgeMockResult(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestBridgeMockResultAcceptsValidSignature(t *testing.T) {
	s := newBridgeTestServer(true)
	body := []byte(`{"request_id":"req-2","success":true,"image_path":"c:/tmp/x.png"}`)
	ts := time.Now().Unix()
	headers := signedBridgeHeaders("tok-test", body, ts, "nonce-req-2")

	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/bridge/mock/result", strings.NewReader(string(body)))
	setLoopbackBridgeRequest(req)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	s.handleScreenshotBridgeMockResult(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if ok, _ := resp["success"].(bool); !ok {
		t.Fatalf("expected success=true, got %v", resp)
	}
}

func TestBridgeMockResultRejectsReplayNonce(t *testing.T) {
	s := newBridgeTestServer(true)
	body := []byte(`{"request_id":"req-3","success":true,"image_path":"c:/tmp/x.png"}`)
	ts := time.Now().Unix()
	nonce := "nonce-replay-1"
	headers := signedBridgeHeaders("tok-test", body, ts, nonce)

	firstReq := httptest.NewRequest(http.MethodPost, "/api/screenshot/bridge/mock/result", strings.NewReader(string(body)))
	setLoopbackBridgeRequest(firstReq)
	firstReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		firstReq.Header.Set(k, v)
	}
	firstW := httptest.NewRecorder()
	s.handleScreenshotBridgeMockResult(firstW, firstReq)
	if firstW.Code != http.StatusOK {
		t.Fatalf("first request expected 200, got %d", firstW.Code)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/screenshot/bridge/mock/result", strings.NewReader(string(body)))
	setLoopbackBridgeRequest(secondReq)
	secondReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		secondReq.Header.Set(k, v)
	}
	secondW := httptest.NewRecorder()
	s.handleScreenshotBridgeMockResult(secondW, secondReq)
	if secondW.Code != http.StatusUnauthorized {
		t.Fatalf("replay request expected 401, got %d", secondW.Code)
	}
}

func TestBridgeRotateTokenRevokesOldToken(t *testing.T) {
	s := newBridgeTestServer(false)
	body := `{"revoke_old":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/bridge/token/rotate", strings.NewReader(body))
	setLoopbackBridgeRequest(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok-test")

	w := httptest.NewRecorder()
	s.handleScreenshotBridgeRotateToken(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	if s.validateBridgeToken("tok-test") {
		t.Fatalf("expected old token to be revoked")
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	newToken, _ := resp["token"].(string)
	if strings.TrimSpace(newToken) == "" {
		t.Fatalf("expected rotated token in response")
	}
	if !s.validateBridgeToken(newToken) {
		t.Fatalf("expected rotated token to be valid")
	}
}

func TestIsLoopbackRequestRejectsForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/bridge/mock/result", nil)
	setLoopbackBridgeRequest(req)
	req.Header.Set("X-Forwarded-For", "8.8.8.8")

	if isLoopbackRequest(req) {
		t.Fatalf("expected forwarded loopback request to be rejected")
	}
}

func TestIsLoopbackRequestRejectsNonLoopbackHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/screenshot/bridge/mock/result", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Host = "example.com"

	if isLoopbackRequest(req) {
		t.Fatalf("expected non-loopback host to be rejected")
	}
}
