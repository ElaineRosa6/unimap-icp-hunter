package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/notify"
)

// TaskType identifies the type of scheduled task.
type TaskType string

const (
	TaskQuery             TaskType = "query"              // ST-01: UQL 查询
	TaskSearchScreenshot  TaskType = "search_screenshot"  // ST-02: 搜索引擎截图
	TaskBatchScreenshot   TaskType = "batch_screenshot"   // ST-03: 批量截图
	TaskTamperCheck       TaskType = "tamper_check"       // ST-04: 篡改检测
	TaskURLReachability   TaskType = "url_reachability"   // ST-05: URL 可达性检测
	TaskCookieVerify      TaskType = "cookie_verify"      // ST-06: Cookie 验证
	TaskLoginStatusCheck  TaskType = "login_status_check" // ST-07: 登录状态检测
	TaskDistributedSubmit TaskType = "distributed_submit" // ST-08: 分布式任务提交

	// ST-09 ~ ST-16: 中优先级 Runner
	TaskExport            TaskType = "export"             // ST-09: 数据导出
	TaskPortScan          TaskType = "port_scan"          // ST-10: 端口扫描
	TaskScreenshotCleanup TaskType = "screenshot_cleanup" // ST-11: 截图清理
	TaskTamperCleanup     TaskType = "tamper_cleanup"     // ST-12: 篡改记录清理
	TaskQuotaMonitor      TaskType = "quota_monitor"      // ST-13: 配额监控
	TaskAlertSummary      TaskType = "alert_summary"      // ST-14: 告警汇总
	TaskBaselineRefresh   TaskType = "baseline_refresh"   // ST-15: 基线刷新
	TaskURLImport         TaskType = "url_import"         // ST-16: URL 导入

	// ST-17 ~ ST-20: 低优先级 Runner
	TaskPluginHealth      TaskType = "plugin_health"       // ST-17: 插件健康检查
	TaskBridgeHealthCheck TaskType = "bridge_token"        // ST-18: Bridge 健康检查（保留旧持久化键）
	TaskBridgeTokenRotate TaskType = TaskBridgeHealthCheck // Deprecated: 该任务始终执行健康检查，不轮换令牌。
	TaskAlertSilence      TaskType = "alert_silence"       // ST-19: 告警静默窗口
	TaskCacheWarmup       TaskType = "cache_warmup"        // ST-20: 缓存预热
	TaskICPQuery          TaskType = "icp_query"           // ST-21: ICP 备案查询
	TaskICPImport         TaskType = "icp_import"          // ST-22: ICP 关键词 CSV 导入
	TaskBackup            TaskType = "backup"              // ST-23: 配置与数据备份
)

// AllTaskTypes returns all supported task types.
func AllTaskTypes() []TaskType {
	return []TaskType{
		TaskQuery, TaskSearchScreenshot, TaskBatchScreenshot, TaskTamperCheck,
		TaskURLReachability, TaskCookieVerify, TaskLoginStatusCheck, TaskDistributedSubmit,
		TaskExport, TaskPortScan, TaskScreenshotCleanup, TaskTamperCleanup,
		TaskQuotaMonitor, TaskAlertSummary, TaskBaselineRefresh, TaskURLImport,
		TaskPluginHealth, TaskBridgeHealthCheck, TaskAlertSilence, TaskCacheWarmup,
		TaskICPQuery, TaskICPImport, TaskBackup,
	}
}

