package plugin

import (
	"fmt"
	"sync"
)

// HookType 钩子类型
type HookType string

const (
	// 插件生命周期钩子
	HookBeforeLoad   HookType = "before_load"    // 插件加载前
	HookAfterLoad    HookType = "after_load"     // 插件加载后
	HookBeforeStart  HookType = "before_start"   // 插件启动前
	HookAfterStart   HookType = "after_start"    // 插件启动后
	HookBeforeStop   HookType = "before_stop"    // 插件停止前
	HookAfterStop    HookType = "after_stop"     // 插件停止后
	HookBeforeUnload HookType = "before_unload"  // 插件卸载前
	HookAfterUnload  HookType = "after_unload"   // 插件卸载后

	// 查询生命周期钩子
	HookBeforeQuery  HookType = "before_query"   // 查询前
	HookAfterQuery   HookType = "after_query"    // 查询后
	HookQueryError   HookType = "query_error"    // 查询错误

	// 数据处理钩子
	HookBeforeProcess HookType = "before_process" // 数据处理前
	HookAfterProcess  HookType = "after_process"  // 数据处理后

	// 其他钩子
	HookHealthCheckFailed HookType = "health_check_failed" // 健康检查失败
)

// HookFunc 钩子函数类型
type HookFunc func(pluginName string, data map[string]interface{}) error

// HookRegistry 钩子注册表
type HookRegistry struct {
	hooks map[HookType][]HookFunc
	mu    sync.RWMutex
}

// NewHookRegistry 创建钩子注册表
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		hooks: make(map[HookType][]HookFunc),
	}
}

// RegisterHook 注册钩子
func (r *HookRegistry) RegisterHook(hookType HookType, fn HookFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.hooks[hookType] = append(r.hooks[hookType], fn)
}

// UnregisterHook 注销特定类型的所有钩子
func (r *HookRegistry) UnregisterHook(hookType HookType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.hooks, hookType)
}

// TriggerHook 触发钩子
func (r *HookRegistry) TriggerHook(hookType HookType, pluginName string, data map[string]interface{}) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hooks, exists := r.hooks[hookType]
	if !exists || len(hooks) == 0 {
		return nil
	}

	for _, hook := range hooks {
		if err := hook(pluginName, data); err != nil {
			return fmt.Errorf("hook %s failed for plugin %s: %w", hookType, pluginName, err)
		}
	}

	return nil
}

// ListHooks 列出所有钩子类型
func (r *HookRegistry) ListHooks() []HookType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]HookType, 0, len(r.hooks))
	for hookType := range r.hooks {
		types = append(types, hookType)
	}
	return types
}

// CountHooks 统计指定类型的钩子数量
func (r *HookRegistry) CountHooks(hookType HookType) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.hooks[hookType])
}
