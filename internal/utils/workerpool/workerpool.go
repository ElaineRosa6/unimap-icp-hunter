package workerpool

import (
	"sync"
	"sync/atomic"
	"time"
)

type Task interface {
	Execute() error
}

// loadMonitor 负载监控器
type loadMonitor struct {
	taskQueueLength int32
	activeWorkers   int32
	avgTaskDuration int64 // 纳秒
	taskCount       int64
	mutex           sync.Mutex // 只用于avgTaskDuration的计算
}

func newLoadMonitor() *loadMonitor {
	return &loadMonitor{}
}

func (lm *loadMonitor) recordTaskStart() {
	atomic.AddInt32(&lm.activeWorkers, 1)
}

func (lm *loadMonitor) recordTaskEnd(duration time.Duration) {
	atomic.AddInt32(&lm.activeWorkers, -1)
	count := atomic.AddInt64(&lm.taskCount, 1)

	// 使用锁保护avgTaskDuration的计算
	lm.mutex.Lock()
	defer lm.mutex.Unlock()

	if count == 1 {
		lm.avgTaskDuration = duration.Nanoseconds()
	} else {
		// 使用加权平均
		lm.avgTaskDuration = (lm.avgTaskDuration*(count-1) + duration.Nanoseconds()) / count
	}
}

func (lm *loadMonitor) setQueueLength(length int) {
	atomic.StoreInt32(&lm.taskQueueLength, int32(length))
}

func (lm *loadMonitor) getLoadMetrics() (queueLength, activeWorkers int, avgDuration time.Duration) {
	queueLength = int(atomic.LoadInt32(&lm.taskQueueLength))
	activeWorkers = int(atomic.LoadInt32(&lm.activeWorkers))

	lm.mutex.Lock()
	avgNs := lm.avgTaskDuration
	lm.mutex.Unlock()

	return queueLength, activeWorkers, time.Duration(avgNs)
}

type Pool struct {
	tasks              chan Task
	results            chan error
	exitCh             chan struct{}
	wg                 sync.WaitGroup
	minConcurrency     int32
	maxConcurrency     int32
	currentConcurrency int32
	running            int32      // 0: stopped, 1: running
	mutex              sync.Mutex // 只用于调整并发数时的保护
	loadMonitor        *loadMonitor
}

func NewPool(concurrency int) *Pool {
	if concurrency <= 0 {
		concurrency = 5
	}

	return &Pool{
		tasks:              make(chan Task, concurrency*10),
		results:            make(chan error, concurrency*10),
		exitCh:             make(chan struct{}, concurrency*4),
		minConcurrency:     int32(concurrency),
		maxConcurrency:     int32(concurrency * 4), // 默认最大为初始值的4倍
		currentConcurrency: int32(concurrency),
		running:            0,
		loadMonitor:        newLoadMonitor(),
	}
}

// NewDynamicPool 创建动态工作池，支持自动调整工作线程数量
func NewDynamicPool(minConcurrency, maxConcurrency int) *Pool {
	if minConcurrency <= 0 {
		minConcurrency = 5
	}
	if maxConcurrency <= minConcurrency {
		maxConcurrency = minConcurrency * 4
	}

	return &Pool{
		tasks:              make(chan Task, maxConcurrency*10),
		results:            make(chan error, maxConcurrency*10),
		exitCh:             make(chan struct{}, maxConcurrency),
		minConcurrency:     int32(minConcurrency),
		maxConcurrency:     int32(maxConcurrency),
		currentConcurrency: int32(minConcurrency),
		running:            0,
		loadMonitor:        newLoadMonitor(),
	}
}

func (p *Pool) Start() {
	if atomic.LoadInt32(&p.running) == 1 {
		return
	}

	if atomic.CompareAndSwapInt32(&p.running, 0, 1) {
		// 启动初始工作线程
		for i := int32(0); i < p.minConcurrency; i++ {
			p.wg.Add(1)
			go p.worker()
		}

		// 启动负载监控和动态调整线程
		go p.startLoadMonitoring()
	}
}

func (p *Pool) Stop() {
	if atomic.LoadInt32(&p.running) == 0 {
		return
	}

	if atomic.CompareAndSwapInt32(&p.running, 1, 0) {
		close(p.tasks)
		close(p.exitCh)
		p.wg.Wait()
		close(p.results)
	}
}

func (p *Pool) Submit(task Task) {
	if atomic.LoadInt32(&p.running) == 0 {
		return
	}

	p.tasks <- task

	// 更新队列长度
	p.loadMonitor.setQueueLength(len(p.tasks))
}

func (p *Pool) Results() <-chan error {
	return p.results
}

