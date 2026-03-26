# UniMap 插件系统架构指南

## 概述

UniMap 插件系统是一个灵活、可扩展的架构，允许开发者轻松添加新的搜索引擎、数据处理器、导出器和通知器。本文档详细介绍了插件系统的设计理念、核心组件和使用方法。

## 设计理念

### 核心原则

1. **开放扩展，关闭修改** - 通过插件接口添加新功能，无需修改核心代码
2. **统一抽象** - 所有插件遵循统一的生命周期和接口规范
3. **松耦合** - 插件之间互不依赖，可独立开发和测试
4. **易于集成** - 简单的注册机制，支持热插拔
5. **多样性支持** - 支持多种类型的插件（引擎、处理器、导出器、通知器）

### 架构优势

相比其他同类项目的优势：

| 特性 | UniMap | Search_Viewer | ThunderSearch | fshzqSearch |
|------|--------|---------------|---------------|-------------|
| 插件化架构 | ✅ | ❌ | 部分 | ❌ |
| 统一服务层 | ✅ | ❌ | ❌ | ❌ |
| 钩子系统 | ✅ | ❌ | ❌ | ❌ |
| 数据处理管道 | ✅ | 基础 | 基础 | 基础 |
| 多接口支持 | ✅ | GUI | GUI | CLI |
| 作为库使用 | ✅ | ❌ | ❌ | ❌ |

## 核心组件

### 1. 插件接口 (Plugin Interface)

所有插件都必须实现基础插件接口：

```go
type Plugin interface {
    // 元数据
    Name() string
    Version() string
    Description() string
    Author() string
    Type() PluginType

    // 生命周期
    Initialize(config map[string]interface{}) error
    Start(ctx context.Context) error
    Stop() error
    Health() HealthStatus
}
```

### 2. 插件类型

#### 2.1 引擎插件 (EnginePlugin)

用于接入新的搜索引擎：

```go
type EnginePlugin interface {
    Plugin
    
    // 核心功能
    Translate(ast *model.UQLAST) (string, error)  // UQL 转引擎查询
    Search(query string, page, pageSize int) (*model.EngineResult, error)
    Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error)
    
    // 引擎特性
    SupportedFields() []string
    MaxPageSize() int
    RateLimit() RateLimitConfig
}
```

**示例：创建自定义引擎插件**

```go
type CustomEnginePlugin struct {
    apiKey  string
    baseURL string
}

func (p *CustomEnginePlugin) Name() string {
    return "custom_engine"
}

func (p *CustomEnginePlugin) Initialize(config map[string]interface{}) error {
    p.apiKey = config["api_key"].(string)
    p.baseURL = config["base_url"].(string)
    return nil
}

func (p *CustomEnginePlugin) Search(query string, page, pageSize int) (*model.EngineResult, error) {
    // 实现搜索逻辑
    return &model.EngineResult{
        Engine: p.Name(),
        Data:   /* 搜索结果 */,
    }, nil
}

// ... 实现其他方法
```

#### 2.2 处理器插件 (ProcessorPlugin)

用于数据处理（去重、清洗、验证等）：

```go
type ProcessorPlugin interface {
    Plugin
    Process(ctx context.Context, assets []model.UnifiedAsset) ([]model.UnifiedAsset, error)
    Priority() int  // 优先级，数字越小越先执行
}
```

**内置处理器：**

1. **DeduplicationProcessor** - 数据去重
   - 策略：ip_port, url, host, advanced
   - 优先级：100

2. **DataCleaningProcessor** - 数据清洗
   - 功能：去除空白、规范化 URL、小写转换
   - 优先级：10

3. **ValidationProcessor** - 数据验证
   - 验证：IP 地址、端口号、URL 格式
   - 优先级：50

4. **EnrichmentProcessor** - 数据富化
   - 添加：指纹、服务类型推测
   - 优先级：80

#### 2.3 导出器插件 (ExporterPlugin)

用于导出数据到不同格式：

```go
type ExporterPlugin interface {
    Plugin
    Export(assets []model.UnifiedAsset, outputPath string) error
    SupportedFormats() []string
}
```

#### 2.4 通知器插件 (NotifierPlugin)

用于发送通知：

```go
type NotifierPlugin interface {
    Plugin
    Notify(ctx context.Context, message NotificationMessage) error
    SupportedChannels() []string
}
```

### 3. 插件管理器 (PluginManager)

负责插件的生命周期管理：

