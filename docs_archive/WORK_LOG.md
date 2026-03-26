# 项目工作日志

## 2026-03-26 工作记录

### M1: 缓存能力统一配置（按引擎 TTL/QPS 细化）

#### 1. 配置结构扩展
- **文件**: `internal/config/config.go`
- **内容**:
  - 新增 `EngineCacheConfig` 结构体，包含 `Enabled`、`TTL`、`MaxSize` 字段
  - 在 `Cache` 配置中添加 `Engines map[string]EngineCacheConfig` 字段
  - 在 `applyDefaults()` 中添加各引擎的默认缓存配置
  - 新增 `GetEngineCacheConfig()`、`GetCacheTTLForEngine()` 等方法

#### 2. 缓存策略扩展
- **文件**: `internal/utils/cache_strategy.go`
- **内容**:
  - 新增 `EngineCacheConfig` 接口和 `SimpleEngineCacheConfig` 实现
  - 新增 `ConfigBasedCacheStrategy` 策略，支持按引擎配置缓存
  - 添加 `SetEngineConfig()`、`IsCacheEnabledForEngine()` 等方法

#### 3. Orchestrator 改造
- **文件**: `internal/adapter/orchestrator.go`
- **内容**:
  - 添加 `engineCacheTTL` 字段存储按引擎的缓存配置
  - 新增 `SetEngineCacheTTL()`、`GetEngineCacheTTL()` 等方法
  - 修改 `SearchTask.Execute()` 和 `PaginatedSearchTask.Execute()` 使用按引擎的缓存 TTL

#### 4. UnifiedService 集成
- **文件**: `internal/service/unified_service.go`
- **内容**:
  - 在 `NewUnifiedServiceWithConfig()` 中加载引擎级别的缓存配置
  - 将配置传递给 orchestrator

#### 5. 配置示例更新
- **文件**: `configs/config.yaml.example`
- **内容**:
  - 添加 `cache.engines` 配置段，展示各引擎的缓存配置示例

#### 默认配置
| 引擎 | 默认 TTL | 说明 |
|------|----------|------|
| quake | 3600秒 (1小时) | 标准 API 响应 |
| zoomeye | 1800秒 (30分钟) | API 限制较严格 |
| hunter | 3600秒 (1小时) | 标准 API 响应 |
| fofa | 1800秒 (30分钟) | 数据更新频繁 |
| shodan | 7200秒 (2小时) | 数据相对稳定 |

### 问题修复

#### 1. MemoryCache goroutine 泄漏
- **文件**: `internal/utils/cache.go`
- **内容**: 添加 `Close()` 方法和停止机制

#### 2. nil 指针检查
- **文件**: `internal/adapter/orchestrator.go`
- **内容**: 在 `Normalize()` 前添加 nil 检查

#### 3. 敏感信息日志
- **文件**: `internal/adapter/fofa.go`
- **内容**: 移除详细用户信息日志

#### 4. JSON 编码错误处理
- **文件**: `web/http_helpers.go`
- **内容**: 添加错误日志记录

### M3: Application Service 层下沉（完成）

#### 1. ScreenshotAppService 扩展
- **文件**: `internal/service/screenshot_app_service.go`
- **内容**:
  - 新增 `baseDir` 字段与 `GetBaseDir()` 方法
  - 新增 `ListBatches()` 列出所有截图批次
  - 新增 `ListBatchFiles()` 列出批次文件，支持预览URL构建器
  - 新增 `DeleteBatch()` 删除指定批次
  - 新增 `DeleteFile()` 删除批次内指定文件
  - 路径安全检查防止目录穿越

#### 2. TamperAppService 扩展
- **文件**: `internal/service/tamper_app_service.go`
- **内容**:
  - 新增 `HistoryFilter` 结构体支持多维度过滤
  - 新增 `HistoryRecord` / `HistoryResult` 结构体
  - 新增 `QueryHistory()` 支持按 URL/类型/模式/关键词过滤与分页

#### 3. Web Handler 瘦身
- **文件**: `web/screenshot_handlers.go`, `web/tamper_handlers.go`
- **内容**:
  - `handleScreenshotBatches` 改为调用 `screenshotApp.ListBatches()`
  - `handleScreenshotBatchFiles` 改为调用 `screenshotApp.ListBatchFiles()`
  - `handleScreenshotBatchDelete` 改为调用 `screenshotApp.DeleteBatch()`
  - `handleScreenshotFileDelete` 改为调用 `screenshotApp.DeleteFile()`
  - `handleTamperHistory` 改为调用 `tamperApp.QueryHistory()`
  - `handleTamperHistoryDelete` 改为调用 `tamperApp.DeleteCheckRecords()`
  - 移除 handler 中直接操作 `tamper.NewHashStorage` 的代码