func (p *Pool) worker() {
	defer p.wg.Done()

	for {
		select {
		case task, ok := <-p.tasks:
			if !ok {
				return
			}
			startTime := time.Now()
			p.loadMonitor.recordTaskStart()

			err := task.Execute()

			duration := time.Since(startTime)
			p.loadMonitor.recordTaskEnd(duration)

			if err != nil {
				p.results <- err
			}
		case <-p.exitCh:
			return
		}
	}
}

func (p *Pool) Wait() {
	p.wg.Wait()
}

// startLoadMonitoring 启动负载监控和动态调整
func (p *Pool) startLoadMonitoring() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for atomic.LoadInt32(&p.running) == 1 {
		select {
		case <-ticker.C:
			p.adjustConcurrency()
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// adjustConcurrency 根据负载动态调整工作线程数量
func (p *Pool) adjustConcurrency() {
	if atomic.LoadInt32(&p.running) == 0 {
		return
	}

	queueLength, activeWorkers, _ := p.loadMonitor.getLoadMetrics()
	currentConcurrency := atomic.LoadInt32(&p.currentConcurrency)

	// 动态调整逻辑
	if queueLength > int(currentConcurrency*2) && currentConcurrency < p.maxConcurrency {
		p.mutex.Lock()
		// 再次检查，避免竞态条件
		currentConcurrency = atomic.LoadInt32(&p.currentConcurrency)
		if queueLength > int(currentConcurrency*2) && currentConcurrency < p.maxConcurrency {
			// 队列积压，增加工作线程
			newConcurrency := currentConcurrency * 2
			if newConcurrency > p.maxConcurrency {
				newConcurrency = p.maxConcurrency
			}

			// 添加新的工作线程
			for i := currentConcurrency; i < newConcurrency; i++ {
				p.wg.Add(1)
				go p.worker()
			}

			atomic.StoreInt32(&p.currentConcurrency, newConcurrency)
		}
		p.mutex.Unlock()
	} else if queueLength == 0 && activeWorkers < int(currentConcurrency/2) && currentConcurrency > p.minConcurrency {
		p.mutex.Lock()
		// 再次检查，避免竞态条件
		currentConcurrency = atomic.LoadInt32(&p.currentConcurrency)
		if queueLength == 0 && activeWorkers < int(currentConcurrency/2) && currentConcurrency > p.minConcurrency {
			// 负载过低，减少工作线程
			newConcurrency := currentConcurrency / 2
			if newConcurrency < p.minConcurrency {
				newConcurrency = p.minConcurrency
			}

			// 通知多余的 worker 退出
			for i := newConcurrency; i < currentConcurrency; i++ {
				select {
				case p.exitCh <- struct{}{}:
				default:
				}
			}

			atomic.StoreInt32(&p.currentConcurrency, newConcurrency)
		}
		p.mutex.Unlock()
	}
}

// GetConcurrency 获取当前工作线程数量
func (p *Pool) GetConcurrency() int {
	return int(atomic.LoadInt32(&p.currentConcurrency))
}

// SetConcurrency 手动设置工作线程数量
func (p *Pool) SetConcurrency(concurrency int) {
	if concurrency < int(p.minConcurrency) {
		concurrency = int(p.minConcurrency)
	}
	if concurrency > int(p.maxConcurrency) {
		concurrency = int(p.maxConcurrency)
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	currentConcurrency := atomic.LoadInt32(&p.currentConcurrency)
	diff := int32(concurrency) - currentConcurrency
	if diff > 0 {
		// 增加工作线程
		for i := int32(0); i < diff; i++ {
			p.wg.Add(1)
			go p.worker()
		}
	} else if diff < 0 {
		// 减少工作线程，通知多余的 worker 退出
		for i := int32(0); i < -diff; i++ {
			select {
			case p.exitCh <- struct{}{}:
			default:
			}
		}
	}

	atomic.StoreInt32(&p.currentConcurrency, int32(concurrency))
}

// GetLoadMetrics 获取负载指标
func (p *Pool) GetLoadMetrics() (queueLength, activeWorkers int, avgDuration time.Duration) {
	return p.loadMonitor.getLoadMetrics()
}

type TaskWithRetry struct {
	task        Task
	maxAttempts int
	baseDelay   time.Duration
}

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

func (t *TaskWithRetry) Execute() error {
	var err error

	for attempt := 0; attempt < t.maxAttempts; attempt++ {
		err = t.task.Execute()
		if err == nil {
			return nil
		}

		delay := t.baseDelay * time.Duration(1<<attempt)
		time.Sleep(delay)
	}

	return err
}
