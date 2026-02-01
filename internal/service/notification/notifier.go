package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/smtp"
	"time"

	"github.com/unimap-icp-hunter/project/internal/model"
	"go.uber.org/zap"
)

// Notifier 通知服务
type Notifier struct {
	config *NotifyConfig
	logger *zap.Logger
}

// NotifyConfig 通知配置
type NotifyConfig struct {
	Email   EmailConfig   `yaml:"email"`
	Webhook WebhookConfig `yaml:"webhook"`
	Report  ReportConfig  `yaml:"report"`
}

// EmailConfig 邮件配置
type EmailConfig struct {
	Enabled     bool     `yaml:"enabled"`
	SMTPHost    string   `yaml:"smtp_host"`
	SMTPPort    int      `yaml:"smtp_port"`
	Username    string   `yaml:"username"`
	Password    string   `yaml:"password"`
	From        string   `yaml:"from"`
	To          []string `yaml:"to"`
	DailyReport bool     `yaml:"daily_report"`
	ReportTime  string   `yaml:"report_time"`
}

// WebhookConfig Webhook配置
type WebhookConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
	Secret  string `yaml:"secret"`
	Type    string `yaml:"type"` // dingtalk, wechat, lark
}

// ReportConfig 报表配置
type ReportConfig struct {
	DailySummary bool     `yaml:"daily_summary"`
	IncludeStats []string `yaml:"include_stats"`
}

// NewNotifier 创建通知服务
func NewNotifier(config *NotifyConfig, logger *zap.Logger) *Notifier {
	return &Notifier{
		config: config,
		logger: logger,
	}
}

// NotifyUnregistered 发现未备案站点通知
func (n *Notifier) NotifyUnregistered(check *model.ICPCheck, asset *model.Asset) error {
	if !n.config.Email.Enabled && !n.config.Webhook.Enabled {
		return nil
	}

	title := "【告警】发现疑似未备案网站"
	content := fmt.Sprintf(`
发现时间: %s
URL: %s
IP: %s:%d
标题: %s
状态: 疑似未备案

请及时处理。
`, time.Now().Format("2006-01-02 15:04:05"),
		check.URL, asset.IP, asset.Port, check.Title)

	// 邮件通知
	if n.config.Email.Enabled {
		if err := n.sendEmail(title, content); err != nil {
			n.logger.Error("Failed to send email notification", zap.Error(err))
		}
	}

	// Webhook通知
	if n.config.Webhook.Enabled {
		if err := n.sendWebhook(check, asset); err != nil {
			n.logger.Error("Failed to send webhook notification", zap.Error(err))
		}
	}

	return nil
}

// SendDailyReport 发送日报
func (n *Notifier) SendDailyReport(stats *DailyStats) error {
	if !n.config.Email.Enabled || !n.config.Email.DailyReport {
		return nil
	}

	subject := fmt.Sprintf("【日报】ICP-Hunter 每日扫描报告 - %s", time.Now().Format("2006-01-02"))

	// 生成HTML内容
	html, err := n.generateReportHTML(stats)
	if err != nil {
		return err
	}

	return n.sendEmailWithHTML(subject, html)
}

// sendEmail 发送纯文本邮件
func (n *Notifier) sendEmail(subject, body string) error {
	if !n.config.Email.Enabled {
		return nil
	}

	auth := smtp.PlainAuth("", n.config.Email.Username, n.config.Email.Password, n.config.Email.SMTPHost)

	to := n.config.Email.To
	msg := []byte(fmt.Sprintf(
		"To: %s\r\n"+
			"From: %s\r\n"+
			"Subject: %s\r\n"+
			"Content-Type: text/plain; charset=UTF-8\r\n"+
			"\r\n%s",
		to, n.config.Email.From, subject, body,
	))

	addr := fmt.Sprintf("%s:%d", n.config.Email.SMTPHost, n.config.Email.SMTPPort)
	return smtp.SendMail(addr, auth, n.config.Email.From, to, msg)
}