#### 4. 测试修复
- **文件**: `web/screenshot_handlers_test.go`, `web/tamper_handlers_test.go`
- **内容**:
  - 更新测试辅助函数，初始化 `screenshotApp` / `tamperApp` 字段
  - 确保测试中 Server 结构体完整初始化

#### 验证结果
```
go build ./...
go test ./...
```

结果：通过。

### M4: Redis 缓存支持完善（完成）

#### 1. 配置结构扩展
- **文件**: `internal/config/config.go`
- **内容**:
  - 扩展 `Cache.Redis` 配置结构，新增连接池配置字段
  - 新增 `pool_size`、`min_idle_conns`、`max_idle_conns`、`max_retries` 等配置项
  - 新增 `dial_timeout`、`read_timeout`、`write_timeout`、`pool_timeout` 超时配置
  - 新增 `conn_max_lifetime`、`conn_max_idle_time` 连接生命周期配置
  - 添加配置管理器辅助方法：`GetCacheBackend()`、`GetRedisAddr()` 等

#### 2. RedisConfig 结构体
- **文件**: `internal/utils/cache.go`
- **内容**:
  - 新增 `RedisConfig` 结构体封装 Redis 配置
  - 支持完整的连接池参数配置
  - 提供合理的默认值

#### 3. RedisCache 改造
- **文件**: `internal/utils/cache.go`
- **内容**:
  - 重构 `NewRedisCache()` 接受 `RedisConfig` 参数
  - 新增 `IsHealthy()` 方法实现健康检查
  - 健康检查每 30 秒执行一次，避免频繁检测
  - 连接失败时记录详细日志

#### 4. 缓存工厂函数
- **文件**: `internal/utils/cache.go`
- **内容**:
  - 新增 `NewCacheWithConfig()` 支持完整配置
  - 保留 `NewCache()` 兼容简化调用
  - Redis 连接失败时自动回退到内存缓存

#### 5. UnifiedService 集成
- **文件**: `internal/service/unified_service.go`
- **内容**:
  - 使用 `NewCacheWithConfig()` 创建缓存实例
  - 从配置构建完整的 `RedisConfig` 结构体

#### 6. 配置示例更新
- **文件**: `configs/config.yaml.example`
- **内容**:
  - 添加完整的 Redis 连接池配置示例
  - 包含默认值和注释说明

#### 验证结果
```
go build ./...
go test ./...
```

结果：通过。

### L1: Prometheus 指标完善（完成）

#### 1. 自定义时间分位桶
- **文件**: `internal/metrics/metrics.go`
- **内容**:
  - HTTP请求时间桶：100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
  - 查询时间桶：500ms, 1s, 2s, 5s, 10s, 30s, 60s
  - 截图时间桶：1s, 2s, 5s, 10s, 20s, 30s, 60s

#### 2. 引擎查询细分指标
- **文件**: `internal/metrics/metrics.go`, `internal/adapter/orchestrator.go`
- **内容**:
  - 新增 `unimap_engine_query_total` 按引擎和状态统计查询数
  - 新增 `unimap_engine_query_duration_seconds` 按引擎统计查询耗时
  - 新增 `unimap_engine_errors_total` 按引擎统计错误数
  - 在 `SearchTask.Execute` 中记录缓存命中、成功、失败指标

#### 3. 截图指标
- **文件**: `internal/metrics/metrics.go`, `web/screenshot_handlers.go`
- **内容**:
  - 新增 `unimap_screenshot_requests_total` 按类型和状态统计
  - 新增 `unimap_screenshot_duration_seconds` 截图耗时分布
  - 新增 `unimap_screenshot_batch_size` 批量截图大小分布
  - 在搜索引擎截图、目标截图、批量截图处理函数中记录指标

#### 4. WebSocket 指标
- **文件**: `internal/metrics/metrics.go`, `web/websocket_handlers.go`
- **内容**:
  - 新增 `unimap_websocket_connections` 当前连接数
  - 新增 `unimap_websocket_messages_total` 消息计数（入站/出站）
  - 在连接建立/断开、消息收发时记录指标

#### 5. 资源使用指标
- **文件**: `internal/metrics/metrics.go`, `internal/service/unified_service.go`
- **内容**:
  - 新增 `unimap_goroutines_count` goroutine数量
  - 新增 `unimap_memory_alloc_mb` 内存分配量
  - 新增 `unimap_memory_sys_mb` 系统内存使用量
  - 在资源检查时更新内存统计

#### 6. 缓存统计指标
- **文件**: `internal/metrics/metrics.go`
- **内容**:
  - 新增 `unimap_cache_size` 缓存大小
  - 新增 `unimap_cache_hit_rate` 缓存命中率

#### 7. 批量操作指标
- **文件**: `internal/metrics/metrics.go`
- **内容**:
  - 新增 `unimap_batch_operations_total` 批量操作计数
  - 新增 `unimap_batch_operation_size` 批量操作大小分布

#### 验证结果
```
go build ./...
go test ./...
```

结果：通过。

### L2: request_id 日志贯穿（完成）

