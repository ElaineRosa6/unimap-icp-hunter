package plugin

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/model"
)

// PluginManager 插件管理器 - 管理插件的加载、启动、停止和健康检查
type PluginManager struct {
	registry     *PluginRegistry
	ctx          context.Context
	cancel       context.CancelFunc
	healthTicker *time.Ticker
	mu           sync.RWMutex
	hooks        *HookRegistry
}

// NewPluginManager 创建插件管理器
func NewPluginManager() *PluginManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &PluginManager{
		registry:     NewPluginRegistry(),
		ctx:          ctx,
		cancel:       cancel,
		healthTicker: time.NewTicker(30 * time.Second),
		hooks:        NewHookRegistry(),
	}
}

// LoadPlugin 加载插件
func (m *PluginManager) LoadPlugin(plugin Plugin, config map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 触发预加载钩子
	if err := m.hooks.TriggerHook(HookBeforeLoad, plugin.Name(), nil); err != nil {
		return fmt.Errorf("pre-load hook failed: %w", err)
	}

	// 初始化插件
	if err := plugin.Initialize(config); err != nil {
		return fmt.Errorf("failed to initialize plugin %s: %w", plugin.Name(), err)
	}

	// 注册插件
	if err := m.registry.Register(plugin); err != nil {
		return fmt.Errorf("failed to register plugin %s: %w", plugin.Name(), err)
	}

	// 触发后加载钩子
	if err := m.hooks.TriggerHook(HookAfterLoad, plugin.Name(), nil); err != nil {
		// 加载后钩子失败不影响插件注册
		fmt.Printf("Warning: post-load hook failed for %s: %v\n", plugin.Name(), err)
	}

	return nil
}

// StartPlugin 启动插件
func (m *PluginManager) StartPlugin(name string) error {
	plugin, exists := m.registry.Get(name)
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	// 触发预启动钩子
	if err := m.hooks.TriggerHook(HookBeforeStart, name, nil); err != nil {
		return fmt.Errorf("pre-start hook failed: %w", err)
	}

	// 启动插件
	if err := plugin.Start(m.ctx); err != nil {
		return fmt.Errorf("failed to start plugin %s: %w", name, err)
	}

	// 触发后启动钩子
	if err := m.hooks.TriggerHook(HookAfterStart, name, nil); err != nil {
		fmt.Printf("Warning: post-start hook failed for %s: %v\n", name, err)
	}

	return nil
}

// StopPlugin 停止插件
func (m *PluginManager) StopPlugin(name string) error {
	plugin, exists := m.registry.Get(name)
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	// 触发预停止钩子
	if err := m.hooks.TriggerHook(HookBeforeStop, name, nil); err != nil {
		return fmt.Errorf("pre-stop hook failed: %w", err)
	}

	// 停止插件
	if err := plugin.Stop(); err != nil {
		return fmt.Errorf("failed to stop plugin %s: %w", name, err)
	}

	// 触发后停止钩子
	if err := m.hooks.TriggerHook(HookAfterStop, name, nil); err != nil {
		fmt.Printf("Warning: post-stop hook failed for %s: %v\n", name, err)
	}

	return nil
}

// UnloadPlugin 卸载插件
func (m *PluginManager) UnloadPlugin(name string) error {
	// 先停止插件
	if err := m.StopPlugin(name); err != nil {
		return err
	}

	// 触发预卸载钩子
	if err := m.hooks.TriggerHook(HookBeforeUnload, name, nil); err != nil {
		return fmt.Errorf("pre-unload hook failed: %w", err)
	}

	// 注销插件
	if err := m.registry.Unregister(name); err != nil {
		return fmt.Errorf("failed to unregister plugin %s: %w", name, err)
	}

	// 触发后卸载钩子
	if err := m.hooks.TriggerHook(HookAfterUnload, name, nil); err != nil {
		fmt.Printf("Warning: post-unload hook failed for %s: %v\n", name, err)
	}

	return nil
}

// StartAll 启动所有插件
func (m *PluginManager) StartAll() error {
	plugins := m.registry.List()
	for _, plugin := range plugins {
		if err := m.StartPlugin(plugin.Name()); err != nil {
			return err
		}
	}
	return nil
}

// StopAll 停止所有插件
func (m *PluginManager) StopAll() error {
	plugins := m.registry.List()
	for _, plugin := range plugins {
		if err := m.StopPlugin(plugin.Name()); err != nil {
			// 继续停止其他插件，但记录错误
			fmt.Printf("Error stopping plugin %s: %v\n", plugin.Name(), err)
		}
	}
	return nil
}

// HealthCheck 健康检查所有插件
func (m *PluginManager) HealthCheck() map[string]HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugins := m.registry.List()
	results := make(map[string]HealthStatus)

	for _, plugin := range plugins {
		results[plugin.Name()] = plugin.Health()
	}

	return results
}

// StartHealthMonitor 启动健康监控
func (m *PluginManager) StartHealthMonitor() {
	go func() {
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-m.healthTicker.C:
				results := m.HealthCheck()
				for name, status := range results {
					if !status.Healthy {
						fmt.Printf("Warning: Plugin %s is unhealthy: %s\n", name, status.Message)
						// 触发健康检查失败钩子
						m.hooks.TriggerHook(HookHealthCheckFailed, name, map[string]interface{}{
							"status": status,
						})
					}
				}
			}
		}
	}()
}

// Shutdown 关闭插件管理器
func (m *PluginManager) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 停止健康监控
	m.healthTicker.Stop()

	// 停止所有插件
	if err := m.StopAll(); err != nil {
		return err
	}

	// 取消上下文
	m.cancel()

	return nil
}

// GetRegistry 获取插件注册表
func (m *PluginManager) GetRegistry() *PluginRegistry {
	return m.registry
}

// GetHooks 获取钩子注册表
func (m *PluginManager) GetHooks() *HookRegistry {
	return m.hooks
}

// ProcessorPipeline 数据处理管道
type ProcessorPipeline struct {
	processors []ProcessorPlugin
}

// NewProcessorPipeline 创建处理管道
func NewProcessorPipeline(processors []ProcessorPlugin) *ProcessorPipeline {
	// 按优先级排序
	sorted := make([]ProcessorPlugin, len(processors))
	copy(sorted, processors)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority() < sorted[j].Priority()
	})

	return &ProcessorPipeline{
		processors: sorted,
	}
}

// Process 执行处理管道
func (p *ProcessorPipeline) Process(ctx context.Context, assets []model.UnifiedAsset) ([]model.UnifiedAsset, error) {
	result := assets
	var err error

	for _, processor := range p.processors {
		result, err = processor.Process(ctx, result)
		if err != nil {
			return nil, fmt.Errorf("processor %s failed: %w", processor.Name(), err)
		}
	}

	return result, nil
}

// AddProcessor 添加处理器
func (p *ProcessorPipeline) AddProcessor(processor ProcessorPlugin) {
	p.processors = append(p.processors, processor)
	// 重新排序
	sort.Slice(p.processors, func(i, j int) bool {
		return p.processors[i].Priority() < p.processors[j].Priority()
	})
}

// RemoveProcessor 移除处理器
func (p *ProcessorPipeline) RemoveProcessor(name string) {
	for i, processor := range p.processors {
		if processor.Name() == name {
			p.processors = append(p.processors[:i], p.processors[i+1:]...)
			break
		}
	}
}

// GetProcessors 获取所有处理器
func (p *ProcessorPipeline) GetProcessors() []ProcessorPlugin {
	return p.processors
}
