package alerting

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/unimap-icp-hunter/project/internal/logger"
)

// Manager 告警管理器
type Manager struct {
	channels     []AlertChannel
	config       AlertConfig
	alertRecords map[string]*AlertRecord
	mutex        sync.RWMutex
}

// NewManager 创建告警管理器
func NewManager() *Manager {
	return &Manager{
		channels:     make([]AlertChannel, 0),
		alertRecords: make(map[string]*AlertRecord),
		config: AlertConfig{
			Thresholds: []AlertThreshold{
				{Type: "tamper_segments", Value: 1, WindowSize: 300, Enabled: true},
				{Type: "tamper_frequency", Value: 3, WindowSize: 300, Enabled: true},
				{Type: "response_time", Value: 5.0, WindowSize: 300, Enabled: true},
				{Type: "error_rate", Value: 0.1, WindowSize: 300, Enabled: true},
			},
			Silence: SilenceConfig{
				Enabled:     true,
				Duration:    300,
				MinInterval: 60,
				MaxAlerts:   10,
				ByType:      true,
				BySource:    false,
				ByURL:       true,
			},
			Acknowledgment: true,
		},
	}
}

// SetConfig 设置告警配置
func (m *Manager) SetConfig(config AlertConfig) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.config = config
}

// GetConfig 获取告警配置
func (m *Manager) GetConfig() AlertConfig {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.config
}

// RegisterChannel 注册告警渠道
func (m *Manager) RegisterChannel(channel AlertChannel) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if channel.IsEnabled() {
		m.channels = append(m.channels, channel)
		logger.Infof("Registered alert channel: %s", channel.Name())
	}
}

// SendAlert 发送告警到所有启用的渠道
func (m *Manager) SendAlert(level AlertLevel, alertType AlertType, title, message string, details interface{}, source, url string) {
	// 检查是否应该发送告警（基于阈值和静默）
	if !m.shouldSendAlert(alertType, details, source, url) {
		return
	}

	alert := Alert{
		ID:        uuid.New().String(),
		Level:     level,
		Type:      alertType,
		Title:     title,
		Message:   message,
		Details:   details,
		Timestamp: time.Now(),
		Source:    source,
		URL:       url,
	}

	// 创建告警记录
	record := &AlertRecord{
		Alert:        alert,
		Status:       AlertStatusNew,
		LastModified: time.Now(),
	}

	m.mutex.Lock()
	m.alertRecords[alert.ID] = record
	m.mutex.Unlock()

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, channel := range m.channels {
		if !channel.IsEnabled() {
			continue
		}

		go func(ch AlertChannel) {
			if err := ch.Send(alert); err != nil {
				logger.Errorf("Failed to send alert to channel %s: %v", ch.Name(), err)
			}
		}(channel)
	}
}

// SendInfo 发送信息级别告警
func (m *Manager) SendInfo(alertType AlertType, title, message string, details interface{}, source, url string) {
	m.SendAlert(AlertLevelInfo, alertType, title, message, details, source, url)
}

// SendWarning 发送警告级别告警
func (m *Manager) SendWarning(alertType AlertType, title, message string, details interface{}, source, url string) {
	m.SendAlert(AlertLevelWarning, alertType, title, message, details, source, url)
}

// SendError 发送错误级别告警
func (m *Manager) SendError(alertType AlertType, title, message string, details interface{}, source, url string) {
	m.SendAlert(AlertLevelError, alertType, title, message, details, source, url)
}

// SendCritical 发送严重级别告警
func (m *Manager) SendCritical(alertType AlertType, title, message string, details interface{}, source, url string) {
	m.SendAlert(AlertLevelCritical, alertType, title, message, details, source, url)
}

// Close 关闭所有告警渠道
func (m *Manager) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, channel := range m.channels {
		if err := channel.Close(); err != nil {
			logger.Errorf("Failed to close alert channel %s: %v", channel.Name(), err)
		}
	}
}

