# 架构改进说明

> 文档状态：历史架构说明 + 当前实现注记（更新于 2026-03-23）。
> 说明：本文保留了 v2.0 时期的架构设计背景，具体实现状态请以 WORK_LOG.md 和 PROJECT_SUMMARY.md 为准。

## 当前实现注记（2026-03-23）

- Web 层已完成多轮 handler 拆分，`web/server.go` 主要负责初始化与装配。
- 路由已统一由 `web/router.go` 注册，限流已由 `web/middleware_ratelimit.go` 按配置启停。
- 统一错误结构、CORS、请求体限制、Prometheus 基础指标已接入。
- monitor URL 可达性检查已下沉到 application service（`internal/service/monitor_app_service.go`）。
- tamper 批处理并发已统一到 `internal/util/workerpool`。

## 🎯 架构背景（v2.0 时期）

### 基于同类项目分析的优化

通过分析 Search_Viewer、ThunderSearch、fshzqSearch、IntegSearch、shepherd、Search-Tools、koko-moni 等项目，我们实现了以下架构改进：

## 🔌 插件化架构

### 核心优势

1. **开放扩展，关闭修改** - 通过插件接口添加新功能，无需修改核心代码
2. **统一抽象** - 所有插件遵循统一的生命周期和接口规范
3. **松耦合** - 插件之间互不依赖，可独立开发和测试
4. **易于集成** - 简单的注册机制，支持热插拔

### 插件类型

| 插件类型 | 说明 | 示例 |
|---------|------|------|
| 引擎插件 | 接入新的搜索引擎 | FOFA, Hunter, ZoomEye, Quake |
| 处理器插件 | 数据处理和转换 | 去重、清洗、验证、富化 |
| 导出器插件 | 数据导出功能 | JSON, Excel, CSV |
| 通知器插件 | 消息通知功能 | Email, Webhook, Slack |

### 内置数据处理器

```
数据清洗 (优先级 10)
  ↓ 去除空白、规范化
数据验证 (优先级 50)
  ↓ 验证 IP、端口、URL
数据富化 (优先级 80)
  ↓ 添加指纹、服务类型
数据去重 (优先级 100)
  ↓ 高级去重策略
最终结果
```

## 🏗️ 统一服务层

提供统一的接口供 CLI、GUI 和 Web 使用：

```go
service := service.NewUnifiedService()

// 注册引擎和处理器
service.RegisterEngine(enginePlugin, config)
service.RegisterProcessor(processorPlugin, config)

// 执行查询
response := service.Query(ctx, QueryRequest{
    Query:       "country=\"CN\" && port=\"80\"",
    Engines:     []string{"fofa", "hunter"},
    ProcessData: true,
})
```

## 🎣 钩子系统

事件驱动的扩展机制：

```go
hooks.RegisterHook(HookBeforeQuery, func(pluginName string, data map[string]interface{}) error {
    // 查询前的自定义逻辑
    return nil
})
```

支持的钩子：
- 查询生命周期：BeforeQuery, AfterQuery, QueryError
- 插件生命周期：BeforeLoad, AfterLoad, BeforeStart, AfterStart
- 数据处理：BeforeProcess, AfterProcess

## 📊 数据处理管道

灵活的数据处理流程：

```go
// 自动按优先级排序并顺序执行
pipeline := NewProcessorPipeline(processors)
processedData := pipeline.Process(ctx, rawData)
```

## 🔗 多接口支持

### CLI 接口
```bash
unimap query "country=\"CN\"" --engines fofa,hunter --process
```

### GUI 接口
```go
// Fyne GUI 应用
app := fyne.NewApp()
// 使用统一服务
```

### Web API 接口（历史示例）
```bash
POST /api/query
{
  "query": "country=\"CN\"",
  "engines": ["fofa", "hunter"],
  "processData": true
}
```

## 📚 作为库使用

UniMap 可以被其他工具作为库集成：

