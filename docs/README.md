# UniMap 文档索引

## 文档分类

### 📖 用户文档

| 文档 | 说明 |
|------|------|
| [QUICKSTART.md](./QUICKSTART.md) | 快速入门指南 |
| [USAGE.md](./USAGE.md) | 使用手册 |
| [UQL_GUIDE.md](./UQL_GUIDE.md) | UQL 统一查询语言指南 |
| [TAMPER_DETECTION_FEATURE.md](./TAMPER_DETECTION_FEATURE.md) | 篡改检测功能说明 |
| [ZOOMEYE_TROUBLESHOOTING.md](./ZOOMEYE_TROUBLESHOOTING.md) | ZoomEye 故障排除指南 |

### 🔧 技术文档

| 文档 | 说明 |
|------|------|
| [ARCHITECTURE.md](./ARCHITECTURE.md) | 架构设计文档 |
| [API.md](./API.md) | API 接口文档 |
| [API_KEYS.md](./API_KEYS.md) | API 密钥配置文档 |
| [PLUGIN_ARCHITECTURE.md](./PLUGIN_ARCHITECTURE.md) | 插件架构文档 |
| [PLUGIN_DEVELOPMENT_GUIDE.md](./PLUGIN_DEVELOPMENT_GUIDE.md) | 插件开发指南 |
| [DECISIONS/](./DECISIONS/) | 架构决策记录 |

### 🚀 运维文档

| 文档 | 说明 |
|------|------|
| [RUNBOOK.md](./RUNBOOK.md) | 运维手册 |
| [PRODUCTION_READINESS_PLAN.md](./PRODUCTION_READINESS_PLAN.md) | 生产就绪计划（包含定时任务执行切片计划） |
| [SECURITY_AUDIT_REPORT.md](./SECURITY_AUDIT_REPORT.md) | ⚠️ 安全审计报告（2026-04-26，含 4 个严重问题） |
| [DEVELOPMENT_GUIDE.md](./DEVELOPMENT_GUIDE.md) | 开发指南 |
| [GUI_BUILD.md](./GUI_BUILD.md) | GUI 构建指南 |
| [OPS_SCREENSHOT_EXTENSION.md](./OPS_SCREENSHOT_EXTENSION.md) | 截图扩展运维文档 |
| [CHANGELOG.md](./CHANGELOG.md) | 变更日志 |

### 📊 监控和测试文档

| 文档 | 说明 |
|------|------|
| [grafana-dashboard.json](./grafana-dashboard.json) | Grafana 监控面板配置 |
| [perf/](./perf/) | 性能测试结果归档 |
| [evidence/](./evidence/) | 功能验证证据归档 |

## 📁 文档结构

```
docs/
├── README.md                              # 本文档 - 文档索引
├── DECISIONS/
│   └── 0001-unified-utils-directory.md
├── evidence/
│   ├── 2026-04-24-webhook-e2e.md
│   └── 2026-04-25-failover-e2e.md
├── perf/
│   └── 2026-04-24-loadtest.md
├── API.md
├── API_KEYS.md
├── ARCHITECTURE.md
├── CHANGELOG.md
├── DEVELOPMENT_GUIDE.md
├── GUI_BUILD.md
├── OPS_SCREENSHOT_EXTENSION.md
├── PLUGIN_ARCHITECTURE.md
├── PLUGIN_DEVELOPMENT_GUIDE.md
├── PRODUCTION_READINESS_PLAN.md
├── QUICKSTART.md
├── RUNBOOK.md
├── SECURITY_AUDIT_REPORT.md
├── TAMPER_DETECTION_FEATURE.md
├── UQL_GUIDE.md
├── USAGE.md
├── ZOOMEYE_TROUBLESHOOTING.md
└── grafana-dashboard.json
```

## 📦 归档文档

已完成的历史计划文档已归档到 `docs_archive/` 目录：

| 归档文档 | 说明 |
|---------|------|
| [docs_archive/OPTIMIZATION_PLAN.md](../docs_archive/OPTIMIZATION_PLAN.md) | 优化改进计划（已完成） |
| [docs_archive/IMPLEMENTATION_GUIDE.md](../docs_archive/IMPLEMENTATION_GUIDE.md) | 安全重构实施指导（已完成） |
| [docs_archive/TEST_COVERAGE_PLAN.md](../docs_archive/TEST_COVERAGE_PLAN.md) | 测试覆盖率提升计划（已完成） |
| [docs_archive/WORK_LOG_2026-04-21.md](../docs_archive/WORK_LOG_2026-04-21.md) | 工作日志 2026-04-21（已归档） |
| [docs_archive/DOC_REALITY_CHECKLIST_2026-04-24.md](../docs_archive/DOC_REALITY_CHECKLIST_2026-04-24.md) | 文档真实性检查清单（已完成） |

## 维护指南

### 文档更新流程

1. **新增功能：** 更新 CHANGELOG.md
2. **生产就绪：** 更新 PRODUCTION_READINESS_PLAN.md
3. **安全审计：** 更新 SECURITY_AUDIT_REPORT.md，同步更新 PRODUCTION_READINESS_PLAN.md 审计章节
4. **测试验证：** 将证据归档到 evidence/ 目录
5. **性能测试：** 将结果归档到 perf/ 目录
6. **历史计划：** 完成后归档到 docs_archive/ 目录

### 文档质量标准

- 所有"已完成"声明必须附带日期和证据定位
- 状态更新需同步更新最后更新时间
- 性能、安全、稳定性相关结论必须附带可复验证据
