package distributed

import (
	"fmt"
	"testing"
	"time"
)

func TestFailover_NodeGoesOffline_ReleasesTasks(t *testing.T) {
	// 1. 创建 registry 和 task queue
	registry := NewRegistry(100 * time.Millisecond) // 短超时加速测试
	defer registry.Stop()

	taskQueue := NewTaskQueue()
	defer taskQueue.Stop()

	registry.SetTaskQueue(taskQueue)

	// 2. 注册 2 个节点
	_, err := registry.Register(NodeRegistration{NodeID: "node-1", Capabilities: []string{"screenshot"}})
	if err != nil {
		t.Fatalf("register node-1: %v", err)
	}
	_, err = registry.Register(NodeRegistration{NodeID: "node-2", Capabilities: []string{"screenshot"}})
	if err != nil {
		t.Fatalf("register node-2: %v", err)
	}

	// 3. 入队 1 个任务
	_, err = taskQueue.Enqueue(TaskEnvelope{
		TaskID:       "task-1",
		TaskType:     "screenshot",
		RequiredCaps: []string{"screenshot"},
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// 4. 节点1领取任务
	node1, err := registry.Get("node-1")
	if err != nil {
		t.Fatalf("get node-1: %v", err)
	}
	rec, err := taskQueue.ClaimWithNode(node1)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if rec == nil {
		t.Fatal("expected task to be claimed")
	}
	if rec.Status != TaskStatusClaimed {
		t.Errorf("expected status claimed, got %s", rec.Status)
	}
	if rec.AssignedNode != "node-1" {
		t.Errorf("expected assigned_node node-1, got %s", rec.AssignedNode)
	}

	// 5. 模拟节点1故障（标记离线）
	err = registry.MarkOffline("node-1")
	if err != nil {
		t.Fatalf("mark offline: %v", err)
	}

	// 6. 验证任务被释放回 PENDING（可被节点2领取）
	node2, err := registry.Get("node-2")
	if err != nil {
		t.Fatalf("get node-2: %v", err)
	}
	rec2, err := taskQueue.ClaimWithNode(node2)
	if err != nil {
		t.Fatalf("node-2 claim: %v", err)
	}
	if rec2 == nil {
		t.Fatal("expected node-2 to claim the released task, but got nil")
	}
	if rec2.TaskID != "task-1" {
		t.Errorf("expected task-1, got %s", rec2.TaskID)
	}
	if rec2.AssignedNode != "node-2" {
		t.Errorf("expected assigned_node node-2, got %s", rec2.AssignedNode)
	}
}

func TestFailover_HandleNodeFailure_WithHealthyNodes(t *testing.T) {
	registry := NewRegistry(100 * time.Millisecond)
	defer registry.Stop()

	taskQueue := NewTaskQueue()
	defer taskQueue.Stop()

	registry.SetTaskQueue(taskQueue)

	// 注册 3 个节点
	for i := 1; i <= 3; i++ {
		_, err := registry.Register(NodeRegistration{
			NodeID:       fmt.Sprintf("node-%d", i),
			Capabilities: []string{"query"},
		})
		if err != nil {
			t.Fatalf("register node-%d: %v", i, err)
		}
	}

	// 入队 2 个任务
	for i := 1; i <= 2; i++ {
		_, err := taskQueue.Enqueue(TaskEnvelope{
			TaskID:   fmt.Sprintf("task-%d", i),
			TaskType: "query",
		})
		if err != nil {
			t.Fatalf("enqueue task-%d: %v", i, err)
		}
	}

	// 节点1领取 task-1
	node1, _ := registry.Get("node-1")
	taskQueue.ClaimWithNode(node1)

	// 节点2领取 task-2
	node2, _ := registry.Get("node-2")
	taskQueue.ClaimWithNode(node2)

	// 模拟节点1故障
	err := registry.HandleNodeFailure("node-1")
	if err != nil {
		t.Fatalf("handle node failure: %v", err)
	}

	// 验证有健康节点可用
	healthyNodes := registry.GetHealthyNodes()
	if len(healthyNodes) == 0 {
		t.Fatal("expected at least 1 healthy node after failover")
	}

	// 验证节点1已离线
	node1After, _ := registry.Get("node-1")
	if node1After.Online {
		t.Error("expected node-1 to be offline after HandleNodeFailure")
	}
}

func TestFailover_MaxReassignExceeded_MarkedFailed(t *testing.T) {
	registry := NewRegistry(100 * time.Millisecond)
	defer registry.Stop()

	taskQueue := NewTaskQueue()
	defer taskQueue.Stop()

	registry.SetTaskQueue(taskQueue)
	taskQueue.SetDefaultMaxReassign(0) // 不允许重新分配

	// 注册节点
	_, err := registry.Register(NodeRegistration{
		NodeID:       "node-1",
		Capabilities: []string{"tamper"},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// 入队任务，MaxReassign=0 表示不允许重新分配
	_, err = taskQueue.Enqueue(TaskEnvelope{
		TaskID:       "task-max",
		TaskType:     "tamper",
		RequiredCaps: []string{"tamper"},
		MaxReassign:  0,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// 手动设置 Attempt 为 MaxReassign+2，模拟已达到最大重试
	task, _ := taskQueue.Get("task-max")
	task.Attempt = 2 // > MaxReassign+1 (0+1=1)
	taskQueue.mu.Lock()
	taskQueue.tasks["task-max"].Attempt = 2
	taskQueue.mu.Unlock()

	// 节点领取任务
	node, _ := registry.Get("node-1")
	rec, _ := taskQueue.ClaimWithNode(node)
	if rec == nil {
		t.Fatal("expected task claimed")
	}

	// 节点离线 - Attempt=2 > MaxReassign+1=1，应标记为 FAILED
	registry.MarkOffline("node-1")

	// 验证任务状态
	taskAfter, err := taskQueue.Get("task-max")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if taskAfter == nil {
		t.Fatal("task not found")
	}
	if taskAfter.Status != TaskStatusFailed {
		t.Errorf("expected task status failed after max reassign exceeded, got %s", taskAfter.Status)
	}
}

func TestFailover_NoHealthyNodes_ReturnsError(t *testing.T) {
	registry := NewRegistry(100 * time.Millisecond)
	defer registry.Stop()

	// 注册唯一节点
	_, err := registry.Register(NodeRegistration{NodeID: "only-node"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// 标记离线
	err = registry.MarkOffline("only-node")
	if err != nil {
		t.Fatalf("mark offline: %v", err)
	}

	// 尝试获取健康节点
	healthy := registry.GetHealthyNodes()
	if len(healthy) != 0 {
		t.Errorf("expected 0 healthy nodes, got %d", len(healthy))
	}

	// HandleNodeFailure 应返回错误
	err = registry.HandleNodeFailure("only-node")
	if err == nil {
		t.Error("expected error when no healthy nodes available")
	}
}

func TestFailover_Deregister_ReleasesTasks(t *testing.T) {
	registry := NewRegistry(100 * time.Millisecond)
	defer registry.Stop()

	taskQueue := NewTaskQueue()
	defer taskQueue.Stop()

	registry.SetTaskQueue(taskQueue)

	// 注册节点
	_, err := registry.Register(NodeRegistration{
		NodeID:       "node-a",
		Capabilities: []string{"export"},
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// 入队并领取
	_, err = taskQueue.Enqueue(TaskEnvelope{
		TaskID:   "task-export",
		TaskType: "export",
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	node, _ := registry.Get("node-a")
	rec, err := taskQueue.ClaimWithNode(node)
	if err != nil || rec == nil {
		t.Fatal("expected task claimed")
	}

	// 注销节点
	err = registry.Deregister("node-a")
	if err != nil {
		t.Fatalf("deregister: %v", err)
	}

	// 验证任务被释放
	snapshot := taskQueue.Snapshot()
	for _, tsk := range snapshot {
		if tsk.TaskID == "task-export" && tsk.Status == TaskStatusPending {
			return // 任务已释放
		}
	}

	// 检查任务是否存在（可能被 recycle 回收）
	task, _ := taskQueue.Get("task-export")
	if task != nil && task.Status == TaskStatusClaimed {
		t.Error("expected task to be released after node deregister, but still claimed")
	}
}
