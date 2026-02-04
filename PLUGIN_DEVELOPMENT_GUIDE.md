# UniMap 插件开发指南

## 快速开始

本指南将带你从零开始开发一个 UniMap 插件。

## 前置要求

- Go 1.21+
- 了解 UniMap 的基本概念
- 阅读过 [PLUGIN_ARCHITECTURE.md](PLUGIN_ARCHITECTURE.md)

## 开发你的第一个引擎插件

### 步骤 1: 创建插件目录

```bash
mkdir -p plugins/myengine
cd plugins/myengine
```

### 步骤 2: 定义插件结构

创建 `plugin.go`:

```go
package myengine

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/unimap-icp-hunter/project/internal/model"
    "github.com/unimap-icp-hunter/project/internal/plugin"
)

// MyEnginePlugin 自定义搜索引擎插件
type MyEnginePlugin struct {
    apiKey     string
    baseURL    string
    httpClient *http.Client
    initialized bool
}

// NewMyEnginePlugin 创建插件实例
func NewMyEnginePlugin() *MyEnginePlugin {
    return &MyEnginePlugin{
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
    }
}
```

### 步骤 3: 实现基础接口

```go
// Name 返回插件名称
func (p *MyEnginePlugin) Name() string {
    return "myengine"
}

// Version 返回插件版本
func (p *MyEnginePlugin) Version() string {
    return "1.0.0"
}

// Description 返回插件描述
func (p *MyEnginePlugin) Description() string {
    return "My Custom Search Engine Plugin"
}

// Author 返回插件作者
func (p *MyEnginePlugin) Author() string {
    return "Your Name"
}

// Type 返回插件类型
func (p *MyEnginePlugin) Type() plugin.PluginType {
    return plugin.PluginTypeEngine
}
```

### 步骤 4: 实现生命周期方法

```go
// Initialize 初始化插件
func (p *MyEnginePlugin) Initialize(config map[string]interface{}) error {
    // 从配置中读取 API Key
    if apiKey, ok := config["api_key"].(string); ok {
        p.apiKey = apiKey
    } else {
        return fmt.Errorf("api_key is required")
    }

    // 从配置中读取 Base URL
    if baseURL, ok := config["base_url"].(string); ok {
        p.baseURL = baseURL
    } else {
        p.baseURL = "https://api.myengine.com"
    }

    p.initialized = true
    return nil
}

// Start 启动插件
func (p *MyEnginePlugin) Start(ctx context.Context) error {
    if !p.initialized {
        return fmt.Errorf("plugin not initialized")
    }
    
    // 可以在这里进行连接测试
    // ...
    
    return nil
}

// Stop 停止插件
func (p *MyEnginePlugin) Stop() error {
    // 清理资源
    if p.httpClient != nil {
        p.httpClient.CloseIdleConnections()
    }
    return nil
}

// Health 健康检查
func (p *MyEnginePlugin) Health() plugin.HealthStatus {
    if !p.initialized {
        return plugin.HealthStatus{
            Healthy: false,
            Message: "Plugin not initialized",
        }
    }
    
    // 可以进行 ping 测试
    // ...
    
    return plugin.HealthStatus{
        Healthy: true,
        Message: "Running",
        Details: map[string]interface{}{
            "base_url": p.baseURL,
        },
    }
}
```

### 步骤 5: 实现引擎特定方法

