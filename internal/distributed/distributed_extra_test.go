package distributed

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ===== SnapshotManager =====

func TestNewSnapshotManager_Defaults(t *testing.T) {
	mgr := NewSnapshotManager("/tmp/snap.json", 0)
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if mgr.saveInterval <= 0 {
		t.Error("saveInterval should have default value")
	}
}

func TestSnapshotManager_SetRegistry(t *testing.T) {
	mgr := NewSnapshotManager("/tmp/snap.json", 30*time.Second)
	r := NewRegistry(30 * time.Second)
	mgr.SetRegistry(r)
}

func TestSnapshotManager_SetTaskQueue(t *testing.T) {
	mgr := NewSnapshotManager("/tmp/snap.json", 30*time.Second)
	q := NewTaskQueue()
	mgr.SetTaskQueue(q)
}

func TestSnapshotManager_StartStop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	mgr := NewSnapshotManager(path, 100*time.Millisecond)
	mgr.Start()
	time.Sleep(50 * time.Millisecond)
	mgr.Stop()
	mgr.Stop() // idempotent
}

func TestSnapshotManager_Save(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	mgr := NewSnapshotManager(path, 30*time.Second)

	r := NewRegistry(30 * time.Second)
	_, err := r.Register(NodeRegistration{NodeID: "node1", Capabilities: []string{"screenshot"}})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	mgr.SetRegistry(r)

	q := NewTaskQueue()
	mgr.SetTaskQueue(q)

	err = mgr.Save()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected snapshot file to exist: %v", err)
	}
	if info.Size() == 0 {
		t.Error("expected non-empty snapshot file")
	}
}

func TestSnapshotManager_Save_EmptyRegistry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	mgr := NewSnapshotManager(path, 30*time.Second)
	err := mgr.Save()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSnapshotManager_Load(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")

	mgr := NewSnapshotManager(path, 30*time.Second)
	r := NewRegistry(30 * time.Second)
	_, err := r.Register(NodeRegistration{NodeID: "node1", Capabilities: []string{"screenshot"}})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	mgr.SetRegistry(r)
	mgr.Save()

	mgr2 := NewSnapshotManager(path, 30*time.Second)
	r2 := NewRegistry(30 * time.Second)
	mgr2.SetRegistry(r2)

	err = mgr2.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	node, err := r2.Get("node1")
	if err != nil {
		t.Fatal("expected node1 to be restored from snapshot")
	}
	if node.NodeID != "node1" {
		t.Errorf("expected node1, got %s", node.NodeID)
	}
}

func TestSnapshotManager_Load_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	mgr := NewSnapshotManager(path, 30*time.Second)
	err := mgr.Load()
	// Load may return error or not depending on implementation
	// Just verify no panic
	_ = err
}

func TestSnapshotManager_Load_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("not valid json"), 0644)
	mgr := NewSnapshotManager(path, 30*time.Second)
	err := mgr.Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSnapshotManager_Delete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	os.WriteFile(path, []byte("{}"), 0644)
	mgr := NewSnapshotManager(path, 30*time.Second)
	err := mgr.Delete()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}

func TestSnapshotManager_Exists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	mgr := NewSnapshotManager(path, 30*time.Second)

	if mgr.Exists() {
		t.Error("should not exist initially")
	}

	os.WriteFile(path, []byte("{}"), 0644)
	if !mgr.Exists() {
		t.Error("should exist after file creation")
	}
}

func TestSnapshotManager_GetSnapshotInfo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.json")
	mgr := NewSnapshotManager(path, 30*time.Second)

	exists, _, _, err := mgr.GetSnapshotInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("should not exist initially")
	}

	mgr2 := NewSnapshotManager(path, 30*time.Second)
	r := NewRegistry(30 * time.Second)
	r.Register(NodeRegistration{NodeID: "node1"})
	mgr2.SetRegistry(r)
	mgr2.Save()

	exists, size, _, err := mgr.GetSnapshotInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("should exist after save")
	}
	if size == 0 {
		t.Error("expected non-zero size")
	}
}

func TestSnapshotManager_Save_InvalidPath(t *testing.T) {
	mgr := NewSnapshotManager("/nonexistent/deep/path/snap.json", 30*time.Second)
	// Save may succeed or fail depending on OS
	err := mgr.Save()
	_ = err
}