```go
import "github.com/unimap-icp-hunter/project/internal/service"

func main() {
    unimap := service.NewUnifiedService()
    // 使用 UniMap 的功能
}
```

## 与同类项目对比

| 特性 | UniMap v2.0 | Search_Viewer | ThunderSearch | fshzqSearch | IntegSearch |
|------|-------------|---------------|---------------|-------------|-------------|
| 插件化架构 | ✅ | ❌ | 部分 | ❌ | ❌ |
| 统一服务层 | ✅ | ❌ | ❌ | ❌ | ❌ |
| 钩子系统 | ✅ | ❌ | ❌ | ❌ | ❌ |
| 数据处理管道 | ✅ (4种处理器) | 基础 | 基础 | 基础 | 中等 |
| 多接口支持 | ✅ CLI/GUI/Web | GUI | GUI | CLI | GUI |
| 作为库使用 | ✅ | ❌ | ❌ | ❌ | ❌ |
| 可扩展性 | ⭐⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐ |

## 架构亮点

### 1. 优雅的引擎聚合

通过插件接口统一不同引擎的异构 API：

```go
type EnginePlugin interface {
    Translate(ast *UQLAST) (string, error)  // UQL → 引擎查询
    Search(query string, page, size int) (*EngineResult, error)
    Normalize(raw *EngineResult) ([]UnifiedAsset, error)  // 结果规范化
}
```

### 2. 高效的数据处理

- **可配置的去重策略**: ip_port, url, host, advanced
- **数据清洗**: 自动去除空白、规范化 URL、小写转换
- **数据验证**: IP 地址、端口号、URL 格式验证
- **数据富化**: 自动添加指纹、服务类型推测

### 3. 平衡的用户体验

- **CLI**: 适合自动化脚本和批处理
- **GUI**: 适合交互式查询和可视化
- **Web API**: 适合集成和远程调用

### 4. 强大的扩展性

- **插件热加载**: 动态注册和卸载插件
- **事件钩子**: 在关键节点注入自定义逻辑
- **作为库使用**: 被其他工具集成
- **插件市场**: 支持第三方插件生态

## 快速开始

### 使用统一服务层

```go
package main

import (
    "context"
    "github.com/unimap-icp-hunter/project/internal/service"
    "github.com/unimap-icp-hunter/project/internal/plugin/processors"
)

func main() {
    // 创建服务
    svc := service.NewUnifiedService()
    
    // 注册处理器
    svc.RegisterProcessor(processors.NewDataCleaningProcessor(), nil)
    svc.RegisterProcessor(processors.NewDeduplicationProcessor("advanced"), nil)
    
    // 执行查询
    resp, _ := svc.Query(context.Background(), service.QueryRequest{
        Query:       "country=\"CN\" && port=\"80\"",
        Engines:     []string{"fofa"},
        ProcessData: true,
    })
    
    // 使用结果
    for _, asset := range resp.Assets {
        println(asset.IP, asset.Port)
    }
}
```

## 文档

- [插件架构指南](PLUGIN_ARCHITECTURE.md) - 详细的架构设计文档
- [插件开发指南](PLUGIN_DEVELOPMENT_GUIDE.md) - 如何开发自定义插件
- [使用手册](USAGE.md) - 完整的使用说明
- [API 密钥获取](API_KEYS.md) - 各引擎 API Key 获取方法

## 示例

- [插件示例](examples/plugin_demo.go) - 完整的插件使用示例
- [自定义引擎](examples/custom_engine/) - 如何开发自定义引擎插件

## 贡献指南

我们欢迎社区贡献！您可以：

1. **开发新的引擎插件** - 支持更多搜索引擎
2. **开发新的处理器** - 添加新的数据处理功能
3. **开发新的导出器** - 支持更多导出格式
4. **改进文档** - 帮助其他用户更好地使用
5. **报告 Bug** - 帮助我们改进质量

## 许可证

本项目采用 MIT 许可证。

---

**版本口径**: 历史架构说明（v2.0）  
**文档更新日期**: 2026-03-23  
**维护者**: UniMap Team
