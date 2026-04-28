package adapter

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/metrics"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/utils"
	"github.com/unimap-icp-hunter/project/internal/utils/workerpool"
)

const (
	// DefaultConcurrency 默认并发数
	DefaultConcurrency = 5
	// MaxConcurrency 最大并发数
	MaxConcurrency = 8
	// DefaultCacheTTL 默认缓存时间
	DefaultCacheTTL = 30 * time.Minute
	// DefaultRateLimitDelay 默认速率限制延迟
	DefaultRateLimitDelay = 100 * time.Millisecond
	// DefaultCircuitBreakerThreshold 熔断器失败阈值（连续N次失败）
	DefaultCircuitBreakerThreshold = 5
	// DefaultCircuitBreakerDuration 熔断器打开持续时间
	DefaultCircuitBreakerDuration = 2 * time.Minute
)

// EngineCacheTTLConfig 引擎缓存TTL配置
type EngineCacheTTLConfig struct {
	TTL     time.Duration
	Enabled bool
}

// CircuitState 熔断器状态
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"   // 正常状态
	CircuitOpen     CircuitState = "open"     // 熔断状态，跳过该引擎
	CircuitHalfOpen CircuitState = "half_open" // 半开状态，尝试恢复
)

// CircuitBreaker 简单熔断器
type CircuitBreaker struct {
	mu             sync.Mutex
	State          CircuitState
	Failures       int
	LastFailure    time.Time
	Threshold      int
	ResetDuration  time.Duration
}

func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.State == CircuitClosed {
		return true
	}
	if cb.State == CircuitOpen {
		if time.Since(cb.LastFailure) > cb.ResetDuration {
			cb.State = CircuitHalfOpen
			return true
		}
		return false
	}
	// half_open: allow one request
	return true
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.State = CircuitClosed
	cb.Failures = 0
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.Failures++
	cb.LastFailure = time.Now()
	if cb.Failures >= cb.Threshold {
		cb.State = CircuitOpen
	}
}

// GetState returns current state safely
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.State
}

// GetStats returns all stats for monitoring safely
func (cb *CircuitBreaker) GetStats() (state CircuitState, failures int, threshold int, lastFailure time.Time, resetDuration time.Duration) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.State, cb.Failures, cb.Threshold, cb.LastFailure, cb.ResetDuration
}

// EngineOrchestrator 引擎编排器
type EngineOrchestrator struct {
	adapters        map[string]EngineAdapter
	mutex           sync.RWMutex
	cache           utils.QueryCache
	concurrency     int
	engineCacheTTL  map[string]EngineCacheTTLConfig // 按引擎的缓存TTL配置
	defaultCacheTTL time.Duration                   // 默认缓存TTL
	circuitBreakers map[string]*CircuitBreaker      // 按引擎的熔断器
}

// NewEngineOrchestrator 创建引擎编排器
func NewEngineOrchestrator() *EngineOrchestrator {
	return NewEngineOrchestratorWithConfig(false, "", "", 0)
}

// NewEngineOrchestratorWithConfig 使用配置创建引擎编排器
func NewEngineOrchestratorWithConfig(useRedis bool, redisAddr, redisPassword string, redisDB int) *EngineOrchestrator {
	cache := utils.NewCache(
		useRedis,
		redisAddr,
		redisPassword,
		redisDB,
		"unimap:",
		1000,
		5*time.Minute,
	)

	return &EngineOrchestrator{
		adapters:        make(map[string]EngineAdapter),
		cache:           cache,
		concurrency:     DefaultConcurrency,
		engineCacheTTL:  make(map[string]EngineCacheTTLConfig),
		defaultCacheTTL: DefaultCacheTTL,
		circuitBreakers: make(map[string]*CircuitBreaker),
	}
}

// SetEngineCacheTTL 设置引擎缓存TTL配置
func (o *EngineOrchestrator) SetEngineCacheTTL(engineName string, ttl time.Duration, enabled bool) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.engineCacheTTL[strings.ToLower(engineName)] = EngineCacheTTLConfig{
		TTL:     ttl,
		Enabled: enabled,
	}
}

// SetEngineCacheTTLFromConfig 从配置map设置引擎缓存TTL
// configMap 格式: map[engineName]{ttl_seconds, enabled}
func (o *EngineOrchestrator) SetEngineCacheTTLFromConfig(configMap map[string]struct {
	TTL     int
	Enabled bool
}) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	for engine, cfg := range configMap {
		o.engineCacheTTL[strings.ToLower(engine)] = EngineCacheTTLConfig{
			TTL:     time.Duration(cfg.TTL) * time.Second,
			Enabled: cfg.Enabled,
		}
	}
}