#### 1. 基础设施
- **文件**: `internal/requestid/requestid.go`
- **内容**:
  - `New()` 生成唯一请求ID：`rid-{timestamp}-{random}-{counter}`
  - `WithContext()` / `FromContext()` 存取 context 中的 request_id
  - `Normalize()` 清理和验证请求ID
  - `HeaderName` 常量定义 HTTP 头名称 `X-Request-Id`

#### 2. Logger Ctx 方法
- **文件**: `internal/logger/logger.go`
- **内容**:
  - `CtxDebugf()` / `CtxInfof()` / `CtxWarnf()` / `CtxErrorf()` 带上下文日志方法
  - 自动从 context 提取 request_id 并添加到日志前缀：`[rid=xxx]`
  - 保留原有非 Ctx 方法用于无 context 场景

#### 3. Web 中间件
- **文件**: `web/middleware_requestid.go`
- **内容**:
  - 从请求头读取或生成 request_id
  - 写入响应头 `X-Request-Id`
  - 注入到请求 context 中

#### 4. Orchestrator 改造
- **文件**: `internal/adapter/orchestrator.go`
- **内容**:
  - `SearchTask` 已有 `ctx` 字段，使用 `logger.CtxXxx` 方法
  - `PaginatedSearchTask` 新增 `ctx` 字段
  - 所有任务执行日志改为 Ctx 方法

#### 5. Screenshot Manager 改造
- **文件**: `internal/screenshot/manager.go`
- **内容**:
  - `CaptureSearchEngineResult()` 使用 `CtxInfof` 记录截图成功
  - `CaptureTargetWebsite()` 使用 `CtxInfof` 记录截图成功
  - `CaptureBatchURLs()` 使用 `CtxWarnf` / `CtxInfof` 记录批量截图结果

#### 6. Service 层改造
- **文件**: `internal/service/unified_service.go`
- **内容**:
  - `Query()` 方法使用 `CtxInfof` 记录查询开始/结束
  - 使用 `CtxDebugf` 记录缓存命中/未命中
  - `checkResourceLimits()` 使用 `CtxWarnf` 记录内存警告

#### 验证结果
```
go build ./...
go test ./...
```

结果：通过。

### L3: regexp 预编译优化（完成）

#### 1. 发现问题
- **文件**: `internal/plugin/processors/validation.go`, `web/monitor_handlers.go`
- **问题**: 使用 `regexp.MatchString()` 在每次调用时动态编译正则表达式

#### 2. 修复内容
- **文件**: `internal/plugin/processors/validation.go`
  - 新增 `reValidURL` 预编译正则：`^https?://[^\s/$.?#].[^\s]*$`
  - 修改 `isValidURL()` 使用预编译正则

- **文件**: `web/monitor_handlers.go`
  - 新增 `reURLPattern` 预编译正则：`^(https?://)?([\w.-]+)(:\d+)?(/.*)?$`
  - 修改 `filterValidURLs()` 使用预编译正则

#### 3. 性能提升
- 避免每次调用时解析和编译正则表达式
- 减少内存分配
- 提高批量 URL 验证场景的性能

#### 验证结果
```
go build ./...
go test ./...
```

结果：通过。

### L4: 文档自动化校验（完成）

#### 1. 创建校验脚本
- **文件**: `scripts/verify_docs.sh`
- **功能**:
  - 检查目录结构一致性（cmd、internal、web、configs）
  - 检查关键文件存在性（server.go、config.yaml.example、QUICKSTART.md）
  - 检查配置参数一致性（MaxConcurrent、CacheTTL、CacheMaxSize、CacheCleanupInterval）
  - 检查引擎适配器存在性（fofa、hunter、zoomeye、quake、shodan）
  - 检查 Go 版本一致性（go.mod 与 README.md）
  - 检查编译状态
  - 检查测试状态
  - 检查相关文档存在性（QUICKSTART.md、USAGE.md、README_LIGHT.md、PROJECT_SUMMARY.md）

#### 2. 发现并修复的问题
- **Go 版本格式**: README.md 中 "Go 1.24" 改为 "Go 1.24.0"，与 go.mod 一致

#### 3. 校验结果
```
bash scripts/verify_docs.sh
```

输出：
- 目录结构：12/12 通过
- 关键文件：3/3 通过
- 配置参数：4/4 通过
- 引擎适配器：5/5 通过
- Go 版本：一致 (1.24.0)
- 编译：通过
- 测试：通过
- 相关文档：4/4 通过

#### 验证结果
```
go build ./...
go test ./...
```

结果：通过。

---

## 2026-03-23 工作记录（一次性统一）

### GUI API-first 全路径统一（保留本地回退）

