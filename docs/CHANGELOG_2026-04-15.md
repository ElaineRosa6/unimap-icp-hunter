# 变更日志 — 2026-04-15

> **分支**: `release/major-upgrade-vNEXT`
> **变更类型**: 测试修复闭环 + 覆盖率提升 + 未测试包覆盖
> **涉及模块**: workerpool、resourcepool、node handlers、middleware、query handlers、websocket handlers、proxypool、decoder、codequality、database、priority、threshold、circuitbreaker、degradation、requestid、memory、objectpool

---

## 一、竞态测试修复

### 1.1 workerpool 竞态（3 个测试）

**问题**: `MockTask.executed` 字段为普通 `bool`，在 `Execute()` 中被 worker goroutine 写入，在测试主 goroutine 中读取，`go test -race` 检测到 data race。

**修复**: `executed bool` → `executed atomic.Bool`，所有读写改为 `Load()`/`Store()`。

| 文件 | 变更 |
|------|------|
| `internal/utils/workerpool/workerpool_test.go` | `MockTask.executed` 改为 `atomic.Bool`，7 处读取/写入同步修改 |

**受影响测试**: `TestPoolSubmitTask`、`TestPoolSubmitMultipleTasks`、`TestPoolWait`

### 1.2 resourcepool 竞态（1 个测试）

**问题**: `PoolMetrics` 的 `TotalCreated`、`TotalAcquired` 等 `int64` 字段在多个 goroutine 并发 `++`，无原子保护。

**修复**: 所有 `p.metrics.XXX++` 改为 `atomic.AddInt64(&p.metrics.XXX, 1)`。

| 文件 | 变更 |
|------|------|
| `internal/utils/resourcepool/pool.go` | 13 处 `++` 改为 `atomic.AddInt64` |
| `internal/utils/resourcepool/pool_test.go` | 并发测试中 `t.Errorf` 移至主 goroutine，错误收集至 slice 后统一报告 |

**受影响测试**: `TestPool_ConcurrentAcquireRelease`

---

## 二、Handler 鉴权测试修复（3 个测试）

**问题**: 3 个 handler 测试未配置 `AdminToken` 和 `NodeAuthTokens`，导致 auth 校验返回 401。

**修复**:
- `TestNodeRegisterHeartbeatStatus` — 补充 `NodeAuthTokens["node-a"]` + `AdminToken` + `Authorization` header
- `TestNodeNetworkProfile` — 补充 `AdminToken` + `Authorization` header
- `TestNodeTaskFlow` — 补充 `NodeAuthTokens` + `AdminToken` + 各环节对应的 `Authorization` header

| 文件 | 变更 |
|------|------|
| `web/node_handlers_test.go` | 3 个测试补充 token 配置和请求头 |
| `web/node_task_handlers_test.go` | 1 个测试补充 token 配置和请求头 |

---

## 三、新增 Middleware 测试

### 3.1 adminAuthMiddleware

| 用例 | 覆盖场景 |
|------|---------|
| `TestAdminAuthMiddleware_MissingToken_Returns401` | 无 token 被拒绝 |
| `TestAdminAuthMiddleware_WrongToken_Returns401` | 错误 token 被拒绝 |
| `TestAdminAuthMiddleware_ValidHeader_Passes` | Header token 通过 |
| `TestAdminAuthMiddleware_ValidQuery_Passes` | Query param token 通过 |
| `TestAdminAuthMiddleware_PublicPath_SkipsAuth` | 公路径跳过鉴权 |
| `TestAdminAuthMiddleware_AuthDisabled_Passes` | Auth 禁用时行为 |
| `TestIsPublicPath` | 公路径判断表驱动测试 |
| `TestAdminToken` | token 读取逻辑（启用/禁用/nil config） |

### 3.2 requestIDMiddleware

| 用例 | 覆盖场景 |
|------|---------|
| `TestRequestIDMiddleware_GeneratesID` | 自动生成请求 ID |
| `TestRequestIDMiddleware_PreservesExistingID` | 保留已有请求 ID |
| `TestRequestIDMiddleware_SetsContext` | 请求 ID 写入上下文 |

### 3.3 限流配置

| 用例 | 覆盖场景 |
|------|---------|
| `TestSetRateLimitConfig` | 自定义限流配置生效 |
| `TestSetRateLimitConfig_InvalidValues` | 非法值回退默认 |
| `TestSetRateLimitEnabled` | 限流开关控制 |

---

## 四、新增 WebSocket 测试

| 用例 | 覆盖场景 |
|------|---------|
| `TestHandleWebSocket_ValidationFailure_Returns401` | 无 token 返回 401 |
| `TestHandleWebSocketQuery_WithEngines_SendsQueryStart` | 带引擎发送 query_start |
| `TestHandleWebSocketQuery_SendsQueryID` | 消息包含 query_id |
| `TestHandleWebSocketQuery_QueryIDTracked` | query_id 已注册到 queryStatus |
| `TestBroadcastMessage_WithConnections` | 广播消息无 panic |
| `TestUpdateQueryProgress_ExistingQuery_UpdatesState` | 已有查询进度更新 |
| `TestUpdateQueryProgress_NonExistentQuery_NoChange` | 不存在的查询无变化 |
| `TestValidateWebSocketRequest_NoToken_DevelopmentMode` | 开发模式允许无 token |
| `TestValidateWebSocketRequest_NoToken_ProductionMode` | 生产模式拒绝无 token |
| `TestHandleWebSocketQuery_PingPongMessages` | 空引擎返回 query_error |

**关键修复**: websocket 测试中 `messages` 切片在异步 goroutine 写入、测试主 goroutine 读取，已加 `sync.Mutex` 保护并快照读取，消除 data race。

