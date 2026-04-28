# UniMap 文档真实性修订清单

> **最近更新：** 2026-04-25

## 目标

将仓库文档状态与当前分支实际实现保持一致，避免”已完成”与”待实施”并存造成误导。

## 已完成修订

### 2026-04-25

- P0-3 分布式故障转移证据归档完成：`docs/evidence/2026-04-25-failover-e2e.md`
- PRODUCTION_READINESS_PLAN.md 状态更新为”✅ 全部完成”，最后更新 2026-04-25
- CHANGELOG.md 新增 2026-04-25 条目

### 2026-04-24

- README 完成度描述改为可验证口径，不再写固定测试包数字。
- 生产就绪文档顶部状态、最后更新时间、评估总览状态同步修正。
- 生产就绪文档新增“2026-04-24 文档真实性复核”章节。

## 补充证据完成情况（2026-04-24）

### 1) Webhook 端到端证据（已完成）

- 目标：验证“篡改检测触发告警 -> Webhook 发送 -> 对端收到请求体”。
- 证据文件：`docs/evidence/2026-04-24-webhook-e2e.md`
- 关键命令：
  - `go test ./internal/alerting -run TestManager_SendWarning_TamperWebhookE2E -v`
  - `go test ./internal/alerting -run TestWebhookChannel_Send_Success -v`
- 结果：两条验证命令均 PASS。

### 2) 压测结果归档（已完成）

- 目标：补齐 P50/P99 实测值并可追溯。
- 证据文件：`docs/perf/2026-04-24-loadtest.md`
- 原始结果：`results/load_test_20260424_pwsh.json`
- 执行说明：
  - 当前 shell 环境执行 `scripts/load_test.sh` 时出现 bash 语法兼容问题。
  - 已使用 PowerShell 并发压测脚本完成同口径统计，结果已归档。

### 3) 测试口径统一（已完成）

- 目标：统一 README、CHANGELOG、生产就绪文档中的测试规模描述。
- 当前规则：
  - 用“命令实测通过”作为主描述。
  - 固定包数字不作为常驻结论，避免随代码规模变化而失真。

## 复核命令（建议）

```bash
go build ./...
go test ./...
go test -race ./...
```

## 维护规则（建议长期执行）

- 文档出现“已完成”时，必须附带日期与证据定位。
- 状态更新需同步更新最后更新时间。
- 对历史计划文档，保留历史内容但新增“当前真实性复核”区块。
