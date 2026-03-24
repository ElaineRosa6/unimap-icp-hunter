package adapter

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/util/workerpool"
	"github.com/unimap-icp-hunter/project/internal/utils"
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
)

// EngineOrchestrator 引擎编排器
type EngineOrchestrator struct {
	adapters    map[string]EngineAdapter
	mutex       sync.RWMutex
	cache       utils.QueryCache
	concurrency int
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
		adapters:    make(map[string]EngineAdapter),
		cache:       cache,
		concurrency: DefaultConcurrency,
	}
}

// SetConcurrency 设置并发数
func (o *EngineOrchestrator) SetConcurrency(concurrency int) {
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
	return o.concurrency
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
	orchestrator *EngineOrchestrator
	ctx          context.Context
	query        model.EngineQuery
	pageSize     int
	resultChan   chan *model.EngineResult
	errorChan    chan error
	wg           *sync.WaitGroup
}

// Execute 执行搜索任务
func (t *SearchTask) Execute() error {
	defer t.wg.Done()

	adapter, exists := t.orchestrator.GetAdapter(t.query.EngineName)
	if !exists {
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

		select {
		case t.resultChan <- result:
		default:
			logger.CtxErrorf(t.ctx, "failed to send cached result: channel full")
		}
		return nil
	}

	// 执行搜索，获取第一页
	result, err := adapter.Search(t.query.Query, 1, t.pageSize)
	if err != nil {
		select {
		case t.errorChan <- fmt.Errorf("%s search error: %v", t.query.EngineName, err):
		default:
			logger.CtxErrorf(t.ctx, "failed to send error: %s search error: %v", t.query.EngineName, err)
		}
		return nil
	}

	// 标准化结果并存入缓存
	normalized, err := adapter.Normalize(result)
	if err != nil {
		logger.CtxWarnf(t.ctx, "failed to normalize results from %s: %v", t.query.EngineName, err)
		// 标准化失败，但仍返回原始结果
	} else if len(normalized) > 0 {
		t.orchestrator.cache.Set(cacheKey, normalized, DefaultCacheTTL)
	}

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

	// 限制并发数
	concurrency := o.concurrency
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
			orchestrator: o,
			ctx:          ctx,
			query:        q,
			pageSize:     pageSize,
			resultChan:   resultChan,
			errorChan:    errorChan,
			wg:           &wg,
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
			logger.Errorf("Failed to send error: adapter %s not found", t.query.EngineName)
		}
		return nil
	}

	// 分页获取
	for page := 1; page <= t.maxPages; page++ {
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
				logger.Errorf("Failed to send cached result: channel full")
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
				logger.Errorf("Failed to send error: search failed on page %d: %v", page, err)
			}
			break
		}

		// 标准化结果并存入缓存
		normalized, err := adapter.Normalize(result)
		if err != nil {
			logger.Warnf("Failed to normalize results from %s page %d: %v", t.query.EngineName, page, err)
			// 标准化失败，但仍返回原始结果
		} else if len(normalized) > 0 {
			t.orchestrator.cache.Set(cacheKey, normalized, DefaultCacheTTL)
		}

		select {
		case t.resultChan <- result:
		default:
			logger.Errorf("Failed to send result: channel full")
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

	// 限制并发数
	concurrency := o.concurrency
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
