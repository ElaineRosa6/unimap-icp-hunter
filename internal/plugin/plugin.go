package plugin

import (
	"context"
	"fmt"
	"sync"

	"github.com/unimap-icp-hunter/project/internal/model"
)

// Plugin 插件接口 - 定义了所有插件必须实现的方法
type Plugin interface {
	// 元数据方法
	Name() string                    // 插件名称
	Version() string                 // 插件版本
	Description() string             // 插件描述
	Author() string                  // 插件作者
	Type() PluginType               // 插件类型

	// 生命周期方法
	Initialize(config map[string]interface{}) error  // 初始化
	Start(ctx context.Context) error                 // 启动
	Stop() error                                      // 停止
	Health() HealthStatus                            // 健康检查
}

// EnginePlugin 搜索引擎插件接口
type EnginePlugin interface {
	Plugin
	
	// 查询能力
	Translate(ast *model.UQLAST) (string, error)
	Search(query string, page, pageSize int) (*model.EngineResult, error)
	Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error)
	
	// 引擎特性
	SupportedFields() []string       // 支持的查询字段
	MaxPageSize() int                // 最大分页大小
	RateLimit() RateLimitConfig      // 速率限制配置
}

// ProcessorPlugin 数据处理插件接口
type ProcessorPlugin interface {
	Plugin
	
	// 数据处理
	Process(ctx context.Context, assets []model.UnifiedAsset) ([]model.UnifiedAsset, error)
	Priority() int  // 处理优先级，数字越小优先级越高
}

// ExporterPlugin 导出器插件接口
type ExporterPlugin interface {
	Plugin
	
	// 导出功能
	Export(assets []model.UnifiedAsset, outputPath string) error
	SupportedFormats() []string  // 支持的导出格式
}

// NotifierPlugin 通知插件接口
type NotifierPlugin interface {
	Plugin
	
	// 通知功能
	Notify(ctx context.Context, message NotificationMessage) error
	SupportedChannels() []string  // 支持的通知渠道
}

// PluginType 插件类型
type PluginType string

const (
	PluginTypeEngine    PluginType = "engine"     // 搜索引擎插件
	PluginTypeProcessor PluginType = "processor"  // 数据处理插件
	PluginTypeExporter  PluginType = "exporter"   // 导出器插件
	PluginTypeNotifier  PluginType = "notifier"   // 通知插件
)

// HealthStatus 健康状态
type HealthStatus struct {
	Healthy bool
	Message string
	Details map[string]interface{}
}

// RateLimitConfig 速率限制配置
type RateLimitConfig struct {
	RequestsPerSecond int
	RequestsPerMinute int
	RequestsPerHour   int
	RequestsPerDay    int
}

// NotificationMessage 通知消息
type NotificationMessage struct {
	Title    string
	Content  string
	Level    string  // info, warning, error
	Metadata map[string]interface{}
}

// PluginRegistry 插件注册表
type PluginRegistry struct {
	plugins map[string]Plugin
	mu      sync.RWMutex
}

// NewPluginRegistry 创建插件注册表
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins: make(map[string]Plugin),
	}
}

// Register 注册插件
func (r *PluginRegistry) Register(plugin Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := plugin.Name()
	if name == "" {
		return fmt.Errorf("plugin name cannot be empty")
	}

	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}

	r.plugins[name] = plugin
	return nil
}

// Unregister 注销插件
func (r *PluginRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[name]; !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	delete(r.plugins, name)
	return nil
}

// Get 获取插件
func (r *PluginRegistry) Get(name string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, exists := r.plugins[name]
	return plugin, exists
}

// List 列出所有插件
func (r *PluginRegistry) List() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]Plugin, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		plugins = append(plugins, plugin)
	}
	return plugins
}

// ListByType 按类型列出插件
func (r *PluginRegistry) ListByType(pluginType PluginType) []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]Plugin, 0)
	for _, plugin := range r.plugins {
		if plugin.Type() == pluginType {
			plugins = append(plugins, plugin)
		}
	}
	return plugins
}

// GetEnginePlugins 获取所有引擎插件
func (r *PluginRegistry) GetEnginePlugins() []EnginePlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	engines := make([]EnginePlugin, 0)
	for _, plugin := range r.plugins {
		if engine, ok := plugin.(EnginePlugin); ok {
			engines = append(engines, engine)
		}
	}
	return engines
}

// GetProcessorPlugins 获取所有处理器插件
func (r *PluginRegistry) GetProcessorPlugins() []ProcessorPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	processors := make([]ProcessorPlugin, 0)
	for _, plugin := range r.plugins {
		if processor, ok := plugin.(ProcessorPlugin); ok {
			processors = append(processors, processor)
		}
	}
	return processors
}

// GetExporterPlugins 获取所有导出器插件
func (r *PluginRegistry) GetExporterPlugins() []ExporterPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	exporters := make([]ExporterPlugin, 0)
	for _, plugin := range r.plugins {
		if exporter, ok := plugin.(ExporterPlugin); ok {
			exporters = append(exporters, exporter)
		}
	}
	return exporters
}

// GetNotifierPlugins 获取所有通知插件
func (r *PluginRegistry) GetNotifierPlugins() []NotifierPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	notifiers := make([]NotifierPlugin, 0)
	for _, plugin := range r.plugins {
		if notifier, ok := plugin.(NotifierPlugin); ok {
			notifiers = append(notifiers, notifier)
		}
	}
	return notifiers
}
