package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/unimap-icp-hunter/project/internal/alerting"
)

func TestAlerting_WebhookEndToEnd(t *testing.T) {
	// 1. 启动 mock Webhook server 捕获请求
	var receivedAlerts []alerting.Alert
	var mu sync.Mutex

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			http.Error(w, "bad content type", http.StatusBadRequest)
			return
		}

		var alert alerting.Alert
		if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}

		mu.Lock()
		receivedAlerts = append(receivedAlerts, alert)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	// 2. 创建告警管理器，注册 Webhook 渠道
	mgr := alerting.NewManager()
	webhook := alerting.NewWebhookChannelForTest(mockServer.URL, nil, true)
	mgr.RegisterChannel(webhook)

	// 3. 触发篡改检测告警
	mgr.SendWarning(
		alerting.AlertTypeTamper,
		"Tamper Detected",
		"Homepage content modified",
		map[string]interface{}{
			"segments": []string{"header", "nav", "footer"},
			"url":      "https://example.com",
		},
		"tamper_detector",
		"https://example.com",
	)

	// 4. 等待异步发送完成
	time.Sleep(500 * time.Millisecond)

	// 5. 验证
	mu.Lock()
	count := len(receivedAlerts)
	mu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 alert, got %d", count)
	}
	if count > 0 {
		alert := receivedAlerts[0]
		if alert.Type != alerting.AlertTypeTamper {
			t.Errorf("expected alert type %s, got %s", alerting.AlertTypeTamper, alert.Type)
		}
		if alert.Level != alerting.AlertLevelWarning {
			t.Errorf("expected alert level Warning, got %s", alert.Level)
		}
		if alert.Title != "Tamper Detected" {
			t.Errorf("expected title 'Tamper Detected', got %s", alert.Title)
		}
		if alert.URL != "https://example.com" {
			t.Errorf("expected URL 'https://example.com', got %s", alert.URL)
		}
	}
}

func TestAlerting_WebhookDedupSilencing(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	mgr := alerting.NewManager()
	webhook := alerting.NewWebhookChannel(mockServer.URL, nil, true)
	mgr.RegisterChannel(webhook)

	// 快速发送 5 个相同告警，验证静默机制
	for i := 0; i < 5; i++ {
		mgr.SendWarning(
			alerting.AlertTypeTamper,
			"Same Tamper Alert",
			"Same message",
			map[string]interface{}{
				"segments": []string{"header"},
				"url":      "https://example.com",
			},
			"tamper_detector",
			"https://example.com",
		)
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	count := callCount
	mu.Unlock()

	// 由于静默机制，实际发送数应小于 5
	if count >= 5 {
		t.Errorf("expected deduplication/silencing to reduce alerts, got %d sent", count)
	}
}

func TestAlerting_WebhookWithAuthHeader(t *testing.T) {
	var mu sync.Mutex
	var receivedAuth string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	mgr := alerting.NewManager()
	webhook := alerting.NewWebhookChannelForTest(mockServer.URL, map[string]string{
		"Authorization": "Bearer test-token-123",
	}, true)
	mgr.RegisterChannel(webhook)

	mgr.SendInfo(
		alerting.AlertTypeSystem,
		"System Check",
		"Auth header test",
		nil,
		"system",
		"",
	)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	auth := receivedAuth
	mu.Unlock()

	if auth != "Bearer test-token-123" {
		t.Errorf("expected Authorization header 'Bearer test-token-123', got %q", auth)
	}
}
