# 变更日志 — 2026-04-10

> **分支**: `release/major-upgrade-vNEXT`
> **变更类型**: 缺陷修复 + 功能增强
> **涉及模块**: Web 前端、Cookie 管理、截图系统、Server 结构、插件系统、配置系统

---

## 一、新增功能

### 1.1 Cookie 登录状态检测与智能 UI

**问题背景**: Cookie 填写界面对 CDP/Extension 用户是冗余的 — 这两种模式都复用浏览器已登录会话，手动填写的 Cookie 仅在无头 Chrome 模式下才生效。但用户界面上 Cookie 输入框始终展开，造成混淆。

**实现方案**: 新增 `GET /api/cookies/login-status` API + 前端自动响应。

| 组件 | 变更 |
|------|------|
| `internal/screenshot/manager.go` | 新增 `CheckEngineLoginStatus()` — 打开引擎页面检测登录墙关键词；新增 `EngineLoginURL()` — 返回各引擎首页 URL |
| `web/cookie_handlers.go` | 新增 `handleCookieLoginStatus` — 返回 CDP 连接状态、Extension 配对状态、各引擎登录状态 |
| `web/router.go` | 注册新路由 |
| `web/templates/index.html` | Cookie 区域改为可折叠；新增 CDP/Extension 状态栏；新增各引擎登录状态指示器 + "点击登录"按钮 |
| `web/static/css/style.css` | 新增登录状态栏、引擎状态行、折叠按钮、展开动画样式 |
| `web/static/js/main.js` | 新增 `refreshLoginStatus()` 轮询（15s）、`updateEngineLoginStatus()` 动态更新、自动折叠/展开逻辑 |

**行为效果**:

| 场景 | UI 表现 |
|------|---------|
| CDP 已连接 + 引擎已登录 | 显示 "✓ 已登录"，Cookie 输入区折叠 |
| CDP 已连接 + 引擎未登录 | 显示 "需要登录" + "点击登录"按钮，Cookie 区展开 |
| Extension 已配对 + 引擎已登录 | 同上 |
| Extension 已配对 + 引擎未登录 | 通过 Bridge 打开页面检测，显示未登录 + 登录链接 |
| 无浏览器会话（headless） | 显示 "无浏览器会话"/"Cookie 已配置"，Cookie 区展开供填备用 Cookie |

### 1.2 Server 结构体拆分

**问题背景**: Server 结构体有 37 个字段，职责过多。

**实现方案**: 提取高内聚的子组件结构体，清理 dead code。

| 组件 | 变更 |
|------|------|
| `web/distributed_state.go` | 新建 — `DistributedState` 封装 `NodeRegistry` + `NodeTaskQueue` |
| `web/bridge_state.go` | 新建 — `BridgeState` 封装 bridge 相关字段 + 自带 mutex |
| `web/server.go` | 删除 dead code（`distributedEnabled`、`alertManager`）；将 6 个 bridge 字段替换为 `bridge *BridgeState`；将 2 个 distributed 字段替换为 `distributed *DistributedState`；字段从 37 降至 25 |
| `web/node_handlers.go` | `s.nodeRegistry` → `s.distributed.NodeRegistry` |
| `web/node_task_handlers.go` | `s.nodeTaskQueue` → `s.distributed.NodeTaskQueue` |
| `web/screenshot_bridge_handlers.go` | bridge 字段 → `s.bridge.*` |
| `web/cookie_handlers.go` | `s.bridgeService` → `s.bridge.Service` |
| 所有测试文件 | 更新 Server struct literal 使用新字段名 |

---

## 二、已修复缺陷

### P0 级（影响正确性）

| # | 缺陷 | 文件 | 修复方式 |
|---|------|------|----------|
| P0-1 | `string(rune(int))` 产生 Unicode 控制字符 | `validation.go` | 改用 `strconv.Itoa()` |
| P0-2 | Worker 池缩容 goroutine 泄漏 | `workerpool.go` | 添加 exit signal channel |
| P0-3 | Logger `asyncRunning` 数据竞争 | `logger.go` | 改为 `atomic.Bool` |

### P1 级（影响稳定性）

| # | 缺陷 | 文件 | 修复方式 |
|---|------|------|----------|
| P1-1 | 优雅关闭泄漏分布式 goroutine | `server.go` | Shutdown() 中补充 Stop() 调用 |
| P1-2 | 分页任务忽略 Context 取消 | `orchestrator.go` | 循环顶部检查 Done() |
| P1-3 | Config.Clone() 静默丢弃错误 | `config.go` | 返回 `*Config`（手写深拷贝），签名从 `(*Config, error)` 改为 `*Config` |
| P1-4 | 任务队列重试逻辑缺陷 | `task_queue.go` | 修复 Attempt 递增顺序 |
| P1-5 | 核心 handler 缺乏单元测试 | `web/*_test.go` | 补充 query/cookie/health 测试 |
| P1-6 | 告警通知通道未实现 | `alerting/` | WebhookChannel 已存在 |

### P2 级（技术债务）

