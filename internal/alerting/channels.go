package alerting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/utils"
)

// LogChannel 日志告警渠道
type LogChannel struct {
	enabled bool
}

// NewLogChannel 创建日志告警渠道
func NewLogChannel(enabled bool) *LogChannel {
	return &LogChannel{
		enabled: enabled,
	}
}

// Name 返回渠道名称
func (c *LogChannel) Name() string {
	return "log"
}

// Send 发送告警到日志
func (c *LogChannel) Send(alert Alert) error {
	switch alert.Level {
	case AlertLevelInfo:
		logger.Infof("[ALERT] %s: %s - %s", alert.Type, alert.Title, alert.Message)
	case AlertLevelWarning:
		logger.Warnf("[ALERT] %s: %s - %s", alert.Type, alert.Title, alert.Message)
	case AlertLevelError:
		logger.Errorf("[ALERT] %s: %s - %s", alert.Type, alert.Title, alert.Message)
	case AlertLevelCritical:
		logger.Errorf("[ALERT] %s: %s - %s", alert.Type, alert.Title, alert.Message)
	}
	return nil
}

// IsEnabled 检查渠道是否启用
func (c *LogChannel) IsEnabled() bool {
	return c.enabled
}

// Close 关闭渠道
func (c *LogChannel) Close() error {
	return nil
}

// WebhookChannel Webhook告警渠道
type WebhookChannel struct {
	enabled bool
	url     string
	headers map[string]string
	client  *http.Client
}

// NewWebhookChannel 创建Webhook告警渠道
func NewWebhookChannel(url string, headers map[string]string, enabled bool) *WebhookChannel {
	return &WebhookChannel{
		enabled: enabled,
		url:     url,
		headers: headers,
		client:  utils.DefaultHTTPClient(),
	}
}

// Name 返回渠道名称
func (c *WebhookChannel) Name() string {
	return "webhook"
}

// Send 发送告警到Webhook
func (c *WebhookChannel) Send(alert Alert) error {
	if !c.enabled || c.url == "" {
		return nil
	}

	data, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	req, err := http.NewRequest("POST", c.url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("webhook returned status code: %d", resp.StatusCode)
	}

	return nil
}

// IsEnabled 检查渠道是否启用
func (c *WebhookChannel) IsEnabled() bool {
	return c.enabled
}

// Close 关闭渠道
func (c *WebhookChannel) Close() error {
	return nil
}
