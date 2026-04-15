package distributed

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	TaskStatusPending   = "pending"
	TaskStatusClaimed   = "claimed"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
)

// TaskEnvelope is the controller-side task description used by node workers.
type TaskEnvelope struct {
	TaskID         string                 `json:"task_id"`
	TaskType       string                 `json:"task_type"`
	Payload        map[string]interface{} `json:"payload,omitempty"`
	Priority       int                    `json:"priority,omitempty"`
	TimeoutSeconds int                    `json:"timeout_seconds,omitempty"`
	TraceID        string                 `json:"trace_id,omitempty"`
	RequiredCaps   []string               `json:"required_caps,omitempty"`
	MaxReassign    int                    `json:"max_reassign,omitempty"`
}

// TaskResult is the node callback payload for task completion.
type TaskResult struct {
	TaskID     string                 `json:"task_id"`
	NodeID     string                 `json:"node_id"`
	Status     string                 `json:"status"`
	DurationMS int64                  `json:"duration_ms,omitempty"`
	Output     map[string]interface{} `json:"output,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Retryable  bool                   `json:"retryable,omitempty"` // 是否可重试
}

// RetryRecord 重试记录
type RetryRecord struct {
	Attempt     int       `json:"attempt"`
	NodeID      string    `json:"node_id"`
	Error       string    `json:"error"`
	RetryDelay  int64     `json:"retry_delay_ms"`
	NextRetryAt time.Time `json:"next_retry_at"`
	Timestamp   time.Time `json:"timestamp"`
}

// TaskRecord is the queue-side task state.
type TaskRecord struct {
	TaskID         string                 `json:"task_id"`
	TaskType       string                 `json:"task_type"`
	Payload        map[string]interface{} `json:"payload,omitempty"`
	Priority       int                    `json:"priority"`
	TimeoutSeconds int                    `json:"timeout_seconds"`
	TraceID        string                 `json:"trace_id,omitempty"`
	RequiredCaps   []string               `json:"required_caps,omitempty"`
	Status         string                 `json:"status"`
	AssignedNode   string                 `json:"assigned_node,omitempty"`
	Attempt        int                    `json:"attempt"`
	MaxReassign    int                    `json:"max_reassign"`
	LeaseUntil     time.Time              `json:"lease_until,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	LastError      string                 `json:"last_error,omitempty"`
	Result         map[string]interface{} `json:"result,omitempty"`

	// 重试相关字段
	Retryable    bool          `json:"retryable"`               // 是否可重试
	RetryDelay   time.Duration `json:"retry_delay,omitempty"`   // 重试延迟
	NextRetryAt  time.Time     `json:"next_retry_at,omitempty"` // 下次重试时间
	RetryHistory []RetryRecord `json:"retry_history,omitempty"` // 重试历史
}

// TaskQueue stores distributed tasks in memory and supports node claim/result workflow.
type TaskQueue struct {
	tasks              map[string]*TaskRecord
	pending            []string
	leaseJitter        time.Duration
	defaultMaxReassign int
	scheduler          Scheduler
	mu                 sync.Mutex
	stopChan           chan struct{}
	stopped            bool
}

func NewTaskQueue() *TaskQueue {
	q := &TaskQueue{
		tasks:              make(map[string]*TaskRecord),
		pending:            make([]string, 0),
		leaseJitter:        2 * time.Second,
		defaultMaxReassign: 1,
		stopChan:           make(chan struct{}),
	}
	go q.startBackgroundRecycle()
	return q
}

// Stop stops the background recycle goroutine
func (q *TaskQueue) Stop() {
	q.mu.Lock()
	if !q.stopped {
		q.stopped = true
		close(q.stopChan)
	}
	q.mu.Unlock()
}

// startBackgroundRecycle periodically recycles expired tasks
func (q *TaskQueue) startBackgroundRecycle() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-q.stopChan:
			return
		case <-ticker.C:
			q.mu.Lock()
			q.recycleExpiredLocked()
			q.mu.Unlock()
		}
	}
}

func (q *TaskQueue) SetDefaultMaxReassign(v int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.defaultMaxReassign = normalizeMaxReassign(v)
}

// SetScheduler sets the task scheduler for selecting tasks during claim
func (q *TaskQueue) SetScheduler(s Scheduler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.scheduler = s
}