| # | 缺陷 | 文件 | 修复方式 |
|---|------|------|----------|
| P2-1 | 输入安全函数质量不足 | `validation.go` | 改用 `bluemonday.StrictPolicy()` |
| P2-2 | 截图文件权限过于宽松 | `manager.go` | `0644` → `0600` |
| P2-3 | 分布式调度器 dead code | `node_task_handlers.go` | 接入调度器 |
| P2-4 | 插件系统无实际插件 | `plugin/example_engine_plugin.go` | 新增 ExampleEnginePlugin |
| P2-5 | CI 管道不完善 | `.github/workflows/ci.yml` | 新增全量测试+lint+security |
| P2-6 | Docker 容器安全 | `Dockerfile` | 非 root 用户 + pinned image + HEALTHCHECK |
| P2-7 | Config checksum 使用 MD5 | `watcher.go` | 改为 SHA-256 |
| P2-8 | 重试配置未接入 | `orchestrator.go` | 从配置读取 RetryAttempts |
| P2-9 | Server 结构体职责过多 | `server.go` | 拆分为 DistributedState + BridgeState |
| P2-10 | Config.Clone 使用 YAML 序列化 | `config.go` | 手写深拷贝 + 辅助函数 |
| P2-11 | CDP 跨平台路径发现不完善 | `cdp_handlers.go` | 统一 runtime.GOOS + 补充各平台路径 |
| P2-12 | CDP 远程调试地址硬编码 | `config.go` | 新增 ChromeRemoteDebugAddress 配置 |

---

## 三、架构增强

### ARC-1: ScreenshotRouter 双模式高可用

- **ScreenshotRouter**: 统一入口，自动路由 + 降级（`internal/screenshot/router.go`）
- **HealthChecker**: CDP（HTTP `/json/version`）和 Extension（BridgeService 状态）健康探针（`internal/screenshot/health.go`）
- **ExtensionProvider**: 实现 `Provider` 接口
- **Prometheus 指标**: `screenshot_mode_switches_total`、`screenshot_current_mode`、`screenshot_health_check_total`
- **单元测试**: 11 个测试覆盖路由/降级/探针/泄漏场景
- **向后兼容**: 现有 `engine: "cdp"` / `engine: "extension"` 配置无需修改

### ARC-2: CDP 跨平台完整支持

- macOS 路径: `/Applications/Google Chrome.app/...`, `/Applications/Chromium.app/...`
- Linux 路径: `/usr/bin/google-chrome`, `/usr/bin/chromium`, `/snap/bin/chromium`, 等
- Windows 路径: Program Files + 注册表查询
- 统一使用 `runtime.GOOS` 进行平台检测

### Cookie 登录状态智能管理（新增）

- 自动检测 CDP 连接和 Extension 配对状态
- 逐引擎检测登录状态（打开页面 → 检测登录墙 → 返回结果）
- 前端自动响应：已登录则折叠 Cookie 输入区，未登录则展开并显示登录链接
- 15 秒轮询 + 手动刷新

---

## 四、构建与测试状态

| 检查项 | 状态 |
|--------|------|
| `go build ./...` | ✅ 通过 |
| `go test ./...` | ✅ 全绿（0 失败） |
| `go vet ./...` | ✅ 无新增警告 |

### 新增测试文件

| 文件 | 覆盖内容 |
|------|----------|
| `internal/utils/validation_test.go` | SanitizeInput 8 个用例 |
| `internal/plugin/plugin_test.go` | 插件生命周期、注册器、管理器、管道（9 个用例） |
| `internal/config/config_test.go` | Config.Clone 深拷贝（5 个用例） |
| `internal/screenshot/router_test.go` | 路由模式切换、降级、探针、健康检查（11 个用例） |
| `web/node_handlers_test.go` | 节点注册、心跳、状态、鉴权（6 个用例） |
| `web/node_task_handlers_test.go` | 任务队列、Claim、Result、鉴权（4 个用例） |
| `web/query_cookie_helpers_test.go` | Query 状态、Health、Cookie 导入/保存（多个用例） |
| `web/screenshot_bridge_handlers_test.go` | Bridge Mock、签名、重放、Token 轮换（5 个用例） |

---

## 五、修改文件清单

### 新建文件（6 个）

| 文件 | 说明 |
|------|------|
| `web/distributed_state.go` | DistributedState 结构体 |
| `web/bridge_state.go` | BridgeState 结构体 |
| `internal/plugin/example_engine_plugin.go` | 示例 EnginePlugin |
| `internal/plugin/plugin_test.go` | 插件测试 |
| `internal/utils/validation_test.go` | 输入验证测试 |
| `.github/workflows/ci.yml` | 完整 CI 管道 |

### 修改文件（15+ 个）

| 文件 | 变更类型 |
|------|----------|
| `web/server.go` | 删除 dead code，拆分子组件 |
| `web/cookie_handlers.go` | 新增 handleCookieLoginStatus |
| `web/router.go` | 新增路由 |
| `web/node_handlers.go` | 字段引用更新 |
| `web/node_task_handlers.go` | 字段引用更新 |
| `web/screenshot_bridge_handlers.go` | 字段引用更新 |
| `web/templates/index.html` | Cookie 区域可折叠 + 登录状态 |
| `web/static/css/style.css` | 新增登录状态样式 |
| `web/static/js/main.js` | 登录状态轮询 + 自动折叠 |
| `internal/screenshot/manager.go` | 新增 CheckEngineLoginStatus + EngineLoginURL |
| `internal/screenshot/router.go` | ScreenshotRouter 双模式 |
| `internal/screenshot/health.go` | 健康探针 Override 支持 |
| `internal/screenshot/screenshot_handlers.go` | SSRF 防护 |
| `internal/config/config.go` | Clone() 手写深拷贝 |
| `internal/config/hot_update.go` | Clone 错误处理更新 |
| `internal/utils/validation.go` | bluemonday 替代正则 |
| `internal/adapter/orchestrator.go` | Context 取消 + 重试配置 |
| `internal/distributed/task_queue.go` | 重试逻辑修复 |
| `internal/logger/logger.go` | asyncRunning 改为 atomic.Bool |
| `Dockerfile` | 非 root 用户 + HEALTHCHECK |

---

**记录人**: Claude Code
**日期**: 2026-04-10
**分支**: `release/major-upgrade-vNEXT`
