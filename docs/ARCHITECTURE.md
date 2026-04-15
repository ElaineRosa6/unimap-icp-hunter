# UniMap 项目架构文档

## 1. 架构概述

UniMap 是一个多引擎统一查询与监控系统，采用分层架构设计，确保系统的可扩展性、可维护性和可测试性。

### 1.1 核心设计原则

- **分层架构**：清晰的层次分离，遵循依赖倒置原则
- **模块化**：功能模块独立，低耦合高内聚
- **接口驱动**：通过接口定义行为，实现可插拔性
- **配置驱动**：支持多环境配置，便于部署和维护

## 2. 架构分层

### 2.1 分层结构图

```
┌─────────────────────────────────────────────────────────┐
│                   Presentation Layer                    │
│   ┌─────────────────┐  ┌─────────────────┐             │
│   │     Web API     │  │      GUI        │             │
│   └─────────────────┘  └─────────────────┘             │
└─────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────┐
│                  Application Layer                      │
│   ┌─────────────────┐  ┌─────────────────┐             │
│   │  Query Service  │  │ Tamper Service  │             │
│   ├─────────────────┤  ├─────────────────┤             │
│   │Monitor Service  │  │Screenshot Service│            │
│   └─────────────────┘  └─────────────────┘             │
└─────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────┐
│                     Domain Layer                        │
│   ┌─────────────────┐  ┌─────────────────┐             │
│   │   Adapters      │  │    Core Logic   │             │
│   ├─────────────────┤  ├─────────────────┤             │
│   │  Orchestrator   │  │    Models       │             │
│   └─────────────────┘  └─────────────────┘             │
└─────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────┐
│                  Infrastructure Layer                   │
│   ┌─────────────────┐  ┌─────────────────┐             │
│   │    Config       │  │    Storage      │             │
│   ├─────────────────┤  ├─────────────────┤             │
│   │     Logger      │  │    Network      │             │
│   └─────────────────┘  └─────────────────┘             │
└─────────────────────────────────────────────────────────┘
```

### 2.2 分层职责说明

#### 2.2.1 Presentation Layer（表现层）
- **位置**：`web/`, `cmd/unimap-gui/`, `cmd/unimap-cli/`
- **职责**：处理用户交互，展示数据，接收用户输入
- **组件**：
  - Web API：HTTP接口，WebSocket连接
  - GUI：桌面应用界面
  - CLI：命令行工具

#### 2.2.2 Application Layer（应用层）
- **位置**：`internal/service/`
- **职责**：协调领域对象完成业务功能，处理业务规则
- **组件**：
  - `UnifiedService`：统一服务入口
  - `QueryAppService`：查询业务逻辑
  - `TamperAppService`：篡改检测业务逻辑
  - `MonitorAppService`：监控业务逻辑
  - `ScreenshotAppService`：截图业务逻辑

#### 2.2.3 Domain Layer（领域层）
- **位置**：`internal/adapter/`, `internal/core/`, `internal/model/`
- **职责**：核心业务逻辑，领域模型，业务规则
- **组件**：
  - `EngineAdapter`：多引擎适配器接口
  - `EngineOrchestrator`：引擎编排器
  - `UQLParser`：统一查询语言解析器
  - `ResultMerger`：结果合并器

#### 2.2.4 Infrastructure Layer（基础设施层）
- **位置**：`internal/config/`, `internal/logger/`, `internal/utils/`
- **职责**：提供技术支持，实现技术细节
- **组件**：
  - `Config`：配置管理
  - `Logger`：日志系统
  - `Cache`：缓存实现
  - `WorkerPool`：工作池
  - `ProxyPool`：代理池

## 3. 核心模块

### 3.1 多引擎查询模块

```go
// EngineAdapter 接口定义
type EngineAdapter interface {
    Name() string
    Translate(ast *model.UQLAST) (string, error)
    Search(query string, page, pageSize int) (*model.EngineResult, error)
    Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error)
    GetQuota() (*model.QuotaInfo, error)
    IsWebOnly() bool
}
```

**支持的引擎**：
- FOFA
- Hunter
- ZoomEye
- Quake
- Shodan

### 3.2 篡改检测模块

**核心功能**：
- 网页内容哈希计算
- 恶意内容检测（挂马、黄色网站）
- 历史记录管理
- 结果比对分析

### 3.3 截图模块

**支持模式**：
- CDP模式：使用 Chrome DevTools Protocol
- Extension模式：使用浏览器扩展
- ScreenshotRouter：双模式共存 + 健康探测 + 自动降级

**Cookie/登录状态管理**：
- CDP 已连接或 Extension 已配对时，自动复用浏览器登录会话，无需手动填写 Cookie
- 前端自动检测各引擎登录状态，已登录时折叠 Cookie 输入区
- 未登录时展开并显示 "点击登录" 跳转链接
- 无浏览器会话（headless 模式）时，Cookie 输入区展开供填备用 Cookie
- 后端 API: `GET /api/cookies/login-status` 返回 CDP/Extension 状态及各引擎登录检测结果

