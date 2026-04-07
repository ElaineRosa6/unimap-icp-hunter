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

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/core/unimap"
	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/metrics"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/plugin"
	"github.com/unimap-icp-hunter/project/internal/utils"
)

// UnifiedService 统一服务层 - 为 CLI、GUI 和 Web 提供统一接口
type UnifiedService struct {
	pluginManager *plugin.PluginManager
	orchestrator  *adapter.EngineOrchestrator
	parser        *unimap.UQLParser
	merger        *unimap.ResultMerger
	cache         utils.QueryCache
	cacheTTL      time.Duration
	cacheMaxSize  int
	cacheCleanup  time.Duration
	cacheBackend  string
	mu            sync.RWMutex
	maxMemoryMB   int        // 最大内存使用限制（MB）
	maxConcurrent int        // 最大并发查询数
	activeQueries int        // 当前活跃查询数
	queryMutex    sync.Mutex // 查询并发控制锁
}

// NewUnifiedService 创建统一服务
func NewUnifiedService() *UnifiedService {
	return NewUnifiedServiceWithConfig(nil)
}

// NewUnifiedServiceWithConfig 使用配置创建统一服务。
func NewUnifiedServiceWithConfig(cfg *config.Config) *UnifiedService {
	cacheTTL := 30 * time.Minute
	cacheCleanupInterval := 5 * time.Minute
	memoryMaxSize := 1000
	cacheBackend := "memory"
	maxMemoryMB := 512  // 默认最大内存限制512MB
	maxConcurrent := 10 // 默认最大并发查询数

	var redisCfg utils.RedisConfig

	if cfg != nil {
		if cfg.System.CacheTTL > 0 {
			cacheTTL = time.Duration(cfg.System.CacheTTL) * time.Second
		}
		if cfg.System.CacheCleanupInterval > 0 {
			cacheCleanupInterval = time.Duration(cfg.System.CacheCleanupInterval) * time.Second
		}
		if cfg.System.CacheMaxSize > 0 {
			memoryMaxSize = cfg.System.CacheMaxSize
		}
		if cfg.System.MaxConcurrent > 0 {
			maxConcurrent = cfg.System.MaxConcurrent
		}
		cacheBackend = strings.ToLower(strings.TrimSpace(cfg.Cache.Backend))
		if cacheBackend == "" {
			cacheBackend = "memory"
		}

		// 构建Redis配置
		redisCfg = utils.RedisConfig{
			Addr:            strings.TrimSpace(cfg.Cache.Redis.Addr),
			Password:        cfg.Cache.Redis.Password,
			DB:              cfg.Cache.Redis.DB,
			Prefix:          strings.TrimSpace(cfg.Cache.Redis.Prefix),
			PoolSize:        cfg.Cache.Redis.PoolSize,
			MinIdleConns:    cfg.Cache.Redis.MinIdleConns,
			MaxIdleConns:    cfg.Cache.Redis.MaxIdleConns,
			MaxRetries:      cfg.Cache.Redis.MaxRetries,
			DialTimeout:     time.Duration(cfg.Cache.Redis.DialTimeout) * time.Millisecond,
			ReadTimeout:     time.Duration(cfg.Cache.Redis.ReadTimeout) * time.Millisecond,
			WriteTimeout:    time.Duration(cfg.Cache.Redis.WriteTimeout) * time.Millisecond,
			PoolTimeout:     time.Duration(cfg.Cache.Redis.PoolTimeout) * time.Millisecond,
			ConnMaxLifetime: time.Duration(cfg.Cache.Redis.ConnMaxLifetime) * time.Millisecond,
			ConnMaxIdleTime: time.Duration(cfg.Cache.Redis.ConnMaxIdleTime) * time.Millisecond,
		}
	}

	// 初始化缓存
	cache := utils.NewCacheWithConfig(cacheBackend, redisCfg, memoryMaxSize, cacheCleanupInterval)

	// 检测实际使用的缓存后端
	useRedis := strings.EqualFold(cacheBackend, "redis")
	orchestrator := adapter.NewEngineOrchestratorWithConfig(useRedis, redisCfg.Addr, redisCfg.Password, redisCfg.DB)
	if cfg != nil {
		orchestrator.SetConcurrency(cfg.System.MaxConcurrent)

		// 设置默认缓存TTL
		orchestrator.SetDefaultCacheTTL(cacheTTL)

		// 从配置加载引擎级别的缓存设置
		for engineName, engineCfg := range cfg.Cache.Engines {
			if engineCfg.TTL > 0 {
				orchestrator.SetEngineCacheTTL(engineName, time.Duration(engineCfg.TTL)*time.Second, engineCfg.Enabled)
			}
		}
	}

	// Redis连接失败时，缓存工厂会回退到内存缓存。
	if _, ok := cache.(*utils.RedisCache); !ok {
		cacheBackend = "memory"
	}

	return &UnifiedService{
		pluginManager: plugin.NewPluginManager(),
		orchestrator:  orchestrator,
		parser:        unimap.NewUQLParser(),
		merger:        unimap.NewResultMerger(),
		cache:         cache,
		cacheTTL:      cacheTTL,
		cacheMaxSize:  memoryMaxSize,
		cacheCleanup:  cacheCleanupInterval,
		cacheBackend:  cacheBackend,
		maxMemoryMB:   maxMemoryMB,
		maxConcurrent: maxConcurrent,
		activeQueries: 0,
	}
}

