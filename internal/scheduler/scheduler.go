// Package scheduler provides cron-based task scheduling for UniMap operations.
package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/unimap/project/internal/logger"
	"github.com/unimap/project/internal/metrics"
	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/notify"
)

var (
	ErrTaskNotFound = errors.New("scheduler task not found")
	ErrPersistence  = errors.New("scheduler persistence failure")
	ErrSchedule     = errors.New("scheduler runtime scheduling failure")
)

// NewScheduler creates a new Scheduler. If storePath is non-empty, tasks are
// persisted to that JSON file.
func NewScheduler(storePath string, historyPath string, maxHistory int) *Scheduler {
	return NewSchedulerWithAutomaticEnabled(storePath, historyPath, maxHistory, true)
}

// NewSchedulerWithAutomaticEnabled fixes automatic scheduling policy for this
// instance before persisted cron or timer tasks are loaded. Manual runs remain available.
func NewSchedulerWithAutomaticEnabled(storePath string, historyPath string, maxHistory int, enabled bool) *Scheduler {
	c := cron.New(cron.WithSeconds())
	// Delay cron start until caller registers handlers and tasks.
	// Call s.Start() after setup is complete.

	if maxHistory <= 0 {
		maxHistory = 500
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Scheduler{
		automaticDisabled: !enabled,
		tasks:             make(map[string]*ScheduledTask),
		cron:              c,
		cronIDs:           make(map[string]cron.EntryID),
		handlers:          make(map[TaskType]TaskHandler),
		history:           make([]ExecutionRecord, 0),
		maxHistory:        maxHistory,
		stopCh:            make(chan struct{}),
		ctx:               ctx,
		cancel:            cancel,
		notifyTimeout:     60 * time.Second, // 默认 60 秒，覆盖慢 DNS 解析场景
	}

	if storePath != "" {
		s.store = NewStore(storePath, historyPath)
	}

	return s
}

// SetNotifyRegistry 设置通知渠道注册表
func (s *Scheduler) SetNotifyRegistry(reg *notify.Registry) {
	s.notifyRegistry = reg
}

// SetNotifyCfgProvider 设置全局通知配置提供者
func (s *Scheduler) SetNotifyCfgProvider(provider func() *notify.NotifyGlobalCfg) {
	s.notifyCfgProvider = provider
}

// SetNotificationLogRecorder installs an optional callback that persists a
// notification push audit event (see history.NotificationPushLog). When unset,
// pushes still send normally but are not recorded.
func (s *Scheduler) SetNotificationLogRecorder(fn func(PushLogRecord) error) {
	s.recordPushLog = fn
}

// Start begins the internal cron scheduler. Call this after registering
// handlers and loading persisted tasks.
func (s *Scheduler) Start() {
	if s.automaticDisabled {
		return
	}
	s.cron.Start()
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
				logger.Errorf("[scheduler] failed to schedule persisted task %s (%s): %v — task loaded but will not auto-fire", t.ID, t.Name, err)
			}
		} else {
			t.RuntimeStatus = "disabled"
			t.ScheduleError = ""
		}
	}
	s.history = history
	s.updateMetrics()
	return nil
}

// Save persists current tasks and history to disk. It takes a read lock
// (RLock) because saveLocked performs a full deep copy of every task before
// serialization: the s.tasks map structure is protected by RLock, and each
// task's Payload is replaced immutably (AddTask/UpdateTask swap in a new
// *TaskPayload via sanitizePayload rather than mutating in place), so reading
// payload fields under the read lock is race-free. Pointer fields (LastRunAt,
// NextRunAt) are value-copied. This is why a read lock suffices.
func (s *Scheduler) Save() error {
	if s.store == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.saveLocked()
}

// saveLocked persists tasks and history. Caller must hold the mutex (write or read).
func (s *Scheduler) saveLocked() error {
	if s.store == nil {
		return nil
	}
	tasks := s.taskSnapshotLocked()
	history := make([]ExecutionRecord, len(s.history))
	copy(history, s.history)

	return s.store.Save(tasks, history)
}

func (s *Scheduler) saveTasksLocked() error {
	if s.store == nil {
		return nil
	}
	return s.store.SaveTasks(s.taskSnapshotLocked())
}