```go
// Translate 将 UQL AST 转换为引擎查询语句
func (p *MyEnginePlugin) Translate(ast *model.UQLAST) (string, error) {
    if ast == nil {
        return "", fmt.Errorf("AST cannot be nil")
    }
    
    // 将 AST 转换为引擎的查询语法
    query := ""
    
    switch ast.Type {
    case model.NodeTypeComparison:
        // 处理比较操作: country="CN"
        field := ast.Field
        operator := ast.Operator
        value := ast.Value
        
        // 转换字段名（如果引擎使用不同的字段名）
        engineField := p.translateField(field)
        
        // 构建查询
        query = fmt.Sprintf("%s:%s", engineField, value)
        
    case model.NodeTypeLogical:
        // 处理逻辑操作: A && B
        left, err := p.Translate(ast.Left)
        if err != nil {
            return "", err
        }
        
        right, err := p.Translate(ast.Right)
        if err != nil {
            return "", err
        }
        
        operator := ast.Operator
        if operator == "&&" {
            query = fmt.Sprintf("(%s AND %s)", left, right)
        } else if operator == "||" {
            query = fmt.Sprintf("(%s OR %s)", left, right)
        }
        
    case model.NodeTypeNegation:
        // 处理否定操作: !A
        inner, err := p.Translate(ast.Inner)
        if err != nil {
            return "", err
        }
        query = fmt.Sprintf("NOT (%s)", inner)
    }
    
    return query, nil
}

// translateField 转换字段名
func (p *MyEnginePlugin) translateField(field string) string {
    // 将 UQL 字段映射到引擎字段
    fieldMap := map[string]string{
        "ip":       "ip",
        "port":     "port",
        "country":  "country",
        "protocol": "protocol",
        "title":    "title",
        "body":     "content",
        "host":     "hostname",
    }
    
    if engineField, ok := fieldMap[field]; ok {
        return engineField
    }
    return field
}

// Search 执行搜索
func (p *MyEnginePlugin) Search(query string, page, pageSize int) (*model.EngineResult, error) {
    // 构建请求 URL
    url := fmt.Sprintf("%s/api/search?query=%s&page=%d&size=%d&key=%s",
        p.baseURL, query, page, pageSize, p.apiKey)
    
    // 发送 HTTP 请求
    resp, err := p.httpClient.Get(url)
    if err != nil {
        return nil, fmt.Errorf("search request failed: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("search failed with status: %d", resp.StatusCode)
    }
    
    // 解析响应
    var apiResponse struct {
        Code    int         `json:"code"`
        Message string      `json:"message"`
        Data    interface{} `json:"data"`
        Total   int         `json:"total"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
        return nil, fmt.Errorf("failed to decode response: %w", err)
    }
    
    // 返回引擎结果
    return &model.EngineResult{
        Engine: p.Name(),
        Data:   apiResponse.Data,
        Total:  apiResponse.Total,
    }, nil
}

// Normalize 将原生结果规范化为统一资产模型
func (p *MyEnginePlugin) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
    if raw == nil || raw.Data == nil {
        return []model.UnifiedAsset{}, nil
    }
    
    // 将 Data 转换为引擎特定的结构
    items, ok := raw.Data.([]interface{})
    if !ok {
        return nil, fmt.Errorf("invalid data format")
    }
    
    assets := make([]model.UnifiedAsset, 0, len(items))
    
    for _, item := range items {
        itemMap, ok := item.(map[string]interface{})
        if !ok {
            continue
        }
        
        // 提取字段并映射到统一模型
        asset := model.UnifiedAsset{
            IP:          getString(itemMap, "ip"),
            Port:        getInt(itemMap, "port"),
            Protocol:    getString(itemMap, "protocol"),
            Host:        getString(itemMap, "hostname"),
            Title:       getString(itemMap, "title"),
            CountryCode: getString(itemMap, "country"),
            Source:      p.Name(),
            Extra:       make(map[string]interface{}),
        }
        
        // 保存额外字段
        for k, v := range itemMap {
            if k != "ip" && k != "port" && k != "protocol" {
                asset.Extra[k] = v
            }
        }
        
        assets = append(assets, asset)
    }
    
    return assets, nil
}

// 辅助函数
func getString(m map[string]interface{}, key string) string {
    if v, ok := m[key]; ok {
        if s, ok := v.(string); ok {
            return s
        }
    }
    return ""
}

func getInt(m map[string]interface{}, key string) int {
    if v, ok := m[key]; ok {
        if i, ok := v.(float64); ok {
            return int(i)
        }
    }
    return 0
}
```

### 步骤 6: 实现引擎特性方法

```go
// SupportedFields 返回支持的查询字段
func (p *MyEnginePlugin) SupportedFields() []string {
    return []string{
        "ip",
        "port",
        "protocol",
        "country",
        "title",
        "body",
        "host",
        "url",
    }
}

// MaxPageSize 返回最大分页大小
func (p *MyEnginePlugin) MaxPageSize() int {
    return 1000
}