// sendEmailWithHTML 发送HTML邮件
func (n *Notifier) sendEmailWithHTML(subject, html string) error {
	if !n.config.Email.Enabled {
		return nil
	}

	auth := smtp.PlainAuth("", n.config.Email.Username, n.config.Email.Password, n.config.Email.SMTPHost)

	to := n.config.Email.To
	msg := []byte(fmt.Sprintf(
		"To: %s\r\n"+
			"From: %s\r\n"+
			"Subject: %s\r\n"+
			"Content-Type: text/html; charset=UTF-8\r\n"+
			"\r\n%s",
		to, n.config.Email.From, subject, html,
	))

	addr := fmt.Sprintf("%s:%d", n.config.Email.SMTPHost, n.config.Email.SMTPPort)
	return smtp.SendMail(addr, auth, n.config.Email.From, to, msg)
}

// sendWebhook 发送Webhook通知
func (n *Notifier) sendWebhook(check *model.ICPCheck, asset *model.Asset) error {
	if !n.config.Webhook.Enabled {
		return nil
	}

	var payload interface{}

	switch n.config.Webhook.Type {
	case "dingtalk":
		payload = n.generateDingTalkPayload(check, asset)
	case "wechat":
		payload = n.generateWeChatPayload(check, asset)
	case "lark":
		payload = n.generateLarkPayload(check, asset)
	default:
		return fmt.Errorf("unsupported webhook type: %s", n.config.Webhook.Type)
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// 发送HTTP请求
	// 这里简化实现，实际应使用HTTP客户端
	n.logger.Info("Sending webhook", zap.String("url", n.config.Webhook.URL), zap.String("payload", string(jsonData)))

	return nil
}

// generateDingTalkPayload 生成钉钉消息
func (n *Notifier) generateDingTalkPayload(check *model.ICPCheck, asset *model.Asset) map[string]interface{} {
	return map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]interface{}{
			"title": "未备案网站告警",
			"text": fmt.Sprintf(`## 未备案网站告警

**发现时间**: %s
**URL**: %s
**IP**: %s:%d
**标题**: %s
**状态**: 疑似未备案

请及时处理。`,
				time.Now().Format("2006-01-02 15:04:05"),
				check.URL, asset.IP, asset.Port, check.Title),
		},
	}
}

// generateWeChatPayload 生成企业微信消息
func (n *Notifier) generateWeChatPayload(check *model.ICPCheck, asset *model.Asset) map[string]interface{} {
	return map[string]interface{}{
		"msgtype": "text",
		"text": map[string]interface{}{
			"content": fmt.Sprintf(`未备案网站告警
发现时间: %s
URL: %s
IP: %s:%d
标题: %s`,
				time.Now().Format("2006-01-02 15:04:05"),
				check.URL, asset.IP, asset.Port, check.Title),
		},
	}
}

// generateLarkPayload 生成飞书消息
func (n *Notifier) generateLarkPayload(check *model.ICPCheck, asset *model.Asset) map[string]interface{} {
	return map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"config": map[string]interface{}{
				"wide_screen_mode": true,
			},
			"elements": []map[string]interface{}{
				{
					"tag": "div",
					"text": map[string]interface{}{
						"tag": "markdown",
						"content": fmt.Sprintf(`**未备案网站告警**
发现时间: %s
URL: %s
IP: %s:%d
标题: %s`,
							time.Now().Format("2006-01-02 15:04:05"),
							check.URL, asset.IP, asset.Port, check.Title),
					},
				},
			},
		},
	}
}