func (s *Scheduler) taskSnapshotLocked() []*ScheduledTask {
	tasks := make([]*ScheduledTask, 0, len(s.tasks))
	for _, t := range s.tasks {
		cp := *t
		if cp.Payload != nil {
			raw, err := json.Marshal(t.Payload)
			if err != nil {
				logger.Warnf("[scheduler] failed to deep-copy payload for task %s: %v", t.ID, err)
				cp.Payload = &model.TaskPayload{}
			} else {
				var newPayload model.TaskPayload
				_ = json.Unmarshal(raw, &newPayload)
				cp.Payload = &newPayload
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
	return tasks
}

// RegisterHandler registers a task handler. Must be called before Start().
func (s *Scheduler) RegisterHandler(h TaskHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[h.Type()] = h
	metrics.SetSchedulerTasksRegistered(string(h.Type()), 1)
}

// validateScheduleTypeLocked validates the schedule-type-specific fields of a
// task against the same rules AddTask enforces. It is shared by AddTask and
// UpdateTask so that the update path cannot bypass creation-time validation.
//
// All arguments are the *effective* (post-merge) values to be persisted; the
// caller is responsible for merging incoming updates onto the existing task
// before calling this. Must be called with s.mu held (reads s.handlers only
// indirectly via callers; no mutation here, but kept consistent with locked
// convention).
func (s *Scheduler) validateScheduleTypeLocked(scheduleType, cronExpr string, runAt *time.Time, delaySeconds int) error {
	switch scheduleType {
	case "cron":
		parser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
		if _, err := parser.Parse(cronExpr); err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
	case "once":
		if runAt == nil {
			return fmt.Errorf("run_at is required for once schedule type")
		}
		if runAt.Before(time.Now()) {
			return fmt.Errorf("run_at must be in the future")
		}
	case "delay":
		// A delay task may already have RunAt set (computed from a previous
		// scheduling). When RunAt is absent, DelaySeconds must be positive.
		if runAt == nil && delaySeconds <= 0 {
			return fmt.Errorf("delay_seconds must be positive for delay schedule type")
		}
	default:
		return fmt.Errorf("unknown schedule type: %s (valid: cron, once, delay)", scheduleType)
	}
	return nil
}

// AddTask adds a new scheduled task and schedules it in cron.
func (s *Scheduler) AddTask(task *ScheduledTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Sanitize task name and payload strings at creation time to handle
	// GBK-encoded input from Windows terminals / Chinese HTTP clients.
	task.Name = sanitizeUTF8(task.Name)
	task.Payload = sanitizePayload(task.Payload)

	if task.ID == "" {
		task.ID = s.generateID()
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	if task.TimeoutSec <= 0 {
		task.TimeoutSec = 300
	}

	// Normalize schedule type
	if task.ScheduleType == "" {
		task.ScheduleType = "cron"
	}

	// Validate schedule type (mirrors UpdateTask — see validateScheduleTypeLocked)
	if err := s.validateScheduleTypeLocked(task.ScheduleType, task.CronExpr, task.RunAt, task.DelaySeconds); err != nil {
		return err
	}

	// Validate task type
	if _, ok := s.handlers[task.Type]; !ok {
		return fmt.Errorf("unknown task type: %s", task.Type)
	}

	// Validate webhook URL if configured
	if task.Notifications != nil {
		if err := validateWebhookURL(task.Notifications.WebhookURL); err != nil {
			return err
		}
	}

	// Check for cyclic dependencies
	if s.hasCyclicDependencyLocked(task.ID, task.DependsOn) {
		return fmt.Errorf("task %s has cyclic dependencies", task.ID)
	}
	prepareDelayedRunAt(task)

	s.tasks[task.ID] = task
	if err := s.saveTasksLocked(); err != nil {
		delete(s.tasks, task.ID)
		return fmt.Errorf("%w: persist task: %v", ErrPersistence, err)
	}
	if err := s.scheduleTask(task); err != nil {
		delete(s.tasks, task.ID)
		if rollbackErr := s.saveTasksLocked(); rollbackErr != nil {
			return fmt.Errorf("%w: schedule task: %v; persist rollback: %v", ErrPersistence, err, rollbackErr)
		}
		return fmt.Errorf("%w: %v", ErrSchedule, err)
	}
	return nil
}

// UpdateTask updates an existing task's configuration.
func (s *Scheduler) UpdateTask(task *ScheduledTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.tasks[task.ID]
	if !ok {
		return fmt.Errorf("%w: task %s", ErrTaskNotFound, task.ID)
	}
	previous := cloneScheduledTask(existing)

	// Sanitize on update too
	task.Name = sanitizeUTF8(task.Name)
	task.Payload = sanitizePayload(task.Payload)

	// Validate cron if changed (kept for a clear early-failure error before
	// the full schedule-type validation below, which also covers cron).
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

	// Validate webhook URL if configured
	if task.Notifications != nil {
		if err := validateWebhookURL(task.Notifications.WebhookURL); err != nil {
			return err
		}
	}

	// Check for cyclic dependencies if dependencies changed
	if !s.equalStringSlices(existing.DependsOn, task.DependsOn) {
		if s.hasCyclicDependencyLocked(task.ID, task.DependsOn) {
			return fmt.Errorf("task %s has cyclic dependencies", task.ID)
		}
	}

	// Remove old cron entry
	if entryID, ok := s.cronIDs[task.ID]; ok {
		s.cron.Remove(entryID)
		delete(s.cronIDs, task.ID)
	}
	// Stop old timer for one-time tasks
	if existing.timer != nil {
		existing.timer.Stop()
		existing.timer = nil
	}

	// Update fields
	existing.Name = task.Name
	existing.Type = task.Type
	existing.Enabled = task.Enabled
	existing.CronExpr = task.CronExpr
	existing.Payload = task.Payload
	existing.TimeoutSec = task.TimeoutSec
	existing.MaxRetries = task.MaxRetries
	existing.Notifications = task.Notifications
	existing.DependsOn = task.DependsOn
	existing.ExecutionWindow = task.ExecutionWindow

	// Update schedule type fields — merge incoming updates onto existing,
	// then validate the *effective* values. This closes the gap where
	// UpdateTask previously accepted arbitrary schedule_type / past RunAt /
	// non-positive DelaySeconds that AddTask rejects.
	if task.ScheduleType != "" {
		existing.ScheduleType = task.ScheduleType
	} else if existing.ScheduleType == "" {
		existing.ScheduleType = "cron"
	}
	if task.RunAt != nil {
		existing.RunAt = task.RunAt
	}
	if task.DelaySeconds > 0 {
		existing.DelaySeconds = task.DelaySeconds
		if task.RunAt == nil {
			existing.RunAt = nil
		}
	}
	if err := s.validateScheduleTypeLocked(existing.ScheduleType, existing.CronExpr, existing.RunAt, existing.DelaySeconds); err != nil {
		s.tasks[task.ID] = previous
		if previous.Enabled {
			if scheduleErr := s.scheduleTask(previous); scheduleErr != nil {
				return fmt.Errorf("validate updated task: %v; restore schedule: %w", err, scheduleErr)
			}
		}
		return err
	}
	prepareDelayedRunAt(existing)
	existing.NextRunAt = nil

	if err := s.saveTasksLocked(); err != nil {
		s.tasks[task.ID] = previous
		if previous.Enabled {
			if scheduleErr := s.scheduleTask(previous); scheduleErr != nil {
				return fmt.Errorf("%w: persist task: %v; restore schedule: %v", ErrPersistence, err, scheduleErr)
			}
		}
		return fmt.Errorf("%w: persist task: %v", ErrPersistence, err)
	}
	if existing.Enabled {
		if err := s.scheduleTask(existing); err != nil {
			s.tasks[task.ID] = previous
			rollbackErr := s.saveTasksLocked()
			var scheduleErr error
			if previous.Enabled {
				scheduleErr = s.scheduleTask(previous)
			}
			if rollbackErr != nil || scheduleErr != nil {
				return fmt.Errorf("%w: schedule task: %v; persist rollback: %v; restore schedule: %v", ErrPersistence, err, rollbackErr, scheduleErr)
			}
			return fmt.Errorf("%w: %v", ErrSchedule, err)
		}
	} else {
		existing.RuntimeStatus = "disabled"
		existing.ScheduleError = ""
	}
	return nil
}

func prepareDelayedRunAt(task *ScheduledTask) {
	if task != nil && task.ScheduleType == "delay" && task.RunAt == nil && task.DelaySeconds > 0 {
		runAt := time.Now().Add(time.Duration(task.DelaySeconds) * time.Second)
		task.RunAt = &runAt
	}
}

func (s *Scheduler) unscheduleTaskLocked(task *ScheduledTask) {
	if task == nil {
		return
	}
	if entryID, ok := s.cronIDs[task.ID]; ok {
		s.cron.Remove(entryID)
		delete(s.cronIDs, task.ID)
	}
	if task.timer != nil {
		task.timer.Stop()
		task.timer = nil
	}
	task.NextRunAt = nil
}

func cloneScheduledTask(task *ScheduledTask) *ScheduledTask {
	if task == nil {
		return nil
	}
	clone := *task
	if task.Payload != nil {
		data, _ := json.Marshal(task.Payload)
		var payload model.TaskPayload
		if json.Unmarshal(data, &payload) == nil {
			clone.Payload = &payload
		}
	}
	if task.RunAt != nil {
		value := *task.RunAt
		clone.RunAt = &value
	}
	if task.LastRunAt != nil {
		value := *task.LastRunAt
		clone.LastRunAt = &value
	}
	if task.NextRunAt != nil {
		value := *task.NextRunAt
		clone.NextRunAt = &value
	}
	return &clone
}

// DeleteTask removes a task from the scheduler.
func (s *Scheduler) DeleteTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("%w: task %s", ErrTaskNotFound, id)
	}
	previous := cloneScheduledTask(task)
	previous.timer = nil
	s.unscheduleTaskLocked(task)
	// Stop timer for one-time tasks
	if task.timer != nil {
		task.timer.Stop()
		task.timer = nil
	}
	delete(s.tasks, id)
	if err := s.saveTasksLocked(); err != nil {
		s.tasks[id] = previous
		if previous.Enabled {
			if restoreErr := s.scheduleTask(previous); restoreErr != nil {
				return fmt.Errorf("%w: persist delete: %v; restore schedule: %v", ErrPersistence, err, restoreErr)
			}
		}
		return fmt.Errorf("%w: persist delete: %v", ErrPersistence, err)
	}
	return nil
}

// EnableTask enables a task and schedules it.
func (s *Scheduler) EnableTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("%w: task %s", ErrTaskNotFound, id)
	}

	// Prevent re-enabling expired one-time tasks. This applies to BOTH
	// already-run tasks (LastRunAt != nil) and never-run tasks whose RunAt
	// is in the past (e.g. process restarted before the once task fired).
	// The latter previously fell through and scheduleOneTimeTask executed
	// them immediately because delay <= 0, contradicting AddTask's "run_at
	// must be in the future" rule. Now we reject consistently.
	if (task.ScheduleType == "once" || task.ScheduleType == "delay") && task.RunAt != nil && task.RunAt.Before(time.Now()) {
		return fmt.Errorf("cannot re-enable expired one-time task %s (was scheduled for %s)", id, task.RunAt.Format(time.RFC3339))
	}

	task.Enabled = true
	prepareDelayedRunAt(task)
	if err := s.saveTasksLocked(); err != nil {
		task.Enabled = false
		return fmt.Errorf("%w: persist task: %v", ErrPersistence, err)
	}
	if err := s.scheduleTask(task); err != nil {
		task.Enabled = false
		if rollbackErr := s.saveTasksLocked(); rollbackErr != nil {
			return fmt.Errorf("%w: schedule task: %v; persist rollback: %v", ErrPersistence, err, rollbackErr)
		}
		return fmt.Errorf("%w: %v", ErrSchedule, err)
	}
	return nil
}

