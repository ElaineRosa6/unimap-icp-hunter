package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/core/unimap"
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
	mu            sync.RWMutex
}

// NewUnifiedService 创建统一服务
func NewUnifiedService() *UnifiedService {
	// 初始化缓存
	cache := utils.NewCache(
		false,         // 暂时不使用Redis
		"", "", 0, "", // Redis配置
		1000,          // 内存缓存最大大小
		5*time.Minute, // 清理间隔
	)

	return &UnifiedService{
		pluginManager: plugin.NewPluginManager(),
		orchestrator:  adapter.NewEngineOrchestrator(),
		parser:        unimap.NewUQLParser(),
		merger:        unimap.NewResultMerger(),
		cache:         cache,
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
	// 验证请求
	if req.Query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if len(req.Engines) == 0 {
		return nil, fmt.Errorf("at least one engine must be specified")
	}
	if req.PageSize <= 0 {
		req.PageSize = 100
	}

	// 尝试从缓存获取结果
	sortedEngines := make([]string, len(req.Engines))
	copy(sortedEngines, req.Engines)
	sort.Strings(sortedEngines)

	cacheKey := fmt.Sprintf("%s:%s:%d:%t", strings.Join(sortedEngines, ","), req.Query, req.PageSize, req.ProcessData)
	if cachedAssets, found := s.cache.Get(cacheKey); found {
		// 触发查询前钩子
		if err := s.pluginManager.GetHooks().TriggerHook(plugin.HookBeforeQuery, "query", map[string]interface{}{
			"query":   req.Query,
			"engines": req.Engines,
			"cached":  true,
		}); err != nil {
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

	// 触发查询前钩子
	if err := s.pluginManager.GetHooks().TriggerHook(plugin.HookBeforeQuery, "query", map[string]interface{}{
		"query":   req.Query,
		"engines": req.Engines,
		"cached":  false,
	}); err != nil {
		return nil, fmt.Errorf("pre-query hook failed: %w", err)
	}

	// 解析 UQL
	ast, err := s.parser.Parse(req.Query)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %w", err)
	}

	// 转换为各引擎查询
	queries, err := s.orchestrator.TranslateQuery(ast, req.Engines)
	if err != nil {
		return nil, fmt.Errorf("failed to translate query: %w", err)
	}

	// 并行搜索
	engineResults, err := s.orchestrator.SearchEngines(queries, req.PageSize)
	if err != nil {
		// 记录错误但继续处理
		s.pluginManager.GetHooks().TriggerHook(plugin.HookQueryError, "query", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// 规范化和合并结果
	var allAssets []model.UnifiedAsset
	engineStats := make(map[string]int)
	var errors []string

	for _, result := range engineResults {
		if result == nil {
			continue
		}

		// 处理引擎返回的错误
		if result.Error != "" {
			errors = append(errors, fmt.Sprintf("engine %s error: %s", result.EngineName, result.Error))
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
			continue
		}

		// 规范化结果
		assets, err := adapterInstance.Normalize(result)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to normalize results from %s: %v", result.EngineName, err))
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
		}
	}

	// 将结果存入缓存
	if len(allAssets) > 0 {
		s.cache.Set(cacheKey, allAssets, 30*time.Minute)
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