// GetEngineCacheTTL 获取引擎缓存TTL
func (o *EngineOrchestrator) GetEngineCacheTTL(engineName string) (time.Duration, bool) {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	if cfg, exists := o.engineCacheTTL[strings.ToLower(engineName)]; exists && cfg.Enabled {
		return cfg.TTL, true
	}
	return o.defaultCacheTTL, true
}

// IsCacheEnabledForEngine 检查引擎是否启用缓存
func (o *EngineOrchestrator) IsCacheEnabledForEngine(engineName string) bool {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	if cfg, exists := o.engineCacheTTL[strings.ToLower(engineName)]; exists {
		return cfg.Enabled
	}
	return true // 默认启用
}

// SetDefaultCacheTTL 设置默认缓存TTL
func (o *EngineOrchestrator) SetDefaultCacheTTL(ttl time.Duration) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.defaultCacheTTL = ttl
}

// SetConcurrency 设置并发数
func (o *EngineOrchestrator) SetConcurrency(concurrency int) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}
	if concurrency > MaxConcurrency {
		concurrency = MaxConcurrency
	}
	o.concurrency = concurrency
}

// GetConcurrency 获取当前并发设置。
func (o *EngineOrchestrator) GetConcurrency() int {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	return o.concurrency
}

// SetCircuitBreakerConfig 设置熔断器配置
func (o *EngineOrchestrator) SetCircuitBreakerConfig(engineName string, threshold int, resetDuration time.Duration) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	name := strings.ToLower(engineName)
	if _, exists := o.circuitBreakers[name]; !exists {
		o.circuitBreakers[name] = &CircuitBreaker{
			State:         CircuitClosed,
			Threshold:     threshold,
			ResetDuration: resetDuration,
		}
	} else {
		cb := o.circuitBreakers[name]
		cb.Threshold = threshold
		cb.ResetDuration = resetDuration
	}
}

// GetCircuitState 获取引擎熔断器状态
func (o *EngineOrchestrator) GetCircuitState(engineName string) CircuitState {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	cb, exists := o.circuitBreakers[strings.ToLower(engineName)]
	if !exists {
		return CircuitClosed
	}
	return cb.GetState()
}

// IsEngineCircuited 检查引擎是否被熔断（true = 应跳过）
func (o *EngineOrchestrator) IsEngineCircuited(engineName string) bool {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	cb, exists := o.circuitBreakers[strings.ToLower(engineName)]
	if !exists {
		return false
	}
	state, _, _, lastFailure, resetDuration := cb.GetStats()
	return state == CircuitOpen && time.Since(lastFailure) <= resetDuration
}

// RecordEngineSuccess 记录引擎成功（关闭熔断器）
func (o *EngineOrchestrator) RecordEngineSuccess(engineName string) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	cb, exists := o.circuitBreakers[strings.ToLower(engineName)]
	if !exists {
		return
	}
	cb.RecordSuccess()
}

// RecordEngineFailure 记录引擎失败（可能触发熔断）
func (o *EngineOrchestrator) RecordEngineFailure(engineName string) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	name := strings.ToLower(engineName)
	if _, exists := o.circuitBreakers[name]; !exists {
		o.circuitBreakers[name] = &CircuitBreaker{
			State:         CircuitClosed,
			Threshold:     DefaultCircuitBreakerThreshold,
			ResetDuration: DefaultCircuitBreakerDuration,
		}
	}
	cb := o.circuitBreakers[name]
	cb.RecordFailure()
	state, _, threshold, _, _ := cb.GetStats()
	if state == CircuitOpen {
		logger.Warnf("Circuit breaker opened for engine %s after %d consecutive failures", engineName, threshold)
	}
}

// GetCircuitBreakerStats 获取所有熔断器状态（用于调试/监控）
func (o *EngineOrchestrator) GetCircuitBreakerStats() map[string]map[string]interface{} {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	stats := make(map[string]map[string]interface{})
	for name, cb := range o.circuitBreakers {
		state, failures, threshold, lastFailure, resetDuration := cb.GetStats()
		stats[name] = map[string]interface{}{
			"state":         string(state),
			"failures":      failures,
			"threshold":     threshold,
			"last_failure":  lastFailure,
			"reset_duration": resetDuration,
		}
	}
	return stats
}