// DisableTask disables a task and removes it from cron.
func (s *Scheduler) DisableTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("%w: task %s", ErrTaskNotFound, id)
	}
	previous := cloneScheduledTask(task)
	previous.timer = nil
	task.Enabled = false
	task.RuntimeStatus = "disabled"
	task.ScheduleError = ""
	s.unscheduleTaskLocked(task)
	if err := s.saveTasksLocked(); err != nil {
		s.tasks[id] = previous
		if previous.Enabled {
			if restoreErr := s.scheduleTask(previous); restoreErr != nil {
				return fmt.Errorf("%w: persist disable: %v; restore schedule: %v", ErrPersistence, err, restoreErr)
			}
		}
		return fmt.Errorf("%w: persist disable: %v", ErrPersistence, err)
	}
	return nil
}

// RunTaskNow executes a task immediately, regardless of its enabled state.
func (s *Scheduler) RunTaskNow(id string) error {
	s.mu.RLock()
	task, ok := s.tasks[id]
	if !ok {
		s.mu.RUnlock()
		return fmt.Errorf("%w: task %s", ErrTaskNotFound, id)
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
		raw, err := json.Marshal(task.Payload)
		if err == nil {
			var newPayload model.TaskPayload
			_ = json.Unmarshal(raw, &newPayload)
			taskCopy.Payload = &newPayload
		}
	}
	s.mu.RUnlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("scheduler panic in RunTaskNow (%s): %v", taskCopy.ID, r)
			}
		}()
		s.executeTask(&taskCopy, handler, timeoutSec, retries)
	}()
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
			raw, err := json.Marshal(t.Payload)
			if err == nil {
				var newPayload model.TaskPayload
				_ = json.Unmarshal(raw, &newPayload)
				cp.Payload = &newPayload
			}
		}
		// Prefer the live cron next-run time over the persisted value, which is
		// stale until the first execution after a restart (the persisted time is
		// only refreshed in finalizeTaskExecution).
		if t.Enabled && t.ScheduleType == "cron" {
			if next := s.getNextRunTime(t.ID); !next.IsZero() {
				cp.NextRunAt = &next
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
		return nil, fmt.Errorf("%w: task %s", ErrTaskNotFound, id)
	}
	cp := *task
	// Prefer the live cron next-run time over the persisted value (see ListTasks).
	if task.Enabled && task.ScheduleType == "cron" {
		if next := s.getNextRunTime(id); !next.IsZero() {
			cp.NextRunAt = &next
		}
	}
	return &cp, nil
}

