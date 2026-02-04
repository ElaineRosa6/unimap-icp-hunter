package processors

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/plugin"
)

// ValidationProcessor 数据验证处理器
type ValidationProcessor struct {
	strictMode      bool
	validateIP      bool
	validatePort    bool
	validateURL     bool
	allowPrivateIP  bool
}

// NewValidationProcessor 创建数据验证处理器
func NewValidationProcessor(strictMode bool) *ValidationProcessor {
	return &ValidationProcessor{
		strictMode:     strictMode,
		validateIP:     true,
		validatePort:   true,
		validateURL:    true,
		allowPrivateIP: true,
	}
}

// Name 返回插件名称
func (p *ValidationProcessor) Name() string {
	return "validation_processor"
}

// Version 返回插件版本
func (p *ValidationProcessor) Version() string {
	return "1.0.0"
}

// Description 返回插件描述
func (p *ValidationProcessor) Description() string {
	return "数据验证处理器，验证资产数据的有效性"
}

// Author 返回插件作者
func (p *ValidationProcessor) Author() string {
	return "UniMap Team"
}

// Type 返回插件类型
func (p *ValidationProcessor) Type() plugin.PluginType {
	return plugin.PluginTypeProcessor
}

// Initialize 初始化插件
func (p *ValidationProcessor) Initialize(config map[string]interface{}) error {
	if val, ok := config["strictMode"].(bool); ok {
		p.strictMode = val
	}
	if val, ok := config["validateIP"].(bool); ok {
		p.validateIP = val
	}
	if val, ok := config["validatePort"].(bool); ok {
		p.validatePort = val
	}
	if val, ok := config["validateURL"].(bool); ok {
		p.validateURL = val
	}
	if val, ok := config["allowPrivateIP"].(bool); ok {
		p.allowPrivateIP = val
	}
	return nil
}

// Start 启动插件
func (p *ValidationProcessor) Start(ctx context.Context) error {
	return nil
}

// Stop 停止插件
func (p *ValidationProcessor) Stop() error {
	return nil
}

// Health 健康检查
func (p *ValidationProcessor) Health() plugin.HealthStatus {
	return plugin.HealthStatus{
		Healthy: true,
		Message: "Running",
	}
}

// Process 执行数据验证
func (p *ValidationProcessor) Process(ctx context.Context, assets []model.UnifiedAsset) ([]model.UnifiedAsset, error) {
	result := make([]model.UnifiedAsset, 0)

	for _, asset := range assets {
		if p.validateAsset(asset) {
			result = append(result, asset)
		}
	}

	return result, nil
}

// Priority 返回处理优先级
func (p *ValidationProcessor) Priority() int {
	return 50 // 验证应该在清洗后、去重前执行
}

// validateAsset 验证单个资产
func (p *ValidationProcessor) validateAsset(asset model.UnifiedAsset) bool {
	// 验证 IP
	if p.validateIP && asset.IP != "" {
		if !p.isValidIP(asset.IP) {
			if p.strictMode {
				return false
			}
		}
	}

	// 验证端口
	if p.validatePort && asset.Port > 0 {
		if !p.isValidPort(asset.Port) {
			if p.strictMode {
				return false
			}
		}
	}

	// 验证 URL
	if p.validateURL && asset.URL != "" {
		if !p.isValidURL(asset.URL) {
			if p.strictMode {
				return false
			}
		}
	}

	return true
}

// isValidIP 验证 IP 地址
func (p *ValidationProcessor) isValidIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// 如果不允许私有 IP，检查是否为私有地址
	if !p.allowPrivateIP {
		return !p.isPrivateIP(parsedIP)
	}

	return true
}

// isPrivateIP 检查是否为私有 IP
func (p *ValidationProcessor) isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}

	for _, cidr := range privateRanges {
		_, subnet, _ := net.ParseCIDR(cidr)
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}

// isValidPort 验证端口号
func (p *ValidationProcessor) isValidPort(port int) bool {
	return port > 0 && port <= 65535
}

// isValidURL 验证 URL
func (p *ValidationProcessor) isValidURL(url string) bool {
	// 简单的 URL 验证
	pattern := `^https?://[^\s/$.?#].[^\s]*$`
	matched, err := regexp.MatchString(pattern, url)
	if err != nil {
		return false
	}
	return matched
}

