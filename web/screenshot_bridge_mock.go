package web

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/screenshot"
)

type bridgeMockClient struct {
	mu         sync.Mutex
	pending    map[string]screenshot.BridgeTask
	dispatched map[string]screenshot.BridgeTask
	order      []string
	results    map[string]screenshot.BridgeResult
	waiters    map[string]chan screenshot.BridgeResult
}

func newBridgeMockClient() *bridgeMockClient {
	return &bridgeMockClient{
		pending:    make(map[string]screenshot.BridgeTask),
		dispatched: make(map[string]screenshot.BridgeTask),
		order:      make([]string, 0),
		results:    make(map[string]screenshot.BridgeResult),
		waiters:    make(map[string]chan screenshot.BridgeResult),
	}
}

func (m *bridgeMockClient) SubmitTask(ctx context.Context, task screenshot.BridgeTask) error {
	if m == nil {
		return fmt.Errorf("mock bridge client not initialized")
	}
	m.mu.Lock()
	m.pending[task.RequestID] = task
	m.order = append(m.order, task.RequestID)
	m.mu.Unlock()
	return nil
}

func (m *bridgeMockClient) AwaitResult(ctx context.Context, requestID string) (screenshot.BridgeResult, error) {
	m.mu.Lock()
	if res, ok := m.results[requestID]; ok {
		delete(m.results, requestID)
		delete(m.pending, requestID)
		delete(m.dispatched, requestID)
		m.mu.Unlock()
		return res, nil
	}
	ch := make(chan screenshot.BridgeResult, 1)
	m.waiters[requestID] = ch
	m.mu.Unlock()

	select {
	case <-ctx.Done():
		m.mu.Lock()
		delete(m.waiters, requestID)
		m.mu.Unlock()
		return screenshot.BridgeResult{}, ctx.Err()
	case res := <-ch:
		return res, nil
	}
}

func (m *bridgeMockClient) PushResult(res screenshot.BridgeResult) {
	if m == nil {
		return
	}
	if res.DurationMS == 0 {
		res.DurationMS = 1
	}
	m.mu.Lock()
	if ch, ok := m.waiters[res.RequestID]; ok {
		delete(m.waiters, res.RequestID)
		delete(m.pending, res.RequestID)
		delete(m.dispatched, res.RequestID)
		m.removeFromOrderLocked(res.RequestID)
		m.mu.Unlock()
		select {
		case ch <- res:
		case <-time.After(100 * time.Millisecond):
		}
		return
	}
	m.results[res.RequestID] = res
	delete(m.pending, res.RequestID)
	delete(m.dispatched, res.RequestID)
	m.removeFromOrderLocked(res.RequestID)
	m.mu.Unlock()
}

func (m *bridgeMockClient) Stats() (pending int, waiters int) {
	if m == nil {
		return 0, 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.pending), len(m.waiters)
}

func (m *bridgeMockClient) NextTask() (screenshot.BridgeTask, bool) {
	if m == nil {
		return screenshot.BridgeTask{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for len(m.order) > 0 {
		id := m.order[0]
		m.order = m.order[1:]
		task, ok := m.pending[id]
		if !ok {
			continue
		}
		delete(m.pending, id)
		m.dispatched[id] = task
		return task, true
	}
	return screenshot.BridgeTask{}, false
}

func (m *bridgeMockClient) TaskForRequest(requestID string) (screenshot.BridgeTask, bool) {
	if m == nil {
		return screenshot.BridgeTask{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if task, ok := m.dispatched[requestID]; ok {
		return task, true
	}
	if task, ok := m.pending[requestID]; ok {
		return task, true
	}
	return screenshot.BridgeTask{}, false
}

func (m *bridgeMockClient) removeFromOrderLocked(requestID string) {
	for i, id := range m.order {
		if id == requestID {
			m.order = append(m.order[:i], m.order[i+1:]...)
			return
		}
	}
}
