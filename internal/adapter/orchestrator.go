package adapter

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/util/workerpool"
	"github.com/unimap-icp-hunter/project/internal/utils"
)

// EngineOrchestrator 引擎编排器
type EngineOrchestrator struct {
	adapters map[string]EngineAdapter
	mutex    sync.RWMutex
	cache    utils.QueryCache
}

// NewEngineOrchestrator 创建引擎编排器
func NewEngineOrchestrator() *EngineOrchestrator {
	// 默认使用内存缓存，后续可通过配置文件启用Redis
	cache := utils.NewCache(
		false,            // 默认不使用Redis
		"localhost:6379", // Redis地址
		"",               // Redis密码
		0,                // Redis数据库
		"unimap:",        // Redis键前缀
		1000,             // 内存缓存最大大小
		5*time.Minute,    // 清理间隔
	)

	return &EngineOrchestrator{
		adapters: make(map[string]EngineAdapter),
		cache:    cache,
	}
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

	queries := []model.EngineQuery{}

	for _, name := range engineNames {
		adapter, exists := o.GetAdapter(name)
		if !exists {
			continue
		}

		query, err := adapter.Translate(ast)
		if err != nil {
			return nil, fmt.Errorf("failed to translate for %s: %v", name, err)
		}

		queries = append(queries, model.EngineQuery{
			EngineName: name,
			Query:      query,
		})
	}

	return queries, nil
}

// SearchTask 搜索任务
// 实现workerpool.Task接口
type SearchTask struct {
	orchestrator *EngineOrchestrator
	query        model.EngineQuery
	pageSize     int
	resultChan   chan *model.EngineResult
	errorChan    chan error
}

// Execute 执行搜索任务
func (t *SearchTask) Execute() error {
	adapter, exists := t.orchestrator.GetAdapter(t.query.EngineName)
	if !exists {
		t.errorChan <- fmt.Errorf("adapter %s not found", t.query.EngineName)
		return nil
	}

	// 生成缓存键
	cacheKey := utils.GenerateCacheKey(t.query.EngineName, t.query.Query, 1, t.pageSize)

	// 检查缓存中是否存在结果
	if cachedResults, found := t.orchestrator.cache.Get(cacheKey); found {
		// 构建EngineResult
		result := &model.EngineResult{
			EngineName: t.query.EngineName,
			RawData:    make([]interface{}, len(cachedResults)),
			Total:      len(cachedResults),
			Page:       1,
			HasMore:    false,
		}
		// 转换为RawData格式
		for i, asset := range cachedResults {
			result.RawData[i] = asset.Extra
		}

		t.resultChan <- result
		return nil
	}

	// 执行搜索，获取第一页
	result, err := adapter.Search(t.query.Query, 1, t.pageSize)
	if err != nil {
		t.errorChan <- fmt.Errorf("%s search error: %v", t.query.EngineName, err)
		return nil
	}

	// 标准化结果并存入缓存
	if normalized, err := adapter.Normalize(result); err == nil && len(normalized) > 0 {
		t.orchestrator.cache.Set(cacheKey, normalized, 30*time.Minute)
	}

	t.resultChan <- result
	return nil
}

// SearchEngines 并行搜索多个引擎
func (o *EngineOrchestrator) SearchEngines(queries []model.EngineQuery, pageSize int) ([]*model.EngineResult, error) {
	if len(queries) == 0 {
		return nil, fmt.Errorf("no queries provided")
	}

	// 限制并发数，最大不超过8个
	concurrency := 5
	if len(queries) < concurrency {
		concurrency = len(queries)
	}

	// 创建工作池
	pool := workerpool.NewPool(concurrency)
	pool.Start()
	defer pool.Stop()

	// 创建结果通道
	resultChan := make(chan *model.EngineResult, len(queries))
	errorChan := make(chan error, len(queries))

	// 提交任务
	for _, q := range queries {
		task := &SearchTask{
			orchestrator: o,
			query:        q,
			pageSize:     pageSize,
			resultChan:   resultChan,
			errorChan:    errorChan,
		}
		pool.Submit(task)
	}

	// 关闭工作池并等待所有任务完成
	pool.Stop()

	// 关闭结果通道和错误通道
	close(resultChan)
	close(errorChan)

	// 收集结果
	results := []*model.EngineResult{}
	for result := range resultChan {
		results = append(results, result)
	}

	// 检查是否有任何成功的结果
	// 即使有错误，只要有部分结果成功，也返回部分结果
	// 所有的错误已经通过日志记录（如果有日志系统的话），或者我们应该把错误也返回出去
	// 但鉴于函数签名，我们优先返回得到的结果
	// 如果没有任何结果且有错误，才返回错误

	errs := []string{}
	for err := range errorChan {
		if err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(results) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("all engines failed: %s", strings.Join(errs, "; "))
	}

	// 这里可以考虑把错误信息附加到结果中，但目前架构可能不支持
	// 或者我们可以简单地忽略部分错误，只返回成功的

	return results, nil
}