#### 1. Monitor 页
- **文件**: `cmd/unimap-gui/monitor_native.go`
- **内容**:
  - 设置基线改为 API-first：`POST /api/tamper/baseline`，失败自动回退本地 `Detector.BatchSetBaseline`
  - 基线列表改为 API-first：`GET /api/tamper/baseline/list`，失败回退本地 `Detector.ListBaselines`
  - 删除基线改为 API-first：`DELETE /api/tamper/baseline/delete?url=...`，失败回退本地 `Detector.DeleteBaseline`
  - 批量截图改为 API-first：`POST /api/screenshot/batch-urls`，失败回退本地 `ScreenshotMgr.CaptureBatchURLs`

#### 2. History 页
- **文件**: `cmd/unimap-gui/monitor_native.go`
- **内容**:
  - 历史列表改为 API-first：`GET /api/tamper/history`，失败回退本地 `TamperStorage.ListAllCheckRecords`
  - 删除历史改为 API-first：`DELETE /api/tamper/history/delete?url=...`，失败回退本地 `TamperStorage.DeleteCheckRecords`
  - 删除基线同样改为 API-first + 本地回退

#### 3. Screenshot 页
- **文件**: `cmd/unimap-gui/monitor_native.go`
- **内容**:
  - 批次列表改为 API-first：`GET /api/screenshot/batches`
  - 文件列表改为 API-first：`GET /api/screenshot/batches/files?batch=...`
  - 新增批次删除按钮：API-first `DELETE /api/screenshot/batches/delete?batch=...`
  - 新增文件删除按钮：API-first `DELETE /api/screenshot/file/delete?batch=...&file=...`
  - 上述操作均保留本地回退（`os.ReadDir`/`os.Remove`/`os.RemoveAll`）

### Web 端方法契约修正

#### 1. tamper baseline delete 方法对齐
- **文件**: `web/tamper_handlers.go`
- **内容**:
  - `handleTamperBaselineDelete` 从 `POST` 校验改为 `DELETE` 校验
  - 删除 URL 改为读取查询参数 `?url=...`，与当前路由及 API-first 客户端一致

### 验证结果
```
go test ./cmd/unimap-gui
go test ./web
go test -tags gui ./cmd/unimap-gui
go test ./...
```

结果：通过。

## 2026-03-23 工作记录

### 三端能力对照与改造启动（Web/GUI/CLI）

#### 1. 对照文档落地
- **文件**: `FEATURE_PARITY_PLAN_2026-03-23.md`
- **内容**:
  - 固化当前 Web/GUI/CLI 能力矩阵（Full/Partial/Missing）
  - 明确目标：Web 与 GUI 功能对齐，CLI 提供 API-first 易用路径
  - 给出最小改造分期：P0/P1/P2

#### 2. 本轮已启动改造（P0 第一批）
- **文件**: `web/router.go`, `web/tamper_handlers.go`, `web/screenshot_handlers.go`
- **内容**:
  - 新增篡改历史删除 API：`DELETE /api/tamper/history/delete?url={url}`
  - 新增截图批次管理 API：`GET /api/screenshot/batches`
  - 新增截图文件列表 API：`GET /api/screenshot/batches/files?batch={batch}`
  - 新增截图删除 API：
    - `DELETE /api/screenshot/batches/delete?batch={batch}`
    - `DELETE /api/screenshot/file/delete?batch={batch}&file={file}`
  - 增加批次/文件名校验与目录穿越防护

#### 3. CLI API-first（P0 第二批）
- **文件**: `cmd/unimap-cli/main.go`, `cmd/unimap-cli/api_subcommands.go`
- **内容**:
  - 新增 CLI API 子命令分发：`query`、`tamper-check`、`screenshot-batch`
  - `query` 对接 `POST /api/query`，支持 `-q/-e/-l/-o/--api-base`
  - `tamper-check` 对接 `POST /api/tamper/check`，支持 `--urls/--mode/--concurrency`
  - `screenshot-batch` 对接 `POST /api/screenshot/batch-urls`，支持 `--urls/--batch-id/--concurrency`
  - 保留原有直连引擎模式，确保兼容现有使用方式

#### 4. 回归测试补齐（P0 稳定性）
- **文件**: `cmd/unimap-cli/api_subcommands_test.go`, `web/screenshot_handlers_test.go`, `web/tamper_handlers_test.go`
- **内容**:
  - 新增 CLI API 子命令辅助函数测试（CSV 解析、HTTP 请求成功/失败路径）
  - 新增截图管理接口测试（批次/文件列表、删除与路径安全校验）
  - 新增篡改历史删除接口测试（缺参错误与删除成功路径）

#### 验证结果
```
go test ./cmd/unimap-cli ./web
go test ./...
```

结果：通过。

#### 5. GUI 首条 API 链路改造（tamper-check）
- **文件**: `cmd/unimap-gui/monitor_native.go`
- **内容**:
  - `篡改检测` 操作改为优先调用 Web API：`POST /api/tamper/check`
  - 新增 GUI 侧 API 基地址解析：优先 `UNIMAP_API_BASE`，默认 `http://127.0.0.1:8448`
  - 保留本地检测回退：当 API 不可用或调用失败时自动使用本地 `Detector.BatchCheckTampering`
  - 状态文案增加来源标注（API/本地），便于定位当前执行路径