// RegisterAdapter 注册引擎适配器
func (o *EngineOrchestrator) RegisterAdapter(adapter EngineAdapter) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.adapters[adapter.Name()] = adapter
}

// GetAdapter 获取指定引擎适配器
func (o *EngineOrchestrator) GetAdapter(name string) (EngineAdapter, bool) {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	adapter, exists := o.adapters[name]
	return adapter, exists
}

// ListAdapters 列出所有适配器
func (o *EngineOrchestrator) ListAdapters() []string {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	names := make([]string, 0, len(o.adapters))
	for name := range o.adapters {
		names = append(names, name)
	}
	return names
}

// TranslateQuery 将UQL转换为各引擎查询
func (o *EngineOrchestrator) TranslateQuery(ast *model.UQLAST, engineNames []string) ([]model.EngineQuery, error) {
	if ast == nil {
		return nil, fmt.Errorf("AST cannot be nil")
	}
	if len(engineNames) == 0 {
		return nil, fmt.Errorf("engine names cannot be empty")
	}

	queries := []model.EngineQuery{}
	translateErrs := []string{}

	for _, name := range engineNames {
		// 跳过熔断的引擎
		if o.IsEngineCircuited(name) {
			logger.Warnf("Engine %s circuit breaker open, skipping translation", name)
			continue
		}

		adapter, exists := o.GetAdapter(name)
		if !exists {
			translateErrs = append(translateErrs, fmt.Sprintf("adapter %s not found", name))
			continue
		}

		query, err := adapter.Translate(ast)
		if err != nil {
			translateErrs = append(translateErrs, fmt.Sprintf("failed to translate for %s: %v", name, err))
			continue
		}

		queries = append(queries, model.EngineQuery{
			EngineName: name,
			Query:      query,
		})
	}

	if len(translateErrs) > 0 {
		logger.Warnf("query translation had partial failures: %s", strings.Join(translateErrs, "; "))
	}

	if len(queries) == 0 {
		if len(translateErrs) > 0 {
			return nil, fmt.Errorf("no translatable engines: %s", strings.Join(translateErrs, "; "))
		}
		return nil, fmt.Errorf("no translatable engines available")
	}

	return queries, nil
}

// SearchTask 搜索任务
// 实现workerpool.Task接口
type SearchTask struct {
	orchestrator  *EngineOrchestrator
	ctx           context.Context
	query         model.EngineQuery
	pageSize      int
	resultChan    chan *model.EngineResult
	errorChan     chan error
	wg            *sync.WaitGroup
	retryAttempts int
}

