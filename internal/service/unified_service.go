package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unimap/project/internal/adapter"
	"github.com/unimap/project/internal/collection"
	"github.com/unimap/project/internal/config"
	"github.com/unimap/project/internal/core/unimap"
	"github.com/unimap/project/internal/logger"
	"github.com/unimap/project/internal/metrics"
	"github.com/unimap/project/internal/model"
	"github.com/unimap/project/internal/plugin"
	"github.com/unimap/project/internal/utils"
)

// BrowserFallbackConfig holds browser fallback settings.
type BrowserFallbackConfig struct {
	Enabled       bool
	OnAPIError    bool
	OnEmptyResult bool
	Engines       map[string]bool // set of allowed engine names (lowercase)
}

// UnifiedService 统一服务层 - 为 CLI、GUI 和 Web 提供统一接口
type UnifiedService struct {
	pluginManager      *plugin.PluginManager
	orchestrator       *adapter.EngineOrchestrator
	parser             *unimap.UQLParser
	merger             *unimap.ResultMerger
	cache              utils.QueryCache
	cacheTTL           time.Duration
	cacheMaxSize       int
	cacheCleanup       time.Duration
	cacheBackend       string
	strategyManager    *utils.CacheStrategyManager
	mu                 sync.RWMutex                // nolint:unused
	maxMemoryMB        int                         // 最大内存使用限制（MB）
	maxConcurrent      int                         // 最大并发查询数
	activeQueries      int                         // 当前活跃查询数
	queryMutex         sync.Mutex                  // 查询并发控制锁
	browserBackend     adapter.BrowserQueryBackend // browser fallback backend
	browserFallbackCfg *BrowserFallbackConfig      // fallback configuration
}

// NewUnifiedService 创建统一服务
func NewUnifiedService() *UnifiedService {
	return NewUnifiedServiceWithConfig(nil)
}

// NewUnifiedServiceWithConfig 使用配置创建统一服务。
func NewUnifiedServiceWithConfig(cfg *config.Config) *UnifiedService {
	svcCfg := extractServiceConfig(cfg)
	cache := utils.NewCacheWithConfig(svcCfg.cacheBackend, svcCfg.redisCfg, svcCfg.memoryMaxSize, svcCfg.cacheCleanupInterval)
	strategyManager, configStrategy := initCacheStrategies(svcCfg.cacheTTL)
	orchestrator := initOrchestrator(cfg, svcCfg, configStrategy)

	if _, ok := cache.(*utils.RedisCache); !ok {
		svcCfg.cacheBackend = "memory"
	}

	return &UnifiedService{
		pluginManager: plugin.NewPluginManager(), orchestrator: orchestrator,
		parser: unimap.NewUQLParser(), merger: unimap.NewResultMerger(),
		cache: cache, cacheTTL: svcCfg.cacheTTL, cacheMaxSize: svcCfg.memoryMaxSize,
		cacheCleanup: svcCfg.cacheCleanupInterval, cacheBackend: svcCfg.cacheBackend,
		strategyManager: strategyManager, maxMemoryMB: svcCfg.maxMemoryMB,
		maxConcurrent: svcCfg.maxConcurrent,
	}
}

type serviceConfig struct {
	cacheTTL             time.Duration
	cacheCleanupInterval time.Duration
	memoryMaxSize        int
	cacheBackend         string
	maxMemoryMB          int
	maxConcurrent        int
	redisCfg             utils.RedisConfig
}

func extractServiceConfig(cfg *config.Config) serviceConfig {
	c := serviceConfig{
		cacheTTL: 30 * time.Minute, cacheCleanupInterval: 5 * time.Minute,
		memoryMaxSize: 1000, cacheBackend: "memory", maxMemoryMB: 512, maxConcurrent: 10,
	}
	if cfg == nil {
		return c
	}
	if cfg.System.CacheTTL > 0 {
		c.cacheTTL = time.Duration(cfg.System.CacheTTL) * time.Second
	}
	if cfg.System.CacheCleanupInterval > 0 {
		c.cacheCleanupInterval = time.Duration(cfg.System.CacheCleanupInterval) * time.Second
	}
	if cfg.System.CacheMaxSize > 0 {
		c.memoryMaxSize = cfg.System.CacheMaxSize
	}
	if cfg.System.MaxConcurrent > 0 {
		c.maxConcurrent = cfg.System.MaxConcurrent
	}
	c.cacheBackend = strings.ToLower(strings.TrimSpace(cfg.Cache.Backend))
	if c.cacheBackend == "" {
		c.cacheBackend = "memory"
	}
	c.redisCfg = buildRedisConfig(cfg)
	return c
}