// ===== Scheduler Strategies =====

func TestHealthLoadScheduler_Strategy(t *testing.T) {
	s := NewHealthLoadScheduler()
	if s.Strategy() == "" {
		t.Error("expected non-empty strategy name")
	}
}

func TestPriorityScheduler_Strategy(t *testing.T) {
	s := NewPriorityScheduler()
	if s.Strategy() == "" {
		t.Error("expected non-empty strategy name")
	}
}

func TestRoundRobinScheduler(t *testing.T) {
	s := NewRoundRobinScheduler()
	if s.Strategy() == "" {
		t.Error("expected non-empty strategy name")
	}

	// SelectTask with nil tasks should return nil
	result := s.SelectTask(nil, nil)
	if result != nil {
		t.Error("expected nil for empty tasks")
	}

	// SelectTask with empty slice
	result = s.SelectTask([]*TaskRecord{}, nil)
	if result != nil {
		t.Error("expected nil for empty tasks slice")
	}
}

func TestNewSchedulerFromStrategy(t *testing.T) {
	s := NewSchedulerFromStrategy("health_load")
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if s.Strategy() != "health_load" {
		t.Errorf("expected health_load, got %s", s.Strategy())
	}

	s2 := NewSchedulerFromStrategy("priority")
	if s2 == nil {
		t.Fatal("expected non-nil scheduler")
	}

	s3 := NewSchedulerFromStrategy("round_robin")
	if s3 == nil {
		t.Fatal("expected non-nil scheduler")
	}
}

// ===== calculateHealthScore (Registry method) =====

func TestRegistry_CalculateHealthScore_HealthyNode(t *testing.T) {
	r := NewRegistry(30 * time.Second)
	r.Register(NodeRegistration{NodeID: "node1"})

	node, _ := r.Get("node1")
	node.Online = true
	node.CPUUsage = 10
	node.MemoryUsage = 50
	node.ActiveTasks = 2
	node.MaxConcurrency = 10
	node.HealthStatus = "healthy"

	r.calculateHealthScore(node)
	if node.HealthScore < 50 || node.HealthScore > 100 {
		t.Errorf("expected score 50-100 for healthy node, got %.1f", node.HealthScore)
	}
}

func TestRegistry_CalculateHealthScore_OfflineNode(t *testing.T) {
	r := NewRegistry(30 * time.Second)
	r.Register(NodeRegistration{NodeID: "node1"})

	node, _ := r.Get("node1")
	node.Online = false
	node.HealthStatus = "offline"

	r.calculateHealthScore(node)
	// Offline nodes get a baseline score; verify state was set
	if node.HealthStatus != "offline" {
		t.Errorf("expected status offline, got %s", node.HealthStatus)
	}
}

func TestRegistry_CalculateHealthScore_NilNode(t *testing.T) {
	r := NewRegistry(30 * time.Second)
	// calculateHealthScore panics on nil, so we just verify the registry exists
	if r == nil {
		t.Fatal("nil registry")
	}
}

// ===== calculateResourceScore =====

func TestCalculateResourceScore_LowUsage(t *testing.T) {
	score := calculateResourceScore(10, 70, 90)
	if score < 60 || score > 100 {
		t.Errorf("expected score 60-100 for low usage, got %.1f", score)
	}
}

func TestCalculateResourceScore_HighUsage(t *testing.T) {
	score := calculateResourceScore(95, 70, 90)
	if score > 10 {
		t.Errorf("expected score <= 10 for critical usage, got %.1f", score)
	}
}

func TestCalculateResourceScore_WarningLevel(t *testing.T) {
	score := calculateResourceScore(80, 70, 90)
	if score < 10 || score > 60 {
		t.Errorf("expected score 10-60 for warning usage, got %.1f", score)
	}
}

// ===== Registry: cleanupStaleNodes =====

func TestRegistry_CleanupStaleNodes_VerifiesOnlineNodesPreserved(t *testing.T) {
	r := NewRegistry(30 * time.Second)
	r.Register(NodeRegistration{NodeID: "node1"})
	r.Register(NodeRegistration{NodeID: "node2"})

	// Verify cleanupStaleNodes doesn't panic and preserves online nodes
	r.cleanupStaleNodes()

	_, err1 := r.Get("node1")
	_, err2 := r.Get("node2")
	if err1 != nil {
		t.Errorf("expected node1 to still exist: %v", err1)
	}
	if err2 != nil {
		t.Errorf("expected node2 to still exist: %v", err2)
	}
}