// Execute 执行搜索任务
func (t *SearchTask) Execute() error {
	defer t.wg.Done()
	startTime := time.Now()

	// 检查熔断器状态
	if t.orchestrator.IsEngineCircuited(t.query.EngineName) {
		logger.CtxWarnf(t.ctx, "Engine %s circuit breaker open, skipping", t.query.EngineName)
		metrics.IncEngineQuery(t.query.EngineName, "circuited")
		select {
		case t.resultChan <- &model.EngineResult{
			EngineName: t.query.EngineName,
			Error:      "circuit breaker open",
		}:
		default:
		}
		return nil
	}

	adapter, exists := t.orchestrator.GetAdapter(t.query.EngineName)
	if !exists {
		metrics.IncEngineQuery(t.query.EngineName, "error")
		t.orchestrator.RecordEngineFailure(t.query.EngineName)
		select {
		case t.errorChan <- fmt.Errorf("adapter %s not found", t.query.EngineName):
		default:
			logger.CtxErrorf(t.ctx, "failed to send error: adapter %s not found", t.query.EngineName)
		}
		return nil
	}

	// 生成缓存键
	cacheKey := utils.GenerateCacheKey(t.query.EngineName, t.query.Query, 1, t.pageSize)

	// 检查缓存中是否存在结果
	if cachedResults, found := t.orchestrator.cache.Get(cacheKey); found {
		// 缓存中存储的是已标准化的UnifiedAsset列表
		// 直接返回标准化结果，避免再次调用Normalize
		result := &model.EngineResult{
			EngineName:     t.query.EngineName,
			RawData:        []interface{}{}, // 空原始数据，表示来自缓存
			Total:          len(cachedResults),
			Page:           1,
			HasMore:        false,
			Cached:         true,
			NormalizedData: cachedResults, // 保存已标准化的数据
		}

		metrics.IncEngineQuery(t.query.EngineName, "cached")
		metrics.ObserveEngineQueryDuration(t.query.EngineName, time.Since(startTime))
		t.orchestrator.RecordEngineSuccess(t.query.EngineName)

		select {
		case t.resultChan <- result:
		default:
			logger.CtxErrorf(t.ctx, "failed to send cached result: channel full")
		}
		return nil
	}

	// 获取重试次数，默认为3次
	retryCount := t.retryAttempts
	if retryCount <= 0 {
		retryCount = 3
	}

	var result *model.EngineResult
	var err error

	// 执行搜索，带重试机制
	for attempt := 0; attempt <= retryCount; attempt++ {
		result, err = adapter.Search(t.query.Query, 1, t.pageSize)
		if err == nil {
			break
		}

		// 如果是最后一次尝试，不再重试
		if attempt == retryCount {
			logger.CtxErrorf(t.ctx, "%s search failed after %d attempts: %v", t.query.EngineName, retryCount+1, err)
			metrics.IncEngineQuery(t.query.EngineName, "error")
			metrics.IncEngineErrorByName(t.query.EngineName)
			t.orchestrator.RecordEngineFailure(t.query.EngineName)
			select {
			case t.errorChan <- fmt.Errorf("%s search error: %v", t.query.EngineName, err):
			default:
				logger.CtxErrorf(t.ctx, "failed to send error: %s search error: %v", t.query.EngineName, err)
			}
			return nil
		}

		// 指数退避策略
		backoff := time.Duration(1<<uint(attempt)) * 100 * time.Millisecond
		if backoff > 2*time.Second {
			backoff = 2 * time.Second
		}

		logger.CtxWarnf(t.ctx, "%s search attempt %d failed, retrying in %s: %v", t.query.EngineName, attempt+1, backoff, err)

		// 等待退避时间，但可以被上下文取消
		select {
		case <-time.After(backoff):
			continue
		case <-t.ctx.Done():
			logger.CtxWarnf(t.ctx, "%s search cancelled during retry: %v", t.query.EngineName, t.ctx.Err())
			select {
			case t.errorChan <- fmt.Errorf("%s search cancelled: %v", t.query.EngineName, t.ctx.Err()):
			default:
				logger.CtxErrorf(t.ctx, "failed to send cancellation error")
			}
			return nil
		}
	}

	// 标准化结果并存入缓存
	if result == nil {
		logger.CtxWarnf(t.ctx, "nil result from %s", t.query.EngineName)
		metrics.IncEngineQuery(t.query.EngineName, "error")
		select {
		case t.resultChan <- &model.EngineResult{
			EngineName: t.query.EngineName,
			Error:      "nil result from search",
		}:
		default:
			logger.CtxErrorf(t.ctx, "failed to send nil result error: channel full")
		}
		return nil
	}
	normalized, err := adapter.Normalize(result)
	if err != nil {
		logger.CtxWarnf(t.ctx, "failed to normalize results from %s: %v", t.query.EngineName, err)
		// 标准化失败，但仍返回原始结果
	} else if len(normalized) > 0 {
		// 使用按引擎的缓存TTL
		cacheTTL, _ := t.orchestrator.GetEngineCacheTTL(t.query.EngineName)
		t.orchestrator.cache.Set(cacheKey, normalized, cacheTTL)
	}

	metrics.IncEngineQuery(t.query.EngineName, "success")
	metrics.ObserveEngineQueryDuration(t.query.EngineName, time.Since(startTime))
	t.orchestrator.RecordEngineSuccess(t.query.EngineName)

	select {
	case t.resultChan <- result:
	default:
		logger.CtxErrorf(t.ctx, "failed to send result: channel full")
	}
	return nil
}

// SearchEngines 并行搜索多个引擎
func (o *EngineOrchestrator) SearchEngines(queries []model.EngineQuery, pageSize int) ([]*model.EngineResult, error) {
	return o.SearchEnginesWithContext(context.Background(), queries, pageSize)
}