#### 验证结果
```
go test ./...
go test -tags gui ./cmd/unimap-gui
```

结果：通过。

### M3 持续推进（query/tamper/screenshot 下沉）

#### 1. 新增应用服务层
- **文件**: `internal/service/query_app_service.go`, `internal/service/tamper_app_service.go`, `internal/service/screenshot_app_service.go`
- **内容**:
  - 新增 `QueryAppService`，封装引擎选择、统一查询执行、可选浏览器联动查询流程
  - 新增 `TamperAppService`，封装检测/基线的业务聚合与统计逻辑
  - 新增 `ScreenshotAppService`，封装单目标与批量截图的业务编排

#### 2. Web Handler 进一步瘦身
- **文件**: `web/server.go`, `web/query_handlers.go`, `web/tamper_handlers.go`, `web/screenshot_handlers.go`
- **内容**:
  - `Server` 注入 `queryApp` / `tamperApp` / `screenshotApp`
  - query/tamper/screenshot handler 从“业务执行”改为“协议层编排 + 调用应用服务”
  - 保留请求解析、参数校验、错误映射、响应编码职责

#### 3. 验证结果
```
go test ./...
```

结果：通过。

### 后续推进（用户选择 1、2）

#### 1. Query 浏览器联动响应收敛
- **文件**: `web/query_handlers.go`
- **内容**:
  - 新增统一响应拼装函数 `buildQueryAPIPayload`
  - 去除成功/失败路径中重复的 browser/capture 错误合并逻辑

#### 2. request_id 全链路（首版）
- **文件**: `internal/requestid/requestid.go`, `web/middleware_requestid.go`, `web/server.go`, `web/http_helpers.go`, `internal/logger/logger.go`, `internal/service/unified_service.go`, `internal/adapter/orchestrator.go`
- **内容**:
  - 新增 request_id 生成与 context 存取能力
  - Web 中间件注入 request_id，回写响应头 `X-Request-Id`
  - CORS 允许/暴露 request_id 头
  - logger 增加 `CtxInfof/CtxWarnf/CtxErrorf/CtxDebugf`，自动附带 rid 前缀
  - unified_service/orchestrator 搜索链路接入 context 日志
  - UnifiedService 查询调用改为 `SearchEnginesWithContext`，确保 rid 进入并发搜索路径

#### 验证结果
```
go test ./...
```

结果：通过。

### 文档状态收敛（本次）

- 同步更新待办任务状态（已完成/进行中/未开始）
- 同步更新核心文档中的客观指标（Go 文件数、server 行数）
- 统一 Phase B 与后续推进的完成描述，减少文档口径冲突

### Phase B 启动（第一批落地）

#### 1. CORS 白名单配置化
- **文件**: `internal/config/config.go`, `web/http_helpers.go`, `web/server.go`
- **内容**:
  - 新增 `web.cors` 配置段（`allowed_origins`、`allowed_methods`、`allowed_headers`、`exposed_headers`、`allow_credentials`、`max_age`）
  - WebSocket `CheckOrigin` 改为读取配置白名单
  - HTTP 层新增 CORS 中间件，统一处理预检请求与跨域头

#### 2. 请求体大小限制
- **文件**: `internal/config/config.go`, `web/http_helpers.go`, `web/server.go`, `web/monitor_handlers.go`
- **内容**:
  - 新增 `web.request_limits.max_body_bytes` 与 `web.request_limits.max_multipart_memory_bytes`
  - 启动时注入全局请求体大小限制中间件
  - 文件导入接口 `ParseMultipartForm` 改为读取配置项

#### 3. 限流参数配置化
- **文件**: `internal/config/config.go`, `web/middleware_ratelimit.go`, `web/server.go`
- **内容**:
  - 新增 `web.rate_limit.enabled`、`requests_per_window`、`window_seconds`
  - 服务启动时按配置启停限流并设置窗口参数
  - 优化客户端 IP 识别，避免 `RemoteAddr` 端口导致限流失效

#### 4. 错误码标准化（首批）
- **文件**: `web/http_helpers.go`, `web/middleware_ratelimit.go`, `web/tamper_handlers.go`, `web/screenshot_handlers.go`, `web/monitor_handlers.go`
- **内容**:
  - 新增统一 JSON 错误结构：`{ success:false, error:{ code,message,details } }`
  - 新增统一 JSON 请求体解析函数，覆盖超大请求、空体、多对象、非法 JSON 场景
  - 篡改检测、批量截图、可达性检测等 JSON 接口改用统一解析入口

### 验证结果
```
go test ./...
```

结果：通过。

### Phase B 第二批落地（1/2/3）