func buildRedisConfig(cfg *config.Config) utils.RedisConfig {
	r := cfg.Cache.Redis
	return utils.RedisConfig{
		Addr: strings.TrimSpace(r.Addr), Password: r.Password, DB: r.DB,
		Prefix: strings.TrimSpace(r.Prefix), PoolSize: r.PoolSize,
		MinIdleConns: r.MinIdleConns, MaxIdleConns: r.MaxIdleConns, MaxRetries: r.MaxRetries,
		DialTimeout:     time.Duration(r.DialTimeout) * time.Millisecond,
		ReadTimeout:     time.Duration(r.ReadTimeout) * time.Millisecond,
		WriteTimeout:    time.Duration(r.WriteTimeout) * time.Millisecond,
		PoolTimeout:     time.Duration(r.PoolTimeout) * time.Millisecond,
		ConnMaxLifetime: time.Duration(r.ConnMaxLifetime) * time.Millisecond,
		ConnMaxIdleTime: time.Duration(r.ConnMaxIdleTime) * time.Millisecond,
	}
}

func initCacheStrategies(cacheTTL time.Duration) (*utils.CacheStrategyManager, *utils.ConfigBasedCacheStrategy) {
	sm := utils.NewCacheStrategyManager()
	sm.RegisterStrategy("dynamic", utils.NewDynamicCacheStrategy(cacheTTL, 5*time.Minute, 2*time.Hour))
	cs := utils.NewConfigBasedCacheStrategy(cacheTTL)
	sm.RegisterStrategy("config", cs)
	return sm, cs
}

func initOrchestrator(cfg *config.Config, svcCfg serviceConfig, configStrategy *utils.ConfigBasedCacheStrategy) *adapter.EngineOrchestrator {
	useRedis := strings.EqualFold(svcCfg.cacheBackend, "redis")
	orch := adapter.NewEngineOrchestratorWithConfig(useRedis, svcCfg.redisCfg.Addr, svcCfg.redisCfg.Password, svcCfg.redisCfg.DB)
	if cfg != nil {
		orch.SetConcurrency(cfg.System.MaxConcurrent)
		orch.SetDefaultCacheTTL(svcCfg.cacheTTL)
		for engineName, engineCfg := range cfg.Cache.Engines {
			ttl := svcCfg.cacheTTL
			if engineCfg.TTL > 0 {
				ttl = time.Duration(engineCfg.TTL) * time.Second
			}
			orch.SetEngineCacheTTL(engineName, ttl, engineCfg.Enabled)
			configStrategy.SetEngineConfig(engineName, &utils.SimpleEngineCacheConfig{
				Enabled: engineCfg.Enabled, TTL: ttl, MaxSize: engineCfg.MaxSize,
			})
		}
	}
	return orch
}

// RegisterAdapter 注册引擎适配器
func (s *UnifiedService) RegisterAdapter(adapter adapter.EngineAdapter) {
	s.orchestrator.RegisterAdapter(adapter)
}

// GetOrchestrator 获取引擎编排器
func (s *UnifiedService) GetOrchestrator() *adapter.EngineOrchestrator {
	return s.orchestrator
}

// SetWebOnlyBrowserBackend wires a browser query backend into all WebOnly adapters.
func (s *UnifiedService) SetWebOnlyBrowserBackend(backend adapter.BrowserQueryBackend) {
	s.browserBackend = backend
	s.orchestrator.SetWebOnlyBrowserBackend(backend)
}

// SetBrowserFallbackConfig configures the browser fallback behavior.
func (s *UnifiedService) SetBrowserFallbackConfig(cfg BrowserFallbackConfig) {
	s.browserFallbackCfg = &cfg
}

// QueryRequest 查询请求
type QueryRequest struct {
	Query       string   // UQL 查询语句
	Engines     []string // 要使用的引擎列表
	PageSize    int      // 每页大小
	ProcessData bool     // 是否处理数据（去重、清洗等）
}