```go
manager := plugin.NewPluginManager()

// 加载插件
config := map[string]interface{}{
    "api_key": "your_key",
}
manager.LoadPlugin(enginePlugin, config)

// 启动插件
manager.StartPlugin("custom_engine")

// 停止插件
manager.StopPlugin("custom_engine")

// 卸载插件
manager.UnloadPlugin("custom_engine")

// 健康检查
status := manager.HealthCheck()
```

### 4. 钩子系统 (Hooks System)

提供事件驱动的扩展能力：

```go
hooks := manager.GetHooks()

// 注册钩子
hooks.RegisterHook(plugin.HookBeforeQuery, func(pluginName string, data map[string]interface{}) error {
    fmt.Printf("查询前: %s\n", data["query"])
    return nil
})

// 支持的钩子类型
- HookBeforeLoad / HookAfterLoad
- HookBeforeStart / HookAfterStart
- HookBeforeStop / HookAfterStop
- HookBeforeQuery / HookAfterQuery
- HookBeforeProcess / HookAfterProcess
- HookHealthCheckFailed
```

### 5. 统一服务层 (UnifiedService)

为 CLI、GUI 和 Web 提供统一接口：

```go
service := service.NewUnifiedService()

// 注册引擎
service.RegisterEngine(fofaPlugin, config)

// 执行查询
response, err := service.Query(ctx, service.QueryRequest{
    Query:       "country=\"CN\" && port=\"80\"",
    Engines:     []string{"fofa", "hunter"},
    PageSize:    100,
    ProcessData: true,  // 启用数据处理管道
})

// 导出结果
service.Export(ctx, service.ExportRequest{
    Assets:     response.Assets,
    Format:     "excel",
    OutputPath: "output.xlsx",
})
```

## 数据处理管道

### 处理流程

```
原始数据 → 数据清洗 → 数据验证 → 数据富化 → 数据去重 → 最终结果
Priority:    10          50          80         100
```

### 配置处理器

```go
// 创建去重处理器
dedup := processors.NewDeduplicationProcessor(processors.StrategyAdvanced)

// 创建清洗处理器
cleaner := processors.NewDataCleaningProcessor()

// 创建验证处理器
validator := processors.NewValidationProcessor(false) // 非严格模式

// 创建富化处理器
enricher := processors.NewEnrichmentProcessor()

// 注册到服务
service.RegisterProcessor(cleaner, map[string]interface{}{
    "removeEmpty": true,
    "normalizeURLs": true,
})
service.RegisterProcessor(validator, nil)
service.RegisterProcessor(enricher, nil)
service.RegisterProcessor(dedup, map[string]interface{}{
    "strategy": "advanced",
})
```

## 使用场景

### 场景 1：CLI 工具

```go
func main() {
    service := service.NewUnifiedService()
    
    // 注册引擎
    service.RegisterEngine(fofaPlugin, fofaConfig)
    service.RegisterEngine(hunterPlugin, hunterConfig)
    
    // 注册处理器
    service.RegisterProcessor(processors.NewDataCleaningProcessor(), nil)
    service.RegisterProcessor(processors.NewDeduplicationProcessor("ip_port"), nil)
    
    // 执行查询
    resp, _ := service.Query(context.Background(), service.QueryRequest{
        Query:       os.Args[1],
        Engines:     []string{"fofa", "hunter"},
        ProcessData: true,
    })
    
    // 输出结果
    for _, asset := range resp.Assets {
        fmt.Printf("%s:%d\n", asset.IP, asset.Port)
    }
}
```

### 场景 2：GUI 应用

```go
// 在 GUI 中使用统一服务
service := service.NewUnifiedService()

// 异步查询
go func() {
    resp, err := service.Query(ctx, req)
    // 更新 GUI
    updateResultTable(resp.Assets)
}()
```

### 场景 3：Web API

```go
// HTTP 处理器
func handleQuery(w http.ResponseWriter, r *http.Request) {
    var req service.QueryRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    resp, err := globalService.Query(r.Context(), req)
    json.NewEncoder(w).Encode(resp)
}
```

### 场景 4：作为库集成

```go
// 其他项目可以导入 UniMap 作为库
import "github.com/unimap-icp-hunter/project/internal/service"

func yourFunction() {
    unimap := service.NewUnifiedService()
    // 使用 UniMap 的功能
}
```

## 扩展 UniMap

### 1. 开发自定义引擎插件