// RateLimit 返回速率限制配置
func (p *MyEnginePlugin) RateLimit() plugin.RateLimitConfig {
    return plugin.RateLimitConfig{
        RequestsPerSecond: 10,
        RequestsPerMinute: 100,
        RequestsPerHour:   1000,
        RequestsPerDay:    10000,
    }
}
```

### 步骤 7: 编写测试

创建 `plugin_test.go`:

```go
package myengine

import (
    "context"
    "testing"

    "github.com/unimap-icp-hunter/project/internal/model"
)

func TestMyEnginePlugin(t *testing.T) {
    plugin := NewMyEnginePlugin()
    
    // 测试初始化
    config := map[string]interface{}{
        "api_key":  "test_key",
        "base_url": "https://api.test.com",
    }
    
    if err := plugin.Initialize(config); err != nil {
        t.Fatalf("Initialize failed: %v", err)
    }
    
    // 测试启动
    if err := plugin.Start(context.Background()); err != nil {
        t.Fatalf("Start failed: %v", err)
    }
    
    // 测试健康检查
    health := plugin.Health()
    if !health.Healthy {
        t.Fatalf("Health check failed: %s", health.Message)
    }
    
    // 测试 UQL 转换
    ast := &model.UQLAST{
        Type:     model.NodeTypeComparison,
        Field:    "country",
        Operator: "=",
        Value:    "CN",
    }
    
    query, err := plugin.Translate(ast)
    if err != nil {
        t.Fatalf("Translate failed: %v", err)
    }
    
    if query != "country:CN" {
        t.Errorf("Expected 'country:CN', got '%s'", query)
    }
    
    // 测试停止
    if err := plugin.Stop(); err != nil {
        t.Fatalf("Stop failed: %v", err)
    }
}
```

### 步骤 8: 注册和使用插件

```go
package main

import (
    "context"
    
    "github.com/unimap-icp-hunter/project/internal/service"
    "github.com/unimap-icp-hunter/project/plugins/myengine"
)

func main() {
    // 创建统一服务
    svc := service.NewUnifiedService()
    
    // 创建插件实例
    enginePlugin := myengine.NewMyEnginePlugin()
    
    // 配置
    config := map[string]interface{}{
        "api_key":  "your_api_key",
        "base_url": "https://api.myengine.com",
    }
    
    // 注册插件
    if err := svc.RegisterEngine(enginePlugin, config); err != nil {
        panic(err)
    }
    
    // 使用插件查询
    resp, err := svc.Query(context.Background(), service.QueryRequest{
        Query:       "country=\"CN\" && port=\"80\"",
        Engines:     []string{"myengine"},
        PageSize:    100,
        ProcessData: true,
    })
    
    if err != nil {
        panic(err)
    }
    
    // 处理结果
    for _, asset := range resp.Assets {
        fmt.Printf("%s:%d\n", asset.IP, asset.Port)
    }
}
```

## 开发处理器插件

### 示例：IP 地理位置富化处理器

```go
package geoip

import (
    "context"
    "net"
    
    "github.com/unimap-icp-hunter/project/internal/model"
    "github.com/unimap-icp-hunter/project/internal/plugin"
)

type GeoIPProcessor struct {
    // GeoIP 数据库连接
}

func (p *GeoIPProcessor) Process(ctx context.Context, assets []model.UnifiedAsset) ([]model.UnifiedAsset, error) {
    for i := range assets {
        if assets[i].IP != "" {
            // 查询 IP 地理位置
            country, city := p.lookupGeoIP(assets[i].IP)
            assets[i].CountryCode = country
            assets[i].City = city
        }
    }
    return assets, nil
}

func (p *GeoIPProcessor) Priority() int {
    return 70  // 在验证后、富化前
}

// ... 实现其他方法
```

## 开发导出器插件

### 示例：CSV 导出器

```go
package csvexporter

import (
    "encoding/csv"
    "os"
    
    "github.com/unimap-icp-hunter/project/internal/model"
    "github.com/unimap-icp-hunter/project/internal/plugin"
)

type CSVExporter struct{}

func (e *CSVExporter) Export(assets []model.UnifiedAsset, outputPath string) error {
    file, err := os.Create(outputPath)
    if err != nil {
        return err
    }
    defer file.Close()
    
    writer := csv.NewWriter(file)
    defer writer.Flush()
    
    // 写入表头
    writer.Write([]string{"IP", "Port", "Protocol", "Host", "Title"})
    
    // 写入数据
    for _, asset := range assets {
        writer.Write([]string{
            asset.IP,
            fmt.Sprintf("%d", asset.Port),
            asset.Protocol,
            asset.Host,
            asset.Title,
        })
    }
    
    return nil
}