// QueryResponse 查询响应
type QueryResponse struct {
	Assets      []model.UnifiedAsset // 查询结果
	TotalCount  int                  // 总数量
	EngineStats map[string]int       // 各引擎统计
	Errors      []string             // 错误信息
	Persistence PersistenceStatus    // 查询历史持久化状态
}

type PersistenceStatus struct {
	Status  string `json:"status,omitempty"`
	Warning string `json:"warning,omitempty"`
}

// Query 执行查询
func (s *UnifiedService) Query(ctx context.Context, req QueryRequest) (*QueryResponse, error) {
	queryStart := time.Now()
	queryStatus := "success"
	logger.CtxInfof(ctx, "query start: engines=%v page_size=%d", req.Engines, req.PageSize)
	defer func() {
		metrics.IncQueryRequest(queryStatus)
		metrics.ObserveQueryDuration(queryStatus, time.Since(queryStart))
		logger.CtxInfof(ctx, "query finish: status=%s duration=%s", queryStatus, time.Since(queryStart))
	}()

	if err := s.validateQueryRequest(&req); err != nil {
		queryStatus = "error"
		return nil, err
	}
	if err := s.checkResourceLimits(ctx); err != nil {
		queryStatus = "error"
		return nil, err
	}
	if !s.acquireQueryLock() {
		queryStatus = "error"
		return nil, fmt.Errorf("too many concurrent queries, please try again later")
	}
	defer s.releaseQueryLock()

	cacheKey := s.buildQueryCacheKey(req)
	if resp, ok := s.handleCachedQueryResult(ctx, req, cacheKey); ok {
		return resp, nil
	}
	metrics.ObserveCacheLookup(s.cacheBackend, "miss")
	logger.CtxDebugf(ctx, "query cache miss: backend=%s", s.cacheBackend)

	if err := s.pluginManager.GetHooks().TriggerHook(plugin.HookBeforeQuery, "query", &model.HookData{
		Query:   req.Query,
		Engines: req.Engines,
		Extra:   map[string]any{"cached": false},
	}); err != nil {
		queryStatus = "error"
		return nil, fmt.Errorf("pre-query hook failed: %w", err)
	}

	allAssets, engineStats, queryErrors, err := s.executeAndNormalizeQuery(ctx, req)
	if err != nil {
		queryStatus = "error"
		return nil, err
	}

	if len(allAssets) > 0 && s.queryCacheEnabled(req) {
		cacheTTL := s.resolveCacheTTL(req)
		if snapshotCache, ok := s.cache.(utils.QuerySnapshotCache); ok {
			snapshotCache.SetQuerySnapshot(cacheKey, allAssets, utils.QueryCacheMetadata{
				EngineStats: engineStats,
				Errors:      queryErrors,
			}, cacheTTL)
		}
	}

	if err := s.pluginManager.GetHooks().TriggerHook(plugin.HookAfterQuery, "query", &model.HookData{
		Extra: map[string]any{"result_count": len(allAssets), "cached": false},
	}); err != nil {
		logger.CtxWarnf(ctx, "post-query hook failed: %v", err)
	}

	resp := &QueryResponse{
		Assets: allAssets, TotalCount: len(allAssets),
		EngineStats: engineStats, Errors: queryErrors,
	}
	if len(engineStats) == 0 && len(queryErrors) > 0 {
		queryStatus = "error"
		return resp, fmt.Errorf("all query engines failed: %s", strings.Join(queryErrors, "; "))
	}
	return resp, nil
}

// validateQueryRequest 验证查询请求参数
func (s *UnifiedService) validateQueryRequest(req *QueryRequest) error {
	if req.Query == "" {
		return fmt.Errorf("query cannot be empty")
	}
	if len(req.Engines) == 0 {
		return fmt.Errorf("at least one engine must be specified")
	}
	if req.PageSize <= 0 {
		req.PageSize = 100
	}
	return nil
}

// buildQueryCacheKey 构建查询缓存键（SHA256 避免特殊字符冲突）
func (s *UnifiedService) buildQueryCacheKey(req QueryRequest) string {
	sortedEngines := make([]string, len(req.Engines))
	copy(sortedEngines, req.Engines)
	sort.Strings(sortedEngines)
	keyData := fmt.Sprintf("v2|%s|%s|%d|%t", strings.Join(sortedEngines, ","), req.Query, req.PageSize, req.ProcessData)
	hash := sha256.Sum256([]byte(keyData))
	return hex.EncodeToString(hash[:])
}

