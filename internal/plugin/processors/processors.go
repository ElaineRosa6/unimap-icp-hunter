package processors

import (
	"context"
	"fmt"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/plugin"
)

// DeduplicationProcessor 去重处理器
type DeduplicationProcessor struct {
	strategy DeduplicationStrategy
}

// DeduplicationStrategy 去重策略
type DeduplicationStrategy string

const (
	StrategyIPPort   DeduplicationStrategy = "ip_port"    // 基于 IP:Port 去重
	StrategyURL      DeduplicationStrategy = "url"        // 基于 URL 去重
	StrategyHost     DeduplicationStrategy = "host"       // 基于 Host 去重
	StrategyAdvanced DeduplicationStrategy = "advanced"   // 高级去重（综合多字段）
)

// NewDeduplicationProcessor 创建去重处理器
func NewDeduplicationProcessor(strategy DeduplicationStrategy) *DeduplicationProcessor {
	return &DeduplicationProcessor{
		strategy: strategy,
	}
}

// Name 返回插件名称
func (p *DeduplicationProcessor) Name() string {
	return "deduplication_processor"
}

// Version 返回插件版本
func (p *DeduplicationProcessor) Version() string {
	return "1.0.0"
}

// Description 返回插件描述
func (p *DeduplicationProcessor) Description() string {
	return "数据去重处理器，支持多种去重策略"
}

// Author 返回插件作者
func (p *DeduplicationProcessor) Author() string {
	return "UniMap Team"
}

// Type 返回插件类型
func (p *DeduplicationProcessor) Type() plugin.PluginType {
	return plugin.PluginTypeProcessor
}

// Initialize 初始化插件
func (p *DeduplicationProcessor) Initialize(config map[string]interface{}) error {
	if strategy, ok := config["strategy"].(string); ok {
		p.strategy = DeduplicationStrategy(strategy)
	}
	return nil
}

// Start 启动插件
func (p *DeduplicationProcessor) Start(ctx context.Context) error {
	return nil
}

// Stop 停止插件
func (p *DeduplicationProcessor) Stop() error {
	return nil
}

// Health 健康检查
func (p *DeduplicationProcessor) Health() plugin.HealthStatus {
	return plugin.HealthStatus{
		Healthy: true,
		Message: "Running",
	}
}

// Process 执行数据去重
func (p *DeduplicationProcessor) Process(ctx context.Context, assets []model.UnifiedAsset) ([]model.UnifiedAsset, error) {
	if len(assets) == 0 {
		return assets, nil
	}

	switch p.strategy {
	case StrategyIPPort:
		return p.deduplicateByIPPort(assets), nil
	case StrategyURL:
		return p.deduplicateByURL(assets), nil
	case StrategyHost:
		return p.deduplicateByHost(assets), nil
	case StrategyAdvanced:
		return p.deduplicateAdvanced(assets), nil
	default:
		return p.deduplicateByIPPort(assets), nil
	}
}

// Priority 返回处理优先级
func (p *DeduplicationProcessor) Priority() int {
	return 100 // 去重应该在大多数处理后执行
}

// deduplicateByIPPort 基于 IP:Port 去重
func (p *DeduplicationProcessor) deduplicateByIPPort(assets []model.UnifiedAsset) []model.UnifiedAsset {
	seen := make(map[string]bool)
	result := make([]model.UnifiedAsset, 0)

	for _, asset := range assets {
		key := fmt.Sprintf("%s:%d", asset.IP, asset.Port)
		if !seen[key] {
			seen[key] = true
			result = append(result, asset)
		}
	}

	return result
}

// deduplicateByURL 基于 URL 去重
func (p *DeduplicationProcessor) deduplicateByURL(assets []model.UnifiedAsset) []model.UnifiedAsset {
	seen := make(map[string]bool)
	result := make([]model.UnifiedAsset, 0)

	for _, asset := range assets {
		if asset.URL != "" {
			if !seen[asset.URL] {
				seen[asset.URL] = true
				result = append(result, asset)
			}
		} else {
			// 没有 URL 的资产保留
			result = append(result, asset)
		}
	}

	return result
}

// deduplicateByHost 基于 Host 去重
func (p *DeduplicationProcessor) deduplicateByHost(assets []model.UnifiedAsset) []model.UnifiedAsset {
	seen := make(map[string]bool)
	result := make([]model.UnifiedAsset, 0)

	for _, asset := range assets {
		if asset.Host != "" {
			if !seen[asset.Host] {
				seen[asset.Host] = true
				result = append(result, asset)
			}
		} else {
			// 没有 Host 的资产使用 IP
			key := asset.IP
			if !seen[key] {
				seen[key] = true
				result = append(result, asset)
			}
		}
	}

	return result
}

