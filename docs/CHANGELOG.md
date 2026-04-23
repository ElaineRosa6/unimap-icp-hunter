# UniMap 变更日志

---

## [2026-04-15] 测试修复闭环 + 覆盖率提升 + 未测试包覆盖

> **分支**: `release/major-upgrade-vNEXT`
> **变更类型**: 测试修复闭环 + 覆盖率提升 + 未测试包覆盖
> **涉及模块**: workerpool、resourcepool、node handlers、middleware、query handlers、websocket handlers、proxypool、decoder、codequality、database、priority、threshold、circuitbreaker、degradation、requestid、memory、objectpool

### 一、竞态测试修复

#### 1.1 workerpool 竞态（3 个测试）

**问题**: `MockTask.executed` 字段为普通 `bool`，在 `Execute()` 中被 worker goroutine 写入，在测试主 goroutine 中读取，`go test -race` 检测到 data race。

**修复**: `executed bool` → `executed atomic.Bool`，所有读写改为 `Load()`/`Store()`。

**受影响测试**: `TestPoolSubmitTask`、`TestPoolSubmitMultipleTasks`、`TestPoolWait`

#### 1.2 resourcepool 竞态（1 个测试）

**问题**: `PoolMetrics` 的 `TotalCreated`、`TotalAcquired` 等 `int64` 字段在多个 goroutine 并发 `++`，无原子保护。

**修复**: 所有 `p.metrics.XXX++` 改为 `atomic.AddInt64(&p.metrics.XXX, 1)`。

**受影响测试**: `TestPool_ConcurrentAcquireRelease`

### 二、Handler 鉴权测试修复（3 个测试）

**问题**: 3 个 handler 测试未配置 `AdminToken` 和 `NodeAuthTokens`，导致 auth 校验返回 401。

**修复**:
- `TestNodeRegisterHeartbeatStatus` — 补充 `NodeAuthTokens["node-a"]` + `AdminToken` + `Authorization` header
- `TestNodeNetworkProfile` — 补充 `AdminToken` + `Authorization` header
- `TestNodeTaskFlow` — 补充 `NodeAuthTokens` + `AdminToken` + 各环节对应的 `Authorization` header

### 三、新增 Middleware/WebSocket/Query 测试

- Middleware: 8 个 auth + 3 个 requestID + 3 个限流配置测试
- WebSocket: 11 个测试覆盖验证、消息广播、进度更新、模式切换
- Query Handler: 7 个测试覆盖状态查询、空白验证、分页解析

### 四、未测试包覆盖（11 个包，198 个测试）

| 包路径 | 测试数 | 覆盖率 |
|--------|--------|--------|
| `internal/proxypool` | 19 | 97.0% |
| `internal/tamper/decoder` | 22 | 92.2% |
| `internal/utils/codequality` | 16 | 78.5% |
| `internal/tamper/database` | 26 | 77.9% |
| `internal/tamper/priority` | 20+ | 92.9% |
| `internal/tamper/threshold` | 24+ | 99.2% |
| `internal/utils/circuitbreaker` | 12 | 87.3% |
| `internal/utils/degradation` | 13 | 94.0% |
| `internal/requestid` | 16 | 100.0% |
| `internal/utils/memory` | 9 | 95.3% |
| `internal/utils/objectpool` | 11 | 74.8% |

### 五、测试结果汇总

| 检查项 | 结果 |
|--------|------|
| `go build ./...` | 0 错误 |
| `go vet ./...` | 0 警告 |
| `go test -race ./...` | **31 packages, 0 failures, 0 races** |
| 本轮修复/新增测试 | 234 个 |

---

## [2026-04-14] 生产就绪 Phase 2/3 功能增强

> **分支**: `release/major-upgrade-vNEXT`
> **变更类型**: 生产就绪 Phase 2/3 功能增强
> **涉及模块**: 限流中间件、备份系统、审计中间件、健康检查、熔断器、日志告警、CI/CD

### 一、速率限制中间件增强

- **固定窗口 → 滑动窗口**: 窗口边界突发问题修复
- **X-RateLimit-* 响应头**: Limit/Remaining/Reset/Retry-After
- **测试覆盖**: 9 个用例覆盖滑动窗口、并发安全、响应头验证

### 二、数据备份模块

- `internal/backup/backup.go`: tar.gz 归档、多源备份、保留策略
- API 端点: `/api/backup/create`、`/api/backup/list`
- 配置: `backup.enabled`、`output_dir`、`max_backups`、`sources`
- 测试覆盖: 17 个用例

### 三、负载测试脚本

**新建 `scripts/load_test.sh`**:
- 纯 bash/curl 实现，无外部依赖
- 测试 6 个关键端点：健康检查、查询、截图、篡改检测、导入、就绪
- 统计 RPS、成功率、p50/p99/max 延迟、429 限流次数