// handleCachedQueryResult 处理缓存命中逻辑，返回 (响应, true) 表示命中
func (s *UnifiedService) handleCachedQueryResult(ctx context.Context, req QueryRequest, cacheKey string) (*QueryResponse, bool) {
	if !s.queryCacheEnabled(req) {
		return nil, false
	}
	snapshotCache, ok := s.cache.(utils.QuerySnapshotCache)
	if !ok {
		return nil, false
	}
	cachedAssets, metadata, found := snapshotCache.GetQuerySnapshot(cacheKey)
	if !found {
		return nil, false
	}
	metrics.ObserveCacheLookup(s.cacheBackend, "hit")
	logger.CtxDebugf(ctx, "query cache hit: backend=%s", s.cacheBackend)

	if err := s.pluginManager.GetHooks().TriggerHook(plugin.HookBeforeQuery, "query", &model.HookData{
		Query:   req.Query,
		Engines: req.Engines,
		Extra:   map[string]any{"cached": true},
	}); err != nil {
		logger.CtxWarnf(ctx, "pre-query hook failed: %v", err)
		return nil, false
	}

	if err := s.pluginManager.GetHooks().TriggerHook(plugin.HookAfterQuery, "query", &model.HookData{
		Extra: map[string]any{"result_count": len(cachedAssets), "cached": true},
	}); err != nil {
		logger.CtxWarnf(ctx, "post-query hook failed: %v", err)
	}
	return &QueryResponse{
		Assets: cachedAssets, TotalCount: len(cachedAssets),
		EngineStats: metadata.EngineStats, Errors: metadata.Errors,
	}, true
}

// executeAndNormalizeQuery 执行引擎搜索、规范化、合并结果
func (s *UnifiedService) executeAndNormalizeQuery(ctx context.Context, req QueryRequest) ([]model.UnifiedAsset, map[string]int, []string, error) {
	var queryErrors []string

	ast, err := s.parser.Parse(req.Query)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse query: %w", err)
	}
	queries, err := s.orchestrator.TranslateQuery(ast, req.Engines)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to translate query: %w", err)
	}

	engineResults, err := s.orchestrator.SearchEnginesWithContext(ctx, queries, req.PageSize)
	if err != nil {
		queryErrors = append(queryErrors, err.Error())
		if hookErr := s.pluginManager.GetHooks().TriggerHook(plugin.HookQueryError, "query", &model.HookData{
			Error: err.Error(),
		}); hookErr != nil {
			logger.CtxWarnf(ctx, "query error hook failed: %v", hookErr)
		}
	}

	engineResults = s.tryBrowserFallback(ctx, engineResults, queries)
	allAssets, engineStats := s.normalizeEngineResults(ctx, engineResults, &queryErrors)

	if len(allAssets) > 0 && s.merger != nil {
		mergeResult := s.merger.Merge(allAssets)
		sorted := s.merger.GetSortedAssets(mergeResult)
		allAssets = make([]model.UnifiedAsset, 0, len(sorted))
		for _, asset := range sorted {
			if asset != nil {
				allAssets = append(allAssets, *asset)
			}
		}
	}

	if req.ProcessData {
		allAssets, err = s.processAssets(ctx, allAssets)
		if err != nil {
			queryErrors = append(queryErrors, fmt.Sprintf("data processing failed: %v", err))
			metrics.IncEngineError()
		}
	}

	return allAssets, engineStats, queryErrors, nil
}

// normalizeEngineResults 规范化各引擎返回结果
func (s *UnifiedService) normalizeEngineResults(ctx context.Context, engineResults []*model.EngineResult, queryErrors *[]string) ([]model.UnifiedAsset, map[string]int) {
	var allAssets []model.UnifiedAsset
	engineStats := make(map[string]int)

	for _, result := range engineResults {
		if result == nil {
			continue
		}
		if result.Error != "" {
			*queryErrors = append(*queryErrors, fmt.Sprintf("engine %s error: %s", result.EngineName, result.Error))
			metrics.IncEngineError()
			continue
		}
		if result.Cached && result.NormalizedData != nil {
			allAssets = append(allAssets, result.NormalizedData...)
			engineStats[result.EngineName] = len(result.NormalizedData)
			continue
		}
		adapterInstance, exists := s.orchestrator.GetAdapter(result.EngineName)
		if !exists {
			*queryErrors = append(*queryErrors, fmt.Sprintf("adapter for engine %s not found", result.EngineName))
			metrics.IncEngineError()
			continue
		}
		assets, err := adapterInstance.Normalize(result)
		if err != nil {
			*queryErrors = append(*queryErrors, fmt.Sprintf("failed to normalize results from %s: %v", result.EngineName, err))
			metrics.IncEngineError()
			continue
		}
		allAssets = append(allAssets, assets...)
		engineStats[result.EngineName] = len(assets)
	}
	return allAssets, engineStats
}

