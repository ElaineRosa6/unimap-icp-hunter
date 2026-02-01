package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/core/unimap"
	"github.com/unimap-icp-hunter/project/internal/model"
	"go.uber.org/zap"
)

// UniMapService UniMap核心服务
type UniMapService struct {
	orchestrator *adapter.EngineOrchestrator
	logger       *zap.Logger
}

// NewUniMapService 创建UniMap服务
func NewUniMapService(orchestrator *adapter.EngineOrchestrator, logger *zap.Logger) *UniMapService {
	return &UniMapService{
		orchestrator: orchestrator,
		logger:       logger,
	}
}

// QueryUnified 执行统一查询
func (s *UniMapService) QueryUnified(uql string, engines []string, pageSize, maxPages int) ([]model.UnifiedAsset, error) {
	// 1. 解析UQL
	parser := unimap.NewUQLParser()
	ast, err := parser.Parse(uql)
	if err != nil {
		return nil, fmt.Errorf("UQL解析失败: %v", err)
	}

	// 2. 执行统一查询
	assets, err := s.orchestrator.ExecuteUnifiedQuery(ast, engines, pageSize, maxPages)
	if err != nil {
		return nil, fmt.Errorf("查询执行失败: %v", err)
	}

	return assets, nil
}

// PrintAssetsTable 以表格形式输出资产
func (s *UniMapService) PrintAssetsTable(assets []model.UnifiedAsset) {
	if len(assets) == 0 {
		fmt.Println("未找到结果")
		return
	}

	// 计算各列最大宽度
	maxIP, maxPort, maxURL, maxTitle := 2, 4, 3, 5
	for _, a := range assets {
		if len(a.IP) > maxIP {
			maxIP = len(a.IP)
		}
		if len(fmt.Sprintf("%d", a.Port)) > maxPort {
			maxPort = len(fmt.Sprintf("%d", a.Port))
		}
		if len(a.URL) > maxURL {
			maxURL = len(a.URL)
		}
		if len(a.Title) > maxTitle {
			maxTitle = len(a.Title)
		}
	}

	// 打印表头
	fmt.Printf("%-*s %-*s %-*s %-*s %s\n",
		maxIP, "IP",
		maxPort, "PORT",
		maxURL, "URL",
		maxTitle, "TITLE",
		"SOURCE")

	// 打印分隔线
	fmt.Println(strings.Repeat("-", maxIP+maxPort+maxURL+maxTitle+15))

	// 打印数据
	for _, a := range assets {
		title := a.Title
		if len(title) > 30 {
			title = title[:27] + "..."
		}
		url := a.URL
		if len(url) > 40 {
			url = url[:37] + "..."
		}

		fmt.Printf("%-*s %-*d %-*s %-*s %s\n",
			maxIP, a.IP,
			maxPort, a.Port,
			maxURL, url,
			maxTitle, title,
			a.Source)
	}
}

// PrintAssetsJSON 以JSON形式输出资产
func (s *UniMapService) PrintAssetsJSON(assets []model.UnifiedAsset) {
	data, err := json.MarshalIndent(assets, "", "  ")
	if err != nil {
		fmt.Printf("JSON格式化失败: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

// GetEngineStatus 获取引擎状态
func (s *UniMapService) GetEngineStatus() map[string]interface{} {
	status := make(map[string]interface{})
	engines := s.orchestrator.ListAdapters()

	for _, name := range engines {
		adapter, exists := s.orchestrator.GetAdapter(name)
		if exists {
			status[name] = map[string]interface{}{
				"enabled":  true,
				"adapter":  adapter.Name(),
				"status":   "active",
			}
		}
	}

	return status
}

// SearchEngines 搜索多个引擎
func (s *UniMapService) SearchEngines(uql string, engines []string, pageSize int) ([]*model.EngineResult, error) {
	parser := unimap.NewUQLParser()
	ast, err := parser.Parse(uql)
	if err != nil {
		return nil, err
	}

	queries, err := s.orchestrator.TranslateQuery(ast, engines)
	if err != nil {
		return nil, err
	}

	// 设置分页参数
	for i := range queries {
		queries[i].PageSize = pageSize
	}

	return s.orchestrator.SearchEngines(queries, pageSize)
}

// GetUnifiedStats 获取统一统计信息
func (s *UniMapService) GetUnifiedStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// 引擎数量
	engines := s.orchestrator.ListAdapters()
	stats["engine_count"] = len(engines)
	stats["engines"] = engines

	return stats
}