// SearchEnginesWithContext 带上下文的并行搜索
func (o *EngineOrchestrator) SearchEnginesWithContext(ctx context.Context, queries []model.EngineQuery, pageSize int) ([]*model.EngineResult, error) {
	if len(queries) == 0 {
		return nil, fmt.Errorf("no queries provided")
	}

	// 限制并发数（使用 mutex 保护读取）
	concurrency := o.GetConcurrency()
	if len(queries) < concurrency {
		concurrency = len(queries)
	}

	// 创建工作池
	pool := workerpool.NewPool(concurrency)
	pool.Start()

	// 创建结果通道和错误通道
	resultChan := make(chan *model.EngineResult, len(queries))
	errorChan := make(chan error, len(queries))

	// 使用 WaitGroup 等待所有任务完成
	var wg sync.WaitGroup

	// 提交任务
	for _, q := range queries {
		wg.Add(1)
		task := &SearchTask{
			orchestrator:  o,
			ctx:           ctx,
			query:         q,
			pageSize:      pageSize,
			resultChan:    resultChan,
			errorChan:     errorChan,
			wg:            &wg,
			retryAttempts: 3, // 默认重试3次
		}
		pool.Submit(task)
	}

	// 在 goroutine 中等待所有任务完成并关闭通道
	go func() {
		wg.Wait()
		pool.Stop()
		close(resultChan)
		close(errorChan)
	}()

	// 收集结果
	results := []*model.EngineResult{}
	errs := []string{}

	// 使用 select 监听上下文取消和结果收集
	done := false
	for !done {
		select {
		case <-ctx.Done():
			return results, fmt.Errorf("search cancelled: %w", ctx.Err())
		case result, ok := <-resultChan:
			if ok && result != nil {
				results = append(results, result)
			} else if !ok {
				resultChan = nil
			}
		case err, ok := <-errorChan:
			if ok && err != nil {
				errs = append(errs, err.Error())
				logger.CtxErrorf(ctx, "engine search error: %v", err)
			} else if !ok {
				errorChan = nil
			}
		}
		if resultChan == nil && errorChan == nil {
			done = true
		}
	}

	if len(results) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("all engines failed: %s", strings.Join(errs, "; "))
	}

	return results, nil
}

// PaginatedSearchTask 分页搜索任务
type PaginatedSearchTask struct {
	orchestrator *EngineOrchestrator
	ctx          context.Context
	query        model.EngineQuery
	pageSize     int
	maxPages     int
	resultChan   chan *model.EngineResult
	wg           *sync.WaitGroup
}

// Execute 执行分页搜索任务
func (t *PaginatedSearchTask) Execute() error {
	defer t.wg.Done()

	adapter, exists := t.orchestrator.GetAdapter(t.query.EngineName)
	if !exists {
		select {
		case t.resultChan <- &model.EngineResult{
			EngineName: t.query.EngineName,
			Error:      fmt.Sprintf("failed to find adapter: %s", t.query.EngineName),
		}:
		default:
			logger.CtxErrorf(t.ctx, "Failed to send error: adapter %s not found", t.query.EngineName)
		}
		return nil
	}

	// 分页获取
	for page := 1; page <= t.maxPages; page++ {
		if t.ctx.Err() != nil {
			return nil
		}

		// 生成缓存键
		cacheKey := utils.GenerateCacheKey(t.query.EngineName, t.query.Query, page, t.pageSize)

		// 检查缓存中是否存在结果
		if cachedResults, found := t.orchestrator.cache.Get(cacheKey); found {
			// 缓存中存储的是已标准化的UnifiedAsset列表
			// 直接返回标准化结果，避免再次调用Normalize
			result := &model.EngineResult{
				EngineName:     t.query.EngineName,
				RawData:        []interface{}{},
				Total:          len(cachedResults),
				Page:           page,
				HasMore:        page < t.maxPages,
				Cached:         true,
				NormalizedData: cachedResults,
			}

			select {
			case t.resultChan <- result:
			default:
				logger.CtxErrorf(t.ctx, "Failed to send cached result: channel full")
			}
			continue
		}

		result, err := adapter.Search(t.query.Query, page, t.pageSize)
		if err != nil {
			select {
			case t.resultChan <- &model.EngineResult{
				EngineName: t.query.EngineName,
				Error:      fmt.Sprintf("search failed on page %d: %v", page, err),
			}:
			default:
				logger.CtxErrorf(t.ctx, "Failed to send error: search failed on page %d: %v", page, err)
			}
			break
		}

		// 检查 result 是否为 nil
		if result == nil {
			select {
			case t.resultChan <- &model.EngineResult{
				EngineName: t.query.EngineName,
				Error:      fmt.Sprintf("nil result on page %d", page),
			}:
			default:
				logger.CtxErrorf(t.ctx, "Failed to send error: nil result on page %d", page)
			}
			break
		}

		// 标准化结果并存入缓存
		normalized, err := adapter.Normalize(result)
		if err != nil {
			logger.CtxWarnf(t.ctx, "Failed to normalize results from %s page %d: %v", t.query.EngineName, page, err)
			// 标准化失败，但仍返回原始结果
		} else if len(normalized) > 0 {
			// 使用按引擎的缓存TTL
			cacheTTL, _ := t.orchestrator.GetEngineCacheTTL(t.query.EngineName)
			t.orchestrator.cache.Set(cacheKey, normalized, cacheTTL)
		}

		select {
		case t.resultChan <- result:
		default:
			logger.CtxErrorf(t.ctx, "Failed to send result: channel full")
		}

		if !result.HasMore || page >= t.maxPages {
			break
		}

		// 简单的速率控制
		time.Sleep(DefaultRateLimitDelay)
	}

	return nil
}

