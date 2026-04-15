# UniMap 剩余工作计划

> **创建日期：** 2026-04-15
> **分支：** `release/major-upgrade-vNEXT`
> **状态：** ✅ 全部完成（2026-04-16）
> **最后更新：** 2026-04-15

---

## 现状

当前项目核心功能完成约 **98%**，33 个测试包通过 `-race` 检测，所有 P0/P1/P2 项均已清零。

---

## Phase 0：提交未加入文件（第1天）

将 20 个 `??` 标记的新文件/目录加入 git 并提交，确保工作区干净。

| # | 文件/目录 | 说明 |
|---|----------|------|
| 1 | `.dockerignore` | Docker 构建排除规则 |
| 2 | `.github/workflows/ci.yml` | CI 管道 |
| 3 | `.golangci.yml` | linter 配置 |
| 4 | `internal/alerting/` | 告警系统 |
| 5 | `internal/auth/` | 认证模块 |
| 6 | `internal/backup/` | 备份系统 |
| 7 | `internal/config/hot_update.go` | 配置热更新 |
| 8 | `internal/config/watcher.go` | 文件监控 |
| 9 | `docs/ARCHITECTURE.md` | 架构文档 |
| 10 | `docs/DECISIONS/` | 架构决策记录 |
| 11-14 | `docs/CHANGELOG_*.md` | 更新日志 |

**验收标准：** `git status` 无 `??` 标记的新文件，所有新增文件有对应 commit。

---

## Phase 1：P0 验证（第2-4天）

### P0-2：告警 Webhook 端到端验证

编写 e2e 测试覆盖告警通知链路：
1. 创建 mock Webhook server（`httptest.NewServer`）
2. 触发篡改检测告警
3. 验证 HTTP 请求体格式、URL、状态码
4. 验证告警去重和静默逻辑

**涉及文件：** `internal/alerting/`, `web/*_test.go`
**验收标准：** 新增 ≥3 个测试用例，Webhook 请求体断言通过。

### P0-3：分布式故障转移验证

在 `node_task_handlers.go` 中接入调度器，验证节点宕机后任务重新分配：
1. 编写测试：注册 2 个节点 → 提交任务 → 模拟节点1宕机
2. 验证调度器将任务重新分配给节点2
3. 验证任务状态正确流转（PENDING → RUNNING → COMPLETED）

**涉及文件：** `internal/distributed/`, `web/node_task_handlers.go`
**验收标准：** 新增 ≥3 个测试用例，故障转移在 60s 内完成。

---

## Phase 2：定时任务系统（第5-15天）

### 2.1 基础设施（第5-7天）

搭建定时任务调度框架：

| 任务 | 文件 | 说明 |
|------|------|------|
| 引入 cron 库 | `go.mod` | `github.com/robfig/cron/v3` |
| 调度器核心 | `internal/scheduler/scheduler.go` | Cron 解析、任务注册、生命周期管理 |
| 任务注册表 | `internal/scheduler/registry.go` | 任务 CRUD、启停控制、持久化 |
| 执行器基座 | `internal/scheduler/executor.go` | 超时控制、互斥执行、重试策略 |
| 任务执行历史 | `internal/scheduler/history.go` | 执行记录存储、查询、清理 |

**验收标准：** `go build ./...` 通过，基础 API 有单元测试。

### 2.2 高优先级任务 Runner（第8-11天）

| ID | Runner | 复用现有代码 | 说明 |
|----|--------|------------|------|
| ST-01 | QueryRunner | `internal/service/unified_service.go` | 定时 UQL 查询 |
| ST-02 | ScreenshotRunner | `internal/screenshot/` | 定时搜索引擎截图 |
| ST-03 | BatchScreenshotRunner | `internal/screenshot/manager.go` | 定时批量截图 |
| ST-04 | TamperCheckRunner | `internal/tamper/detector.go` | 定时篡改检测 |
| ST-05 | MonitorRunner | `internal/service/monitor_app_service.go` | 定时 URL 可达性检测 |
| ST-06 | CookieVerifyRunner | `web/cookie_handlers.go` | 定时 Cookie 验证 |
| ST-07 | LoginStatusRunner | `web/cookie_handlers.go` | 定时登录状态检测 |
| ST-08 | DistributedTaskRunner | `internal/distributed/` | 定时分布式任务提交 |

### 2.3 中优先级 Runner（第12-13天）

