# UniMap 安全重构实施指导

> **创建日期：** 2026-04-15
> **分支：** `release/major-upgrade-vNEXT`
> **状态：** 待执行
> **最后更新：** 2026-04-15

---

## 执行原则

**禁止直接生成最终代码。** 本计划的核心原则是"安全的执行切片"，每一步必须：
1. 向后兼容，不破坏现有 API
2. 通过防腐层隔离破坏性改动
3. 有明确的验收标准才能进入下一步

---

## 依赖关系图谱

```
步骤 1 (git commit)
    │
    ├──▶ 串行链 A: 步骤 2 → 步骤 4 → 步骤 5 → 步骤 6 → 步骤 7 → 步骤 8
    │         │            │
    │         └── 串行链 B: 步骤 3 ─┘
    │
    └──▶ 并行链 C: 步骤 9 (Runbook) ── 全程可并行
         并行链 D: 步骤 10 (Grafana) ── 全程可并行
```

**关键约束：**
- 步骤 3 不依赖步骤 2，二者可并行
- 步骤 4 和 5 在步骤 2、3 之后开始，可并行于 9、10
- 步骤 6 必须等待步骤 4+5 全部完成（所有 Runner 就绪后才能统一注册）
- 步骤 7 必须等待步骤 6（前端依赖 API 和新 Runner 类型）
- 步骤 8 必须等待步骤 6+7（端到端测试需要完整系统）

---

## 步骤切片

### 步骤 1：提交未加入文件到 git

**具体动作：** 将 `??` 标记的 20 个文件/目录加入 git，分 3 次 commit：
- commit 1: CI/CD 配置 (`.dockerignore`, `.github/workflows/ci.yml`, `.golangci.yml`)
- commit 2: 新功能模块 (`internal/alerting/`, `internal/auth/`, `internal/backup/`, `internal/config/hot_update.go`, `internal/config/watcher.go`)
- commit 3: 文档 (`docs/ARCHITECTURE.md`, `docs/DECISIONS/`, `docs/CHANGELOG_*.md`)

**目的：** 清理工作区，确保后续所有步骤基于已提交的干净状态。
**影响面：** 仅 git 历史记录，不改变任何运行时行为。
**验收标准：** `git status` 无 `??` 标记，`go build ./...` 通过。

---

### 步骤 2：P0-2 告警 Webhook 端到端验证

**依赖前提：** 步骤 1 验收通过（`internal/alerting/` 已提交）。
**具体动作：**
1. 在 `web/` 下创建 `alerting_e2e_test.go`
2. 使用 `httptest.NewServer` 启动 mock Webhook 接收端，捕获请求体
3. 构造篡改检测告警场景，调用 `alertManager.Send()` 触发 Webhook
4. 验证 mock server 收到的 HTTP 请求：URL、Content-Type、JSON body 结构
5. 验证告警去重逻辑（相同告警短时间不重复发送）

**目的：** 验证告警通知链路端到端畅通，满足生产发布条件。
**影响面：** 仅新增测试文件，不修改生产代码。
**验收标准：** 新增 ≥3 个测试用例全部通过，`go test -race ./web/ -run Alerting` 0 失败。

---

### 步骤 3：P0-3 分布式故障转移验证

**依赖前提：** 步骤 1 验收通过（`internal/distributed/` 已提交），`internal/distributed/scheduler.go` 已存在。
**具体动作：**
1. 分析现状：`web/node_task_handlers.go:handleNodeTaskClaim` 已使用调度器（`ClaimWithNode`），但**缺少节点宕机后的任务重新分配逻辑**
2. 在 `internal/distributed/task_queue.go` 中新增 `ReassignOrphanedTasks()` 方法：
   - 遍历 `ASSIGNED` 状态的任务
   - 检查关联节点是否超时未心跳（通过 `NodeRegistry` 查询）
   - 将超时任务标记回 `PENDING`，递增 `ReassignCount`
   - 超过 `MaxReassign` 次数的任务标记为 `FAILED`
3. 在 `internal/distributed/registry.go` 的心跳检查循环中，检测到节点变 offline 时调用 `ReassignOrphanedTasks()`
4. 编写集成测试：注册 2 节点 → 节点1领取任务 → 模拟节点1心跳超时 → 验证任务回到 PENDING → 节点2可领取

**目的：** 验证节点宕机后 60s 内任务自动重新分配。
**影响面：**
- 修改 `internal/distributed/task_queue.go`（新增 `ReassignOrphanedTasks` 方法，**向后兼容**，不改变现有方法签名）
- 修改 `internal/distributed/registry.go`（在心跳检查循环中新增调用，**不改变** `Registry` 的公开 API）
- 不修改任何 handler 代码，不影响现有 API 端点

