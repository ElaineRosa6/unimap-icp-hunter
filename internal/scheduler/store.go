package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store handles persistence of scheduled tasks and execution history to JSON files.
type Store struct {
	taskPath    string
	historyPath string
	mu          sync.Mutex
}

// NewStore creates a new Store. Directories are created automatically.
func NewStore(taskPath, historyPath string) *Store {
	// Ensure parent directories exist
	for _, p := range []string{taskPath, historyPath} {
		if dir := filepath.Dir(p); dir != "" && dir != "." {
			os.MkdirAll(dir, 0755)
		}
	}
	return &Store{
		taskPath:    taskPath,
		historyPath: historyPath,
	}
}

type persistedData struct {
	Tasks   []*ScheduledTask `json:"tasks"`
	History []ExecutionRecord `json:"history"`
}

// Load reads tasks and history from disk.
func (s *Store) Load() ([]*ScheduledTask, []ExecutionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks, err := s.loadTasks()
	if err != nil {
		return nil, nil, fmt.Errorf("load tasks: %w", err)
	}

	history, err := s.loadHistory()
	if err != nil {
		return nil, nil, fmt.Errorf("load history: %w", err)
	}

	return tasks, history, nil
}

// Save writes tasks and history to disk.
func (s *Store) Save(tasks []*ScheduledTask, history []ExecutionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.saveTasks(tasks); err != nil {
		return fmt.Errorf("save tasks: %w", err)
	}
	if err := s.saveHistory(history); err != nil {
		return fmt.Errorf("save history: %w", err)
	}
	return nil
}

func (s *Store) loadTasks() ([]*ScheduledTask, error) {
	data, err := os.ReadFile(s.taskPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var tasks []*ScheduledTask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *Store) saveTasks(tasks []*ScheduledTask) error {
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.taskPath, data, 0600)
}

func (s *Store) loadHistory() ([]ExecutionRecord, error) {
	data, err := os.ReadFile(s.historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var history []ExecutionRecord
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}
	return history, nil
}

func (s *Store) saveHistory(history []ExecutionRecord) error {
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.historyPath, data, 0600)
}
