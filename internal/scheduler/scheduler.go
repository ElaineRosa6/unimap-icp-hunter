// Package scheduler provides cron-based task scheduling for UniMap operations.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
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
)

// AllTaskTypes returns all supported task types.
func AllTaskTypes() []TaskType {
	return []TaskType{
		TaskQuery, TaskSearchScreenshot, TaskBatchScreenshot, TaskTamperCheck,
		TaskURLReachability, TaskCookieVerify, TaskLoginStatusCheck, TaskDistributedSubmit,
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
	}
	if l, ok := labels[t]; ok {
		return l
	}
	return string(t)
}

// ScheduledTask represents a user-configured scheduled task.
type ScheduledTask struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       TaskType               `json:"type"`
	Enabled    bool                   `json:"enabled"`
	CronExpr   string                 `json:"cron_expr"`
	Payload    map[string]interface{} `json:"payload"`
	TimeoutSec int                    `json:"timeout_seconds"`
	MaxRetries int                    `json:"max_retries"`
	LastRunAt  *time.Time             `json:"last_run_at,omitempty"`
	NextRunAt  *time.Time             `json:"next_run_at,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
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
	Execute(ctx context.Context, payload map[string]interface{}) (string, error)
}

// Scheduler manages cron-based task scheduling with persistence.
type Scheduler struct {
	tasks      map[string]*ScheduledTask
	cron       *cron.Cron
	cronIDs    map[string]cron.EntryID // taskID -> cron entry ID
	handlers   map[TaskType]TaskHandler
	history    []ExecutionRecord
	store      *Store
	stopCh     chan struct{}
	stopped    bool
	mu         sync.RWMutex
	maxHistory int
	idCounter  int64 // monotonic counter for unique task IDs
}

// NewScheduler creates a new Scheduler. If storePath is non-empty, tasks are
// persisted to that JSON file.
func NewScheduler(storePath string, historyPath string, maxHistory int) *Scheduler {
	c := cron.New(cron.WithSeconds())
	c.Start()

	if maxHistory <= 0 {
		maxHistory = 500
	}

	s := &Scheduler{
		tasks:      make(map[string]*ScheduledTask),
		cron:       c,
		cronIDs:    make(map[string]cron.EntryID),
		handlers:   make(map[TaskType]TaskHandler),
		history:    make([]ExecutionRecord, 0),
		maxHistory: maxHistory,
		stopCh:     make(chan struct{}),
	}

	if storePath != "" {
		s.store = NewStore(storePath, historyPath)
	}

	return s
}

// Load persists loads tasks and history from disk.
func (s *Scheduler) Load() error {
	if s.store == nil {
		return nil
	}
	tasks, history, err := s.store.Load()
	if err != nil {
		return fmt.Errorf("load scheduler data: %w", err)
	}
	for _, t := range tasks {
		s.tasks[t.ID] = t
		if t.Enabled {
			if err := s.scheduleTask(t); err != nil {
				log.Printf("[scheduler] failed to schedule persisted task %s (%s): %v — task loaded but will not auto-fire", t.ID, t.Name, err)
			}
		}
	}
	s.rebuildIDCounterFromTasks()
	s.history = history
	return nil
}

// Save persists current tasks and history to disk.
func (s *Scheduler) Save() error {
	if s.store == nil {
		return nil
	}
	s.mu.RLock()
	tasks := make([]*ScheduledTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		cp := *t
		if cp.Payload != nil {
			cp.Payload = make(map[string]interface{})
			for k, v := range t.Payload {
				cp.Payload[k] = v
			}
		}
		if cp.LastRunAt != nil {
			v := *cp.LastRunAt
			cp.LastRunAt = &v
		}
		if cp.NextRunAt != nil {
			v := *cp.NextRunAt
			cp.NextRunAt = &v
		}
		tasks = append(tasks, &cp)
	}
	history := make([]ExecutionRecord, len(s.history))
	copy(history, s.history)
	s.mu.RUnlock()

	return s.store.Save(tasks, history)
}

// RegisterHandler registers a task handler. Must be called before Start().
func (s *Scheduler) RegisterHandler(h TaskHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[h.Type()] = h
}

// AddTask adds a new scheduled task and schedules it in cron.
func (s *Scheduler) AddTask(task *ScheduledTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if task.ID == "" {
		task.ID = s.generateID()
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	if task.TimeoutSec <= 0 {
		task.TimeoutSec = 300
	}

	// Validate cron expression (5 or 6 fields)
	parser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	_, err := parser.Parse(task.CronExpr)
	if err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	// Validate task type
	if _, ok := s.handlers[task.Type]; !ok {
		return fmt.Errorf("unknown task type: %s", task.Type)
	}

	s.tasks[task.ID] = task
	if err := s.scheduleTask(task); err != nil {
		delete(s.tasks, task.ID)
		return fmt.Errorf("failed to schedule task: %w", err)
	}
	s.saveAsync()
	return nil
}

// UpdateTask updates an existing task's configuration.
func (s *Scheduler) UpdateTask(task *ScheduledTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.tasks[task.ID]
	if !ok {
		return fmt.Errorf("task %s not found", task.ID)
	}

	// Validate cron if changed
	if task.CronExpr != "" && task.CronExpr != existing.CronExpr {
		parser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		_, err := parser.Parse(task.CronExpr)
		if err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
	}

	// Validate task type has a handler
	if task.Type != existing.Type {
		if _, hasHandler := s.handlers[task.Type]; !hasHandler {
			return fmt.Errorf("unknown task type: %s", task.Type)
		}
	}

	// Remove old cron entry
	if entryID, ok := s.cronIDs[task.ID]; ok {
		s.cron.Remove(entryID)
		delete(s.cronIDs, task.ID)
	}

	// Update fields
	existing.Name = task.Name
	existing.Type = task.Type
	existing.Enabled = task.Enabled
	existing.CronExpr = task.CronExpr
	existing.Payload = task.Payload
	existing.TimeoutSec = task.TimeoutSec
	existing.MaxRetries = task.MaxRetries

	if existing.Enabled {
		if err := s.scheduleTask(existing); err != nil {
			return fmt.Errorf("failed to schedule task: %w", err)
		}
	}
	s.saveAsync()
	return nil
}

// DeleteTask removes a task from the scheduler.
func (s *Scheduler) DeleteTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, ok := s.cronIDs[id]; ok {
		s.cron.Remove(entryID)
		delete(s.cronIDs, id)
	}
	if _, ok := s.tasks[id]; !ok {
		return fmt.Errorf("task %s not found", id)
	}
	delete(s.tasks, id)
	s.saveAsync()
	return nil
}

// EnableTask enables a task and schedules it.
func (s *Scheduler) EnableTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	task.Enabled = true
	if err := s.scheduleTask(task); err != nil {
		task.Enabled = false
		return fmt.Errorf("failed to schedule task: %w", err)
	}
	s.saveAsync()
	return nil
}

// DisableTask disables a task and removes it from cron.
func (s *Scheduler) DisableTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	task.Enabled = false
	if entryID, ok := s.cronIDs[id]; ok {
		s.cron.Remove(entryID)
		delete(s.cronIDs, id)
	}
	s.saveAsync()
	return nil
}

// RunTaskNow executes a task immediately, regardless of its enabled state.
func (s *Scheduler) RunTaskNow(id string) error {
	s.mu.RLock()
	task, ok := s.tasks[id]
	if !ok {
		s.mu.RUnlock()
		return fmt.Errorf("task %s not found", id)
	}
	// Copy task data to avoid holding the lock during execution
	handler := s.handlers[task.Type]
	if handler == nil {
		s.mu.RUnlock()
		return fmt.Errorf("no handler registered for task type %s", task.Type)
	}
	timeoutSec := task.TimeoutSec
	retries := task.MaxRetries
	// Deep copy the task for execution
	taskCopy := *task
	if task.Payload != nil {
		taskCopy.Payload = make(map[string]interface{})
		for k, v := range task.Payload {
			taskCopy.Payload[k] = v
		}
	}
	s.mu.RUnlock()

	go s.executeTask(&taskCopy, handler, timeoutSec, retries)
	return nil
}

// ListTasks returns a copy of all scheduled tasks.
func (s *Scheduler) ListTasks() []*ScheduledTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*ScheduledTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		// Copy to avoid mutation
		cp := *t
		if t.Payload != nil {
			cp.Payload = make(map[string]interface{})
			for k, v := range t.Payload {
				cp.Payload[k] = v
			}
		}
		result = append(result, &cp)
	}
	return result
}

// GetTask returns a single task by ID.
func (s *Scheduler) GetTask(id string) (*ScheduledTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %s not found", id)
	}
	cp := *task
	return &cp, nil
}

// GetHistory returns execution history, most recent first.
func (s *Scheduler) GetHistory(limit int, taskType string, status string) []ExecutionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ExecutionRecord, 0, len(s.history))
	for i := len(s.history) - 1; i >= 0; i-- {
		r := s.history[i]
		if taskType != "" && r.TaskType != taskType {
			continue
		}
		if status != "" && r.Status != status {
			continue
		}
		result = append(result, r)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

// normalizeCronExpr converts a 5-field cron (min hour dom month dow) to a
// 6-field expression (sec min hour dom month dow) by prepending "0".
// The scheduler is initialized with cron.WithSeconds() which requires 6 fields.
func normalizeCronExpr(expr string) string {
	fields := strings.Fields(expr)
	if len(fields) == 5 {
		return "0 " + expr
	}
	return expr
}

// scheduleTask registers a task in the cron scheduler. Returns error if the
// task cannot be added to cron (caller can then remove the task or retry).
func (s *Scheduler) scheduleTask(task *ScheduledTask) error {
	if !task.Enabled {
		return nil
	}
	handler := s.handlers[task.Type]
	if handler == nil {
		log.Printf("[scheduler] no handler registered for task type %s (id=%s)", task.Type, task.ID)
		return nil
	}

	schedule := func() {
		s.executeTask(task, handler, task.TimeoutSec, task.MaxRetries)
	}

	cronExpr := normalizeCronExpr(task.CronExpr)
	entryID, err := s.cron.AddFunc(cronExpr, schedule)
	if err != nil {
		log.Printf("[scheduler] failed to schedule task %s (cron=%q): %v", task.ID, task.CronExpr, err)
		return err
	}
	s.cronIDs[task.ID] = entryID

	// Calculate next run time
	next := s.cron.Entry(entryID).Next
	if !next.IsZero() {
		task.NextRunAt = &next
	}
	return nil
}

// executeTask runs a single task execution with optional retries.
func (s *Scheduler) executeTask(task *ScheduledTask, handler TaskHandler, timeoutSec int, maxRetries int) {
	now := time.Now()
	record := ExecutionRecord{
		TaskID:     task.ID,
		TaskName:   task.Name,
		TaskType:   string(task.Type),
		StartedAt:  now.Format(time.RFC3339),
		RetryCount: 0,
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			record.RetryCount = attempt
			time.Sleep(time.Duration(attempt*2) * time.Second) // simple backoff
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
		result, err := handler.Execute(ctx, task.Payload)
		cancel()

		elapsed := time.Since(now)
		record.FinishedAt = time.Now().Format(time.RFC3339)
		record.DurationMs = elapsed.Milliseconds()

		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				record.Status = "timeout"
				record.Error = fmt.Sprintf("task timed out after %s", elapsed.Round(time.Millisecond))
			} else {
				record.Status = "failed"
				record.Error = err.Error()
			}
			continue
		}

		record.Status = "success"
		record.Result = result
		break
	}

	// Update task state
	s.mu.Lock()
	if t, ok := s.tasks[task.ID]; ok {
		t.LastRunAt = &now
		if next := s.getNextRunTime(task.ID); !next.IsZero() {
			t.NextRunAt = &next
		}
	}

	// Append history
	s.history = append(s.history, record)
	if len(s.history) > s.maxHistory {
		s.history = s.history[len(s.history)-s.maxHistory:]
	}
	s.mu.Unlock()
}

func (s *Scheduler) getNextRunTime(taskID string) time.Time {
	if entryID, ok := s.cronIDs[taskID]; ok {
		return s.cron.Entry(entryID).Next
	}
	return time.Time{}
}

// saveAsync persists data to disk in a background goroutine.
func (s *Scheduler) saveAsync() {
	go func() {
		s.Save()
	}()
}

// Stop gracefully stops the scheduler and all background goroutines.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	close(s.stopCh)
	s.mu.Unlock()

	// Stop cron
	s.cron.Stop()
}

// generateID creates a short unique ID using a monotonic counter.
func (s *Scheduler) generateID() string {
	s.idCounter++
	return fmt.Sprintf("task_%d", s.idCounter)
}

func (s *Scheduler) rebuildIDCounterFromTasks() {
	maxID := int64(0)
	for id := range s.tasks {
		if !strings.HasPrefix(id, "task_") {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimPrefix(id, "task_"), 10, 64)
		if err != nil || n <= 0 {
			continue
		}
		if n > maxID {
			maxID = n
		}
	}
	if maxID > s.idCounter {
		s.idCounter = maxID
	}
}
