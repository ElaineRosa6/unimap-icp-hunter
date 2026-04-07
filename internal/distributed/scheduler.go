package distributed

import (
	"sort"
	"strings"
)

// SchedulerStrategy defines the scheduling strategy type
type SchedulerStrategy string

const (
	StrategyHealthLoad SchedulerStrategy = "health_load"
	StrategyRoundRobin SchedulerStrategy = "round_robin"
	StrategyPriority   SchedulerStrategy = "priority"
)

// Scheduler selects tasks for nodes based on a strategy
type Scheduler interface {
	// SelectTask selects the best task for a node from available tasks
	SelectTask(tasks []*TaskRecord, node *NodeRecord) *TaskRecord
	// Strategy returns the scheduler's strategy name
	Strategy() SchedulerStrategy
}

// HealthLoadScheduler implements health_load scheduling strategy
// It prioritizes tasks by priority, then considers node health/load
type HealthLoadScheduler struct{}

func NewHealthLoadScheduler() *HealthLoadScheduler {
	return &HealthLoadScheduler{}
}

func (s *HealthLoadScheduler) Strategy() SchedulerStrategy {
	return StrategyHealthLoad
}

// SelectTask selects the best matching task for a node
// Tasks are sorted by priority (higher first), then by creation time (earlier first)
func (s *HealthLoadScheduler) SelectTask(tasks []*TaskRecord, node *NodeRecord) *TaskRecord {
	if len(tasks) == 0 {
		return nil
	}

	// Filter tasks that the node can handle based on capabilities
	eligible := make([]*TaskRecord, 0, len(tasks))
	nodeCaps := make(map[string]struct{})
	for _, cap := range node.Capabilities {
		nodeCaps[strings.TrimSpace(strings.ToLower(cap))] = struct{}{}
	}

	for _, task := range tasks {
		if s.canHandleTask(task, nodeCaps) {
			eligible = append(eligible, task)
		}
	}

	if len(eligible) == 0 {
		return nil
	}

	// Sort by priority (descending), then by creation time (ascending)
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].Priority != eligible[j].Priority {
			return eligible[i].Priority > eligible[j].Priority
		}
		if !eligible[i].CreatedAt.Equal(eligible[j].CreatedAt) {
			return eligible[i].CreatedAt.Before(eligible[j].CreatedAt)
		}
		return eligible[i].TaskID < eligible[j].TaskID
	})

	return eligible[0]
}

// canHandleTask checks if a node has the required capabilities for a task
func (s *HealthLoadScheduler) canHandleTask(task *TaskRecord, nodeCaps map[string]struct{}) bool {
	if len(task.RequiredCaps) == 0 {
		return true
	}
	for _, req := range task.RequiredCaps {
		if _, ok := nodeCaps[strings.TrimSpace(strings.ToLower(req))]; !ok {
			return false
		}
	}
	return true
}

// PriorityScheduler implements simple priority-based scheduling
type PriorityScheduler struct{}

func NewPriorityScheduler() *PriorityScheduler {
	return &PriorityScheduler{}
}

func (s *PriorityScheduler) Strategy() SchedulerStrategy {
	return StrategyPriority
}

func (s *PriorityScheduler) SelectTask(tasks []*TaskRecord, node *NodeRecord) *TaskRecord {
	if len(tasks) == 0 {
		return nil
	}

	// Sort by priority (descending), then by creation time (ascending)
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority > tasks[j].Priority
		}
		if !tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
		}
		return tasks[i].TaskID < tasks[j].TaskID
	})

	return tasks[0]
}

// RoundRobinScheduler implements round-robin scheduling
type RoundRobinScheduler struct {
	lastIndex int
}

func NewRoundRobinScheduler() *RoundRobinScheduler {
	return &RoundRobinScheduler{}
}

func (s *RoundRobinScheduler) Strategy() SchedulerStrategy {
	return StrategyRoundRobin
}

func (s *RoundRobinScheduler) SelectTask(tasks []*TaskRecord, node *NodeRecord) *TaskRecord {
	if len(tasks) == 0 {
		return nil
	}

	if s.lastIndex >= len(tasks) {
		s.lastIndex = 0
	}

	task := tasks[s.lastIndex]
	s.lastIndex++
	return task
}

// NewScheduler creates a scheduler based on the strategy
func NewSchedulerFromStrategy(strategy SchedulerStrategy) Scheduler {
	switch strategy {
	case StrategyHealthLoad:
		return NewHealthLoadScheduler()
	case StrategyRoundRobin:
		return NewRoundRobinScheduler()
	case StrategyPriority:
		return NewPriorityScheduler()
	default:
		return NewHealthLoadScheduler()
	}
}