// tryBrowserFallback attempts browser collection for engines that failed API queries.
func (s *UnifiedService) tryBrowserFallback(ctx context.Context, engineResults []*model.EngineResult, queries []model.EngineQuery) []*model.EngineResult {
	if s.browserFallbackCfg == nil || !s.browserFallbackCfg.Enabled || s.browserBackend == nil {
		return engineResults
	}

	// Build query map for lookup
	queryMap := make(map[string]string, len(queries))
	for _, q := range queries {
		queryMap[strings.ToLower(q.EngineName)] = q.Query
	}

	var fallbackResults []*model.EngineResult
	for _, result := range engineResults {
		if result == nil {
			continue
		}
		engine := strings.ToLower(result.EngineName)

		// Check if this engine is in the whitelist
		if !s.browserFallbackCfg.Engines[engine] {
			continue
		}

		// Determine if fallback should trigger
		shouldFallback := false
		var trigger string
		if result.Error != "" && s.browserFallbackCfg.OnAPIError {
			shouldFallback = true
			trigger = "api_error"
		} else if result.Error == "" && len(result.NormalizedData) == 0 && s.browserFallbackCfg.OnEmptyResult {
			shouldFallback = true
			trigger = "empty_result"
		}

		if !shouldFallback {
			continue
		}
		metrics.IncBrowserFallbackTriggered(engine, trigger)

		query, ok := queryMap[engine]
		if !ok || query == "" {
			continue
		}

		logger.CtxInfof(ctx, "browser fallback triggered for engine %s", engine)
		queryID := fmt.Sprintf("fallback_%s_%d", engine, time.Now().UnixNano())
		collectResults, err := s.browserBackend.CollectSearchEngineResult(ctx, engine, query, queryID)
		if err != nil {
			metrics.IncBrowserFallbackFailure(engine, "backend_error")
			logger.CtxWarnf(ctx, "browser fallback failed for engine %s: %v", engine, err)
			continue
		}

		// Build assets from collected results and tag source
		var assets []model.UnifiedAsset
		for _, cr := range collectResults {
			for _, asset := range cr.Assets {
				if asset.Extra == nil {
					asset.Extra = make(map[string]interface{})
				}
				asset.Extra["collection_method"] = "browser_fallback"
				assets = append(assets, asset)
			}
		}
		collection.NormalizeAssets(engine, assets)

		if len(assets) > 0 {
			metrics.IncBrowserFallbackSuccess(engine)
			fallbackResults = append(fallbackResults, &model.EngineResult{
				EngineName:     result.EngineName,
				NormalizedData: assets,
				Total:          len(assets),
			})
		}
	}

	return append(engineResults, fallbackResults...)
}

// resolveCacheTTL 通过策略管理器动态计算缓存TTL
func (s *UnifiedService) resolveCacheTTL(req QueryRequest) time.Duration {
	engine := "default"
	if len(req.Engines) > 0 {
		engine = req.Engines[0]
	}
	// 优先使用动态策略（根据查询频率、引擎性能等自动调整）
	if s.strategyManager != nil {
		return s.strategyManager.GetCacheDuration("dynamic", engine, req.Query, req.PageSize, req.PageSize)
	}
	// 降级到原有逻辑：使用引擎级TTL或全局默认TTL
	if engineTTL, ok := s.orchestrator.GetEngineCacheTTL(engine); ok {
		return engineTTL
	}
	return s.cacheTTL
}

