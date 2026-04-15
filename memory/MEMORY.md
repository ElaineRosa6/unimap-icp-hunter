# Project Memory Index

## 工作进展

- [跨平台适配 2026-04-13](project_crossplatform_2026-04-13.md) — macOS/Linux适配：自动查询检查、定时任务渲染、SIGHUP兼容性、Chrome路径、CI多平台
- [实施进度 2026-04-15](project_implementation_progress_2026-04-15.md) — 安全重构步骤1-3完成，P0清零
- [实施指导完成 2026-04-16](project_implementation_guide_progress_2026-04-16.md) — 10步全部完成：20个Runner、E2E测试、Runbook、Grafana面板

### ✅ 全部项目完成状态

| 阶段 | 状态 | 说明 |
|------|------|------|
| P0 缺陷修复 | ✅ 完成 | Unicode错误、Worker池泄漏、Logger竞态 |
| P1 缺陷修复 | ✅ 完成 | 优雅关闭、Context取消、Clone错误、重试逻辑、测试补充、告警通道 |
| P2 技术债务 | ✅ 完成 | SSRF防护、文件权限、CI完善、Docker安全、MD5→SHA256等 |
| 架构增强 | ✅ 完成 | ScreenshotRouter双模式、CDP跨平台 |
| 定时任务系统 | ✅ 完成 | 20个Runner(ST-01~ST-20)、Web API、前端页面、E2E测试 |
| 运维文档 | ✅ 完成 | RUNBOOK.md(6场景)、Grafana面板(7面板) |
| 测试覆盖 | ✅ 完成 | 33包通过-race检测，0失败 |

### 遗留低优先级事项

| # | 项目 | 严重度 | 说明 |
|---|------|--------|------|
| 1 | Scheduler 编辑 UI | 低 | 后端已实现，前端缺少编辑按钮和表单 |
| 2 | tamper 测试覆盖 ~14% | 低 | 1739 LOC 覆盖率较低 |
| 3 | screenshot 测试覆盖 ~10% | 低 | 1972 LOC 覆盖率较低 |
| 4 | 部分 util 子包覆盖 | 低 | circuitbreaker/codequality/degradation/memory/objectpool/database/decoder/priority/threshold 已补充测试 |
