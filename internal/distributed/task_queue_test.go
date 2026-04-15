package distributed

import (
	"testing"
	"time"
)

func TestTaskQueueEnqueueClaimResult(t *testing.T) {
	q := NewTaskQueue()

	_, err := q.Enqueue(TaskEnvelope{
		TaskID:       "task-1",
		TaskType:     "port_scan",
		Priority:     10,
		RequiredCaps: []string{"port_scan"},
	})
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	rec, err := q.Claim("node-a", []string{"port_scan"})
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if rec == nil || rec.TaskID != "task-1" {
		t.Fatalf("expected task-1 to be claimed, got %+v", rec)
	}
	if rec.Status != TaskStatusClaimed {
		t.Fatalf("expected status claimed, got %s", rec.Status)
	}

	updated, err := q.SubmitResult(TaskResult{TaskID: "task-1", NodeID: "node-a", Status: "completed"})
	if err != nil {
		t.Fatalf("submit result failed: %v", err)
	}
	if updated.Status != TaskStatusCompleted {
		t.Fatalf("expected completed, got %s", updated.Status)
	}
}

func TestTaskQueueCapabilityFilter(t *testing.T) {
	q := NewTaskQueue()
	_, _ = q.Enqueue(TaskEnvelope{TaskID: "task-a", TaskType: "screenshot", RequiredCaps: []string{"screenshot"}})
	_, _ = q.Enqueue(TaskEnvelope{TaskID: "task-b", TaskType: "port_scan", RequiredCaps: []string{"port_scan"}})

	rec, err := q.Claim("node-x", []string{"port_scan"})
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if rec == nil || rec.TaskID != "task-b" {
		t.Fatalf("expected task-b, got %+v", rec)
	}
}

func TestTaskQueueClaimWithNode_NoScheduler(t *testing.T) {
	q := NewTaskQueue()
	// No scheduler set — should use fallback priority selection

	now := time.Now()
	_, _ = q.Enqueue(TaskEnvelope{TaskID: "low-pri", TaskType: "scan", Priority: 1})
	_, _ = q.Enqueue(TaskEnvelope{TaskID: "high-pri", TaskType: "scan", Priority: 10})
	// Fix creation order for second task
	q.mu.Lock()
	q.tasks["low-pri"].CreatedAt = now
	q.tasks["high-pri"].CreatedAt = now.Add(time.Millisecond)
	q.mu.Unlock()

	node := &NodeRecord{
		NodeID:       "node-1",
		Online:       true,
		HealthStatus: "healthy",
		Capabilities: []string{"scan"},
	}

	rec, err := q.ClaimWithNode(node)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	// Without scheduler, should pick highest priority
	if rec == nil || rec.TaskID != "high-pri" {
		t.Fatalf("expected high-pri, got %+v", rec)
	}
}

func TestTaskQueueClaimWithNode_HealthLoadScheduler(t *testing.T) {
	q := NewTaskQueue()
	sched := NewHealthLoadScheduler()
	q.SetScheduler(sched)

	_, _ = q.Enqueue(TaskEnvelope{TaskID: "task-a", TaskType: "scan", Priority: 5, RequiredCaps: []string{"scan"}})
	_, _ = q.Enqueue(TaskEnvelope{TaskID: "task-b", TaskType: "scan", Priority: 5, RequiredCaps: []string{"scan"}})

	node := &NodeRecord{
		NodeID:       "node-1",
		Online:       true,
		HealthStatus: "healthy",
		Capabilities: []string{"scan"},
	}

	rec, err := q.ClaimWithNode(node)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if rec == nil {
		t.Fatal("expected a task")
	}
	if rec.Status != TaskStatusClaimed {
		t.Fatalf("expected claimed status, got %s", rec.Status)
	}
	if rec.AssignedNode != "node-1" {
		t.Fatalf("expected node-1, got %s", rec.AssignedNode)
	}
}

func TestTaskQueueClaimWithNode_OfflineNodeRejected(t *testing.T) {
	q := NewTaskQueue()
	_, _ = q.Enqueue(TaskEnvelope{TaskID: "task-1", TaskType: "scan"})

	node := &NodeRecord{
		NodeID:       "node-off",
		Online:       false,
		HealthStatus: "offline",
	}

	rec, err := q.ClaimWithNode(node)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if rec != nil {
		t.Fatalf("expected nil for offline node, got %+v", rec)
	}
}

func TestTaskQueueClaimWithNode_CriticalHealthRejected(t *testing.T) {
	q := NewTaskQueue()
	_, _ = q.Enqueue(TaskEnvelope{TaskID: "task-1", TaskType: "scan"})

	node := &NodeRecord{
		NodeID:       "node-crit",
		Online:       true,
		HealthStatus: "critical",
	}

	rec, err := q.ClaimWithNode(node)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if rec != nil {
		t.Fatalf("expected nil for critical node, got %+v", rec)
	}
}

func TestTaskQueueClaimWithNode_CapabilityMismatch(t *testing.T) {
	q := NewTaskQueue()
	sched := NewHealthLoadScheduler()
	q.SetScheduler(sched)

	_, _ = q.Enqueue(TaskEnvelope{TaskID: "task-req", TaskType: "scan", RequiredCaps: []string{"gpu"}})

	node := &NodeRecord{
		NodeID:       "node-no-gpu",
		Online:       true,
		HealthStatus: "healthy",
		Capabilities: []string{"cpu"},
	}

	rec, err := q.ClaimWithNode(node)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if rec != nil {
		t.Fatalf("expected nil for capability mismatch, got %+v", rec)
	}
}

func TestTaskQueueClaimWithNode_NilNode(t *testing.T) {
	q := NewTaskQueue()
	_, err := q.ClaimWithNode(nil)
	if err == nil {
		t.Fatal("expected error for nil node")
	}
}

func TestTaskQueueClaimWithNode_RemovedFromPending(t *testing.T) {
	q := NewTaskQueue()
	_, _ = q.Enqueue(TaskEnvelope{TaskID: "task-1", TaskType: "scan"})

	node := &NodeRecord{
		NodeID:       "node-1",
		Online:       true,
		HealthStatus: "healthy",
		Capabilities: []string{"scan"},
	}

	_, _ = q.ClaimWithNode(node)

	// Second claim should return nil (task already claimed)
	rec, _ := q.ClaimWithNode(node)
	if rec != nil {
		t.Fatalf("expected no task after first claim, got %+v", rec)
	}
}

func TestTaskQueueClaimWithNode_SchedulerPriorityScheduler(t *testing.T) {
	q := NewTaskQueue()
	q.SetScheduler(NewPriorityScheduler())

	_, _ = q.Enqueue(TaskEnvelope{TaskID: "low", TaskType: "scan", Priority: 1})
	_, _ = q.Enqueue(TaskEnvelope{TaskID: "high", TaskType: "scan", Priority: 100})

	node := &NodeRecord{
		NodeID:       "node-1",
		Online:       true,
		HealthStatus: "healthy",
	}

	rec, err := q.ClaimWithNode(node)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if rec == nil || rec.TaskID != "high" {
		t.Fatalf("expected 'high', got %+v", rec)
	}
}