#### 1. API 错误标准化（补全）
- **文件**: `web/http_helpers.go`, `web/query_handlers.go`, `web/cookie_handlers.go`, `web/cdp_handlers.go`, `web/monitor_handlers.go`, `web/screenshot_handlers.go`, `web/tamper_handlers.go`, `web/websocket_handlers.go`
- **内容**:
  - 统一 API 方法校验与来源校验辅助函数（`requireMethod`、`requireTrustedRequest`）
  - 截图、篡改检测、CDP、Cookie、监控、WebSocket 握手拒绝等接口改为统一 JSON 错误结构
  - 全量移除 `web` 目录中 `http.Error` 的 API 错误返回路径

#### 2. Prometheus 指标接入
- **文件**: `internal/metrics/metrics.go`, `web/metrics.go`, `web/server.go`, `web/router.go`, `web/middleware_ratelimit.go`, `internal/service/unified_service.go`, `web/tamper_handlers.go`
- **内容**:
  - 新增 HTTP 请求总量、耗时、并发中、限流拒绝、查询耗时/状态、缓存命中、引擎错误、篡改检测状态指标
  - 新增 `/metrics` 暴露端点并接入全局 HTTP 指标中间件
  - 查询链路、限流、篡改结果写入指标

#### 3. Redis 缓存最小集成
- **文件**: `internal/config/config.go`, `internal/service/unified_service.go`, `configs/config.yaml`, `configs/config.yaml.example`
- **内容**:
  - 新增 `cache.backend` 与 `cache.redis` 配置结构（addr/password/db/prefix）
  - `UnifiedService` 按配置切换 `memory/redis` 后端并自动回退内存缓存
  - 同步更新配置示例与默认值/校验规则

### 第二批验证结果
```
go test ./...
```

结果：通过。

### Phase B 后续推进（M2 并发模型统一）

#### 1. tamper 批处理接入统一 workerpool
- **文件**: `internal/tamper/detector.go`
- **内容**:
  - `BatchCheckTampering` 从本地 `semaphore + goroutine` 改为复用 `internal/util/workerpool`
  - `BatchSetBaseline` 从本地 `semaphore + goroutine` 改为复用 `internal/util/workerpool`
  - 新增批处理任务结构体（check/baseline）与结果通道收集，保持原有返回结构不变

#### 2. 验证结果
```
go test ./internal/tamper
go test ./...
```

结果：通过。

### 继续推进（任务 1/2/3）

#### 1. 补充 tamper 批处理顺序一致性单测
- **文件**: `internal/tamper/detector.go`, `internal/tamper/detector_test.go`
- **内容**:
  - 新增有序收集辅助函数：`collectOrderedTamperCheckResults`、`collectOrderedTamperBaselineResults`
  - 新增对应单测，覆盖乱序输入与越界索引过滤，确保批处理结果按输入索引回填

#### 2. monitor 可达性检测并发统一到 workerpool
- **文件**: `internal/service/monitor_app_service.go`, `web/monitor_handlers.go`
- **内容**:
  - 新增 `MonitorAppService`，使用统一 `internal/util/workerpool` 执行 URL 可达性批处理任务
  - `handleURLReachability` 移除本地 `semaphore + goroutine`，改为调用应用服务

#### 3. 推进 M3（handler 业务下沉）
- **文件**: `internal/service/monitor_app_service.go`, `web/server.go`, `web/monitor_handlers.go`
- **内容**:
  - 将 URL 归一化、可达性探测、批量并发调度与统计汇总下沉到 Application Service 层
  - Web handler 仅保留协议层职责：请求解析、参数校验、调用服务、统一响应编码
  - `Server` 新增 `monitorApp` 成员并在初始化时注入

#### 验证结果
```
go test ./internal/tamper ./web ./internal/service
go test ./...
```

结果：通过。

## 2026-03-20 工作记录

### 已完成任务

#### 1. 仓库公开化
- 检查并确认敏感信息保护（`.gitignore` 正确配置）
- 将 GitHub 仓库从 private 设置为 public
- 仓库地址：https://github.com/ElaineRosa6/unimap-icp-hunter

#### 2. 统一路由注册
- **文件**: `web/router.go`
- **内容**:
  - 创建 `Route` 结构体，包含路由名称、方法、路径、处理器、限流标记
  - 创建 `Router` 管理器，统一注册所有路由
  - 路由按功能分类：页面路由、查询 API、Cookie 管理、CDP、WebSocket、截图、导入、篡改检测
  - 支持自动应用限流中间件

#### 3. API 限流保护
- **文件**: `web/middleware_ratelimit.go`
- **内容**:
  - 实现 `RateLimiter` 基于内存的滑动窗口限流器
  - 默认配置：每分钟 60 次请求
  - 支持客户端 IP 识别（X-Forwarded-For、X-Real-IP、RemoteAddr）
  - 自动清理过期记录
  - 对高频接口自动应用限流保护：
    - 查询：`/api/query`、`/query`、`/api/query/status`
    - 截图：`/api/screenshot/*`
    - 导入：`/api/import/urls`、`/api/url/reachability`
    - 篡改检测：`/api/tamper/check`、`/api/tamper/baseline`

