package alerting

import (
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.channels == nil {
		t.Fatal("expected non-nil channels")
	}
	if m.alertRecords == nil {
		t.Fatal("expected non-nil alertRecords")
	}
}

func TestManager_RegisterChannel(t *testing.T) {
	m := NewManager()
	ch := NewLogChannel(true)
	m.RegisterChannel(ch)

	if len(m.channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(m.channels))
	}
}

func TestManager_RegisterChannel_Disabled(t *testing.T) {
	m := NewManager()
	ch := NewLogChannel(false)
	m.RegisterChannel(ch)

	if len(m.channels) != 0 {
		t.Fatalf("expected 0 channels for disabled channel, got %d", len(m.channels))
	}
}

func TestManager_SendAlert(t *testing.T) {
	m := NewManager()
	// 使用 mock channel 来验证发送
	sentCh := make(chan Alert, 1)
	mockCh := &mockChannel{sendFunc: func(a Alert) error {
		sentCh <- a
		return nil
	}}
	m.RegisterChannel(mockCh)

	m.SendAlert(AlertLevelInfo, AlertTypeSystem, "title", "msg", nil, "src", "url")

	// 等待 goroutine 发送
	select {
	case alert := <-sentCh:
		if alert.Title != "title" {
			t.Fatalf("expected title 'title', got %s", alert.Title)
		}
		if alert.Message != "msg" {
			t.Fatalf("expected message 'msg', got %s", alert.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for alert")
	}
}

func TestManager_SendInfo(t *testing.T) {
	m := NewManager()
	recordsBefore := len(m.GetAlertRecords())
	m.SendInfo(AlertTypeSystem, "t", "m", nil, "s", "u")
	records := m.GetAlertRecords()
	if len(records) != recordsBefore+1 {
		t.Fatalf("expected %d records, got %d", recordsBefore+1, len(records))
	}
}

func TestManager_SendWarning(t *testing.T) {
	m := NewManager()
	m.SendWarning(AlertTypeTamper, "t", "m", nil, "s", "u")
	records := m.GetAlertRecords(AlertStatusNew)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Alert.Level != AlertLevelWarning {
		t.Fatalf("expected warning level, got %s", records[0].Alert.Level)
	}
}

func TestManager_SendError(t *testing.T) {
	m := NewManager()
	m.SendError(AlertTypeSecurity, "t", "m", nil, "s", "u")
	records := m.GetAlertRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Alert.Level != AlertLevelError {
		t.Fatalf("expected error level, got %s", records[0].Alert.Level)
	}
}

func TestManager_SendCritical(t *testing.T) {
	m := NewManager()
	m.SendCritical(AlertTypeTamper, "t", "m", nil, "s", "u")
	records := m.GetAlertRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Alert.Level != AlertLevelCritical {
		t.Fatalf("expected critical level, got %s", records[0].Alert.Level)
	}
}

func TestManager_AcknowledgeAlert(t *testing.T) {
	m := NewManager()
	m.SendInfo(AlertTypeSystem, "t", "m", nil, "s", "u")
	records := m.GetAlertRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	alertID := records[0].Alert.ID
	err := m.AcknowledgeAlert(alertID, "user1", "Test User", "ack comment")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records = m.GetAlertRecords()
	if records[0].Status != AlertStatusAcknowledged {
		t.Fatalf("expected acknowledged status, got %s", records[0].Status)
	}
	if records[0].Acknowledgment == nil {
		t.Fatal("expected acknowledgment info")
	}
	if records[0].Acknowledgment.UserID != "user1" {
		t.Fatalf("expected user1, got %s", records[0].Acknowledgment.UserID)
	}
}

func TestManager_AcknowledgeAlert_NotFound(t *testing.T) {
	m := NewManager()
	err := m.AcknowledgeAlert("nonexistent", "user1", "user", "comment")
	if err == nil {
		t.Fatal("expected error for nonexistent alert")
	}
}

func TestManager_SilenceAlert(t *testing.T) {
	m := NewManager()
	m.SendInfo(AlertTypeSystem, "t", "m", nil, "s", "u")
	records := m.GetAlertRecords()
	alertID := records[0].Alert.ID

	err := m.SilenceAlert(alertID, 5*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records = m.GetAlertRecords()
	if records[0].Status != AlertStatusSilenced {
		t.Fatalf("expected silenced status, got %s", records[0].Status)
	}
	if records[0].SilenceUntil == nil {
		t.Fatal("expected silence until")
	}
}

func TestManager_SilenceAlertsByType(t *testing.T) {
	m := NewManager()
	m.SendInfo(AlertTypeTamper, "t1", "m1", nil, "s", "u1")
	m.SendInfo(AlertTypeTamper, "t2", "m2", nil, "s", "u2")
	m.SendInfo(AlertTypeSystem, "t3", "m3", nil, "s", "u3")

	m.SilenceAlertsByType(AlertTypeTamper, 5*time.Minute)

	records := m.GetAlertRecords()
	silencedCount := 0
	for _, r := range records {
		if r.Status == AlertStatusSilenced {
			silencedCount++
		}
	}
	if silencedCount != 2 {
		t.Fatalf("expected 2 silenced, got %d", silencedCount)
	}
}

func TestManager_ResolveAlert(t *testing.T) {
	m := NewManager()
	m.SendInfo(AlertTypeSystem, "t", "m", nil, "s", "u")
	records := m.GetAlertRecords()
	alertID := records[0].Alert.ID

	err := m.ResolveAlert(alertID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records = m.GetAlertRecords()
	if records[0].Status != AlertStatusResolved {
		t.Fatalf("expected resolved status, got %s", records[0].Status)
	}
}

func TestManager_GetAlertRecords_ByStatus(t *testing.T) {
	m := NewManager()
	m.SendInfo(AlertTypeSystem, "t1", "m1", nil, "s", "u")
	m.SendWarning(AlertTypeTamper, "t2", "m2", nil, "s", "u")

	newRecords := m.GetAlertRecords(AlertStatusNew)
	if len(newRecords) != 2 {
		t.Fatalf("expected 2 new records, got %d", len(newRecords))
	}
}

func TestManager_CleanupOldRecords(t *testing.T) {
	m := NewManager()
	m.SendInfo(AlertTypeSystem, "t", "m", nil, "s", "u")

	// 清理所有超过 1 秒的记录
	m.CleanupOldRecords(time.Second)

	records := m.GetAlertRecords()
	// 刚创建的记录不应被清理
	if len(records) != 1 {
		t.Fatalf("expected 1 record after cleanup, got %d", len(records))
	}
}

func TestManager_GetConfig(t *testing.T) {
	m := NewManager()
	cfg := m.GetConfig()
	if len(cfg.Thresholds) == 0 {
		t.Fatal("expected default thresholds")
	}
}

func TestManager_SetConfig(t *testing.T) {
	m := NewManager()
	newCfg := AlertConfig{
		Thresholds: []AlertThreshold{{Type: "test", Value: 10, WindowSize: 60, Enabled: true}},
	}
	m.SetConfig(newCfg)

	cfg := m.GetConfig()
	if len(cfg.Thresholds) != 1 {
		t.Fatalf("expected 1 threshold, got %d", len(cfg.Thresholds))
	}
}

func TestManager_Close(t *testing.T) {
	m := NewManager()
	ch := NewLogChannel(true)
	m.RegisterChannel(ch)

	// 不应 panic
	m.Close()
}

func TestManager_TamperFrequencyCheck(t *testing.T) {
	t.Run("allows first alert", func(t *testing.T) {
		m := NewManager()
		// With default threshold of 3, first alert should be allowed
		m.SendWarning(AlertTypeTamper, "t1", "m1", nil, "s1", "u1")
		records := m.GetAlertRecords()
		if len(records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(records))
		}
	})

	t.Run("suppresses after threshold exceeded", func(t *testing.T) {
		m := NewManager()
		// Set frequency threshold to 2 (suppress after 2 alerts in window)
		m.SetConfig(AlertConfig{
			Thresholds: []AlertThreshold{
				{Type: "tamper_frequency", Value: 2, WindowSize: 300, Enabled: true},
			},
		})

		// First two alerts should go through
		m.SendWarning(AlertTypeTamper, "t1", "m1", nil, "s1", "u1")
		m.SendWarning(AlertTypeTamper, "t2", "m2", nil, "s2", "u2")

		records := m.GetAlertRecords()
		if len(records) != 2 {
			t.Fatalf("expected 2 records after 2 alerts, got %d", len(records))
		}

		// Third alert should be suppressed (2 >= 2 threshold)
		m.SendWarning(AlertTypeTamper, "t3", "m3", nil, "s3", "u3")
		records = m.GetAlertRecords()
		if len(records) != 2 {
			t.Fatalf("expected 2 records after suppressed 3rd alert, got %d", len(records))
		}
	})

	t.Run("does not affect different alert types", func(t *testing.T) {
		m := NewManager()
		m.SetConfig(AlertConfig{
			Thresholds: []AlertThreshold{
				{Type: "tamper_frequency", Value: 1, WindowSize: 300, Enabled: true},
			},
		})

		// One tamper alert
		m.SendWarning(AlertTypeTamper, "t1", "m1", nil, "s1", "u1")
		records := m.GetAlertRecords()
		if len(records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(records))
		}

		// Second tamper alert should be suppressed
		m.SendWarning(AlertTypeTamper, "t2", "m2", nil, "s2", "u2")
		records = m.GetAlertRecords()
		if len(records) != 1 {
			t.Fatalf("expected 1 record (second suppressed), got %d", len(records))
		}

		// System alert should still go through (not tamper type)
		m.SendInfo(AlertTypeSystem, "t3", "m3", nil, "s3", "u3")
		records = m.GetAlertRecords()
		if len(records) != 2 {
			t.Fatalf("expected 2 records (system alert went through), got %d", len(records))
		}
	})

	t.Run("disabled frequency check allows all", func(t *testing.T) {
		m := NewManager()
		m.SetConfig(AlertConfig{
			Thresholds: []AlertThreshold{
				{Type: "tamper_frequency", Value: 1, WindowSize: 300, Enabled: false},
			},
		})

		// All alerts should go through when check is disabled
		m.SendWarning(AlertTypeTamper, "t1", "m1", nil, "s1", "u1")
		m.SendWarning(AlertTypeTamper, "t2", "m2", nil, "s2", "u2")
		m.SendWarning(AlertTypeTamper, "t3", "m3", nil, "s3", "u3")

		records := m.GetAlertRecords()
		if len(records) != 3 {
			t.Fatalf("expected 3 records, got %d", len(records))
		}
	})
}

// mockChannel is a simple mock AlertChannel for testing
type mockChannel struct {
	name     string
	enabled  bool
	sendFunc func(Alert) error
}

func (c *mockChannel) Name() string {
	if c.name == "" {
		return "mock"
	}
	return c.name
}

func (c *mockChannel) Send(alert Alert) error {
	if c.sendFunc != nil {
		return c.sendFunc(alert)
	}
	return nil
}

func (c *mockChannel) IsEnabled() bool {
	if c.name == "" {
		return true
	}
	return c.enabled
}

func (c *mockChannel) Close() error {
	return nil
}
