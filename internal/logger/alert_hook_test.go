package logger

import (
	"sync"
	"testing"
	"time"
)

func TestErrorAlertHook_SingleError_NoTrigger(t *testing.T) {
	hook := NewErrorAlertHook(time.Second, 3, nil)
	hook.OnError()

	if hook.GetTriggerCount() != 0 {
		t.Fatal("expected 0 triggers with single error")
	}
}

func TestErrorAlertHook_Threshold_Triggered(t *testing.T) {
	var mu sync.Mutex
	triggered := 0
	hook := NewErrorAlertHook(time.Second, 3, func(count, triggers int) {
		mu.Lock()
		defer mu.Unlock()
		triggered++
	})

	for i := 0; i < 3; i++ {
		hook.OnError()
	}

	// 等待 goroutine 完成
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if triggered != 1 {
		t.Fatalf("expected 1 trigger, got %d", triggered)
	}
}

func TestErrorAlertHook_MultipleTriggers(t *testing.T) {
	var mu sync.Mutex
	triggerCount := 0
	hook := NewErrorAlertHook(time.Second, 2, func(count, triggers int) {
		mu.Lock()
		defer mu.Unlock()
		triggerCount++
	})

	for i := 0; i < 6; i++ {
		hook.OnError()
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if triggerCount < 2 {
		t.Fatalf("expected at least 2 triggers, got %d", triggerCount)
	}
}

func TestErrorAlertHook_WindowExpiry_NoTrigger(t *testing.T) {
	hook := NewErrorAlertHook(50*time.Millisecond, 3, nil)

	// 发送 2 个错误
	hook.OnError()
	hook.OnError()

	// 等待窗口过期
	time.Sleep(100 * time.Millisecond)

	// 再发送 1 个错误，不应触发（前 2 个已过期）
	hook.OnError()

	if hook.GetCurrentErrorCount() != 1 {
		t.Fatalf("expected 1 error in window, got %d", hook.GetCurrentErrorCount())
	}
}

func TestErrorAlertHook_Reset(t *testing.T) {
	hook := NewErrorAlertHook(time.Second, 3, nil)

	hook.OnError()
	hook.OnError()

	if hook.GetCurrentErrorCount() != 2 {
		t.Fatalf("expected 2, got %d", hook.GetCurrentErrorCount())
	}

	hook.Reset()

	if hook.GetCurrentErrorCount() != 0 {
		t.Fatalf("expected 0 after reset, got %d", hook.GetCurrentErrorCount())
	}
	if hook.GetTriggerCount() != 0 {
		t.Fatalf("expected 0 triggers after reset, got %d", hook.GetTriggerCount())
	}
}

func TestErrorAlertHook_NilCallback(t *testing.T) {
	// 不应 panic
	hook := NewErrorAlertHook(time.Second, 1, nil)
	hook.OnError()
	hook.OnError()

	if hook.GetTriggerCount() < 1 {
		t.Fatal("expected at least 1 trigger")
	}
}

func TestErrorAlertHook_ConcurrentSafety(t *testing.T) {
	hook := NewErrorAlertHook(time.Second, 10, nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hook.OnError()
		}()
	}
	wg.Wait()

	// 不应 panic
	_ = hook.GetCurrentErrorCount()
	_ = hook.GetTriggerCount()
}