// deduplicateAdvanced 高级去重（综合多字段）
func (p *DeduplicationProcessor) deduplicateAdvanced(assets []model.UnifiedAsset) []model.UnifiedAsset {
	seen := make(map[string]bool)
	result := make([]model.UnifiedAsset, 0)

	for _, asset := range assets {
		// 组合多个字段生成唯一键
		key := fmt.Sprintf("%s:%d:%s:%s", asset.IP, asset.Port, asset.Protocol, asset.Host)
		if !seen[key] {
			seen[key] = true
			result = append(result, asset)
		}
	}

	return result
}

// DataCleaningProcessor 数据清洗处理器
type DataCleaningProcessor struct {
	removeEmpty      bool
	normalizeURLs    bool
	trimWhitespace   bool
	lowercaseFields  []string
}

// NewDataCleaningProcessor 创建数据清洗处理器
func NewDataCleaningProcessor() *DataCleaningProcessor {
	return &DataCleaningProcessor{
		removeEmpty:     true,
		normalizeURLs:   true,
		trimWhitespace:  true,
		lowercaseFields: []string{"protocol", "host"},
	}
}

// Name 返回插件名称
func (p *DataCleaningProcessor) Name() string {
	return "data_cleaning_processor"
}

// Version 返回插件版本
func (p *DataCleaningProcessor) Version() string {
	return "1.0.0"
}

// Description 返回插件描述
func (p *DataCleaningProcessor) Description() string {
	return "数据清洗处理器，规范化和清理数据"
}

// Author 返回插件作者
func (p *DataCleaningProcessor) Author() string {
	return "UniMap Team"
}

// Type 返回插件类型
func (p *DataCleaningProcessor) Type() plugin.PluginType {
	return plugin.PluginTypeProcessor
}

// Initialize 初始化插件
func (p *DataCleaningProcessor) Initialize(config map[string]interface{}) error {
	if val, ok := config["removeEmpty"].(bool); ok {
		p.removeEmpty = val
	}
	if val, ok := config["normalizeURLs"].(bool); ok {
		p.normalizeURLs = val
	}
	if val, ok := config["trimWhitespace"].(bool); ok {
		p.trimWhitespace = val
	}
	return nil
}

// Start 启动插件
func (p *DataCleaningProcessor) Start(ctx context.Context) error {
	return nil
}

// Stop 停止插件
func (p *DataCleaningProcessor) Stop() error {
	return nil
}

// Health 健康检查
func (p *DataCleaningProcessor) Health() plugin.HealthStatus {
	return plugin.HealthStatus{
		Healthy: true,
		Message: "Running",
	}
}

// Process 执行数据清洗
func (p *DataCleaningProcessor) Process(ctx context.Context, assets []model.UnifiedAsset) ([]model.UnifiedAsset, error) {
	result := make([]model.UnifiedAsset, 0)

	for _, asset := range assets {
		cleaned := p.cleanAsset(asset)
		
		// 如果配置为移除空资产且资产为空，则跳过
		if p.removeEmpty && p.isEmptyAsset(cleaned) {
			continue
		}
		
		result = append(result, cleaned)
	}

	return result, nil
}

// Priority 返回处理优先级
func (p *DataCleaningProcessor) Priority() int {
	return 10 // 清洗应该优先执行
}

// cleanAsset 清洗单个资产
func (p *DataCleaningProcessor) cleanAsset(asset model.UnifiedAsset) model.UnifiedAsset {
	if p.trimWhitespace {
		asset.IP = strings.TrimSpace(asset.IP)
		asset.Protocol = strings.TrimSpace(asset.Protocol)
		asset.Host = strings.TrimSpace(asset.Host)
		asset.URL = strings.TrimSpace(asset.URL)
		asset.Title = strings.TrimSpace(asset.Title)
		asset.Server = strings.TrimSpace(asset.Server)
		asset.CountryCode = strings.TrimSpace(asset.CountryCode)
		asset.Region = strings.TrimSpace(asset.Region)
		asset.City = strings.TrimSpace(asset.City)
		asset.ASN = strings.TrimSpace(asset.ASN)
		asset.Org = strings.TrimSpace(asset.Org)
		asset.ISP = strings.TrimSpace(asset.ISP)
	}

	// 规范化小写字段
	for _, field := range p.lowercaseFields {
		switch field {
		case "protocol":
			asset.Protocol = strings.ToLower(asset.Protocol)
		case "host":
			asset.Host = strings.ToLower(asset.Host)
		}
	}

	// 规范化 URL
	if p.normalizeURLs && asset.URL != "" {
		asset.URL = p.normalizeURL(asset.URL)
	}

	return asset
}

// normalizeURL 规范化 URL
func (p *DataCleaningProcessor) normalizeURL(url string) string {
	url = strings.TrimSpace(url)
	
	// 移除尾部斜杠
	if strings.HasSuffix(url, "/") && len(url) > 1 {
		url = strings.TrimSuffix(url, "/")
	}
	
	return url
}

// isEmptyAsset 检查资产是否为空
func (p *DataCleaningProcessor) isEmptyAsset(asset model.UnifiedAsset) bool {
	return asset.IP == "" && asset.Host == "" && asset.URL == ""
}