// GetHistory returns execution history, most recent first. When taskID is
// supplied, filtering happens before the limit is applied.
func (s *Scheduler) GetHistory(limit int, taskType string, status string, taskIDs ...string) []ExecutionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	taskID := ""
	if len(taskIDs) > 0 {
		taskID = taskIDs[0]
	}
	result := make([]ExecutionRecord, 0, len(s.history))
	for i := len(s.history) - 1; i >= 0; i-- {
		r := s.history[i]
		if taskID != "" && r.TaskID != taskID {
			continue
		}
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

// hasCyclicDependency checks for cyclic dependencies in a task's dependency chain.
func (s *Scheduler) hasCyclicDependencyLocked(taskID string, dependsOn []string) bool {
	visiting := make(map[string]bool) // nodes in current path
	visited := make(map[string]bool)  // fully explored nodes

	var dfs func(string) bool
	dfs = func(current string) bool {
		if visiting[current] {
			return true // cycle: back-edge to node in current path
		}
		if visited[current] {
			return false // already fully explored, no cycle from here
		}
		visiting[current] = true

		task, ok := s.tasks[current]
		if ok {
			for _, depID := range task.DependsOn {
				if dfs(depID) {
					return true
				}
			}
		}

		visiting[current] = false
		visited[current] = true
		return false
	}

	for _, depID := range dependsOn {
		if dfs(depID) {
			return true
		}
	}

	return false
}

// hasCyclicDependency checks for cyclic dependencies in a task's dependency chain.
// nolint:unused
func (s *Scheduler) hasCyclicDependency(taskID string, dependsOn []string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasCyclicDependencyLocked(taskID, dependsOn)
}

// equalStringSlices checks if two string slices are equal.
func (s *Scheduler) equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
//
// Lock contract: the caller MUST hold s.mu (write lock). AddTask, UpdateTask,
// EnableTask all hold the write lock. Load calls this during start-up before
// Start() opens the door to concurrent access, so it is single-threaded there;
// if Load is ever invoked at runtime, it must take the write lock first.
// scheduleOneTimeTask mutates task.timer / task.NextRunAt / task.RunAt, which
// must not race with concurrent readers — hence the write-lock requirement.
func (s *Scheduler) scheduleTask(task *ScheduledTask) error {
	if s.automaticDisabled {
		task.RuntimeStatus = "scheduler_disabled"
		task.ScheduleError = ""
		task.NextRunAt = nil
		return nil
	}
	if !task.Enabled {
		task.RuntimeStatus = "disabled"
		task.ScheduleError = ""
		return nil
	}
	handler := s.handlers[task.Type]
	if handler == nil {
		err := fmt.Errorf("no handler registered for task type %s", task.Type)
		task.RuntimeStatus = "schedule_error"
		task.ScheduleError = err.Error()
		logger.Warnf("[scheduler] %v (id=%s)", err, task.ID)
		return err
	}

	var err error
	switch task.ScheduleType {
	case "once", "delay":
		err = s.scheduleOneTimeTask(task, handler)
	default: // "cron"
		err = s.scheduleCronTask(task, handler)
	}
	if err != nil {
		task.RuntimeStatus = "schedule_error"
		task.ScheduleError = err.Error()
		return err
	}
	task.RuntimeStatus = "scheduled"
	task.ScheduleError = ""
	return nil
}

// scheduleCronTask schedules a recurring task via cron.
func (s *Scheduler) scheduleCronTask(task *ScheduledTask, handler TaskHandler) error {
	schedule := func() {
		s.executeTask(task, handler, task.TimeoutSec, task.MaxRetries)
	}

	cronExpr := normalizeCronExpr(task.CronExpr)
	entryID, err := s.cron.AddFunc(cronExpr, schedule)
	if err != nil {
		logger.Errorf("[scheduler] failed to schedule task %s (cron=%q): %v", task.ID, task.CronExpr, err)
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

// scheduleOneTimeTask schedules a one-time or delayed task using a timer.
func (s *Scheduler) scheduleOneTimeTask(task *ScheduledTask, handler TaskHandler) error {
	var delay time.Duration
	switch task.ScheduleType {
	case "once":
		if task.RunAt == nil {
			return fmt.Errorf("run_at is nil for once task %s", task.ID)
		}
		delay = time.Until(*task.RunAt)
	case "delay":
		// Prefer persisted RunAt (survives restart correctly), fall back to DelaySeconds
		if task.RunAt != nil {
			delay = time.Until(*task.RunAt)
		} else if task.DelaySeconds > 0 {
			delay = time.Duration(task.DelaySeconds) * time.Second
			runAt := time.Now().Add(delay)
			task.RunAt = &runAt
		} else {
			return fmt.Errorf("delay task %s has no run_at or delay_seconds", task.ID)
		}
	}

	if delay <= 0 {
		// Past due — execute immediately
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("[scheduler] panic in one-time task %s: %v", task.ID, r)
				}
			}()
			s.executeTask(task, handler, task.TimeoutSec, task.MaxRetries)
			s.disableOneTimeTask(task.ID)
		}()
		return nil
	}

	task.timer = time.AfterFunc(delay, func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("[scheduler] panic in one-time task %s: %v", task.ID, r)
			}
		}()
		s.executeTask(task, handler, task.TimeoutSec, task.MaxRetries)
		s.disableOneTimeTask(task.ID)
	})

	// Set NextRunAt for display
	next := time.Now().Add(delay)
	task.NextRunAt = &next
	return nil
}

