package utils

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

// ShutdownManager 优雅关闭管理器
type ShutdownManager struct {
	ctx       context.Context
	cancel    context.CancelFunc
	signals   chan os.Signal
	wg        sync.WaitGroup
	handlers  []ShutdownHandler
	mu        sync.RWMutex
	timeout   time.Duration
}

// ShutdownHandler 关闭处理函数类型
type ShutdownHandler func(ctx context.Context) error

// NewShutdownManager 创建关闭管理器
func NewShutdownManager(timeout time.Duration) *ShutdownManager {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ShutdownManager{
		ctx:      ctx,
		cancel:   cancel,
		signals:  make(chan os.Signal, 1),
		handlers: make([]ShutdownHandler, 0),
		timeout:  timeout,
	}
}

// RegisterHandler 注册关闭处理函数
func (s *ShutdownManager) RegisterHandler(handler ShutdownHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers = append(s.handlers, handler)
}

// Start 启动信号监听
func (s *ShutdownManager) Start() {
	// 注册要监听的信号
	signal.Notify(s.signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// 在 goroutine 中监听信号
	go s.listen()
}

// listen 监听系统信号
func (s *ShutdownManager) listen() {
	sig := <-s.signals
	logger.Infof("Received signal: %v, starting graceful shutdown...", sig)

	// 执行优雅关闭
	s.Shutdown()
}

// Shutdown 执行优雅关闭
func (s *ShutdownManager) Shutdown() {
	s.mu.RLock()
	handlers := make([]ShutdownHandler, len(s.handlers))
	copy(handlers, s.handlers)
	s.mu.RUnlock()

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(s.ctx, s.timeout)
	defer cancel()

	// 并发执行所有关闭处理函数
	var wg sync.WaitGroup
	errChan := make(chan error, len(handlers))

	for _, handler := range handlers {
		wg.Add(1)
		go func(h ShutdownHandler) {
			defer wg.Done()
			if err := h(ctx); err != nil {
				errChan <- err
			}
		}(handler)
	}

	// 等待所有处理函数完成或超时
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("All shutdown handlers completed successfully")
	case <-ctx.Done():
		logger.Warn("Shutdown timeout reached, forcing exit")
	}

	// 取消主上下文
	s.cancel()
}

// Wait 等待关闭信号
func (s *ShutdownManager) Wait() {
	<-s.ctx.Done()
}

// Context 返回管理器的上下文
func (s *ShutdownManager) Context() context.Context {
	return s.ctx
}

// Stop 停止信号监听（用于测试）
func (s *ShutdownManager) Stop() {
	signal.Stop(s.signals)
	s.cancel()
}

// GracefulShutdown 简化的优雅关闭辅助函数
// 使用示例:
//   utils.GracefulShutdown(30*time.Second, func(ctx context.Context) error {
//       // 关闭数据库连接
//       return db.Close()
//   }, func(ctx context.Context) error {
//       // 关闭HTTP服务器
//       return server.Shutdown(ctx)
//   })
func GracefulShutdown(timeout time.Duration, handlers ...ShutdownHandler) {
	manager := NewShutdownManager(timeout)

	for _, handler := range handlers {
		manager.RegisterHandler(handler)
	}

	manager.Start()
	manager.Wait()
}