// processAssets 处理资产数据
func (s *UnifiedService) processAssets(ctx context.Context, assets []model.UnifiedAsset) ([]model.UnifiedAsset, error) {
	// 触发处理前钩子
	if err := s.pluginManager.GetHooks().TriggerHook(plugin.HookBeforeProcess, "process", nil); err != nil {
		return assets, fmt.Errorf("pre-process hook failed: %w", err)
	}

	// 获取所有处理器插件
	processors := s.pluginManager.GetRegistry().GetProcessorPlugins()
	if len(processors) == 0 {
		return assets, nil
	}

	// 创建处理管道
	pipeline := plugin.NewProcessorPipeline(processors)

	// 执行处理
	result, err := pipeline.Process(ctx, assets)
	if err != nil {
		return assets, fmt.Errorf("processor pipeline failed: %w", err)
	}

	// 触发处理后钩子
	s.pluginManager.GetHooks().TriggerHook(plugin.HookAfterProcess, "process", &model.HookData{ //nolint:errcheck
		Extra: map[string]any{
			"original_count":  len(assets),
			"processed_count": len(result),
		},
	})

	return result, nil
}

// ExportRequest 导出请求
type ExportRequest struct {
	Assets     []model.UnifiedAsset // 要导出的资产
	Format     string               // 导出格式
	OutputPath string               // 输出路径
}

// Export 导出数据
func (s *UnifiedService) Export(ctx context.Context, req ExportRequest) error {
	// 验证请求
	if len(req.Assets) == 0 {
		return fmt.Errorf("no assets to export")
	}
	if req.Format == "" {
		return fmt.Errorf("export format cannot be empty")
	}
	if req.OutputPath == "" {
		return fmt.Errorf("output path cannot be empty")
	}

	// 查找支持该格式的导出器
	exporters := s.pluginManager.GetRegistry().GetExporterPlugins()
	if len(exporters) == 0 {
		return fmt.Errorf("no exporters registered")
	}

	supportedFormats := []string{}
	for _, exporter := range exporters {
		formats := exporter.SupportedFormats()
		supportedFormats = append(supportedFormats, formats...)
		for _, format := range formats {
			if format == req.Format {
				err := exporter.Export(req.Assets, req.OutputPath)
				if err != nil {
					return fmt.Errorf("exporter %s failed: %w", exporter.Name(), err)
				}
				return nil
			}
		}
	}

	return fmt.Errorf("no exporter found for format: %s, supported formats: %s", req.Format, strings.Join(supportedFormats, ", "))
}

// RegisterEngine 注册引擎插件
func (s *UnifiedService) RegisterEngine(engine plugin.EnginePlugin, config *model.PluginConfig) error {
	// 加载插件
	if err := s.pluginManager.LoadPlugin(engine, config); err != nil {
		return err
	}

	// 启动插件
	if err := s.pluginManager.StartPlugin(engine.Name()); err != nil {
		return err
	}

	// 注册到编排器
	// 创建适配器包装器
	wrapper := &enginePluginAdapter{engine: engine}
	s.orchestrator.RegisterAdapter(wrapper)

	return nil
}

// RegisterProcessor 注册处理器插件
func (s *UnifiedService) RegisterProcessor(processor plugin.ProcessorPlugin, config *model.PluginConfig) error {
	// 加载插件
	if err := s.pluginManager.LoadPlugin(processor, config); err != nil {
		return err
	}

	// 启动插件
	return s.pluginManager.StartPlugin(processor.Name())
}

// RegisterExporter 注册导出器插件
func (s *UnifiedService) RegisterExporter(exporter plugin.ExporterPlugin, config *model.PluginConfig) error {
	// 加载插件
	if err := s.pluginManager.LoadPlugin(exporter, config); err != nil {
		return err
	}

	// 启动插件
	return s.pluginManager.StartPlugin(exporter.Name())
}

// EngineInfo represents a plugin engine's metadata.
type EngineInfo struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Fields      []string `json:"fields"`
	MaxPageSize int      `json:"max_page_size"`
}

// ProcessorInfo represents a plugin processor's metadata.
type ProcessorInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
}

// ListEngines 列出所有引擎
func (s *UnifiedService) ListEngines() []EngineInfo {
	engines := s.pluginManager.GetRegistry().GetEnginePlugins()
	result := make([]EngineInfo, 0, len(engines))

	for _, engine := range engines {
		result = append(result, EngineInfo{
			Name:        engine.Name(),
			Version:     engine.Version(),
			Description: engine.Description(),
			Author:      engine.Author(),
			Fields:      engine.SupportedFields(),
			MaxPageSize: engine.MaxPageSize(),
		})
	}

	return result
}

