package adapter

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unimap/project/internal/logger"
	"github.com/unimap/project/internal/metrics"
	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/utils"
	"github.com/unimap/project/internal/utils/workerpool"
)

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

	if t.orchestrator.IsEngineCircuited(t.query.EngineName) {
		logger.CtxWarnf(t.ctx, "Engine %s circuit breaker open, skipping", t.query.EngineName)
		metrics.IncEngineQuery(t.query.EngineName, "circuited")
		t.sendResult(&model.EngineResult{EngineName: t.query.EngineName, Error: "circuit breaker open"})
		return nil
	}

	adapter, exists := t.orchestrator.GetAdapter(t.query.EngineName)
	if !exists {
		metrics.IncEngineQuery(t.query.EngineName, "error")
		t.orchestrator.RecordEngineFailure(t.query.EngineName)
		t.sendError(fmt.Errorf("adapter %s not found", t.query.EngineName))
		return nil
	}

	page := t.query.Page
	if page <= 0 {
		page = 1
	}
	cacheKey := utils.GenerateCacheKey(t.query.EngineName, t.query.Query, page, t.pageSize)

	if cachedResults, found := t.orchestrator.getEnabledCachedAssets(t.query.EngineName, cacheKey); found {
		metrics.IncEngineQuery(t.query.EngineName, "cached")
		metrics.ObserveEngineQueryDuration(t.query.EngineName, time.Since(startTime))
		t.orchestrator.RecordEngineSuccess(t.query.EngineName)
		t.sendResult(&model.EngineResult{
			EngineName: t.query.EngineName, Total: len(cachedResults), Page: 1,
			Cached: true, NormalizedData: cachedResults, RawData: []interface{}{},
		})
		return nil
	}

	result, err := t.executeSearchWithRetry(adapter)
	if err != nil {
		return nil
	}
	t.normalizeAndCache(adapter, result, startTime)
	return nil
}

// executeSearchWithRetry 带指数退避的重试搜索
func (t *SearchTask) executeSearchWithRetry(adapter EngineAdapter) (*model.EngineResult, error) {
	retryCount := t.retryAttempts
	if retryCount <= 0 {
		retryCount = 3
	}

	for attempt := 0; attempt <= retryCount; attempt++ {
		page := t.query.Page
		if page <= 0 {
			page = 1
		}
		result, err := adapter.Search(t.ctx, t.query.Query, page, t.pageSize)
		if err == nil {
			if result == nil {
				logger.CtxWarnf(t.ctx, "nil result from %s", t.query.EngineName)
				metrics.IncEngineQuery(t.query.EngineName, "error")
				t.sendResult(&model.EngineResult{EngineName: t.query.EngineName, Error: "nil result from search"})
				return nil, fmt.Errorf("nil result")
			}
			return result, nil
		}
		if attempt == retryCount {
			logger.CtxErrorf(t.ctx, "%s search failed after %d attempts: %v", t.query.EngineName, retryCount+1, err)
			metrics.IncEngineQuery(t.query.EngineName, "error")
			metrics.IncEngineErrorByName(t.query.EngineName)
			t.orchestrator.RecordEngineFailure(t.query.EngineName)
			t.sendError(fmt.Errorf("%s search error: %w", t.query.EngineName, err))
			return nil, err
		}
		backoff := time.Duration(1<<uint(attempt)) * 100 * time.Millisecond
		if backoff > 2*time.Second {
			backoff = 2 * time.Second
		}
		logger.CtxWarnf(t.ctx, "%s search attempt %d failed, retrying in %s: %v", t.query.EngineName, attempt+1, backoff, err)
		select {
		case <-time.After(backoff):
		case <-t.ctx.Done():
			logger.CtxWarnf(t.ctx, "%s search cancelled during retry: %v", t.query.EngineName, t.ctx.Err())
			t.sendError(fmt.Errorf("%s search cancelled: %v", t.query.EngineName, t.ctx.Err()))
			return nil, t.ctx.Err()
		}
	}
	return nil, fmt.Errorf("exhausted retries")
}

// normalizeAndCache 标准化结果并存入缓存
func (t *SearchTask) normalizeAndCache(adapter EngineAdapter, result *model.EngineResult, startTime time.Time) {
	normalized, err := adapter.Normalize(result)
	if err != nil || len(normalized) == 0 {
		logger.CtxWarnf(t.ctx, "failed to normalize results from %s: %v", t.query.EngineName, err)
		t.sendResult(result)
		return
	}
	cacheTTL, _ := t.orchestrator.GetEngineCacheTTL(t.query.EngineName)
	if t.orchestrator.IsCacheEnabledForEngine(t.query.EngineName) {
		t.orchestrator.cache.Set(utils.GenerateCacheKey(t.query.EngineName, t.query.Query, result.Page, t.pageSize), normalized, cacheTTL)
	}
	metrics.IncEngineQuery(t.query.EngineName, "success")
	metrics.ObserveEngineQueryDuration(t.query.EngineName, time.Since(startTime))
	t.orchestrator.RecordEngineSuccess(t.query.EngineName)
	t.sendResult(result)
}