func (e *CSVExporter) SupportedFormats() []string {
    return []string{"csv"}
}

// ... 实现其他方法
```

## 最佳实践

### 1. 错误处理

```go
// ✅ 好的做法
if err != nil {
    return nil, fmt.Errorf("failed to search: %w", err)
}

// ❌ 不好的做法
if err != nil {
    return nil, err
}
```

### 2. 配置验证

```go
func (p *MyEnginePlugin) Initialize(config map[string]interface{}) error {
    // ✅ 验证必需参数
    apiKey, ok := config["api_key"].(string)
    if !ok || apiKey == "" {
        return fmt.Errorf("api_key is required and must be non-empty")
    }
    
    // ✅ 提供默认值
    if timeout, ok := config["timeout"].(int); ok {
        p.timeout = timeout
    } else {
        p.timeout = 30  // 默认 30 秒
    }
    
    return nil
}
```

### 3. 资源管理

```go
func (p *MyEnginePlugin) Stop() error {
    // ✅ 清理资源
    if p.httpClient != nil {
        p.httpClient.CloseIdleConnections()
    }
    
    // ✅ 取消上下文
    if p.cancel != nil {
        p.cancel()
    }
    
    return nil
}
```

### 4. 并发安全

```go
type MyEnginePlugin struct {
    mu         sync.RWMutex
    httpClient *http.Client
}

func (p *MyEnginePlugin) getClient() *http.Client {
    p.mu.RLock()
    defer p.mu.RUnlock()
    return p.httpClient
}
```

## 调试技巧

### 1. 使用日志

```go
import "log"

func (p *MyEnginePlugin) Search(query string, page, pageSize int) (*model.EngineResult, error) {
    log.Printf("[MyEngine] Searching with query: %s, page: %d, size: %d", query, page, pageSize)
    // ...
}
```

### 2. 使用健康检查

```go
func (p *MyEnginePlugin) Health() plugin.HealthStatus {
    // 测试 API 连接
    resp, err := p.httpClient.Get(p.baseURL + "/health")
    if err != nil {
        return plugin.HealthStatus{
            Healthy: false,
            Message: fmt.Sprintf("Health check failed: %v", err),
        }
    }
    defer resp.Body.Close()
    
    return plugin.HealthStatus{
        Healthy: resp.StatusCode == 200,
        Message: fmt.Sprintf("Status: %d", resp.StatusCode),
    }
}
```

## 发布插件

### 1. 打包

```bash
# 构建插件
go build -o myengine.so -buildmode=plugin ./plugins/myengine

# 创建发布包
tar -czf myengine-v1.0.0.tar.gz myengine.so README.md LICENSE
```

### 2. 文档

提供以下文档：
- README.md - 插件介绍和使用说明
- CHANGELOG.md - 版本变更日志
- LICENSE - 许可证

### 3. 示例代码

提供完整的使用示例和测试用例。

## 常见问题

### Q: 如何处理 API 速率限制？

A: 在插件中实现速率限制逻辑：

```go
import "golang.org/x/time/rate"

type MyEnginePlugin struct {
    limiter *rate.Limiter
}

func NewMyEnginePlugin() *MyEnginePlugin {
    return &MyEnginePlugin{
        limiter: rate.NewLimiter(rate.Limit(10), 1), // 10 req/s
    }
}

func (p *MyEnginePlugin) Search(...) {
    // 等待速率限制
    p.limiter.Wait(context.Background())
    // 执行搜索
}
```

### Q: 如何处理分页查询？

A: 实现分页逻辑并在 Normalize 中合并结果。

### Q: 如何添加自定义字段？

A: 使用 `Extra` 字段存储自定义数据：

```go
asset.Extra["custom_field"] = value
```

## 获取帮助

- 查看 [PLUGIN_ARCHITECTURE.md](PLUGIN_ARCHITECTURE.md)
- 查看示例代码 `examples/`
- 提交 Issue: https://github.com/ElaineRosa6/unimap-icp-hunter/issues

---

**版本**: 1.0.0  
**最后更新**: 2026-02-04