#### 4. 测试覆盖扩展
- **文件**: `internal/adapter/orchestrator_test.go`
  - 新增表驱动测试：TranslateQuery、SetConcurrency、NormalizeResults
  - 新增并发安全测试、取消测试
  - 新增基准测试

- **文件**: `internal/tamper/detector_test.go`
  - 新增表驱动测试：篡改判定模式、片段变化检测、cleanHTML、SHA256、错误分类
  - 新增基准测试

- **文件**: `internal/service/unified_service_test.go`
  - 新增配置覆盖测试
  - 新增 Query/Export 验证测试
  - 新增适配器注册测试
  - 新增基准测试

### 测试结果
```
ok  github.com/unimap-icp-hunter/project/internal/adapter
ok  github.com/unimap-icp-hunter/project/internal/core/unimap
ok  github.com/unimap-icp-hunter/project/internal/service
ok  github.com/unimap-icp-hunter/project/internal/tamper
```

### 文件变更汇总
```
新增:
  web/router.go               - 统一路由注册
  web/middleware_ratelimit.go - 限流中间件

修改:
  web/server.go                           - 使用新路由系统
  internal/adapter/orchestrator_test.go   - 扩展测试
  internal/tamper/detector_test.go        - 扩展测试
  internal/service/unified_service_test.go - 扩展测试
```

---

## 待办事项优先级规划

### 高优先级（建议本周完成）

| 编号 | 状态 | 任务 | 说明 | 预估工作量 |
|------|------|------|------|------------|
| H1 | ✅ 已完成 | 完善 CORS 白名单配置 | 已完成配置化与中间件接入 | 2h |
| H2 | ✅ 已完成 | 添加请求大小限制 | 已完成全局 body 限制与 multipart 限制配置化 | 1h |
| H3 | ✅ 已完成 | 错误码标准化 | 已完成统一错误结构与主要 API 路径改造 | 2h |
| H4 | ✅ 已完成 | 限流配置化 | 已完成配置项接入与按配置启停 | 1h |

### 中优先级（建议 2 周内完成）

| 编号 | 状态 | 任务 | 说明 | 预估工作量 |
|------|------|------|------|------------|
| M1 | ✅ 已完成 | 缓存能力统一配置 | 已完成按引擎 TTL 配置，支持独立启用/禁用缓存 | 4h |
| M2 | ✅ 已完成 | 并发模型统一 | orchestrator 与 tamper 批处理均已接入统一 workerpool | 3h |
| M3 | ✅ 已完成 | Application Service 层 | 已完成 query/tamper/screenshot 业务下沉，handlers 仅保留协议层职责 | 8h |
| M4 | ✅ 已完成 | Redis 缓存支持 | 已完成连接池配置细化、健康检查机制、配置化支持 | 4h |

### 低优先级（持续改进）

| 编号 | 状态 | 任务 | 说明 | 预估工作量 |
|------|------|------|------|------------|
| L1 | ✅ 已完成 | Prometheus 指标 | 已完成分位细化、引擎细分指标、截图/WebSocket/资源指标 | 4h |
| L2 | ✅ 已完成 | request_id 日志贯穿 | 已完成关键链路Ctx日志方法改造，request_id贯穿请求全链路 | 2h |
| L3 | ✅ 已完成 | regexp 复用优化 | 已预编译 validation 和 monitor_handlers 中的正则表达式 | 1h |
| L4 | ✅ 已完成 | 文档自动化校验 | 已创建 verify_docs.sh 脚本，检查目录/配置/版本一致性 | 3h |

---

## 架构改进建议

### 短期（1-2 周）

```
当前状态                              目标状态
┌─────────────────┐                  ┌─────────────────┐
│   web/server.go │                  │   web/server.go │
│   (路由+处理)    │  ───────────►    │   (仅初始化)     │
└─────────────────┘                  └────────┬────────┘
                                              │
                                     ┌────────▼────────┐
                                     │  web/router.go  │
                                     │  (路由注册)      │
                                     └────────┬────────┘
                                              │
                    ┌─────────────────────────┼─────────────────────────┐
                    │                         │                         │
           ┌────────▼────────┐       ┌────────▼────────┐       ┌────────▼────────┐
           │ query_handlers  │       │screenshot_handlers│     │ tamper_handlers │
           └─────────────────┘       └─────────────────┘       └─────────────────┘
```

### 中期（3-4 周）

