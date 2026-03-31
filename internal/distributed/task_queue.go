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
}

// TaskQueue stores distributed tasks in memory and supports node claim/result workflow.
type TaskQueue struct {
	tasks              map[string]*TaskRecord
	pending            []string
	leaseJitter        time.Duration
	defaultMaxReassign int
	mu                 sync.Mutex
}

func NewTaskQueue() *TaskQueue {
	return &TaskQueue{tasks: make(map[string]*TaskRecord), pending: make([]string, 0), leaseJitter: 2 * time.Second, defaultMaxReassign: 1}
}

func (q *TaskQueue) SetDefaultMaxReassign(v int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.defaultMaxReassign = normalizeMaxReassign(v)
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

	for idx, taskID := range q.pending {
		rec := q.tasks[taskID]
		if rec == nil || rec.Status != TaskStatusPending {
			continue
		}
		if !canHandleCaps(rec.RequiredCaps, caps) {
			continue
		}

		rec.Status = TaskStatusClaimed
		rec.AssignedNode = nodeID
		rec.Attempt++
		rec.UpdatedAt = time.Now()
		rec.LeaseUntil = rec.UpdatedAt.Add(time.Duration(rec.TimeoutSeconds)*time.Second + q.leaseJitter)

		q.pending = append(q.pending[:idx], q.pending[idx+1:]...)
		copyRec := *rec
		return &copyRec, nil
	}

	return nil, nil
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

	rec.Status = status
	rec.LastError = strings.TrimSpace(res.Error)
	rec.Result = cloneMap(res.Output)
	rec.UpdatedAt = time.Now()
	rec.LeaseUntil = time.Time{}
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
	q.sortPendingLocked()
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