// PaginatedSearchTask 分页搜索任务
type PaginatedSearchTask struct {
	orchestrator *EngineOrchestrator
	query        model.EngineQuery
	pageSize     int
	maxPages     int
	resultChan   chan *model.EngineResult
}

// Execute 执行分页搜索任务
func (t *PaginatedSearchTask) Execute() error {
	adapter, exists := t.orchestrator.GetAdapter(t.query.EngineName)
	if !exists {
		t.resultChan <- &model.EngineResult{
			EngineName: t.query.EngineName,
			Error:      fmt.Sprintf("failed to find adapter: %s", t.query.EngineName),
		}
		return nil
	}

	// 分页获取
	for page := 1; page <= t.maxPages; page++ {
		// 生成缓存键
		cacheKey := utils.GenerateCacheKey(t.query.EngineName, t.query.Query, page, t.pageSize)

		// 检查缓存中是否存在结果
		if cachedResults, found := t.orchestrator.cache.Get(cacheKey); found {
			// 构建EngineResult
			result := &model.EngineResult{
				EngineName: t.query.EngineName,
				RawData:    make([]interface{}, len(cachedResults)),
				Total:      len(cachedResults),
				Page:       page,
				HasMore:    page < t.maxPages,
			}
			// 转换为RawData格式
			for i, asset := range cachedResults {
				result.RawData[i] = asset.Extra
			}

			t.resultChan <- result
			continue
		}

		result, err := adapter.Search(t.query.Query, page, t.pageSize)
		if err != nil {
			t.resultChan <- &model.EngineResult{
				EngineName: t.query.EngineName,
				Error:      fmt.Sprintf("search failed on page %d: %v", page, err),
			}
			break
		}

		// 标准化结果并存入缓存
		if normalized, err := adapter.Normalize(result); err == nil && len(normalized) > 0 {
			t.orchestrator.cache.Set(cacheKey, normalized, 30*time.Minute)
		}

		t.resultChan <- result

		if !result.HasMore || page >= t.maxPages {
			break
		}

		// 简单的速率控制
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// SearchEnginesWithPagination 并行搜索多个引擎并支持分页
func (o *EngineOrchestrator) SearchEnginesWithPagination(queries []model.EngineQuery, pageSize, maxPages int) ([]*model.EngineResult, error) {
	if len(queries) == 0 {
		return nil, fmt.Errorf("no queries provided")
	}

	// 限制并发数，最大不超过8个
	concurrency := 5
	if len(queries) < concurrency {
		concurrency = len(queries)
	}

	// 创建工作池
	pool := workerpool.NewPool(concurrency)
	pool.Start()

	// 创建结果通道
	resultsChan := make(chan *model.EngineResult, len(queries)*maxPages)

	// 提交任务
	for _, q := range queries {
		task := &PaginatedSearchTask{
			orchestrator: o,
			query:        q,
			pageSize:     pageSize,
			maxPages:     maxPages,
			resultChan:   resultsChan,
		}
		pool.Submit(task)
	}

	// 关闭工作池并等待所有任务完成
	pool.Stop()

	// 关闭结果通道
	close(resultsChan)

	// 收集结果
	results := []*model.EngineResult{}
	for result := range resultsChan {
		results = append(results, result)
	}

	return results, nil
}

// NormalizeResults 标准化引擎结果
func (o *EngineOrchestrator) NormalizeResults(engineResults []*model.EngineResult) ([]model.UnifiedAsset, error) {
	assets := []model.UnifiedAsset{}

	for _, result := range engineResults {
		if result == nil || result.Error != "" {
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