// TaskTypeLabel returns a human-readable label for a task type.
func TaskTypeLabel(t TaskType) string {
	labels := map[TaskType]string{
		TaskQuery:             "UQL 查询",
		TaskSearchScreenshot:  "搜索引擎截图",
		TaskBatchScreenshot:   "批量截图",
		TaskTamperCheck:       "篡改检测",
		TaskURLReachability:   "URL 可达性检测",
		TaskCookieVerify:      "Cookie 验证",
		TaskLoginStatusCheck:  "登录状态检测",
		TaskDistributedSubmit: "分布式任务提交",
		TaskExport:            "数据导出",
		TaskPortScan:          "端口扫描",
		TaskScreenshotCleanup: "截图清理",
		TaskTamperCleanup:     "篡改记录清理",
		TaskQuotaMonitor:      "配额监控",
		TaskAlertSummary:      "告警汇总",
		TaskBaselineRefresh:   "基线刷新",
		TaskURLImport:         "URL 导入",
		TaskPluginHealth:      "插件健康检查",
		TaskBridgeHealthCheck: "Bridge 健康检查",
		TaskAlertSilence:      "告警静默窗口",
		TaskCacheWarmup:       "缓存预热",
		TaskICPQuery:          "ICP 备案查询",
		TaskICPImport:         "ICP 关键词导入",
		TaskBackup:            "数据备份",
	}
	if l, ok := labels[t]; ok {
		return l
	}
	return string(t)
}

// TaskGroupInfo describes a UI grouping of task types for the scheduler form.
type TaskGroupInfo struct {
	Name  string     // 分组名称
	Icon  string     // 分组图标 emoji
	Types []TaskType // 该分组下的任务类型
}

// GroupedTaskTypes returns task types organized into ordered UI groups.
func GroupedTaskTypes() []TaskGroupInfo {
	return []TaskGroupInfo{
		{Name: "查询与采集", Icon: "📊", Types: []TaskType{
			TaskQuery, TaskSearchScreenshot, TaskBatchScreenshot, TaskExport, TaskICPQuery,
		}},
		{Name: "监控与检测", Icon: "🔍", Types: []TaskType{
			TaskTamperCheck, TaskURLReachability, TaskPortScan, TaskLoginStatusCheck, TaskQuotaMonitor,
		}},
		{Name: "维护与清理", Icon: "🔧", Types: []TaskType{
			TaskScreenshotCleanup, TaskTamperCleanup, TaskBaselineRefresh, TaskAlertSilence, TaskBackup,
		}},
		{Name: "基础设施", Icon: "📡", Types: []TaskType{
			TaskCookieVerify, TaskBridgeTokenRotate, TaskPluginHealth, TaskCacheWarmup, TaskDistributedSubmit,
		}},
		{Name: "导入与汇总", Icon: "📥", Types: []TaskType{
			TaskURLImport, TaskICPImport, TaskAlertSummary,
		}},
	}
}

// TaskTypeGroup returns the UI group name for a task type, or "其他" if ungrouped.
func TaskTypeGroup(t TaskType) string {
	for _, g := range GroupedTaskTypes() {
		for _, tt := range g.Types {
			if tt == t {
				return g.Name
			}
		}
	}
	return "其他"
}