// ListProcessors 列出所有处理器
func (s *UnifiedService) ListProcessors() []ProcessorInfo {
	processors := s.pluginManager.GetRegistry().GetProcessorPlugins()
	result := make([]ProcessorInfo, 0, len(processors))

	for _, processor := range processors {
		result = append(result, ProcessorInfo{
			Name:        processor.Name(),
			Version:     processor.Version(),
			Description: processor.Description(),
			Priority:    processor.Priority(),
		})
	}

	return result
}

// checkResourceLimits 检查资源限制
func (s *UnifiedService) checkResourceLimits(ctx context.Context) error {
	// 更新内存统计指标
	metrics.UpdateMemoryStats()

	// 检查内存使用
	if s.maxMemoryMB > 0 {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		memUsageMB := mem.Alloc / (1024 * 1024)

		if memUsageMB >= uint64(s.maxMemoryMB) {
			logger.CtxWarnf(ctx, "memory usage exceeds limit: %dMB >= %dMB", memUsageMB, s.maxMemoryMB)
			return fmt.Errorf("memory usage exceeds limit: %dMB >= %dMB", memUsageMB, s.maxMemoryMB)
		}
	}
	return nil
}

// acquireQueryLock 获取查询并发锁
func (s *UnifiedService) acquireQueryLock() bool {
	s.queryMutex.Lock()

	if s.activeQueries >= s.maxConcurrent {
		s.queryMutex.Unlock()
		return false
	}

	s.activeQueries++
	s.queryMutex.Unlock()
	return true
}

// releaseQueryLock 释放查询并发锁
func (s *UnifiedService) releaseQueryLock() {
	s.queryMutex.Lock()
	if s.activeQueries > 0 {
		s.activeQueries--
	}
	s.queryMutex.Unlock()
}

// runWithQueryLock 在查询并发锁保护下执行函数，panic 时确保计数器回退
// nolint:unused
func (s *UnifiedService) runWithQueryLock(fn func() error) error {
	if !s.acquireQueryLock() {
		return fmt.Errorf("query concurrency limit reached")
	}
	defer s.releaseQueryLock()
	return fn()
}

// HealthCheck 健康检查
func (s *UnifiedService) HealthCheck() map[string]plugin.HealthStatus {
	return s.pluginManager.HealthCheck()
}

// Shutdown 关闭服务
func (s *UnifiedService) Shutdown() error {
	return s.pluginManager.Shutdown()
}

// GetPluginManager 获取插件管理器
func (s *UnifiedService) GetPluginManager() *plugin.PluginManager {
	return s.pluginManager
}

// enginePluginAdapter 引擎插件适配器，将插件接口转换为 adapter.EngineAdapter
type enginePluginAdapter struct {
	engine plugin.EnginePlugin
}

func (a *enginePluginAdapter) Name() string {
	return a.engine.Name()
}

func (a *enginePluginAdapter) Translate(ast *model.UQLAST) (string, error) {
	return a.engine.Translate(ast)
}

func (a *enginePluginAdapter) Search(ctx context.Context, query string, page, pageSize int) (*model.EngineResult, error) {
	return a.engine.Search(ctx, query, page, pageSize)
}

func (a *enginePluginAdapter) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
	return a.engine.Normalize(raw)
}

func (a *enginePluginAdapter) GetQuota() (*model.QuotaInfo, error) {
	// 检查引擎插件是否实现了GetQuota方法
	if quotaPlugin, ok := a.engine.(interface {
		GetQuota() (*model.QuotaInfo, error)
	}); ok {
		return quotaPlugin.GetQuota()
	}
	// 如果引擎插件没有实现GetQuota方法，返回默认值
	return &model.QuotaInfo{
		Remaining: 0,
		Total:     0,
		Used:      0,
		Unit:      "queries",
		Expiry:    "",
	}, nil
}

func (a *enginePluginAdapter) IsWebOnly() bool {
	// 检查引擎插件是否实现了IsWebOnly方法
	if webOnlyPlugin, ok := a.engine.(interface {
		IsWebOnly() bool
	}); ok {
		return webOnlyPlugin.IsWebOnly()
	}
	// 如果引擎插件没有实现IsWebOnly方法，返回默认值
	return false
}

// A combined snapshot must not override any participating engine's policy.
func (s *UnifiedService) queryCacheEnabled(req QueryRequest) bool {
	for _, engine := range req.Engines {
		if !s.orchestrator.IsCacheEnabledForEngine(engine) {
			return false
		}
	}
	return true
}
