package distributed

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SnapshotData represents the complete state for persistence
type SnapshotData struct {
	Version   int            `json:"version"`
	Timestamp time.Time      `json:"timestamp"`
	Nodes     []NodeRecord   `json:"nodes"`
	Tasks     []TaskRecord   `json:"tasks"`
}

// SnapshotManager handles file-based snapshot persistence
type SnapshotManager struct {
	filePath     string
	saveInterval time.Duration
	stopChan     chan struct{}
	stopped      bool
	mu           sync.Mutex
	registry     *Registry
	taskQueue    *TaskQueue
}

// NewSnapshotManager creates a new snapshot manager
func NewSnapshotManager(filePath string, saveInterval time.Duration) *SnapshotManager {
	if saveInterval <= 0 {
		saveInterval = 30 * time.Second
	}
	return &SnapshotManager{
		filePath:     filePath,
		saveInterval: saveInterval,
		stopChan:     make(chan struct{}),
	}
}

// SetRegistry sets the registry to snapshot
func (s *SnapshotManager) SetRegistry(r *Registry) {
	s.mu.Lock()
	s.registry = r
	s.mu.Unlock()
}

// SetTaskQueue sets the task queue to snapshot
func (s *SnapshotManager) SetTaskQueue(q *TaskQueue) {
	s.mu.Lock()
	s.taskQueue = q
	s.mu.Unlock()
}

// Start begins periodic snapshot saving
func (s *SnapshotManager) Start() {
	go s.startPeriodicSave()
}

// Stop stops the periodic snapshot saving
func (s *SnapshotManager) Stop() {
	s.mu.Lock()
	if !s.stopped {
		s.stopped = true
		close(s.stopChan)
	}
	s.mu.Unlock()
}

// startPeriodicSave periodically saves snapshots
func (s *SnapshotManager) startPeriodicSave() {
	ticker := time.NewTicker(s.saveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			// Save final snapshot before stopping
			_ = s.Save()
			return
		case <-ticker.C:
			_ = s.Save()
		}
	}
}

// Save writes the current state to the snapshot file
func (s *SnapshotManager) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data := &SnapshotData{
		Version:   1,
		Timestamp: time.Now(),
	}

	if s.registry != nil {
		snapshot := s.registry.Snapshot()
		data.Nodes = snapshot.Nodes
	}

	if s.taskQueue != nil {
		data.Tasks = s.taskQueue.Snapshot()
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	// Write to temp file first, then rename for atomicity
	tempPath := s.filePath + ".tmp"
	if err := os.WriteFile(tempPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write temp snapshot: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, s.filePath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename snapshot: %w", err)
	}

	return nil
}

// Load reads the state from the snapshot file
func (s *SnapshotManager) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No snapshot file, nothing to load
		}
		return fmt.Errorf("failed to read snapshot: %w", err)
	}

	var snapshot SnapshotData
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	// Restore nodes
	if s.registry != nil && len(snapshot.Nodes) > 0 {
		for _, node := range snapshot.Nodes {
			rec := &NodeRecord{
				NodeID:          node.NodeID,
				Hostname:        node.Hostname,
				Region:          node.Region,
				Labels:          node.Labels,
				Capabilities:    node.Capabilities,
				Version:         node.Version,
				EgressIP:        node.EgressIP,
				CurrentLoad:     node.CurrentLoad,
				MaxConcurrency:  node.MaxConcurrency,
				AvgLatencyMS:    node.AvgLatencyMS,
				SuccessRate5m:   node.SuccessRate5m,
				LastHeartbeatAt: node.LastHeartbeatAt,
				RegisteredAt:    node.RegisteredAt,
				Online:          false, // Start as offline until heartbeat
			}
			s.registry.mu.Lock()
			s.registry.nodes[node.NodeID] = rec
			s.registry.mu.Unlock()
		}
	}

	// Restore tasks
	if s.taskQueue != nil && len(snapshot.Tasks) > 0 {
		for _, task := range snapshot.Tasks {
			rec := &TaskRecord{
				TaskID:         task.TaskID,
				TaskType:       task.TaskType,
				Payload:        task.Payload,
				Priority:       task.Priority,
				TimeoutSeconds: task.TimeoutSeconds,
				TraceID:        task.TraceID,
				RequiredCaps:   task.RequiredCaps,
				Status:         task.Status,
				AssignedNode:   task.AssignedNode,
				Attempt:        task.Attempt,
				MaxReassign:    task.MaxReassign,
				LeaseUntil:     task.LeaseUntil,
				CreatedAt:      task.CreatedAt,
				UpdatedAt:      task.UpdatedAt,
				LastError:      task.LastError,
				Result:         task.Result,
			}
			s.taskQueue.mu.Lock()
			s.taskQueue.tasks[task.TaskID] = rec
			// Re-add pending tasks to the pending list
			if task.Status == TaskStatusPending {
				s.taskQueue.pending = append(s.taskQueue.pending, task.TaskID)
			}
			s.taskQueue.mu.Unlock()
		}
		// Sort pending queue
		s.taskQueue.mu.Lock()
		s.taskQueue.sortPendingLocked()
		s.taskQueue.mu.Unlock()
	}

	return nil
}

// Delete removes the snapshot file
func (s *SnapshotManager) Delete() error {
	if err := os.Remove(s.filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Exists checks if a snapshot file exists
func (s *SnapshotManager) Exists() bool {
	_, err := os.Stat(s.filePath)
	return err == nil
}

// GetSnapshotInfo returns information about the snapshot file
func (s *SnapshotManager) GetSnapshotInfo() (exists bool, size int64, modTime time.Time, err error) {
	info, err := os.Stat(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, time.Time{}, nil
		}
		return false, 0, time.Time{}, err
	}
	return true, info.Size(), info.ModTime(), nil
}