// DefaultTemplates returns a set of pre-defined task templates.
func DefaultTemplates() []TaskTemplate {
	return []TaskTemplate{
		{
			ID: "tmpl_daily_tamper_check", Name: "每日篡改检测",
			Description: "每天凌晨 2 点对所有重要 URL 进行篡改检测",
			Type:        TaskTamperCheck, CronExpr: "0 0 2 * * *",
			Payload:    &model.TaskPayload{DetectMode: "relaxed"},
			TimeoutSec: 3600, MaxRetries: 2, Tags: []string{"security", "daily"},
		},
		{
			ID: "tmpl_weekly_export", Name: "每周数据导出",
			Description: "每周日午夜导出本周查询数据",
			Type:        TaskExport, CronExpr: "0 0 0 * * 0",
			Payload:    &model.TaskPayload{Format: "json"},
			TimeoutSec: 1800, MaxRetries: 1, Tags: []string{"export", "weekly"},
		},
		{
			ID: "tmpl_hourly_quota_check", Name: "每小时配额检查",
			Description: "每小时检查各引擎 API 配额状态",
			Type:        TaskQuotaMonitor, CronExpr: "0 0 * * * *",
			Payload:    &model.TaskPayload{LowThresh: 10},
			TimeoutSec: 300, MaxRetries: 0, Tags: []string{"monitoring", "hourly"},
		},
		{
			ID: "tmpl_daily_screenshot_cleanup", Name: "每日截图清理",
			Description: "每天凌晨 3 点清理 30 天前的截图",
			Type:        TaskScreenshotCleanup, CronExpr: "0 0 3 * * *",
			Payload:    &model.TaskPayload{MaxAgeDays: 30},
			TimeoutSec: 600, MaxRetries: 1, Tags: []string{"cleanup", "daily"},
		},
		{
			ID: "tmpl_weekly_baseline_refresh", Name: "每周基线刷新",
			Description: "每周日凌晨刷新篡改检测基线",
			Type:        TaskBaselineRefresh, CronExpr: "0 0 4 * * 0",
			Payload:    &model.TaskPayload{},
			TimeoutSec: 1800, MaxRetries: 1, Tags: []string{"security", "weekly"},
		},
		{
			ID: "tmpl_daily_cookie_verify", Name: "每日 Cookie 验证",
			Description: "每天早上 8 点验证各引擎 Cookie 有效性",
			Type:        TaskCookieVerify, CronExpr: "0 0 8 * * *",
			Payload:    &model.TaskPayload{},
			TimeoutSec: 600, MaxRetries: 2, Tags: []string{"auth", "daily"},
		},
		{
			ID: "tmpl_daily_icp_company_watch", Name: "每日企业备案巡检",
			Description: "每天早上 9 点查询关注企业的 ICP 备案状态",
			Type:        TaskICPQuery, CronExpr: "0 0 9 * * *",
			Payload:    &model.TaskPayload{Queries: []string{}, Type: "web", Page: 1, PageSizeICP: 40},
			TimeoutSec: 600, MaxRetries: 1, Tags: []string{"icp", "daily", "compliance"},
		},
		{
			ID: "tmpl_weekly_icp_domain_scan", Name: "每周域名备案变更扫描",
			Description: "每周一凌晨 3 点扫描目标域名 ICP 备案变更",
			Type:        TaskICPQuery, CronExpr: "0 0 3 * * 1",
			Payload:    &model.TaskPayload{Queries: []string{}, Type: "web", Page: 1, PageSizeICP: 40},
			TimeoutSec: 1800, MaxRetries: 1, Tags: []string{"icp", "weekly", "monitoring"},
		},
	}
}

// CreateTaskFromTemplate creates a new task from a template.
func (s *Scheduler) CreateTaskFromTemplate(templateID string, name string, cronExpr string) (*ScheduledTask, error) {
	var tmpl *TaskTemplate
	for _, t := range DefaultTemplates() {
		if t.ID == templateID {
			tmpl = &t
			break
		}
	}
	if tmpl == nil {
		return nil, fmt.Errorf("template %s not found", templateID)
	}

	task := &ScheduledTask{
		Name: name, Type: tmpl.Type, Enabled: true,
		CronExpr: cronExpr, Payload: tmpl.Payload,
		TimeoutSec: tmpl.TimeoutSec, MaxRetries: tmpl.MaxRetries,
	}

	if err := s.AddTask(task); err != nil {
		return nil, fmt.Errorf("failed to create task from template: %w", err)
	}

	return task, nil
}