---

## 五、新增 Query Handler 测试

| 用例 | 覆盖场景 |
|------|---------|
| `TestHandleQueryStatus_CompletedQuery_ReturnsFullStatus` | 完成状态的查询详情 |
| `TestHandleAPIQuery_WhitespaceQuery_Returns400` | 空白查询被拒绝 |
| `TestHandleAPIQuery_PageSizeParsing` | 无效 page_size 回退默认 |
| `TestHandleResults_EmptyQuery_RendersTemplate` | 空查询结果页面 |
| `TestHandleQuota_NoEngines_RendersEmptyTemplate` | 无引擎配额页面 |
| `TestParseEnginesParam_CombinedFormat` | 逗号分隔引擎解析 |
| `TestParseEnginesParam_SingleValue` | 单一引擎解析 |

---

## 九、未测试包覆盖（11 个包，198 个测试）

新增 11 个测试文件，覆盖此前完全无测试的包。

| 包路径 | 测试数 | 覆盖率 | 文件 |
|--------|--------|--------|------|
| `internal/proxypool` | 19 | 97.0% | `proxypool/pool_test.go` |
| `internal/tamper/decoder` | 22 | 92.2% | `tamper/decoder/decoder_test.go` |
| `internal/utils/codequality` | 16 | 78.5% | `utils/codequality/codequality_test.go` |
| `internal/tamper/database` | 26 | 77.9% | `tamper/database/database_test.go` |
| `internal/tamper/priority` | 20+ | 92.9% | `tamper/priority/rule_priority_test.go` |
| `internal/tamper/threshold` | 24+ | 99.2% | `tamper/threshold/dynamic_threshold_test.go` |
| `internal/utils/circuitbreaker` | 12 | 87.3% | `utils/circuitbreaker/circuit_breaker_test.go` |
| `internal/utils/degradation` | 13 | 94.0% | `utils/degradation/degradation_test.go` |
| `internal/requestid` | 16 | 100.0% | `requestid/requestid_test.go` |
| `internal/utils/memory` | 9 | 95.3% | `utils/memory/memory_monitor_test.go` |
| `internal/utils/objectpool` | 11 | 74.8% | `utils/objectpool/object_pool_test.go` |

**覆盖场景**:
- proxypool: 代理解析、轮询选择、失败冷却、直接回退
- decoder: Base64/Hex/Unicode/URL/HTML 解码、多步解码
- codequality: 复杂度分析、覆盖率解析、报告生成
- database: 规则/白名单仓库 CRUD、版本管理、批量操作
- priority: 规则管理、冲突检测、优先级排序
- threshold: 动态阈值、敏感度、规则权重
- circuitbreaker: 状态机转换、统计、并发
- degradation: 负载/错误率/响应时间降级策略
- requestid: ID 生成、base36 编码、上下文传递
- memory: 监控统计、历史限制、GC 触发
- objectpool: 对象池生命周期、关闭、超时

---

## 十、测试结果汇总

| 检查项 | 结果 |
|--------|------|
| `go build ./...` | 0 错误 |
| `go vet ./...` | 0 警告 |
| `go test -race ./...` | **31 packages, 0 failures, 0 races** |
| 本轮修复/新增测试 | 234 个 |

---

## 十一、变更文件清单

| 文件 | 类型 | 说明 |
|------|------|------|
| `internal/utils/workerpool/workerpool_test.go` | 修改 | atomic.Bool 修复 race |
| `internal/utils/resourcepool/pool.go` | 修改 | atomic 操作修复 race |
| `internal/utils/resourcepool/pool_test.go` | 修改 | 并发测试安全修复 |
| `web/node_handlers_test.go` | 修改 | 补充 token 配置 |
| `web/node_task_handlers_test.go` | 修改 | 补充 token 配置 |
| `web/middleware_auth_test.go` | 新增 | 8 个 auth 中间件测试 |
| `web/middleware_requestid_test.go` | 新增 | 3 个 requestID 中间件测试 |
| `web/middleware_ratelimit_test.go` | 修改 | 新增 3 个配置测试 |
| `web/websocket_handlers_test.go` | 新增 | 11 个 websocket 测试 |
| `web/query_handlers_test.go` | 新增 | 7 个 query handler 测试 |
| `internal/proxypool/pool_test.go` | 新增 | 19 个代理池测试 |
| `internal/tamper/decoder/decoder_test.go` | 新增 | 22 个解码器测试 |
| `internal/utils/codequality/codequality_test.go` | 新增 | 16 个代码质量分析测试 |
| `internal/tamper/database/database_test.go` | 新增 | 26 个数据库仓储测试 |
| `internal/tamper/priority/rule_priority_test.go` | 新增 | 20+ 个优先级规则测试 |
| `internal/tamper/threshold/dynamic_threshold_test.go` | 新增 | 24+ 个动态阈值测试 |
| `internal/utils/circuitbreaker/circuit_breaker_test.go` | 新增 | 12 个熔断器测试 |
| `internal/utils/degradation/degradation_test.go` | 新增 | 13 个降级策略测试 |
| `internal/requestid/requestid_test.go` | 新增 | 16 个请求 ID 测试 |
| `internal/utils/memory/memory_monitor_test.go` | 新增 | 9 个内存监控测试 |
| `internal/utils/objectpool/object_pool_test.go` | 新增 | 11 个对象池测试 |

---

**更新日期**: 2026-04-15
**提交**: `db514c2` (竞态修复) + `a4e6c6c` (测试补充) + 未测试包覆盖 (11 包)
**更新者**: Test Coverage Sprint