// generateReportHTML 生成日报HTML
func (n *Notifier) generateReportHTML(stats *DailyStats) (string, error) {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>ICP-Hunter 每日报告</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .header { background: #007bff; color: white; padding: 20px; border-radius: 5px; }
        .stats { margin: 20px 0; }
        .stat-item { background: #f8f9fa; padding: 15px; margin: 10px 0; border-left: 4px solid #007bff; }
        .details { margin-top: 20px; }
        table { width: 100%; border-collapse: collapse; margin-top: 10px; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background: #007bff; color: white; }
        .alert { color: #dc3545; font-weight: bold; }
    </style>
</head>
<body>
    <div class="header">
        <h1>ICP-Hunter 每日扫描报告</h1>
        <p>扫描日期: {{.Date}}</p>
    </div>

    <div class="stats">
        <div class="stat-item">
            <strong>新增疑似未备案站点:</strong> <span class="alert">{{.NewUnregistered}}</span>
        </div>
        <div class="stat-item">
            <strong>总疑似未备案站点:</strong> {{.TotalUnregistered}}
        </div>
        <div class="stat-item">
            <strong>已备案站点:</strong> {{.TotalRegistered}}
        </div>
        <div class="stat-item">
            <strong>扫描总数:</strong> {{.TotalScanned}}
        </div>
    </div>

    {{if .ProvinceStats}}
    <div class="details">
        <h3>按省份分布</h3>
        <table>
            <tr><th>省份</th><th>未备案数</th><th>总数</th></tr>
            {{range .ProvinceStats}}
            <tr>
                <td>{{.Name}}</td>
                <td class="alert">{{.Unregistered}}</td>
                <td>{{.Total}}</td>
            </tr>
            {{end}}
        </table>
    </div>
    {{end}}

    {{if .PortStats}}
    <div class="details">
        <h3>按端口分布</h3>
        <table>
            <tr><th>端口</th><th>未备案数</th><th>总数</th></tr>
            {{range .PortStats}}
            <tr>
                <td>{{.Port}}</td>
                <td class="alert">{{.Unregistered}}</td>
                <td>{{.Total}}</td>
            </tr>
            {{end}}
        </table>
    </div>
    {{end}}

    {{if .TopServers}}
    <div class="details">
        <h3>Top 10 Web服务器</h3>
        <table>
            <tr><th>服务器类型</th><th>数量</th></tr>
            {{range .TopServers}}
            <tr>
                <td>{{.Name}}</td>
                <td>{{.Count}}</td>
            </tr>
            {{end}}
        </table>
    </div>
    {{end}}

    <div style="margin-top: 30px; padding: 15px; background: #f8f9fa; border-radius: 5px;">
        <p style="font-size: 12px; color: #666;">
            此报告由 ICP-Hunter 自动生成<br>
            生成时间: {{.GenerateTime}}
        </p>
    </div>
</body>
</html>`

	t, err := template.New("report").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, stats); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// DailyStats 日报统计
type DailyStats struct {
	Date              string
	NewUnregistered   int
	TotalUnregistered int
	TotalRegistered   int
	TotalScanned      int
	ProvinceStats     []ProvinceStat
	PortStats         []PortStat
	TopServers        []ServerStat
	GenerateTime      string
}

// ProvinceStat 省份统计
type ProvinceStat struct {
	Name         string
	Unregistered int
	Total        int
}

// PortStat 端口统计
type PortStat struct {
	Port         int
	Unregistered int
	Total        int
}

// ServerStat 服务器统计
type ServerStat struct {
	Name  string
	Count int
}

// NotifyRecovery 通知恢复（从未备案变为已备案）
func (n *Notifier) NotifyRecovery(check *model.ICPCheck, asset *model.Asset) error {
	if !n.config.Email.Enabled && !n.config.Webhook.Enabled {
		return nil
	}

	title := "【恢复】网站已备案"
	content := fmt.Sprintf(`
恢复时间: %s
URL: %s
IP: %s:%d
标题: %s
备案号: %s

该网站已检测到备案信息。
`, time.Now().Format("2006-01-02 15:04:05"),
		check.URL, asset.IP, asset.Port, check.Title, check.ICPCode)

	return n.sendEmail(title, content)
}

// NotifyThreshold 超过阈值通知
func (n *Notifier) NotifyThreshold(count int) error {
	if !n.config.Email.Enabled {
		return nil
	}

	title := "【警告】未备案网站数量超过阈值"
	content := fmt.Sprintf(`
时间: %s
新增未备案网站数量: %d

请立即检查系统。
`, time.Now().Format("2006-01-02 15:04:05"), count)

	return n.sendEmail(title, content)
}