**防腐层设计：**
- `ReassignOrphanedTasks` 是新增方法，现有代码不调用它就不会触发任何行为
- 只有当 `Registry` 的心跳检测发现节点 offline 时才调用，不会误伤正常任务
- 任务状态流转严格遵循 `ASSIGNED → PENDING → ASSIGNED` 或 `ASSIGNED → FAILED`，不引入新状态

**验收标准：** 新增 ≥3 个测试用例，故障转移触发后任务状态从 `ASSIGNED` 正确流转回 `PENDING`，`go test -race ./internal/distributed/` 0 失败。

---

### 步骤 4：新增 Runner ST-09~ST-16（中优先级，8个）

**依赖前提：** 步骤 1 验收通过。与步骤 2、3 **并行**。
**具体动作：** 在 `internal/scheduler/executor.go` 末尾追加以下 Runner 结构和构造函数，每个 Runner 实现 `TaskHandler` 接口：

| Runner | 复用服务 | 关键依赖 |
|--------|---------|---------|
| ST-09 ExportRunner | `queryApp` 的导出能力 | 需在 `query_app_service.go` 确认/新增导出方法 |
| ST-10 PortScanRunner | `monitorSvc.ScanPorts()` | 已有 `CheckURLReachability`，需确认端口扫描方法 |
| ST-11 ScreenshotCleanupRunner | 文件系统操作 | 遍历截图目录，删除过期文件 |
| ST-12 TamperCleanupRunner | `tamperApp.CleanupHistory()` | 需在 `tamper_app_service.go` 确认/新增清理方法 |
| ST-13 QuotaMonitorRunner | `orchestrator.GetQuota()` | 已有配额查询 |
| ST-14 AlertSummaryRunner | `alertManager.GetAlerts()` | 步骤 2 验证的 alerting 模块 |
| ST-15 BaselineRefreshRunner | `tamperApp.SetBaseline()` | 已有基线设置 |
| ST-16 URLImportRunner | 文件读取 + URL 解析 | 已有 `handleImportURLs` |

**防腐层设计：**
- 每个 Runner 的服务依赖通过构造函数注入（`NewExportRunner(querySvc *QueryAppService)`），若传入 `nil` 则在 `Execute()` 时返回明确错误，不会 panic
- 对于尚不存在的服务方法，先在对应 service 中添加 **桩方法**（返回 `"not implemented"` 错误），确保编译通过后再逐步实现
- 桩方法与 Runner 在同一 PR 中，Runner 调用桩方法时不会 crash，只是返回错误

**目的：** 补充中优先级定时任务 Runner。
**影响面：** 仅在 `internal/scheduler/executor.go` 追加代码，**不修改**已有的 8 个 Runner，**不改变** `Scheduler` 的公开 API。
**验收标准：** 8 个 Runner 编译通过，每个 Runner 至少 1 个单元测试验证 nil 依赖和正常路径。

---

### 步骤 5：新增 Runner ST-17~ST-20（低优先级，4个）

**依赖前提：** 步骤 3 验收通过（ST-18 BridgeTokenRotate 需要 Bridge 故障转移就绪）。步骤 4 完成后开始。
**具体动作：** 同步骤 4 模式，在 `executor.go` 追加 4 个 Runner：

| Runner | 复用服务 | 关键依赖 |
|--------|---------|---------|
| ST-17 PluginHealthRunner | `service.HealthCheck()` | 已有 |
| ST-18 BridgeTokenRotateRunner | `bridge.Service.RotateToken()` | 依赖步骤 3 Bridge 稳定性 |
| ST-19 AlertSilenceRunner | `alertManager.Silence()` | 步骤 2 验证的 alerting |
| ST-20 CacheWarmupRunner | `cache.Get()/Set()` | 已有缓存模块 |

**目的：** 补充低优先级定时任务 Runner。
**影响面：** 同步骤 4，纯追加。
**验收标准：** 4 个 Runner 编译通过，每个至少 1 个单元测试。

---

### 步骤 6：注册新 Runner 到调度器并绑定到 Server

**依赖前提：** 步骤 4 + 5 全部验收通过。
**具体动作：** 修改 `web/server.go` 的 `NewServer()` 函数（当前行 295-302），在已有的 8 个 `RegisterHandler` 调用后追加 12 个：