### 四、CI/CD Docker 构建

- GitHub Container Registry (ghcr.io) 推送镜像
- 支持 SHA、分支名、semver、latest 四种标签
- 仅在 `main`/`master` 分支 push 时触发

---

## [2026-04-13] 跨平台适配增强 + 缺陷修复

> **分支**: `release/major-upgrade-vNEXT`
> **变更类型**: 缺陷修复 + 跨平台适配增强
> **涉及模块**: Web 前端、定时任务、截图管理器、信号处理、CI/CD、Docker

### 一、缺陷修复

#### 1.1 页面打开自动执行示例查询检查

移除 `initLoginStatusPoll()` 立即调用，仅保留 15 秒间隔轮询。

#### 1.2 定时任务页面渲染错误

模板改用 Go 内置 `{{index $.TaskTypeLabels .}}` 进行 map 查找。

### 二、跨平台适配增强

- **Dockerfile Go 版本对齐**: `golang:1.23-alpine` → `golang:1.26-alpine`
- **信号处理跨平台兼容**: `SIGHUP` 仅在 Unix 平台注册
- **Chrome 路径检测统一**: `runtime.GOOS` 判断，补全 Edge/beta/snap 路径
- **CI 增加 macOS 覆盖**: build/test/lint 加入 `macos-latest` matrix

### 三、服务器部署检查结果

- 容器端口与程序默认端口不一致（Dockerfile 8080 vs 配置 8448）
- 分布式管理接口默认无 token 保护
- CORS 默认仅面向本机

---

## [2026-04-10] 大规模缺陷修复 + 架构增强

> **分支**: `release/major-upgrade-vNEXT`
> **变更类型**: 缺陷修复 + 功能增强
> **涉及模块**: Web 前端、Cookie 管理、截图系统、Server 结构、插件系统、配置系统

### 一、新增功能

#### 1.1 Cookie 登录状态检测与智能 UI

新增 `GET /api/cookies/login-status` API + 前端自动响应：
- CDP/Extension 状态栏 + 各引擎登录状态指示器
- 自动折叠/展开逻辑（15 秒轮询 + 手动刷新）

#### 1.2 Server 结构体拆分

- `DistributedState` 封装 `NodeRegistry` + `NodeTaskQueue`
- `BridgeState` 封装 bridge 相关字段 + 自带 mutex
- Server 字段从 37 降至 25

### 二、已修复缺陷

#### P0 级（影响正确性）

| # | 缺陷 | 修复方式 |
|---|------|----------|
| P0-1 | `string(rune(int))` Unicode 控制字符 | 改用 `strconv.Itoa()` |
| P0-2 | Worker 池缩容 goroutine 泄漏 | 添加 exit signal channel |
| P0-3 | Logger `asyncRunning` 数据竞争 | 改为 `atomic.Bool` |

#### P1 级（影响稳定性）

| # | 缺陷 | 修复方式 |
|---|------|----------|
| P1-1 | 优雅关闭泄漏分布式 goroutine | Shutdown() 中补充 Stop() |
| P1-2 | 分页任务忽略 Context 取消 | 循环顶部检查 Done() |
| P1-3 | Config.Clone() 静默丢弃错误 | 手写深拷贝 |
| P1-4 | 任务队列重试逻辑缺陷 | 修复 Attempt 递增顺序 |
| P1-5 | 核心 handler 缺乏单元测试 | 补充 query/cookie/health 测试 |
| P1-6 | 告警通知通道未实现 | WebhookChannel 已存在 |

#### P2 级（技术债务）

- 输入安全函数改用 `bluemonday.StrictPolicy()`
- 截图文件权限 `0644` → `0600`
- 分布式调度器接入、插件示例实现
- CI 管道完善、Docker 容器安全
- Config checksum 改为 SHA-256、重试配置接入

### 三、架构增强

#### ARC-1: ScreenshotRouter 双模式高可用

- ScreenshotRouter: 统一入口，自动路由 + 降级
- HealthChecker: CDP/Extension 健康探针
- Prometheus 指标: `screenshot_mode_switches_total` 等
- 单元测试: 11 个覆盖路由/降级/探针

#### ARC-2: CDP 跨平台完整支持

- macOS/Linux/Windows 路径统一
- 使用 `runtime.GOOS` 进行平台检测

### 四、构建与测试状态

| 检查项 | 状态 |
|--------|------|
| `go build ./...` | 通过 |
| `go test ./...` | 全绿（0 失败） |
| `go vet ./...` | 无新增警告 |

---

## 历史版本

### [v2.1.5] 篡改检测优化

- 篡改检测系统优化
- 数据库优化
- 日志系统优化
- 性能优化

### [v1.0.0] 初始版本

- 多引擎搜索功能
- 网页截图功能
- 篡改检测功能

---

**记录人**: Claude Code
**项目分支**: `release/major-upgrade-vNEXT`