```
┌─────────────────────────────────────────────────────────────────┐
│                         Web Layer                                │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐            │
│  │ router  │  │middleware│  │handlers │  │websocket│            │
│  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘            │
└───────┼────────────┼────────────┼────────────┼──────────────────┘
        │            │            │            │
┌───────▼────────────▼────────────▼────────────▼──────────────────┐
│                      Application Service Layer                   │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │QueryAppService│  │TamperAppService│  │ScreenshotAppService│   │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
└─────────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────▼───────────────────────────────────┐
│                         Core Layer                               │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐                │
│  │ orchestrator│  │   merger   │  │   parser   │                │
│  └────────────┘  └────────────┘  └────────────┘                │
└─────────────────────────────────────────────────────────────────┘
```

---

## 质量指标

### 当前状态
| 指标 | 状态 | 说明 |
|------|------|------|
| 编译 | ✅ 通过 | `go build ./...` |
| 测试 | ✅ 通过 | `go test ./...` |
| 测试覆盖 | ⏳ 部分 | 4 个测试文件，约 70+ 测试用例 |
| 安全头 | ✅ 完成 | securityMiddleware |
| 限流 | ✅ 完成 | RateLimiter |
| CORS | ✅ 完成 | 已配置化并统一中间件处理 |

### 目标状态（下阶段）
| 指标 | 目标 |
|------|------|
| 测试覆盖率 | > 60% |
| API 响应时间 | P99 < 500ms |
| 错误码 | 统一格式 |
| 文档覆盖 | API 文档完整 |

---

## 相关文档

- [项目完整复核报告](PROJECT_FULL_REVIEW_2026-03-20.md)
- [安全修复总结](SECURITY_FIXES.md)
- [中优先级问题修复](MEDIUM_PRIORITY_FIXES.md)
- [架构改进说明](ARCHITECTURE_IMPROVEMENTS.md)

---

## 2026-03-25 验收结论摘要

### 执行内容
- 对修复清单与功能对齐项进行逐项验收（代码证据 + 自动化验证）。
- 生成独立验收记录文档，沉淀“通过 / 证据 / 残余风险”。

### 验证结果
- 编译：`go build ./...` 通过。
- 测试：`go test ./...` 通过。

### 结论
- 当前修复项在代码存在性和自动化验证维度均通过。
- 未发现阻断发布的回归信号。

### 风险与建议
- 建议发布前补一轮 Web 与 GUI 的端到端冒烟，覆盖网络环境、字体差异、真实 API 波动等环境因素。

### 相关记录
- [修复项验收记录（2026-03-25）](ACCEPTANCE_RECORD_2026-03-25.md)

---

## 2026-03-26 工作记录（续）

### 批量截图功能修复

#### 问题
- 批量截图返回 `screenshot_manager_unavailable` 错误

#### 原因
- `configs/config.yaml` 中 `screenshot.enabled: false`，导致截图管理器未初始化

#### 修复
- **文件**: `configs/config.yaml`
- **内容**: 将 `screenshot.enabled` 改为 `true`

#### 验证
```bash
curl -X POST http://localhost:8448/api/screenshot/batch-urls \
  -H "Content-Type: application/json" \
  -d '{"urls": ["https://www.baidu.com"]}'
```

结果：`{"success": 1, "failed": 0}`

---

### 篡改检测导出功能修复

#### 问题
- 设置基线或执行篡改检测后导出时，不可达的URL缺失

#### 原因
- `currentTamperResults` 只保存后端返回的结果，未包含预检中的不可达URL
- 设置基线流程没有显示结果列表，也未保存到 `currentTamperResults`

#### 修复
- **文件**: `web/templates/monitor.html`
- **内容**:
  - `renderTamperResults`: 将 `currentTamperResults` 改为保存合并后的 `merged` 数组
  - 设置基线流程: 调用 `renderBaselineResults` 显示所有URL状态
  - 新增 `renderBaselineResults` 函数: 显示基线设置结果并保存供导出使用

#### 验证
- 设置基线时输入可达和不可达URL，导出时两者均包含在结果中

---

### 文件整理与归档

#### 操作内容
- 历史文档归档至 `archive/` 目录
- 核心文档整理至 `docs_archive/` 目录
- 示例文件移动至 `examples/` 目录
- 更新 `.gitignore`：
  - 添加 `output/`、`.trae/`、`.venv/`、`archive/`
  - 添加 `hash_store/records/**/*.json` 但保留 `.gitkeep`

---

### 正则表达式预编译优化

#### 文件
- `internal/plugin/processors/validation.go`
- `web/monitor_handlers.go`

#### 内容
- 将 `regexp.MatchString()` 动态编译改为 `regexp.MustCompile` 预编译
- 提升匹配性能，避免重复编译开销

---

### 文档验证脚本

#### 文件
- `scripts/verify_docs.sh`

#### 内容
- 检查目录结构
- 验证文档文件存在性
- 检查配置参数完整性
- 验证引擎适配器
- 检查 Go 版本一致性

---

### 提交记录

| 提交 | 说明 |
|------|------|
| `c36b77f` | fix: 篡改检测导出功能包含不可达URL |
| `76ec33e` | 优化网站篡改检测系统，添加历史记录持久化和导出功能 |