// disableOneTimeTask disables a one-time task after execution.
func (s *Scheduler) disableOneTimeTask(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if task, ok := s.tasks[id]; ok {
		task.Enabled = false
		task.timer = nil
		task.NextRunAt = nil // one-time task won't run again
		_ = s.saveTasksLocked()
	}
}

// executeTask runs a single task execution with optional retries.
func (s *Scheduler) executeTask(task *ScheduledTask, handler TaskHandler, timeoutSec int, maxRetries int) {
	if !s.areDependenciesMet(task) {
		logger.Infof("[scheduler] task %s (%s) skipped: dependencies not met", task.ID, task.Name)
		s.recordSkippedExecution(task, "dependencies_not_met", "dependency tasks not yet successful")
		return
	}
	if task.ExecutionWindow != nil && !s.isWithinExecutionWindow(task.ExecutionWindow) {
		logger.Infof("[scheduler] task %s (%s) skipped: outside execution window", task.ID, task.Name)
		s.recordSkippedExecution(task, "outside_window", "current time outside execution window")
		return
	}

	record := s.executeTaskWithRetry(task, handler, timeoutSec, maxRetries)
	s.finalizeTaskExecution(task, record)
}

// executeTaskWithRetry 带重试的任务执行
func (s *Scheduler) executeTaskWithRetry(task *ScheduledTask, handler TaskHandler, timeoutSec, maxRetries int) ExecutionRecord {
	now := time.Now()
	taskType := string(task.Type)
	record := ExecutionRecord{
		TaskID: task.ID, TaskName: task.Name, TaskType: taskType,
		StartedAt: now.Format(time.RFC3339),
	}

	cancelled := func() ExecutionRecord {
		record.Status = "failed"
		record.Error = s.ctx.Err().Error()
		record.FinishedAt = time.Now().Format(time.RFC3339)
		record.DurationMs = time.Since(now).Milliseconds()
		return record
	}
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if s.ctx.Err() != nil {
			return cancelled()
		}
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(attempt*2) * time.Second)
			select {
			case <-s.ctx.Done():
				timer.Stop()
				return cancelled()
			case <-timer.C:
			}
			if s.ctx.Err() != nil {
				return cancelled()
			}
			record.RetryCount = attempt
			metrics.IncSchedulerTaskRetry(taskType)
		}
		attemptStarted := time.Now()
		result, err := s.runTaskHandler(handler, task, timeoutSec)
		elapsed := time.Since(now)
		record.FinishedAt = time.Now().Format(time.RFC3339)
		record.DurationMs = elapsed.Milliseconds()

		if err != nil {
			if s.ctx.Err() != nil {
				return cancelled()
			}
			if errors.Is(err, context.DeadlineExceeded) {
				record.Status = "timeout"
				record.Error = fmt.Sprintf("task timed out after %s", time.Since(attemptStarted).Round(time.Millisecond))
			} else {
				record.Status = "failed"
				record.Error = err.Error()
			}
			continue
		}
		record.Status = "success"
		record.Error = ""
		record.Result = result
		break
	}
	return record
}

