# Project Memory Index

## 工作进展

- [跨平台适配 2026-04-13](project_crossplatform_2026-04-13.md) — macOS/Linux适配：自动查询检查、定时任务渲染、SIGHUP兼容性、Chrome路径、CI多平台
- [实施进度 2026-04-15](project_implementation_progress_2026-04-15.md) — 安全重构步骤1-3完成，P0清零
- [实施指导完成 2026-04-16](project_implementation_guide_progress_2026-04-16.md) — 10步全部完成：20个Runner、E2E测试、Runbook、Grafana面板
- [测试覆盖率计划 2026-04-20](project_test_coverage_plan_2026-04-20.md) — 从40.4%提升到80%，分3个Phase执行，已制定详细计划
- [测试覆盖率 Phase1 进度 2026-04-21](project_test_coverage_phase1_2026-04-21.md) — Phase 1完成：adapter 17.7%、screenshot 20.8%、service 22.5%，数据竞争修复，整体65.1%

### ✅ 全部项目完成状态

| 阶段 | 状态 | 说明 |
|------|------|------|
| P0 缺陷修复 | ✅ 完成 | Unicode错误、Worker池泄漏、Logger竞态 |
| P1 缺陷修复 | ✅ 完成 | 优雅关闭、Context取消、Clone错误、重试逻辑、测试补充、告警通道 |
| P2 技术债务 | ✅ 完成 | SSRF防护、文件权限、CI完善、Docker安全、MD5→SHA256等 |
| 架构增强 | ✅ 完成 | ScreenshotRouter双模式、CDP跨平台 |
| 定时任务系统 | ✅ 完成 | 20个Runner(ST-01~ST-20)、Web API、前端页面、E2E测试 |
| 运维文档 | ✅ 完成 | RUNBOOK.md(6场景)、Grafana面板(7面板) |
| 测试覆盖 | ✅ 进行中 | 32包通过-race检测，覆盖率65.0% |

### 遗留低优先级事项

| # | 项目 | 严重度 | 说明 |
|---|------|--------|------|
| 1 | Scheduler 编辑 UI | 低 | 后端已实现，前端缺少编辑按钮和表单 |
| 2 | 测试覆盖率提升 | 中 | 当前65.1%，Phase 1已超额完成（目标55%），待继续Phase 2/3达到80%标准 |
| 3 | 数据竞争修复 | ✅ 已完成 | CircuitBreaker添加mutex保护，测试atomic计数器 |
| 4 | Phase 1 剩余模块 | 低 | adapter/screenshot/service 已有基础测试，mock层待完善 |
