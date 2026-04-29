package alerting

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLogChannel_Name(t *testing.T) {
	ch := NewLogChannel(true)
	if ch.Name() != "log" {
		t.Fatalf("expected 'log', got %s", ch.Name())
	}
}

func TestLogChannel_IsEnabled(t *testing.T) {
	ch := NewLogChannel(true)
	if !ch.IsEnabled() {
		t.Fatal("expected enabled")
	}
	ch2 := NewLogChannel(false)
	if ch2.IsEnabled() {
		t.Fatal("expected disabled")
	}
}

func TestLogChannel_Send(t *testing.T) {
	ch := NewLogChannel(true)
	alert := Alert{
		ID:        "1",
		Level:     AlertLevelInfo,
		Type:      AlertTypeTamper,
		Title:     "test",
		Message:   "test message",
		Timestamp: time.Now(),
	}
	if err := ch.Send(alert); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLogChannel_Close(t *testing.T) {
	ch := NewLogChannel(true)
	if err := ch.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebhookChannel_Name(t *testing.T) {
	ch := NewWebhookChannel("http://example.com", nil, true)
	if ch.Name() != "webhook" {
		t.Fatalf("expected 'webhook', got %s", ch.Name())
	}
}

func TestWebhookChannel_IsEnabled(t *testing.T) {
	ch := NewWebhookChannel("http://example.com", nil, true)
	if !ch.IsEnabled() {
		t.Fatal("expected enabled")
	}
	ch2 := NewWebhookChannel("http://example.com", nil, false)
	if ch2.IsEnabled() {
		t.Fatal("expected disabled")
	}
}

func TestWebhookChannel_Send_Disabled(t *testing.T) {
	ch := NewWebhookChannel("http://example.com", nil, false)
	alert := Alert{ID: "1", Level: AlertLevelInfo, Type: AlertTypeTamper, Title: "test", Message: "msg", Timestamp: time.Now()}
	if err := ch.Send(alert); err != nil {
		t.Fatalf("unexpected error when disabled: %v", err)
	}
}

func TestWebhookChannel_Send_EmptyURL(t *testing.T) {
	ch := NewWebhookChannel("", nil, true)
	alert := Alert{ID: "1", Level: AlertLevelInfo, Type: AlertTypeTamper, Title: "test", Message: "msg", Timestamp: time.Now()}
	if err := ch.Send(alert); err != nil {
		t.Fatalf("unexpected error when empty URL: %v", err)
	}
}

func TestWebhookChannel_Send_Success(t *testing.T) {
	var received Alert
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewWebhookChannelForTest(srv.URL, nil, true)
	alert := Alert{
		ID:        "test-1",
		Level:     AlertLevelWarning,
		Type:      AlertTypeTamper,
		Title:     "Tamper Detected",
		Message:   "Page changed",
		Timestamp: time.Now(),
		Source:    "test",
		URL:       "http://example.com",
	}
	if err := ch.Send(alert); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received.ID != "test-1" {
		t.Fatalf("expected ID test-1, got %s", received.ID)
	}
	if received.Level != AlertLevelWarning {
		t.Fatalf("expected level warning, got %s", received.Level)
	}
}

func TestWebhookChannel_Send_WithHeaders(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := NewWebhookChannelForTest(srv.URL, map[string]string{"Authorization": "Bearer secret"}, true)
	alert := Alert{ID: "1", Level: AlertLevelInfo, Type: AlertTypeSystem, Title: "t", Message: "m", Timestamp: time.Now()}
	if err := ch.Send(alert); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("expected Bearer secret, got %s", gotAuth)
	}
}

func TestWebhookChannel_Send_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ch := NewWebhookChannelForTest(srv.URL, nil, true)
	alert := Alert{ID: "1", Level: AlertLevelInfo, Type: AlertTypeSystem, Title: "t", Message: "m", Timestamp: time.Now()}
	err := ch.Send(alert)
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
}

func TestWebhookChannel_Close(t *testing.T) {
	ch := NewWebhookChannel("http://example.com", nil, true)
	if err := ch.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