// EnrichmentProcessor 数据富化处理器 - 添加额外的元数据和信息
type EnrichmentProcessor struct {
	addTimestamp     bool
	addFingerprint   bool
	normalizeCountry bool
}

// NewEnrichmentProcessor 创建数据富化处理器
func NewEnrichmentProcessor() *EnrichmentProcessor {
	return &EnrichmentProcessor{
		addTimestamp:     true,
		addFingerprint:   true,
		normalizeCountry: true,
	}
}

// Name 返回插件名称
func (p *EnrichmentProcessor) Name() string {
	return "enrichment_processor"
}

// Version 返回插件版本
func (p *EnrichmentProcessor) Version() string {
	return "1.0.0"
}

// Description 返回插件描述
func (p *EnrichmentProcessor) Description() string {
	return "数据富化处理器，添加额外的元数据信息"
}

// Author 返回插件作者
func (p *EnrichmentProcessor) Author() string {
	return "UniMap Team"
}

// Type 返回插件类型
func (p *EnrichmentProcessor) Type() plugin.PluginType {
	return plugin.PluginTypeProcessor
}

// Initialize 初始化插件
func (p *EnrichmentProcessor) Initialize(config map[string]interface{}) error {
	if val, ok := config["addTimestamp"].(bool); ok {
		p.addTimestamp = val
	}
	if val, ok := config["addFingerprint"].(bool); ok {
		p.addFingerprint = val
	}
	if val, ok := config["normalizeCountry"].(bool); ok {
		p.normalizeCountry = val
	}
	return nil
}

// Start 启动插件
func (p *EnrichmentProcessor) Start(ctx context.Context) error {
	return nil
}

// Stop 停止插件
func (p *EnrichmentProcessor) Stop() error {
	return nil
}

// Health 健康检查
func (p *EnrichmentProcessor) Health() plugin.HealthStatus {
	return plugin.HealthStatus{
		Healthy: true,
		Message: "Running",
	}
}

// Process 执行数据富化
func (p *EnrichmentProcessor) Process(ctx context.Context, assets []model.UnifiedAsset) ([]model.UnifiedAsset, error) {
	result := make([]model.UnifiedAsset, 0, len(assets))

	for _, asset := range assets {
		enriched := p.enrichAsset(asset)
		result = append(result, enriched)
	}

	return result, nil
}

// Priority 返回处理优先级
func (p *EnrichmentProcessor) Priority() int {
	return 80 // 富化应该在验证后、去重前执行
}

// enrichAsset 富化单个资产
func (p *EnrichmentProcessor) enrichAsset(asset model.UnifiedAsset) model.UnifiedAsset {
	if asset.Extra == nil {
		asset.Extra = make(map[string]interface{})
	}

	// 添加指纹
	if p.addFingerprint {
		asset.Extra["fingerprint"] = p.generateFingerprint(asset)
	}

	// 规范化国家代码
	if p.normalizeCountry && asset.CountryCode != "" {
		asset.CountryCode = p.normalizeCountryCode(asset.CountryCode)
	}

	// 添加服务类型推测
	asset.Extra["service_type"] = p.guessServiceType(asset)

	return asset
}

// generateFingerprint 生成资产指纹
func (p *EnrichmentProcessor) generateFingerprint(asset model.UnifiedAsset) string {
	return fmt.Sprintf("%s:%d:%s", asset.IP, asset.Port, asset.Protocol)
}

// normalizeCountryCode 规范化国家代码
func (p *EnrichmentProcessor) normalizeCountryCode(code string) string {
	// 将国家代码转换为大写
	return strings.ToUpper(code)
}

// guessServiceType 推测服务类型
func (p *EnrichmentProcessor) guessServiceType(asset model.UnifiedAsset) string {
	switch asset.Port {
	case 80, 8080, 8000:
		return "http"
	case 443, 8443:
		return "https"
	case 21:
		return "ftp"
	case 22:
		return "ssh"
	case 3306:
		return "mysql"
	case 5432:
		return "postgresql"
	case 6379:
		return "redis"
	case 27017:
		return "mongodb"
	default:
		if asset.Protocol != "" {
			return asset.Protocol
		}
		return "unknown"
	}
}