// shouldSendAlert 检查是否应该发送告警
func (m *Manager) shouldSendAlert(alertType AlertType, details interface{}, source, url string) bool {
	config := m.GetConfig()

	// 检查静默状态
	if config.Silence.Enabled {
		// 检查是否处于静默状态
		if m.isSilenced(alertType, source, url) {
			return false
		}
	}

	// 根据告警类型检查阈值
	switch alertType {
	case AlertTypeTamper:
		// 检查篡改告警阈值
		for _, threshold := range config.Thresholds {
			if !threshold.Enabled {
				continue
			}

			switch threshold.Type {
			case "tamper_segments":
				// 检查修改的分段数量
				if detailsMap, ok := details.(map[string]interface{}); ok {
					if segments, ok := detailsMap["segments"].([]string); ok {
						if len(segments) < int(threshold.Value) {
							return false
						}
					}
				}

			case "tamper_frequency":
				// 检查时间窗口内的告警频率
				if !m.checkTamperFrequency(threshold.Value, threshold.WindowSize) {
					return false
				}
			}
		}

	case AlertTypePerformance:
		// 检查性能告警阈值
		for _, threshold := range config.Thresholds {
			if !threshold.Enabled {
				continue
			}

			switch threshold.Type {
			case "response_time":
				if detailsMap, ok := details.(map[string]interface{}); ok {
					if responseTime, ok := detailsMap["response_time"].(float64); ok {
						if responseTime < threshold.Value {
							return false
						}
					}
				}

			case "error_rate":
				if detailsMap, ok := details.(map[string]interface{}); ok {
					if errorRate, ok := detailsMap["error_rate"].(float64); ok {
						if errorRate < threshold.Value {
							return false
						}
					}
				}
			}
		}
	}

	return true
}

// isSilenced 检查告警是否被静默
func (m *Manager) isSilenced(alertType AlertType, source, url string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	now := time.Now()

	for _, record := range m.alertRecords {
		if record.SilenceUntil != nil && now.Before(*record.SilenceUntil) {
			// 检查静默条件是否匹配
			if record.Alert.Type == alertType {
				if (source == "" || record.Alert.Source == source) &&
					(url == "" || record.Alert.URL == url) {
					return true
				}
			}
		}
	}

	return false
}

// checkTamperFrequency 检查时间窗口内的篡改告警频率
// 如果窗口内的告警数量超过阈值（太频繁），返回 false（应抑制）
func (m *Manager) checkTamperFrequency(maxAlerts float64, windowSize int) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if windowSize <= 0 {
		windowSize = 300
	}

	now := time.Now()
	windowStart := now.Add(-time.Duration(windowSize) * time.Second)
	count := 0

	for _, record := range m.alertRecords {
		if record.Alert.Type == AlertTypeTamper && record.Alert.Timestamp.After(windowStart) {
			count++
		}
	}

	// 如果已经超过了阈值（太频繁），返回 false 以抑制
	return float64(count) < maxAlerts
}

// AcknowledgeAlert 确认告警
func (m *Manager) AcknowledgeAlert(alertID, userID, userName, comment string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	record, exists := m.alertRecords[alertID]
	if !exists {
		return fmt.Errorf("alert not found: %s", alertID)
	}

	record.Status = AlertStatusAcknowledged
	record.Acknowledgment = &AcknowledgmentInfo{
		UserID:    userID,
		UserName:  userName,
		Timestamp: time.Now(),
		Comment:   comment,
	}
	record.LastModified = time.Now()

	return nil
}

// SilenceAlert 静默告警
func (m *Manager) SilenceAlert(alertID string, duration time.Duration) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	record, exists := m.alertRecords[alertID]
	if !exists {
		return fmt.Errorf("alert not found: %s", alertID)
	}

	silenceUntil := time.Now().Add(duration)
	record.Status = AlertStatusSilenced
	record.SilenceUntil = &silenceUntil
	record.LastModified = time.Now()

	return nil
}

// SilenceAlertsByType 按类型静默告警
func (m *Manager) SilenceAlertsByType(alertType AlertType, duration time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	silenceUntil := time.Now().Add(duration)

	for _, record := range m.alertRecords {
		if record.Alert.Type == alertType {
			record.Status = AlertStatusSilenced
			record.SilenceUntil = &silenceUntil
			record.LastModified = time.Now()
		}
	}
}

// ResolveAlert 解决告警
func (m *Manager) ResolveAlert(alertID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	record, exists := m.alertRecords[alertID]
	if !exists {
		return fmt.Errorf("alert not found: %s", alertID)
	}

	record.Status = AlertStatusResolved
	record.LastModified = time.Now()

	return nil
}

// GetAlertRecords 获取告警记录
func (m *Manager) GetAlertRecords(status ...AlertStatus) []*AlertRecord {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var records []*AlertRecord
	statusMap := make(map[AlertStatus]bool)

	for _, s := range status {
		statusMap[s] = true
	}

	for _, record := range m.alertRecords {
		if len(status) == 0 || statusMap[record.Status] {
			records = append(records, record)
		}
	}

	return records
}

// CleanupOldRecords 清理旧记录
func (m *Manager) CleanupOldRecords(maxAge time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	cutoff := time.Now().Add(-maxAge)

	for id, record := range m.alertRecords {
		if record.LastModified.Before(cutoff) {
			delete(m.alertRecords, id)
		}
	}
}