```go
// 现有 8 个（不变）
sched.RegisterHandler(scheduler.NewQueryRunner(srv.queryApp))
sched.RegisterHandler(scheduler.NewSearchScreenshotRunner(screenshotApp, screenshotMgr))
sched.RegisterHandler(scheduler.NewBatchScreenshotRunner(screenshotApp, screenshotMgr))
sched.RegisterHandler(scheduler.NewTamperCheckRunner(srv.tamperApp, nil))
sched.RegisterHandler(scheduler.NewURLReachabilityRunner(srv.monitorApp))
sched.RegisterHandler(scheduler.NewCookieVerifyRunner(screenshotApp, screenshotMgr))
sched.RegisterHandler(scheduler.NewLoginStatusCheckRunner(screenshotMgr))
sched.RegisterHandler(scheduler.NewDistributedSubmitRunner(nodeTaskQueue))

// 新增 12 个（追加）
sched.RegisterHandler(scheduler.NewExportRunner(srv.queryApp))
sched.RegisterHandler(scheduler.NewPortScanRunner(srv.monitorApp))
sched.RegisterHandler(scheduler.NewScreenshotCleanupRunner(screenshotMgr))
sched.RegisterHandler(scheduler.NewTamperCleanupRunner(srv.tamperApp))
sched.RegisterHandler(scheduler.NewQuotaMonitorRunner(srv.orchestrator))
sched.RegisterHandler(scheduler.NewAlertSummaryRunner())
sched.RegisterHandler(scheduler.NewBaselineRefreshRunner(srv.tamperApp))
sched.RegisterHandler(scheduler.NewURLImportRunner())
sched.RegisterHandler(scheduler.NewPluginHealthRunner(srv.service))
sched.RegisterHandler(scheduler.NewBridgeTokenRotateRunner(srv.bridge.Service))
sched.RegisterHandler(scheduler.NewAlertSilenceRunner())
sched.RegisterHandler(scheduler.NewCacheWarmupRunner())
```

同时在 `internal/scheduler/scheduler.go` 追加 TaskType 常量（行 20-28 后）：

```go
const (
    // 已有 8 个（不变）
    TaskQuery             TaskType = "query"
    // ...
    TaskDistributedSubmit TaskType = "distributed_submit"

    // 新增 12 个
    TaskExport            TaskType = "export"
    TaskPortScan          TaskType = "port_scan"
    TaskScreenshotCleanup TaskType = "screenshot_cleanup"
    TaskTamperCleanup     TaskType = "tamper_cleanup"
    TaskQuotaMonitor      TaskType = "quota_monitor"
    TaskAlertSummary      TaskType = "alert_summary"
    TaskBaselineRefresh   TaskType = "baseline_refresh"
    TaskURLImport         TaskType = "url_import"
    TaskPluginHealth      TaskType = "plugin_health"
    TaskBridgeTokenRotate TaskType = "bridge_token_rotate"
    TaskAlertSilence      TaskType = "alert_silence"
    TaskCacheWarmup       TaskType = "cache_warmup"
)
```

**防腐层设计（关键）：**
- **先不改 `router.go`**，现有 9 个 scheduler API 路由（`/api/scheduler/tasks/*`）保持不变
- 新 Runner 注册后即可通过已有的 `/api/scheduler/tasks/create` 端点创建定时任务，**不新增 API 端点**
- 新增的 Runner 类型仅在 `AllTaskTypes()` 和 `TaskTypeLabel()` 中注册常量和 label

**目的：** 将 12 个新 Runner 接入运行时。
**影响面：**
- 修改 `web/server.go`（追加 12 行 RegisterHandler）
- 修改 `internal/scheduler/scheduler.go`（追加 12 个 TaskType 常量和 label）
- 修改 `internal/scheduler/scheduler.go` 的 `AllTaskTypes()` 和 `TaskTypeLabel()` 映射

**验收标准：** `go build ./...` 通过，启动日志显示 20 个 handler 已注册，`GET /api/scheduler/tasks` 返回空列表（无任务时）。

---

### 步骤 7：定时任务前端页面

**依赖前提：** 步骤 6 验收通过（所有 Runner 已注册，API 可用）。
**具体动作：**
1. 创建 `web/templates/scheduler.html` — 定时任务管理页面（任务列表、创建表单、执行历史）
2. 创建 `web/static/js/scheduler.js` — 前端交互逻辑
3. 在 `web/server.go` 的 `handleSchedulerPage()` 中渲染模板（当前是桩方法）
4. 路由 `/scheduler` 已在 `router.go:39` 注册，**无需修改路由**

**防腐层设计：**
- 前端页面独立文件，**不修改** `index.html` 或其他已有页面
- JS 调用全部走已有的 `/api/scheduler/*` 端点，**不新增 API**
- 如果某个 Runner 类型的前端表单尚未实现，在创建页面中隐藏该类型的选项（通过 `AllTaskTypes()` 返回的列表控制可见性）

