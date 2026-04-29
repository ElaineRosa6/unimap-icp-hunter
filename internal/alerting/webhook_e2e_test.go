package alerting

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestManager_SendWarning_TamperWebhookE2E(t *testing.T) {
	receivedCh := make(chan Alert, 1)
	validationErrCh := make(chan string, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			validationErrCh <- "expected POST"
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			validationErrCh <- "expected application/json content-type"
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var alert Alert
		if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
			validationErrCh <- "failed to decode webhook payload"
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		receivedCh <- alert
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	manager := NewManager()
	manager.RegisterChannel(NewWebhookChannelForTest(srv.URL, map[string]string{
		"X-Test-Channel": "tamper-e2e",
	}, true))

	details := map[string]interface{}{
		"segments": []string{"title"},
		"reason":   "content_changed",
	}

	manager.SendWarning(
		AlertTypeTamper,
		"Tamper Detected",
		"Page content changed",
		details,
		"tamper-detector",
		"https://example.com",
	)

	select {
	case errMsg := <-validationErrCh:
		t.Fatal(errMsg)
	case alert := <-receivedCh:
		t.Logf("received webhook payload: type=%s level=%s title=%s source=%s", alert.Type, alert.Level, alert.Title, alert.Source)
		if alert.Type != AlertTypeTamper {
			t.Fatalf("expected tamper type, got %s", alert.Type)
		}
		if alert.Level != AlertLevelWarning {
			t.Fatalf("expected warning level, got %s", alert.Level)
		}
		if alert.Source != "tamper-detector" {
			t.Fatalf("expected source tamper-detector, got %s", alert.Source)
		}
		if alert.URL != "https://example.com" {
			t.Fatalf("expected URL https://example.com, got %s", alert.URL)
		}
		if alert.Title != "Tamper Detected" {
			t.Fatalf("expected title Tamper Detected, got %s", alert.Title)
		}
		if alert.Timestamp.IsZero() {
			t.Fatal("expected non-zero timestamp")
		}
		detailsMap, ok := alert.Details.(map[string]interface{})
		if !ok {
			t.Fatal("expected details map in payload")
		}
		if detailsMap["reason"] != "content_changed" {
			t.Fatalf("expected reason content_changed, got %v", detailsMap["reason"])
		}
		segments, ok := detailsMap["segments"].([]interface{})
		if !ok || len(segments) == 0 || segments[0] != "title" {
			t.Fatalf("expected segments to contain title, got %v", detailsMap["segments"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook payload")
	}
}
