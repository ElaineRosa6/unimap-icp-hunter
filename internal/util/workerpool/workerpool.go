package workerpool

import (
	"sync"
	"time"
)

// Task 工作任务接口
type Task interface {
	Execute() error
}

// Pool 工作池
type Pool struct {
	tasks       chan Task
	results     chan error
	wg          sync.WaitGroup
	concurrency int
	running     bool
}

// NewPool 创建工作池
func NewPool(concurrency int) *Pool {
	if concurrency <= 0 {
		concurrency = 5
	}

	return &Pool{
		tasks:       make(chan Task, concurrency*10),
		results:     make(chan error, concurrency*10),
		concurrency: concurrency,
		running:     false,
	}
}

// Start 启动工作池
func (p *Pool) Start() {
	if p.running {
		return
	}

	p.running = true

	// 启动工作协程
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// Stop 停止工作池
func (p *Pool) Stop() {
	if !p.running {
		return
	}

	close(p.tasks)
	p.wg.Wait()
	close(p.results)
	p.running = false
}

// Submit 提交任务
func (p *Pool) Submit(task Task) {
	if !p.running {
		return
	}

	p.tasks <- task
}

// Results 获取结果通道
func (p *Pool) Results() <-chan error {
	return p.results
}

// worker 工作协程
func (p *Pool) worker() {
	defer p.wg.Done()

	for task := range p.tasks {
		err := task.Execute()
		if err != nil {
			p.results <- err
		}
	}
}

// Wait 等待所有任务完成
func (p *Pool) Wait() {
	p.wg.Wait()
}

// TaskWithRetry 带重试的任务
type TaskWithRetry struct {
	task        Task
	maxAttempts int
	baseDelay   time.Duration
}

// NewTaskWithRetry 创建带重试的任务
func NewTaskWithRetry(task Task, maxAttempts int, baseDelay time.Duration) *TaskWithRetry {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}

	return &TaskWithRetry{
		task:        task,
		maxAttempts: maxAttempts,
		baseDelay:   baseDelay,
	}
}

// Execute 执行任务（带重试）
func (t *TaskWithRetry) Execute() error {
	var err error

	for attempt := 0; attempt < t.maxAttempts; attempt++ {
		err = t.task.Execute()
		if err == nil {
			return nil
		}

		// 指数退避
		delay := t.baseDelay * time.Duration(1<<attempt)
		time.Sleep(delay)
	}

	return err
}