| ID | Runner | 说明 |
|----|--------|------|
| ST-09 | ExportRunner | 定时数据导出 |
| ST-10 | PortScanRunner | 定时端口扫描 |
| ST-11 | ScreenshotCleanupRunner | 定时截图批次清理 |
| ST-12 | TamperCleanupRunner | 定时篡改记录清理 |
| ST-13 | QuotaMonitorRunner | 定时配额监控 |
| ST-14 | AlertSummaryRunner | 定时告警汇总 |
| ST-15 | BaselineRefreshRunner | 定时基准刷新 |
| ST-16 | URLImportRunner | 定时 URL 导入 |

### 2.4 低优先级 Runner（第14天）

| ID | Runner | 说明 |
|----|--------|------|
| ST-17 | PluginHealthRunner | 定时插件健康检查 |
| ST-18 | BridgeTokenRotateRunner | 定时 Bridge 令牌轮换 |
| ST-19 | AlertSilenceRunner | 定时告警静默窗口 |
| ST-20 | CacheWarmupRunner | 定时缓存预热 |

### 2.5 Web API 和前端（第15天）

| 端点 | 说明 |
|------|------|
| `GET /api/scheduled-tasks` | 任务列表 |
| `POST /api/scheduled-tasks` | 创建任务 |
| `PUT /api/scheduled-tasks/:id` | 更新任务 |
| `DELETE /api/scheduled-tasks/:id` | 删除任务 |
| `POST /api/scheduled-tasks/:id/start` | 启动任务 |
| `POST /api/scheduled-tasks/:id/stop` | 停止任务 |
| `GET /api/scheduled-tasks/:id/history` | 执行历史 |
| `GET /scheduled-tasks` | 前端页面 |

**验收标准：** 所有 API 端点有测试，`go test -race ./...` 0 失败。

---

## Phase 3：运维文档和面板（第16-17天）

### P2-1：运维 Runbook

编写 `docs/RUNBOOK.md`，覆盖以下场景：

| 场景 | 诊断步骤 | 恢复操作 |
|------|---------|---------|
| Chrome 崩溃 | 检查进程、查看崩溃日志 | 自动重启、CDP 重连 |
| Bridge 断连 | 检查 WebSocket 状态、token 有效性 | 令牌轮换、重连 |
| Cookie 失效 | 逐引擎检测登录状态 | 重新导入 Cookie |
| 节点失联 | 检查心跳、网络连通性 | 故障转移、节点重启 |
| 磁盘满 | 检查截图/日志/快照目录 | 清理过期文件、扩容 |
| Redis 不可用 | 检查连接状态 | 降级到内存缓存 |

### P2-2：Grafana 监控面板

基于现有 Prometheus 指标，创建监控面板 JSON：

| 面板 | 指标 | 告警阈值 |
|------|------|---------|
| 查询延迟 | P50/P95/P99 延迟 | P95 > 30s |
| 缓存命中率 | hit/(hit+miss) | < 50% |
| 截图成功率 | success/(success+fail) | < 90% |
| 篡改检测率 | detected/total | 异常波动 |
| 节点健康度 | 在线/总数 | < 80% |
| Goroutine 数 | runtime.NumGoroutine | > 1000 |
| 内存使用 | heap_alloc | > 80% |

**验收标准：** `docs/RUNBOOK.md` 完成，`docs/grafana-dashboard.json` 可导入 Grafana。

---

## 验收标准总览

| 检查项 | 通过标准 | 当前状态 |
|--------|---------|---------|
| `go build ./...` | 0 错误 | 通过 |
| `go vet ./...` | 0 警告 | 通过 |
| `go test -race ./...` | 0 失败，0 数据竞争 | 通过 |
| 核心 handler 测试覆盖率 | ≥ 70% | 已通过 |
| 告警 Webhook | 篡改告警 30s 内送达 | ✅ 通过 |
| 分布式故障转移 | 节点宕机后 60s 内任务重新分配 | ✅ 通过 |
| 定时任务系统 | 20 个 Runner 全部实现 | ✅ 完成 |
| 运维 Runbook | 6 个场景覆盖 | ✅ 完成 |
| Grafana 面板 | 7 个面板可导入 | ✅ 完成 |

---

## 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| 定时任务系统引入新依赖 | go.mod 变更可能引入兼容性问题 | 先创建独立模块，逐步集成 |
| 20 个 Runner 工作量大 | 可能延期 | 先实现高优先级 8 个，中低优先级后续迭代 |
| 分布式故障转移涉及多模块 | 调试复杂 | 编写集成测试，用 mock 控制节点状态 |

---

**文档版本：** v1.0
**维护者：** UniMap Team