func (q *TaskQueue) Enqueue(env TaskEnvelope) (TaskRecord, error) {
	taskID := strings.TrimSpace(env.TaskID)
	if taskID == "" {
		return TaskRecord{}, fmt.Errorf("task_id is required")
	}
	if strings.TrimSpace(env.TaskType) == "" {
		return TaskRecord{}, fmt.Errorf("task_type is required")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.tasks[taskID]; exists {
		return TaskRecord{}, fmt.Errorf("task already exists")
	}

	now := time.Now()
	rec := &TaskRecord{
		TaskID:         taskID,
		TaskType:       strings.TrimSpace(env.TaskType),
		Payload:        cloneMap(env.Payload),
		Priority:       env.Priority,
		TimeoutSeconds: normalizedTimeoutSeconds(env.TimeoutSeconds),
		TraceID:        strings.TrimSpace(env.TraceID),
		RequiredCaps:   normalizeCaps(env.RequiredCaps),
		Status:         TaskStatusPending,
		Attempt:        0,
		MaxReassign:    q.resolveMaxReassign(env.MaxReassign),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	q.tasks[taskID] = rec
	q.pending = append(q.pending, taskID)
	q.sortPendingLocked()
	return *rec, nil
}

func (q *TaskQueue) Claim(nodeID string, nodeCaps []string) (*TaskRecord, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("node_id is required")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	q.recycleExpiredLocked()
	if len(q.pending) == 0 {
		return nil, nil
	}

	caps := make(map[string]struct{}, len(nodeCaps))
	for _, c := range nodeCaps {
		v := strings.TrimSpace(c)
		if v == "" {
			continue
		}
		caps[v] = struct{}{}
	}

	now := time.Now()

	for idx, taskID := range q.pending {
		rec := q.tasks[taskID]
		if rec == nil || rec.Status != TaskStatusPending {
			continue
		}

		// 检查是否到达重试时间
		if !rec.NextRetryAt.IsZero() && now.Before(rec.NextRetryAt) {
			continue
		}

		if !canHandleCaps(rec.RequiredCaps, caps) {
			continue
		}

		rec.Status = TaskStatusClaimed
		rec.AssignedNode = nodeID
		rec.LeaseUntil = now.Add(time.Duration(rec.TimeoutSeconds)*time.Second + q.leaseJitter)
		rec.UpdatedAt = now
		// 清除重试相关状态，准备重新执行
		rec.NextRetryAt = time.Time{}
		rec.RetryDelay = 0

		q.pending = append(q.pending[:idx], q.pending[idx+1:]...)
		copyRec := *rec
		return &copyRec, nil
	}

	return nil, nil
}

// ClaimWithNode claims a task using the configured scheduler and full node record.
// When no scheduler is set, falls back to built-in priority-based selection.
func (q *TaskQueue) ClaimWithNode(node *NodeRecord) (*TaskRecord, error) {
	if node == nil {
		return nil, fmt.Errorf("node is required")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	q.recycleExpiredLocked()
	if len(q.pending) == 0 {
		return nil, nil
	}

	// Skip nodes that are offline or in critical health
	if !node.Online || node.HealthStatus == "critical" {
		return nil, nil
	}

	// Gather eligible pending tasks
	eligible := make([]*TaskRecord, 0, len(q.pending))
	now := time.Now()
	for _, taskID := range q.pending {
		rec := q.tasks[taskID]
		if rec == nil || rec.Status != TaskStatusPending {
			continue
		}
		if !rec.NextRetryAt.IsZero() && now.Before(rec.NextRetryAt) {
			continue
		}
		if !canHandleCaps(rec.RequiredCaps, makeCapSet(node.Capabilities)) {
			continue
		}
		eligible = append(eligible, rec)
	}

	if len(eligible) == 0 {
		return nil, nil
	}

	var selected *TaskRecord
	if q.scheduler != nil {
		selected = q.scheduler.SelectTask(eligible, node)
	} else {
		// Fallback: select highest priority, earliest created task
		selected = eligible[0]
		for _, t := range eligible[1:] {
			if t.Priority > selected.Priority ||
				(t.Priority == selected.Priority && t.CreatedAt.Before(selected.CreatedAt)) {
				selected = t
			}
		}
	}

	if selected == nil {
		return nil, nil
	}

	// Mark task as claimed
	selected.Status = TaskStatusClaimed
	selected.AssignedNode = node.NodeID
	selected.LeaseUntil = now.Add(time.Duration(selected.TimeoutSeconds)*time.Second + q.leaseJitter)
	selected.UpdatedAt = now
	selected.NextRetryAt = time.Time{}
	selected.RetryDelay = 0

	// Remove from pending list
	for i, id := range q.pending {
		if id == selected.TaskID {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			break
		}
	}

	copyRec := *selected
	return &copyRec, nil
}

func makeCapSet(caps []string) map[string]struct{} {
	result := make(map[string]struct{}, len(caps))
	for _, c := range caps {
		v := strings.TrimSpace(c)
		if v != "" {
			result[v] = struct{}{}
		}
	}
	return result
}

func (q *TaskQueue) SubmitResult(res TaskResult) (TaskRecord, error) {
	taskID := strings.TrimSpace(res.TaskID)
	nodeID := strings.TrimSpace(res.NodeID)
	status := normalizeResultStatus(res.Status)
	if taskID == "" {
		return TaskRecord{}, fmt.Errorf("task_id is required")
	}
	if nodeID == "" {
		return TaskRecord{}, fmt.Errorf("node_id is required")
	}
	if status == "" {
		return TaskRecord{}, fmt.Errorf("status must be completed or failed")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	rec, exists := q.tasks[taskID]
	if !exists {
		return TaskRecord{}, fmt.Errorf("task not found")
	}
	if rec.Status == TaskStatusCompleted || rec.Status == TaskStatusFailed {
		return *rec, nil
	}
	if strings.TrimSpace(rec.AssignedNode) != "" && rec.AssignedNode != nodeID {
		return TaskRecord{}, fmt.Errorf("task assigned to another node")
	}

	now := time.Now()

	if status == TaskStatusCompleted {
		// 任务成功完成
		rec.Status = status
		rec.Result = cloneMap(res.Output)
		rec.UpdatedAt = now
		rec.LeaseUntil = time.Time{}
	} else if status == TaskStatusFailed {
		// 任务失败
		rec.LastError = strings.TrimSpace(res.Error)
		rec.UpdatedAt = now
		rec.LeaseUntil = time.Time{}

		// 检查是否应该重试
		if res.Retryable && rec.Attempt < rec.MaxReassign {
			// 计算重试延迟（指数退避）
			retryDelay := q.calculateRetryDelay(rec.Attempt)
			nextRetryAt := now.Add(retryDelay)

			// 创建重试记录
			retryRecord := RetryRecord{
				Attempt:     rec.Attempt,
				NodeID:      nodeID,
				Error:       rec.LastError,
				RetryDelay:  int64(retryDelay / time.Millisecond),
				NextRetryAt: nextRetryAt,
				Timestamp:   now,
			}

			// 更新任务状态为等待重试
			rec.Status = TaskStatusPending
			rec.AssignedNode = ""
			rec.Retryable = true
			rec.RetryDelay = retryDelay
			rec.NextRetryAt = nextRetryAt
			rec.RetryHistory = append(rec.RetryHistory, retryRecord)
			rec.Attempt++

			// 将任务重新加入待处理队列
			q.pending = append(q.pending, taskID)
			q.dedupPendingLocked()
			q.sortPendingLocked()
		} else {
			// 达到最大重试次数或不可重试，标记为失败
			rec.Status = TaskStatusFailed
		}
	}

	return *rec, nil
}

func (q *TaskQueue) Snapshot() []TaskRecord {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.recycleExpiredLocked()
	out := make([]TaskRecord, 0, len(q.tasks))
	for _, rec := range q.tasks {
		out = append(out, *rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].TaskID < out[j].TaskID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// Get retrieves a single task by ID
func (q *TaskQueue) Get(taskID string) (*TaskRecord, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	rec, exists := q.tasks[taskID]
	if !exists {
		return nil, nil
	}
	copyRec := *rec
	return &copyRec, nil
}

// Delete removes a task from the queue
func (q *TaskQueue) Delete(taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("task_id is required")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	rec, exists := q.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found")
	}

	// Remove from pending list if present
	for i, id := range q.pending {
		if id == taskID {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			break
		}
	}

	delete(q.tasks, taskID)
	_ = rec // suppress unused variable warning
	return nil
}

// ReleaseNodeTasks releases all claimed tasks for a specific node (used when node goes offline)
func (q *TaskQueue) ReleaseNodeTasks(nodeID string) int {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return 0
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	released := 0
	now := time.Now()
	for _, rec := range q.tasks {
		if rec.Status != TaskStatusClaimed {
			continue
		}
		if rec.AssignedNode != nodeID {
			continue
		}

		// Re-queue the task
		if rec.Attempt <= rec.MaxReassign+1 {
			rec.Status = TaskStatusPending
			rec.AssignedNode = ""
			rec.LeaseUntil = time.Time{}
			rec.UpdatedAt = now
			q.pending = append(q.pending, rec.TaskID)
			released++
		} else {
			// Max reassign exceeded, mark as failed
			rec.Status = TaskStatusFailed
			rec.LastError = "node offline and max reassign exceeded"
			rec.UpdatedAt = now
		}
	}
	if released > 0 {
		q.sortPendingLocked()
	}
	return released
}

func (q *TaskQueue) recycleExpiredLocked() {
	now := time.Now()
	for _, rec := range q.tasks {
		if rec == nil || rec.Status != TaskStatusClaimed {
			continue
		}
		if rec.LeaseUntil.IsZero() || now.Before(rec.LeaseUntil) {
			continue
		}
		if rec.Attempt > rec.MaxReassign+1 {
			rec.Status = TaskStatusFailed
			rec.LastError = "lease expired and max reassign exceeded"
			rec.UpdatedAt = now
			continue
		}
		rec.Status = TaskStatusPending
		rec.AssignedNode = ""
		rec.LeaseUntil = time.Time{}
		rec.UpdatedAt = now
		q.pending = append(q.pending, rec.TaskID)
	}
	q.dedupPendingLocked()
	q.sortPendingLocked()
}

func (q *TaskQueue) dedupPendingLocked() {
	seen := make(map[string]bool, len(q.pending))
	deduped := q.pending[:0]
	for _, id := range q.pending {
		if !seen[id] {
			seen[id] = true
			deduped = append(deduped, id)
		}
	}
	q.pending = deduped
}

func (q *TaskQueue) sortPendingLocked() {
	sort.SliceStable(q.pending, func(i, j int) bool {
		a := q.tasks[q.pending[i]]
		b := q.tasks[q.pending[j]]
		if a == nil || b == nil {
			return q.pending[i] < q.pending[j]
		}
		if a.Priority == b.Priority {
			if a.CreatedAt.Equal(b.CreatedAt) {
				return a.TaskID < b.TaskID
			}
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.Priority > b.Priority
	})
}

func canHandleCaps(required []string, caps map[string]struct{}) bool {
	if len(required) == 0 {
		return true
	}
	for _, req := range required {
		if _, ok := caps[req]; !ok {
			return false
		}
	}
	return true
}

func normalizeResultStatus(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	if v == TaskStatusCompleted || v == TaskStatusFailed {
		return v
	}
	return ""
}

func normalizeCaps(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, item := range in {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

func normalizeMaxReassign(v int) int {
	if v < 0 {
		return 0
	}
	if v > 3 {
		return 3
	}
	return v
}

func (q *TaskQueue) resolveMaxReassign(v int) int {
	if v <= 0 {
		return normalizeMaxReassign(q.defaultMaxReassign)
	}
	return normalizeMaxReassign(v)
}

func normalizedTimeoutSeconds(v int) int {
	if v <= 0 {
		return 30
	}
	if v > 600 {
		return 600
	}
	return v
}

// calculateRetryDelay 计算重试延迟（指数退避）
func (q *TaskQueue) calculateRetryDelay(attempt int) time.Duration {
	// 基础延迟：1秒
	baseDelay := 1 * time.Second

	// 最大延迟：60秒
	maxDelay := 60 * time.Second

	// 指数退避：2^attempt * baseDelay，加上随机抖动
	delay := baseDelay * time.Duration(1<<uint(attempt))

	// 添加随机抖动（±20%）
	// 使用时间戳作为随机源
	now := time.Now()
	jitterFactor := float64(now.UnixNano()%100) / 100.0 // 0.0-1.0
	jitter := time.Duration(float64(delay) * 0.2 * jitterFactor)

	if now.UnixNano()%2 == 0 {
		delay += jitter
	} else {
		delay -= jitter
	}

	// 限制最大延迟
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