```go
package myplugin

import (
    "github.com/unimap-icp-hunter/project/internal/model"
    "github.com/unimap-icp-hunter/project/internal/plugin"
)

type MyEnginePlugin struct {
    // 配置字段
}

func NewMyEnginePlugin() *MyEnginePlugin {
    return &MyEnginePlugin{}
}

// 实现 EnginePlugin 接口的所有方法
func (p *MyEnginePlugin) Name() string { return "my_engine" }
func (p *MyEnginePlugin) Version() string { return "1.0.0" }
// ... 其他方法
```

### 2. 开发自定义处理器

```go
type MyProcessorPlugin struct{}

func (p *MyProcessorPlugin) Process(ctx context.Context, assets []model.UnifiedAsset) ([]model.UnifiedAsset, error) {
    // 自定义处理逻辑
    for i := range assets {
        // 修改 assets[i]
    }
    return assets, nil
}

func (p *MyProcessorPlugin) Priority() int {
    return 60  // 在验证后、富化前执行
}
```

### 3. 集成外部工具

UniMap 可以作为插件被其他工具集成：

```go
// 在其他工具中
import unimap "github.com/unimap-icp-hunter/project/internal/service"

func main() {
    // 使用 UniMap 的服务
    service := unimap.NewUnifiedService()
    
    // 将 UniMap 集成到你的工具中
    // ...
}
```

## 最佳实践

### 1. 插件开发

- ✅ 实现完整的生命周期方法
- ✅ 提供详细的元数据信息
- ✅ 实现健康检查逻辑
- ✅ 处理错误并返回有意义的错误信息
- ✅ 使用配置参数而非硬编码
- ✅ 编写单元测试

### 2. 性能优化

- ✅ 使用连接池复用 HTTP 连接
- ✅ 实现合理的超时机制
- ✅ 避免在处理器中进行重量级操作
- ✅ 利用并发处理多个引擎查询
- ✅ 合理设置处理器优先级

### 3. 错误处理

- ✅ 使用 context 支持取消和超时
- ✅ 区分可恢复和不可恢复的错误
- ✅ 提供详细的错误上下文
- ✅ 实现优雅降级

### 4. 配置管理

- ✅ 使用环境变量存储敏感信息
- ✅ 提供配置验证
- ✅ 提供合理的默认值
- ✅ 支持配置热更新（通过插件重载）

## 与其他项目对比

### 架构对比

| 项目 | 架构模式 | 扩展性 | 可集成性 |
|------|----------|--------|----------|
| UniMap | 插件化 + 统一服务层 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| Search_Viewer | 单体应用 | ⭐⭐ | ⭐ |
| ThunderSearch | 模块化 | ⭐⭐⭐ | ⭐⭐ |
| IntegSearch | 模块化 | ⭐⭐⭐ | ⭐⭐ |
| shepherd | 单体应用 | ⭐⭐ | ⭐⭐ |

### 功能对比

| 功能 | UniMap | 其他项目 |
|------|--------|----------|
| 动态加载插件 | ✅ | ❌ |
| 钩子系统 | ✅ | ❌ |
| 数据处理管道 | ✅ | 部分 |
| 统一服务层 | ✅ | ❌ |
| 多接口支持 | ✅ CLI/GUI/Web | 单一接口 |
| 健康检查 | ✅ | ❌ |
| 作为库使用 | ✅ | ❌ |

## 未来规划

### 短期目标（1-2个月）

- [ ] 插件热重载功能
- [ ] REST API 服务器
- [ ] WebSocket 实时推送
- [ ] 更多内置处理器
- [ ] 插件市场

### 中期目标（3-6个月）

- [ ] 插件依赖管理
- [ ] 分布式插件系统
- [ ] 插件版本管理
- [ ] 自动化测试框架
- [ ] 性能监控和分析

### 长期目标（6-12个月）

- [ ] 可视化插件开发工具
- [ ] 云端插件存储
- [ ] 插件安全沙箱
- [ ] 跨语言插件支持
- [ ] AI 辅助插件开发

## 示例项目

完整的示例代码请参考：
- `examples/custom_engine/` - 自定义引擎示例
- `examples/custom_processor/` - 自定义处理器示例
- `examples/integration/` - 集成示例

## 技术支持

- GitHub Issues: https://github.com/ElaineRosa6/unimap-icp-hunter/issues
- 文档: https://github.com/ElaineRosa6/unimap-icp-hunter/wiki
- 示例代码: https://github.com/ElaineRosa6/unimap-icp-hunter/tree/main/examples

---

**版本**: 1.0.0  
**最后更新**: 2026-02-04  
**维护者**: UniMap Team
