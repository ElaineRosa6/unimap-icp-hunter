# 项目工作日志

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
| M1 | 🟡 进行中 | 缓存能力统一配置 | 已完成 cache 统一配置入口，按引擎 TTL/QPS 细化未完成 | 4h |
| M2 | ✅ 已完成 | 并发模型统一 | orchestrator 与 tamper 批处理均已接入统一 workerpool | 3h |
| M3 | 🟡 进行中 | Application Service 层 | monitor 可达性已下沉，query/tamper/screenshot 仍有下沉空间 | 8h |
| M4 | 🟡 部分完成 | Redis 缓存支持 | 已完成最小集成与回退，连接池细化与策略完善未完成 | 4h |

### 低优先级（持续改进）

| 编号 | 状态 | 任务 | 说明 | 预估工作量 |
|------|------|------|------|------------|
| L1 | 🟡 进行中 | Prometheus 指标 | 已接入基础指标，分位细化与更多业务指标仍待补充 | 4h |
| L2 | ⏳ 未开始 | request_id 日志贯穿 | 统一请求追踪 | 2h |
| L3 | ⏳ 未开始 | regexp 复用优化 | 篡改 HTML 清洗正则预编译 | 1h |
| L4 | ⏳ 未开始 | 文档自动化校验 | 脚本检查 README 与代码一致性 | 3h |

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