// SearchEnginesWithPagination 并行搜索多个引擎并支持分页
func (o *EngineOrchestrator) SearchEnginesWithPagination(queries []model.EngineQuery, pageSize, maxPages int) ([]*model.EngineResult, error) {
	return o.SearchEnginesWithPaginationAndContext(context.Background(), queries, pageSize, maxPages)
}

// SearchEnginesWithPaginationAndContext 带上下文的分页搜索
func (o *EngineOrchestrator) SearchEnginesWithPaginationAndContext(ctx context.Context, queries []model.EngineQuery, pageSize, maxPages int) ([]*model.EngineResult, error) {
	if len(queries) == 0 {
		return nil, fmt.Errorf("no queries provided")
	}

	// 限制并发数（使用 mutex 保护读取）
	concurrency := o.GetConcurrency()
	if len(queries) < concurrency {
		concurrency = len(queries)
	}

	// 创建工作池
	pool := workerpool.NewPool(concurrency)
	pool.Start()

	// 创建结果通道
	resultsChan := make(chan *model.EngineResult, len(queries)*maxPages)

	// 使用 WaitGroup 等待所有任务完成
	var wg sync.WaitGroup

	// 提交任务
	for _, q := range queries {
		wg.Add(1)
		task := &PaginatedSearchTask{
			orchestrator: o,
			ctx:          ctx,
			query:        q,
			pageSize:     pageSize,
			maxPages:     maxPages,
			resultChan:   resultsChan,
			wg:           &wg,
		}
		pool.Submit(task)
	}

	// 在 goroutine 中等待所有任务完成并关闭通道
	go func() {
		wg.Wait()
		pool.Stop()
		close(resultsChan)
	}()

	// 收集结果
	results := []*model.EngineResult{}

	// 使用 select 监听上下文取消和结果收集
	for {
		select {
		case <-ctx.Done():
			return results, fmt.Errorf("search cancelled: %w", ctx.Err())
		case result, ok := <-resultsChan:
			if !ok {
				return results, nil
			}
			if result != nil {
				results = append(results, result)
			}
		}
	}
}

// NormalizeResults 标准化引擎结果
func (o *EngineOrchestrator) NormalizeResults(engineResults []*model.EngineResult) ([]model.UnifiedAsset, error) {
	assets := []model.UnifiedAsset{}

	for _, result := range engineResults {
		if result == nil || result.Error != "" {
			continue
		}

		if result.Cached && len(result.NormalizedData) > 0 {
			assets = append(assets, result.NormalizedData...)
			continue
		}

		adapter, exists := o.GetAdapter(result.EngineName)
		if !exists {
			continue
		}

		normalized, err := adapter.Normalize(result)
		if err != nil {
			continue
		}

		assets = append(assets, normalized...)
	}

	return assets, nil
}

// ExecuteUnifiedQuery 执行统一查询（完整流程）
func (o *EngineOrchestrator) ExecuteUnifiedQuery(ast *model.UQLAST, engineNames []string, pageSize, maxPages int) ([]model.UnifiedAsset, error) {
	// 1. 翻译查询
	queries, err := o.TranslateQuery(ast, engineNames)
	if err != nil {
		return nil, fmt.Errorf("translate error: %v", err)
	}

	// 2. 并行搜索
	engineResults, err := o.SearchEnginesWithPagination(queries, pageSize, maxPages)
	if err != nil {
		return nil, fmt.Errorf("search error: %v", err)
	}

	// 3. 标准化
	assets, err := o.NormalizeResults(engineResults)
	if err != nil {
		return nil, fmt.Errorf("normalize error: %v", err)
	}

	return assets, nil
}