// ctxKeyTaskName is the context key carrying the executing task's stable name.
// Incremental-push runners (QueryRunner) read it to scope their dedup state to
// the task, so re-creating a task with the same name keeps its pushed set.
type ctxKeyTaskName struct{}

// taskNameFromContext extracts the executing task's name injected by
// runTaskHandler. It is empty for direct (non-scheduled) executions.
func taskNameFromContext(ctx context.Context) string {
	name, _ := ctx.Value(ctxKeyTaskName{}).(string)
	return name
}

// runTaskHandler 执行单次任务 handler（带 panic 恢复和超时）
func (s *Scheduler) runTaskHandler(handler TaskHandler, task *ScheduledTask, timeoutSec int) (string, error) {
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()
	// Scope incremental push state to the task name (stable across re-creation).
	name := task.Name
	if name == "" {
		name = task.ID
	}
	ctx = context.WithValue(ctx, ctxKeyTaskName{}, name)
	var result string
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic in runner: %v", r)
			}
		}()
		result, err = handler.Execute(ctx, task.Payload)
	}()
	// A handler may return nil after its deadline; do not publish that late
	// result as a successful attempt. Cancellation remains cooperative.
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return result, err
}

// finalizeTaskExecution 更新任务状态、记录历史、发送通知
func (s *Scheduler) finalizeTaskExecution(task *ScheduledTask, record ExecutionRecord) {
	metrics.IncSchedulerTaskExecution(record.TaskType, record.Status)
	metrics.ObserveSchedulerTaskExecutionDuration(record.TaskType, time.Duration(record.DurationMs)*time.Millisecond)

	s.mu.Lock()
	if t, ok := s.tasks[task.ID]; ok {
		now := time.Now()
		t.LastRunAt = &now
		if next := s.getNextRunTime(task.ID); !next.IsZero() {
			t.NextRunAt = &next
		}
	}
	s.history = append(s.history, record)
	if len(s.history) > s.maxHistory {
		s.history = s.history[len(s.history)-s.maxHistory:]
	}
	s.mu.Unlock()

	s.updateMetrics()
	if err := s.Save(); err != nil {
		logger.Errorf("[scheduler] persist execution history for task %s: %v", task.ID, err)
	}
	s.sendNotification(task, record)
}