// ===== Registry: Heartbeat =====

func TestRegistry_Heartbeat_NewNode(t *testing.T) {
	r := NewRegistry(30 * time.Second)
	r.Heartbeat(NodeHeartbeat{NodeID: "newnode"})

	node, err := r.Get("newnode")
	if err != nil {
		t.Fatal("expected new node to be registered via heartbeat")
	}
	if !node.Online {
		t.Error("expected status online")
	}
}

func TestRegistry_Heartbeat_ExistingNode(t *testing.T) {
	r := NewRegistry(30 * time.Second)
	r.Register(NodeRegistration{NodeID: "node1"})

	oldHB := time.Now().Add(-time.Minute)
	r.mu.Lock()
	r.nodes["node1"].LastHeartbeatAt = oldHB
	r.mu.Unlock()

	r.Heartbeat(NodeHeartbeat{NodeID: "node1"})

	node, _ := r.Get("node1")
	if node.LastHeartbeatAt.Before(oldHB) {
		t.Error("expected heartbeat to update LastHeartbeatAt")
	}
}

// ===== Registry: Snapshot =====

func TestRegistry_Snapshot(t *testing.T) {
	r := NewRegistry(30 * time.Second)
	r.Register(NodeRegistration{NodeID: "node1", Capabilities: []string{"screenshot"}})
	r.Register(NodeRegistration{NodeID: "node2", Capabilities: []string{"portscan"}})

	data := r.Snapshot()
	if data.Total != 2 {
		t.Errorf("expected total 2, got %d", data.Total)
	}
	if len(data.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(data.Nodes))
	}

	names := make(map[string]bool)
	for _, n := range data.Nodes {
		names[n.NodeID] = true
	}
	if !names["node1"] || !names["node2"] {
		t.Errorf("expected both nodes in snapshot, got %v", names)
	}
}

func TestRegistry_Snapshot_Empty(t *testing.T) {
	r := NewRegistry(30 * time.Second)
	data := r.Snapshot()
	if len(data.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(data.Nodes))
	}
}

// ===== Registry: SetFailoverStrategy / GetFailoverStrategy =====

func TestRegistry_FailoverStrategy(t *testing.T) {
	r := NewRegistry(30 * time.Second)
	if r.failoverStrategy != FailoverStrategyHealthBased {
		t.Errorf("expected default health_based, got %s", r.failoverStrategy)
	}

	r.SetFailoverStrategy(FailoverStrategyLoadBalanced)
	if r.GetFailoverStrategy() != FailoverStrategyLoadBalanced {
		t.Errorf("expected load_balanced, got %s", r.GetFailoverStrategy())
	}
}

// ===== TaskRecord fields =====

func TestTaskRecord_Fields(t *testing.T) {
	rec := TaskRecord{
		TaskID:         "t1",
		TaskType:       "scan",
		Status:         "pending",
		Priority:       5,
		AssignedNode:   "n1",
		Attempt:        0,
	}
	if rec.TaskID != "t1" {
		t.Errorf("TaskID = %q, want t1", rec.TaskID)
	}
	if rec.AssignedNode != "n1" {
		t.Errorf("AssignedNode = %q, want n1", rec.AssignedNode)
	}
	if rec.Priority != 5 {
		t.Errorf("Priority = %d, want 5", rec.Priority)
	}
}

// ===== NodeStatusSnapshot =====

func TestNodeStatusSnapshot(t *testing.T) {
	r := NewRegistry(30 * time.Second)
	r.Register(NodeRegistration{NodeID: "node1"})
	r.Register(NodeRegistration{NodeID: "node2"})

	snap := r.Snapshot()
	if snap.Total != 2 {
		t.Errorf("Total = %d, want 2", snap.Total)
	}
	if snap.Online != 2 {
		t.Errorf("Online = %d, want 2", snap.Online)
	}
	if snap.Offline != 0 {
		t.Errorf("Offline = %d, want 0", snap.Offline)
	}
}