// sendResult 安全发送结果到 channel
func (t *SearchTask) sendResult(result *model.EngineResult) {
	select {
	case t.resultChan <- result:
	default:
		logger.CtxErrorf(t.ctx, "failed to send result: channel full")
	}
}

// sendError 安全发送错误到 channel
func (t *SearchTask) sendError(err error) {
	select {
	case t.errorChan <- err:
	default:
		logger.CtxErrorf(t.ctx, "failed to send error: %v", err)
	}
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
	// 确保任何返回路径（含 ctx 取消）都立即关闭工作池，避免空闲 worker
	// 在主函数提前返回后滞留。Stop 经 CAS 幂等，可与下方清理 goroutine 安全并存。
	defer pool.Stop()

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

	// 在 goroutine 中等待所有任务完成并关闭通道。
	// 不在此处调用 pool.Stop()：主函数已通过 defer pool.Stop() 兜底，
	// ctx 取消时可立即停止工作池，无需等待 wg.Wait() 完成。
	go func() {
		wg.Wait()
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
		t.sendPaginatedResult(&model.EngineResult{EngineName: t.query.EngineName, Error: fmt.Sprintf("failed to find adapter: %s", t.query.EngineName)})
		return nil
	}
	for page := 1; page <= t.maxPages; page++ {
		if t.ctx.Err() != nil {
			return nil
		}
		if page > 1 && t.orchestrator.IsEngineCircuited(t.query.EngineName) {
			logger.Warnf("circuit breaker opened, stopping pagination for %s at page %d", t.query.EngineName, page)
			break
		}
		if stop := t.fetchPaginatedPage(adapter, page); stop {
			break
		}
		time.Sleep(DefaultRateLimitDelay)
	}
	return nil
}

// fetchPaginatedPage 获取单页结果，返回 true 表示应停止分页
func (t *PaginatedSearchTask) fetchPaginatedPage(adapter EngineAdapter, page int) bool {
	cacheKey := utils.GenerateCacheKey(t.query.EngineName, t.query.Query, page, t.pageSize)
	if cachedResults, found := t.orchestrator.getEnabledCachedAssets(t.query.EngineName, cacheKey); found {
		t.sendPaginatedResult(&model.EngineResult{
			EngineName: t.query.EngineName, Page: page, HasMore: page < t.maxPages,
			Cached: true, NormalizedData: cachedResults, RawData: []interface{}{}, Total: len(cachedResults),
		})
		return false
	}
	result, err := adapter.Search(t.ctx, t.query.Query, page, t.pageSize)
	if err != nil {
		t.sendPaginatedResult(&model.EngineResult{EngineName: t.query.EngineName, Error: fmt.Sprintf("search failed on page %d: %v", page, err)})
		return true
	}
	if result == nil {
		t.sendPaginatedResult(&model.EngineResult{EngineName: t.query.EngineName, Error: fmt.Sprintf("nil result on page %d", page)})
		return true
	}
	normalized, nErr := adapter.Normalize(result)
	if nErr != nil {
		logger.CtxWarnf(t.ctx, "Failed to normalize results from %s page %d: %v", t.query.EngineName, page, nErr)
	} else if len(normalized) > 0 && t.orchestrator.IsCacheEnabledForEngine(t.query.EngineName) {
		cacheTTL, _ := t.orchestrator.GetEngineCacheTTL(t.query.EngineName)
		t.orchestrator.cache.Set(cacheKey, normalized, cacheTTL)
	}
	t.sendPaginatedResult(result)
	return !result.HasMore || page >= t.maxPages
}

func (t *PaginatedSearchTask) sendPaginatedResult(result *model.EngineResult) {
	select {
	case t.resultChan <- result:
	default:
		logger.CtxErrorf(t.ctx, "Failed to send result: channel full")
	}
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

	concurrency := o.GetConcurrency()
	if len(queries) < concurrency {
		concurrency = len(queries)
	}

	pool := workerpool.NewPool(concurrency)
	pool.Start()

	resultsChan := make(chan *model.EngineResult, len(queries)*maxPages)
	var wg sync.WaitGroup

	for _, q := range queries {
		wg.Add(1)
		pool.Submit(&PaginatedSearchTask{
			orchestrator: o, ctx: ctx, query: q,
			pageSize: pageSize, maxPages: maxPages,
			resultChan: resultsChan, wg: &wg,
		})
	}

	go func() {
		wg.Wait()
		pool.Stop()
		close(resultsChan)
	}()

	return collectPaginatedResults(ctx, resultsChan)
}

// collectPaginatedResults drains the results channel, respecting context cancellation.
func collectPaginatedResults(ctx context.Context, resultsChan <-chan *model.EngineResult) ([]*model.EngineResult, error) {
	results := []*model.EngineResult{}
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

// Cache policy is checked at each access, including after an upstream query.
func (o *EngineOrchestrator) getEnabledCachedAssets(engine, key string) ([]model.UnifiedAsset, bool) {
	if !o.IsCacheEnabledForEngine(engine) {
		return nil, false
	}
	return o.cache.Get(key)
}