func (s *Scheduler) updateMetrics() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	enabledCount := 0
	for _, t := range s.tasks {
		if t.Enabled {
			enabledCount++
		}
	}
	metrics.SetSchedulerTasksEnabled(enabledCount)
}

// areDependenciesMet checks if all dependency tasks have succeeded in their last execution.
func (s *Scheduler) areDependenciesMet(task *ScheduledTask) bool {
	if len(task.DependsOn) == 0 {
		return true
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, depID := range task.DependsOn {
		_, exists := s.tasks[depID]
		if !exists {
			logger.Warnf("[scheduler] dependency task %s not found for task %s", depID, task.ID)
			return false
		}

		// Find last execution record for this dependency
		lastRecord := s.findLastExecutionRecord(depID)
		if lastRecord == nil || lastRecord.Status != "success" {
			logger.Debugf("[scheduler] dependency task %s last status: %v (need success)", depID, lastRecord)
			return false
		}
	}
	return true
}

// findLastExecutionRecord finds the most recent execution record for a task.
func (s *Scheduler) findLastExecutionRecord(taskID string) *ExecutionRecord {
	for i := len(s.history) - 1; i >= 0; i-- {
		if s.history[i].TaskID == taskID {
			return &s.history[i]
		}
	}
	return nil
}

// isWithinExecutionWindow checks if the current time is within the allowed execution window.
func (s *Scheduler) isWithinExecutionWindow(window *ExecutionWindow) bool {
	now := time.Now()
	if window.Timezone != "" {
		loc, err := time.LoadLocation(window.Timezone)
		if err != nil {
			logger.Warnf("[scheduler] invalid timezone %q, using local time: %v", window.Timezone, err)
		} else {
			now = now.In(loc)
		}
	}

	// Check weekday constraint
	if len(window.Weekdays) > 0 {
		currentWeekday := int(now.Weekday())
		found := false
		for _, wd := range window.Weekdays {
			if wd == currentWeekday {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check hour constraint
	currentHour := now.Hour()
	if window.StartHour <= window.EndHour {
		// Normal range (e.g., 9-17 means 9am to 5pm)
		return currentHour >= window.StartHour && currentHour < window.EndHour
	}
	// Overnight range (e.g., 22-6 means 10pm to 6am next day)
	return currentHour >= window.StartHour || currentHour < window.EndHour
}

// recordSkippedExecution creates a record for a skipped task execution.
func (s *Scheduler) recordSkippedExecution(task *ScheduledTask, status string, reason string) {
	now := time.Now()
	record := ExecutionRecord{
		TaskID:     task.ID,
		TaskName:   task.Name,
		TaskType:   string(task.Type),
		StartedAt:  now.Format(time.RFC3339),
		FinishedAt: now.Format(time.RFC3339),
		DurationMs: 0,
		Status:     "skipped",
		Error:      reason,
	}

	s.mu.Lock()
	s.history = append(s.history, record)
	if len(s.history) > s.maxHistory {
		s.history = s.history[len(s.history)-s.maxHistory:]
	}
	s.mu.Unlock()
	if err := s.Save(); err != nil {
		logger.Errorf("[scheduler] persist skipped execution for task %s: %v", task.ID, err)
	}

	metrics.IncSchedulerTaskExecution(string(task.Type), "skipped")
}

func (s *Scheduler) getNextRunTime(taskID string) time.Time {
	if entryID, ok := s.cronIDs[taskID]; ok {
		return s.cron.Entry(entryID).Next
	}
	return time.Time{}
}

// saveAsync persists data to disk in a background goroutine.
// nolint:unused
func (s *Scheduler) saveAsync() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("scheduler panic in saveAsync: %v", r)
			}
		}()
		if err := s.Save(); err != nil {
			logger.Errorf("[scheduler] saveAsync failed: %v", err)
		}
	}()
}

// Stop gracefully stops the scheduler and all background goroutines.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopping = true
	s.stopped = true
	close(s.stopCh)
	// Stop all one-time task timers
	for _, task := range s.tasks {
		if task.timer != nil {
			task.timer.Stop()
			task.timer = nil
		}
	}
	s.mu.Unlock()

	// 取消所有进行中的任务（context 派生自 s.ctx 的任务将收到取消信号）
	s.cancel()

	// Stop cron and wait for it to finish
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()

	// Wait for notification goroutines to finish
	s.notifyWg.Wait()
}

// generateID creates a short unique ID using a monotonic counter.
func (s *Scheduler) generateID() string {
	return uuid.New().String()
}