### 3.4 监控模块

**功能**：
- URL可达性检测
- 端口扫描
- 定时任务调度

## 4. 数据流向

### 4.1 查询流程

```
用户请求 → Web API → QueryAppService → EngineOrchestrator → EngineAdapter → 外部API
                                                               ↓
                                                         结果合并 → 缓存 → 返回结果
```

### 4.2 篡改检测流程

```
启动检测 → TamperAppService → 获取网页内容 → 计算哈希 → 检测恶意内容 → 比对历史 → 保存结果
```

## 5. 关键设计决策

### 5.1 缓存策略

- **多级缓存**：内存缓存 + Redis缓存
- **TTL管理**：按引擎配置不同的缓存时间
- **缓存键生成**：`md5(engineName + ":" + query + ":" + page + ":" + pageSize)`

### 5.2 并发控制

- **工作池模式**：使用 `WorkerPool` 管理并发任务
- **速率限制**：按引擎配置 QPS 限制
- **资源隔离**：避免单个任务影响整体系统

### 5.3 错误处理

- **统一错误类型**：`UnimapError` 封装错误信息
- **错误分类**：网络、API、配置、运行时、业务、验证
- **堆栈跟踪**：完整的错误调用链

### 5.4 配置管理

- **YAML配置**：支持多环境配置
- **环境变量**：支持敏感信息注入
- **默认值**：提供合理的默认配置
- **配置验证**：启动时验证配置有效性

## 6. 部署架构

### 6.1 单机部署

```
┌─────────────────────────┐
│    UniMap Web Server    │
│  ┌───────────────────┐  │
│  │   HTTP/WebSocket  │  │
│  └───────────────────┘  │
│  ┌───────────────────┐  │
│  │   Services Layer  │  │
│  └───────────────────┘  │
│  ┌───────────────────┐  │
│  │  Storage (Local)  │  │
│  └───────────────────┘  │
└─────────────────────────┘
```

### 6.2 分布式部署

```
┌─────────────────────────────────┐
│          Load Balancer         │
└───────────────┬─────────────────┘
                │
┌───────────────┼─────────────────┐
│               │                 │
▼               ▼                 ▼
┌─────────────┐ ┌─────────────┐ ┌─────────────┐
│  Node 1     │ │  Node 2     │ │  Node 3     │
│ ┌─────────┐ │ │ ┌─────────┐ │ │ ┌─────────┐ │
│ │ Service │ │ │ │ Service │ │ │ │ Service │ │
│ └─────────┘ │ │ └─────────┘ │ │ └─────────┘ │
└─────────────┘ └─────────────┘ └─────────────┘
                │
                ▼
        ┌─────────────────┐
        │   Redis Cache   │
        └─────────────────┘
```

## 7. 技术栈

- **语言**：Go 1.20+
- **Web框架**：标准库 `net/http`
- **配置**：YAML
- **缓存**：内存 + Redis
- **浏览器自动化**：chromedp
- **日志**：自定义日志系统
- **监控**：Prometheus metrics

## 8. 扩展点

### 8.1 新增引擎适配器

```go
// 1. 实现 EngineAdapter 接口
type CustomEngineAdapter struct {
    // 字段定义
}

func (c *CustomEngineAdapter) Name() string {
    return "custom"
}

// 实现其他方法...

// 2. 注册到 Orchestrator
orchestrator.RegisterAdapter("custom", &CustomEngineAdapter{})
```

### 8.2 新增插件

```go
// 实现 Plugin 接口
type CustomPlugin struct{}

func (p *CustomPlugin) Name() string {
    return "custom"
}

// 实现其他方法...

// 注册插件
pluginManager.Register(&CustomPlugin{})
```

## 9. 性能优化

- **缓存策略**：动态缓存时间，热点数据优先
- **并发控制**：工作池限制并发数
- **资源复用**：HTTP连接池，浏览器实例复用
- **延迟加载**：按需初始化组件

## 10. 安全性考虑

- **输入验证**：所有用户输入严格验证
- **认证授权**：API密钥验证，Token认证
- **加密传输**：HTTPS支持
- **敏感信息保护**：环境变量注入，避免硬编码

## 11. 维护建议

- **定期更新依赖**：保持安全补丁更新
- **监控系统健康**：定期检查日志和指标
- **性能监控**：关注缓存命中率，查询响应时间
- **备份策略**：定期备份配置和数据

## 12. 演进路线

- **微服务化**：拆分核心服务为独立微服务
- **容器化**：Docker/Kubernetes部署
- **云原生**：适配云平台特性
- **AI增强**：引入机器学习优化查询和检测