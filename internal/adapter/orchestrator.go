package adapter

import (
	"fmt"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/model"
)

// EngineOrchestrator 引擎编排器
type EngineOrchestrator struct {
	adapters map[string]EngineAdapter
	mu       sync.RWMutex
}

// NewEngineOrchestrator 创建引擎编排器
func NewEngineOrchestrator() *EngineOrchestrator {
	return &EngineOrchestrator{
		adapters: make(map[string]EngineAdapter),
	}
}

// RegisterAdapter 注册引擎适配器
func (o *EngineOrchestrator) RegisterAdapter(adapter EngineAdapter) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.adapters[adapter.Name()] = adapter
}

// GetAdapter 获取指定引擎适配器
func (o *EngineOrchestrator) GetAdapter(name string) (EngineAdapter, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	adapter, exists := o.adapters[name]
	return adapter, exists
}

// ListAdapters 列出所有适配器
func (o *EngineOrchestrator) ListAdapters() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
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

// SearchEngines 并行搜索多个引擎
func (o *EngineOrchestrator) SearchEngines(queries []model.EngineQuery, pageSize int) ([]*model.EngineResult, error) {
	if len(queries) == 0 {
		return nil, fmt.Errorf("no queries provided")
	}

	var wg sync.WaitGroup
	results := make([]*model.EngineResult, len(queries))
	errors := make(chan error, len(queries))

	for i, q := range queries {
		wg.Add(1)
		go func(index int, query model.EngineQuery) {
			defer wg.Done()

			adapter, exists := o.GetAdapter(query.EngineName)
			if !exists {
				errors <- fmt.Errorf("adapter %s not found", query.EngineName)
				return
			}

			// 执行搜索，获取第一页
			result, err := adapter.Search(query.Query, 1, pageSize)
			if err != nil {
				errors <- fmt.Errorf("%s search error: %v", query.EngineName, err)
				return
			}

			results[index] = result
		}(i, q)
	}

	wg.Wait()
	close(errors)

	// 检查错误
	for err := range errors {
		return nil, err
	}

	return results, nil
}

// SearchEnginesWithPagination 并行搜索多个引擎并支持分页
func (o *EngineOrchestrator) SearchEnginesWithPagination(queries []model.EngineQuery, pageSize, maxPages int) ([]*model.EngineResult, error) {
	if len(queries) == 0 {
		return nil, fmt.Errorf("no queries provided")
	}

	var wg sync.WaitGroup
	resultsChan := make(chan *model.EngineResult, len(queries)*maxPages)

	for _, q := range queries {
		wg.Add(1)
		go func(query model.EngineQuery) {
			defer wg.Done()

			adapter, exists := o.GetAdapter(query.EngineName)
			if !exists {
				// Send error result for non-existent adapter
				resultsChan <- &model.EngineResult{
					EngineName: query.EngineName,
					Error:      fmt.Sprintf("adapter not found: %s", query.EngineName),
				}
				return
			}

			// 分页获取
			for page := 1; page <= maxPages; page++ {
				result, err := adapter.Search(query.Query, page, pageSize)
				if err != nil {
					// Send error result instead of silently breaking
					resultsChan <- &model.EngineResult{
						EngineName: query.EngineName,
						Error:      fmt.Sprintf("search failed on page %d: %v", page, err),
					}
					break
				}

				resultsChan <- result

				if !result.HasMore || page >= maxPages {
					break
				}

				// 简单的速率控制
				time.Sleep(100 * time.Millisecond)
			}
		}(q)
	}

	wg.Wait()
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