// RegisterAdapter 注册引擎适配器
func (s *UnifiedService) RegisterAdapter(adapter adapter.EngineAdapter) {
	s.orchestrator.RegisterAdapter(adapter)
}

// GetOrchestrator 获取引擎编排器
func (s *UnifiedService) GetOrchestrator() *adapter.EngineOrchestrator {
	return s.orchestrator
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

	// 验证请求
	if req.Query == "" {
		queryStatus = "error"
		return nil, fmt.Errorf("query cannot be empty")
	}
	if len(req.Engines) == 0 {
		queryStatus = "error"
		return nil, fmt.Errorf("at least one engine must be specified")
	}
	if req.PageSize <= 0 {
		req.PageSize = 100
	}

	// 检查资源限制
	if err := s.checkResourceLimits(ctx); err != nil {
		queryStatus = "error"
		return nil, err
	}

	// 获取查询并发锁
	if !s.acquireQueryLock() {
		queryStatus = "error"
		return nil, fmt.Errorf("too many concurrent queries, please try again later")
	}
	defer s.releaseQueryLock()

	// 尝试从缓存获取结果
	sortedEngines := make([]string, len(req.Engines))
	copy(sortedEngines, req.Engines)
	sort.Strings(sortedEngines)

	// 使用SHA256生成缓存键，避免特殊字符导致的键冲突
	keyData := fmt.Sprintf("%s|%s|%d|%t", strings.Join(sortedEngines, ","), req.Query, req.PageSize, req.ProcessData)
	hash := sha256.Sum256([]byte(keyData))
	cacheKey := hex.EncodeToString(hash[:])

	if cachedAssets, found := s.cache.Get(cacheKey); found {
		metrics.ObserveCacheLookup(s.cacheBackend, "hit")
		logger.CtxDebugf(ctx, "query cache hit: backend=%s", s.cacheBackend)
		// 触发查询前钩子
		if err := s.pluginManager.GetHooks().TriggerHook(plugin.HookBeforeQuery, "query", map[string]interface{}{
			"query":   req.Query,
			"engines": req.Engines,
			"cached":  true,
		}); err != nil {
			queryStatus = "error"
			return nil, fmt.Errorf("pre-query hook failed: %w", err)
		}

		// 构建缓存响应
		engineStats := make(map[string]int)
		for _, engine := range req.Engines {
			engineStats[engine] = 0
		}

		// 触发查询后钩子
		s.pluginManager.GetHooks().TriggerHook(plugin.HookAfterQuery, "query", map[string]interface{}{
			"result_count": len(cachedAssets),
			"engines":      req.Engines,
			"cached":       true,
		})

		return &QueryResponse{
			Assets:      cachedAssets,
			TotalCount:  len(cachedAssets),
			EngineStats: engineStats,
			Errors:      []string{},
		}, nil
	}
	metrics.ObserveCacheLookup(s.cacheBackend, "miss")
	logger.CtxDebugf(ctx, "query cache miss: backend=%s", s.cacheBackend)

	// 触发查询前钩子
	if err := s.pluginManager.GetHooks().TriggerHook(plugin.HookBeforeQuery, "query", map[string]interface{}{
		"query":   req.Query,
		"engines": req.Engines,
		"cached":  false,
	}); err != nil {
		queryStatus = "error"
		return nil, fmt.Errorf("pre-query hook failed: %w", err)
	}

	// 解析 UQL
	ast, err := s.parser.Parse(req.Query)
	if err != nil {
		queryStatus = "error"
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	// 转换为各引擎查询
	queries, err := s.orchestrator.TranslateQuery(ast, req.Engines)
	if err != nil {
		queryStatus = "error"
		return nil, fmt.Errorf("failed to translate query: %w", err)
	}

	// 并行搜索
	var errors []string
	engineResults, err := s.orchestrator.SearchEnginesWithContext(ctx, queries, req.PageSize)
	if err != nil {
		// 记录错误但继续处理
		errors = append(errors, err.Error())
		s.pluginManager.GetHooks().TriggerHook(plugin.HookQueryError, "query", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// 规范化和合并结果
	var allAssets []model.UnifiedAsset
	engineStats := make(map[string]int)

	for _, result := range engineResults {
		if result == nil {
			continue
		}

		// 处理引擎返回的错误
		if result.Error != "" {
			errors = append(errors, fmt.Sprintf("engine %s error: %s", result.EngineName, result.Error))
			metrics.IncEngineError()
			continue
		}

		// 如果是缓存命中的结果，直接使用已标准化的数据
		if result.Cached && result.NormalizedData != nil {
			allAssets = append(allAssets, result.NormalizedData...)
			engineStats[result.EngineName] = len(result.NormalizedData)
			continue
		}

		// 获取对应的适配器
		adapterInstance, exists := s.orchestrator.GetAdapter(result.EngineName)
		if !exists {
			errors = append(errors, fmt.Sprintf("adapter for engine %s not found", result.EngineName))
			metrics.IncEngineError()
			continue
		}

		// 规范化结果
		assets, err := adapterInstance.Normalize(result)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to normalize results from %s: %v", result.EngineName, err))
			metrics.IncEngineError()
			continue
		}

		allAssets = append(allAssets, assets...)
		engineStats[result.EngineName] = len(assets)
	}

	// 如果需要处理数据
	if req.ProcessData {
		allAssets, err = s.processAssets(ctx, allAssets)
		if err != nil {
			errors = append(errors, fmt.Sprintf("data processing failed: %v", err))
			metrics.IncEngineError()
		}
	}

	// 将结果存入缓存
	if len(allAssets) > 0 {
		s.cache.Set(cacheKey, allAssets, s.cacheTTL)
	}

	// 触发查询后钩子
	s.pluginManager.GetHooks().TriggerHook(plugin.HookAfterQuery, "query", map[string]interface{}{
		"result_count": len(allAssets),
		"engines":      req.Engines,
		"cached":       false,
	})

	return &QueryResponse{
		Assets:      allAssets,
		TotalCount:  len(allAssets),
		EngineStats: engineStats,
		Errors:      errors,
	}, nil
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
	s.pluginManager.GetHooks().TriggerHook(plugin.HookAfterProcess, "process", map[string]interface{}{
		"original_count":  len(assets),
		"processed_count": len(result),
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
func (s *UnifiedService) RegisterEngine(engine plugin.EnginePlugin, config map[string]interface{}) error {
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
func (s *UnifiedService) RegisterProcessor(processor plugin.ProcessorPlugin, config map[string]interface{}) error {
	// 加载插件
	if err := s.pluginManager.LoadPlugin(processor, config); err != nil {
		return err
	}

	// 启动插件
	return s.pluginManager.StartPlugin(processor.Name())
}

// RegisterExporter 注册导出器插件
func (s *UnifiedService) RegisterExporter(exporter plugin.ExporterPlugin, config map[string]interface{}) error {
	// 加载插件
	if err := s.pluginManager.LoadPlugin(exporter, config); err != nil {
		return err
	}

	// 启动插件
	return s.pluginManager.StartPlugin(exporter.Name())
}

// ListEngines 列出所有引擎
func (s *UnifiedService) ListEngines() []map[string]interface{} {
	engines := s.pluginManager.GetRegistry().GetEnginePlugins()
	result := make([]map[string]interface{}, 0, len(engines))

	for _, engine := range engines {
		result = append(result, map[string]interface{}{
			"name":          engine.Name(),
			"version":       engine.Version(),
			"description":   engine.Description(),
			"author":        engine.Author(),
			"fields":        engine.SupportedFields(),
			"max_page_size": engine.MaxPageSize(),
		})
	}

	return result
}

// ListProcessors 列出所有处理器
func (s *UnifiedService) ListProcessors() []map[string]interface{} {
	processors := s.pluginManager.GetRegistry().GetProcessorPlugins()
	result := make([]map[string]interface{}, 0, len(processors))

	for _, processor := range processors {
		result = append(result, map[string]interface{}{
			"name":        processor.Name(),
			"version":     processor.Version(),
			"description": processor.Description(),
			"priority":    processor.Priority(),
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
	defer s.queryMutex.Unlock()
	
	if s.activeQueries >= s.maxConcurrent {
		return false
	}
	
	s.activeQueries++
	return true
}

// releaseQueryLock 释放查询并发锁
func (s *UnifiedService) releaseQueryLock() {
	s.queryMutex.Lock()
	defer s.queryMutex.Unlock()
	
	if s.activeQueries > 0 {
		s.activeQueries--
	}
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

func (a *enginePluginAdapter) Search(query string, page, pageSize int) (*model.EngineResult, error) {
	return a.engine.Search(query, page, pageSize)
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