**目的：** 提供定时任务可视化创建、启停、查看历史的能力。
**影响面：** 新增 2 个前端文件，修改 1 个 handler（`handleSchedulerPage` 实现）。
**验收标准：** 访问 `/scheduler` 页面正常渲染，可创建/启停/查看任务，页面包含 20 种任务类型的选择。

---

### 步骤 8：定时任务端到端测试

**依赖前提：** 步骤 6 + 7 全部验收通过。
**具体动作：**
1. 创建 `internal/scheduler/e2e_test.go`
2. 测试完整调度周期：创建任务 → 等待 cron 触发 → 验证执行记录写入 history
3. 测试启停控制：创建任务 → 禁用 → 验证不触发 → 启用 → 验证恢复触发
4. 测试持久化：创建任务 → Stop → NewScheduler + Load → 验证任务恢复
5. 测试 RunTaskNow：创建任务 → 立即执行 → 验证执行记录

**目的：** 验证定时任务系统端到端正确性。
**影响面：** 仅新增测试文件。
**验收标准：** `go test -race ./internal/scheduler/` 0 失败，包含 ≥5 个测试用例。

---

### 步骤 9：P2-1 运维 Runbook

**依赖前提：** 无代码依赖，可与步骤 2-8 **并行**。
**具体动作：** 创建 `docs/RUNBOOK.md`，覆盖 6 个场景的诊断和恢复步骤：

| 场景 | 诊断步骤 | 恢复操作 |
|------|---------|---------|
| Chrome 崩溃 | 检查进程、查看崩溃日志 | 自动重启、CDP 重连 |
| Bridge 断连 | 检查 WebSocket 状态、token 有效性 | 令牌轮换、重连 |
| Cookie 失效 | 逐引擎检测登录状态 | 重新导入 Cookie |
| 节点失联 | 检查心跳、网络连通性 | 故障转移、节点重启 |
| 磁盘满 | 检查截图/日志/快照目录 | 清理过期文件、扩容 |
| Redis 不可用 | 检查连接状态 | 降级到内存缓存 |

**目的：** 提供运维人员可操作的故障处理手册。
**影响面：** 仅新增文档。
**验收标准：** 文档完成，每个场景包含：症状、诊断命令、恢复步骤、预防建议。

---

### 步骤 10：P2-2 Grafana 监控面板

**依赖前提：** 步骤 9 完成（先有 Runbook 明确监控场景）。可与步骤 2-8 **并行**。
**具体动作：** 创建 `docs/grafana-dashboard.json`，包含 7 个面板：

| 面板 | 指标 | 告警阈值 |
|------|------|---------|
| 查询延迟 | P50/P95/P99 延迟 | P95 > 30s |
| 缓存命中率 | hit/(hit+miss) | < 50% |
| 截图成功率 | success/(success+fail) | < 90% |
| 篡改检测率 | detected/total | 异常波动 |
| 节点健康度 | 在线/总数 | < 80% |
| Goroutine 数 | runtime.NumGoroutine | > 1000 |
| 内存使用 | heap_alloc | > 80% |

**目的：** 提供可视化监控面板，可导入 Grafana 使用。
**影响面：** 仅新增文件。
**验收标准：** JSON 可导入 Grafana，7 个面板数据源指向现有 Prometheus 指标。

---

## 并行执行矩阵

| 时间段 | 串行链 A | 串行链 B | 并行链 |
|--------|---------|---------|--------|
| 第 1 天 | 步骤 1：git commit | | |
| 第 2-3 天 | 步骤 2：P0-2 Webhook e2e | 步骤 3：P0-3 分布式故障转移 | |
| 第 4-8 天 | 步骤 4：Runner ST-09~ST-16 | 步骤 5：Runner ST-17~ST-20 | 步骤 9：Runbook |
| 第 9 天 | 步骤 6：注册 Runner 到 Server | | 步骤 10：Grafana |
| 第 10 天 | 步骤 7：前端页面 | | |
| 第 11 天 | 步骤 8：端到端测试 | | |

---

## 关键风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| 步骤 4/5 中服务方法不存在 | 编译失败 | 先写桩方法返回 `not implemented`，确保编译通过 |
| 步骤 3 修改 distributed 模块 | 可能影响节点领取任务的竞态 | 仅在心跳检测路径新增调用，不修改 `ClaimWithNode` 逻辑 |
| 步骤 6 注册 12 个新 Runner | Server 启动时间增加 | Runner 构造函数中 nil 检查，不阻塞启动 |
| 步骤 7 前端页面 | HTML 模板语法错误 | 先验证模板解析：`go test -run SchedulerPage` |

---

**文档版本：** v1.0
**维护者：** UniMap Team