// ScheduledTask represents a user-configured scheduled task.
type ScheduledTask struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Type          TaskType           `json:"type"`
	Enabled       bool               `json:"enabled"`
	CronExpr      string             `json:"cron_expr,omitempty"`
	Payload       *model.TaskPayload `json:"payload"`
	TimeoutSec    int                `json:"timeout_seconds"`
	MaxRetries    int                `json:"max_retries"`
	LastRunAt     *time.Time         `json:"last_run_at,omitempty"`
	NextRunAt     *time.Time         `json:"next_run_at,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
	RuntimeStatus string             `json:"runtime_status"`
	ScheduleError string             `json:"schedule_error,omitempty"`

	// Schedule type: "cron" (default), "once", "delay"
	ScheduleType string `json:"schedule_type,omitempty"`
	// For "once": absolute execution time (RFC3339)
	RunAt *time.Time `json:"run_at,omitempty"`
	// For "delay": seconds from creation to execution
	DelaySeconds int `json:"delay_seconds,omitempty"`

	// 高级功能字段
	DependsOn       []string            `json:"depends_on,omitempty"`
	ExecutionWindow *ExecutionWindow    `json:"execution_window,omitempty"`
	Notifications   *NotificationConfig `json:"notifications,omitempty"`

	// Internal: timer for one-time/delayed tasks
	timer *time.Timer `json:"-"`
}

// ExecutionWindow defines when a task is allowed to run.
type ExecutionWindow struct {
	StartHour int    `json:"start_hour"` // 0-23
	EndHour   int    `json:"end_hour"`   // 0-23
	Weekdays  []int  `json:"weekdays"`   // 0=Sunday..6=Saturday
	Timezone  string `json:"timezone"`   // IANA timezone
}

// NotificationConfig configures task-level notifications.
type NotificationConfig struct {
	Enabled    bool     `json:"enabled"`
	OnSuccess  bool     `json:"on_success"`
	OnFailure  bool     `json:"on_failure"`
	OnTimeout  bool     `json:"on_timeout"`
	ChannelIDs []string `json:"channel_ids"`
	Channels   []string `json:"channels,omitempty"`    // legacy
	WebhookURL string   `json:"webhook_url,omitempty"` // legacy inline webhook
	Recipients []string `json:"recipients,omitempty"`
}

// TaskTemplate is a pre-defined task configuration.
type TaskTemplate struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Type        TaskType           `json:"type"`
	CronExpr    string             `json:"cron_expr"`
	Payload     *model.TaskPayload `json:"payload"`
	TimeoutSec  int                `json:"timeout_seconds"`
	MaxRetries  int                `json:"max_retries"`
	Tags        []string           `json:"tags"`
}

// ExecutionRecord stores the result of a task execution.
type ExecutionRecord struct {
	TaskID     string `json:"task_id"`
	TaskName   string `json:"task_name"`
	TaskType   string `json:"task_type"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	DurationMs int64  `json:"duration_ms"`
	Status     string `json:"status"` // success, failed, timeout, skipped
	Result     string `json:"result"`
	Error      string `json:"error,omitempty"`
	RetryCount int    `json:"retry_count"`
}

// TaskHandler is the interface that wraps the Execute method for a task type.
type TaskHandler interface {
	Type() TaskType
	Execute(ctx context.Context, payload *model.TaskPayload) (string, error)
}

type schedulerStore interface {
	Load() ([]*ScheduledTask, []ExecutionRecord, error)
	Save([]*ScheduledTask, []ExecutionRecord) error
	SaveTasks([]*ScheduledTask) error
}

// PushLogRecord is one notification push audit event captured by the scheduler
// right before a task notification is fanned out to its channels. The web layer
// adapts it into the persistent history repository (notification_push_log).
type PushLogRecord struct {
	TaskID        string
	TaskName      string
	ChannelIDs    []string
	Status        string
	ResultCount   int
	ResultSummary string
	Error         string
}

// Scheduler manages cron-based task scheduling with persistence.
type Scheduler struct {
	automaticDisabled bool // immutable startup policy
	tasks             map[string]*ScheduledTask
	cron              *cron.Cron
	cronIDs           map[string]cron.EntryID
	handlers          map[TaskType]TaskHandler
	history           []ExecutionRecord
	store             schedulerStore
	stopCh            chan struct{}
	stopped           bool
	mu                sync.RWMutex
	maxHistory        int

	// 生命周期控制
	ctx    context.Context
	cancel context.CancelFunc

	// 通知系统
	notifyRegistry    *notify.Registry
	notifyCfgProvider func() *notify.NotifyGlobalCfg
	notifyWg          sync.WaitGroup
	notifyTimeout     time.Duration
	stopping          bool
	// recordPushLog persists a notification push audit event when set. Nil-safe:
	// without a recorder the push still sends, it just isn't logged.
	recordPushLog func(PushLogRecord) error
}
