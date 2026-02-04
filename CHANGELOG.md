# Changelog

All notable changes to UniMap Light will be documented in this file.

## [1.0.0] - 2026-02-04

### 🎉 初始版本 - 轻量化转型

这是 UniMap 的轻量版本，从原 UniMap + ICP-Hunter 项目核心功能提取而来。

### Added (新增)

#### 核心功能
- ✨ **GUI 图形界面**: 基于 Fyne 的跨平台图形用户界面
- 🔍 **多引擎查询**: 支持 FOFA、Hunter、ZoomEye、Quake 四大搜索引擎
- 📝 **UQL 统一查询语言**: 支持标准化的查询语法
- 🔄 **智能结果聚合**: 多引擎结果自动去重和合并
- 📊 **结果导出**: 支持 JSON 和 Excel 两种格式导出
- 🔒 **安全配置**: API Key 仅存储在内存中，不落地持久化

#### 界面组件
- 查询输入区：支持多行 UQL 语句输入
- 引擎选择区：复选框选择要使用的引擎
- 操作按钮区：开始查询、导出 JSON、导出 Excel
- 状态栏：显示查询状态和进度
- 结果表格：展示 IP、端口、协议、域名、URL、标题等信息
- 配置对话框：API Key 配置界面

#### 导出功能
- **JSON 导出**: 包含所有字段的完整数据
- **Excel 导出**: 结构化表格，支持 15 个核心字段

#### 文档
- 📖 README_LIGHT.md: 简化版使用说明
- 📘 USAGE.md: 详细使用指南，包含 8 个查询示例
- 🔧 build.sh / build.bat: 跨平台编译脚本

### Changed (变更)

#### 架构简化
- 从微服务架构转变为单体应用
- 从 CLI 工具转变为 GUI 应用
- 移除 Web API 服务
- 移除数据持久化层

#### 依赖精简
- 移除 Gin Web 框架
- 移除 MySQL/GORM 数据库
- 移除 Redis 缓存/消息队列
- 移除 Cobra/Viper CLI 框架
- 移除 Prometheus/Grafana 监控
- 移除 Docker 容器化部署

新的依赖栈：
- Fyne v2.4.3 (GUI)
- Resty v2.11.0 (HTTP)
- Excelize v2.8.0 (Excel)

#### 项目结构
```
旧结构:
├── cmd/
│   ├── unimap/          (CLI)
│   └── icp-hunter/      (ICP检测服务)
├── internal/
│   ├── repository/      (数据访问层)
│   ├── service/         (业务服务)
│   ├── core/
│   ├── adapter/
│   └── model/
├── pkg/utils/           (工具包)
├── configs/             (配置文件)
├── scripts/             (运维脚本)
└── docker/              (Docker配置)

新结构:
├── cmd/
│   └── unimap-gui/      (GUI入口)
├── internal/
│   ├── core/unimap/     (UQL解析和聚合)
│   ├── adapter/         (引擎适配器)
│   ├── exporter/        (导出功能)
│   └── model/           (数据模型)
└── res/                 (GUI资源)
```

### Removed (移除)

#### 功能模块
- ❌ ICP 备案检测功能
- ❌ 定时任务调度器
- ❌ 工作节点服务
- ❌ 通知系统 (邮件/Webhook)
- ❌ 统计报表功能
- ❌ 白名单管理
- ❌ 截图存档功能
- ❌ 分布式任务队列

#### 技术组件
- ❌ Web API 接口
- ❌ 数据库持久化
- ❌ Redis 缓存
- ❌ 监控系统
- ❌ Docker 部署
- ❌ CLI 命令行工具

#### 配置和脚本
- ❌ configs/ 配置目录
- ❌ scripts/ 运维脚本
- ❌ docker-compose.yml
- ❌ Dockerfile.*
- ❌ docs/ 文档目录（旧版文档）

### Security (安全)

#### 安全加固
- ✅ CodeQL 安全扫描通过，0 个漏洞
- ✅ API Key 仅存储在内存中，程序退出后自动清除
- ✅ 移除数据库，消除 SQL 注入风险
- ✅ 移除 Web 服务，消除 XSS 风险
- ✅ 完善的错误处理，防止信息泄露

### Performance (性能)

#### 优化项
- ⚡ 单可执行文件，启动速度快
- ⚡ 无外部依赖，部署简单
- ⚡ 异步查询，界面不卡顿
- ⚡ 基于 IP:Port 的高效去重算法

### Technical Debt (技术债务)

#### 已解决
- 移除了过度复杂的微服务架构
- 移除了不必要的中间层抽象
- 清理了冗余的配置文件
- 统一了代码风格

#### 待改进
- GUI 需要平台特定的依赖库（开发环境）
- 尚未实现查询历史记录
- 尚未实现配置持久化（可选功能）
- 缺少批量查询能力

### Compatibility (兼容性)

#### 支持的平台
- ✅ Windows (amd64, 386)
- ✅ macOS (amd64, arm64/Apple Silicon)
- ✅ Linux (amd64, 386, arm64)

#### 系统要求
- **最低**: Go 1.21+ (编译时)
- **运行时**: 无特殊要求
- **开发环境**: 需要 OpenGL/X11 等 GUI 库

### Migration Guide (迁移指南)

#### 从旧版迁移

如果您是从完整版 UniMap + ICP-Hunter 迁移：

1. **导出数据**: 使用旧版导出您需要的历史数据
2. **获取 API Keys**: 确保您有各引擎的 API Key
3. **安装新版**: 下载或编译 UniMap Light
4. **配置引擎**: 在 GUI 中配置 API Key
5. **开始使用**: 直接在 GUI 中执行查询

#### 功能对比

| 功能 | 完整版 | 轻量版 |
|------|-------|-------|
| 资产查询 | ✅ | ✅ |
| UQL 语法 | ✅ | ✅ |
| 多引擎支持 | ✅ | ✅ |
| 结果导出 | ✅ | ✅ |
| ICP 检测 | ✅ | ❌ |
| 数据持久化 | ✅ | ❌ |
| 定时任务 | ✅ | ❌ |
| Web API | ✅ | ❌ |
| 监控告警 | ✅ | ❌ |
| 交互方式 | CLI + API | GUI |
| 部署方式 | Docker | 单文件 |

### Known Issues (已知问题)

1. **编译环境**: Linux 编译需要安装 X11 开发库
2. **文件选择**: 文件保存对话框在某些桌面环境可能样式不一致
3. **大数据量**: 单次查询大量数据可能导致界面响应延迟

### Contributors (贡献者)

- 架构设计与开发: Claude Code
- 原版项目: ElaineRosa6

### Notes (备注)

本版本是对原项目的轻量化改造，专注于核心查询功能，适合以下场景：

- ✅ 临时快速查询
- ✅ 个人使用
- ✅ 无持久化需求
- ✅ 跨平台兼容
- ❌ 不适合企业级持续监控
- ❌ 不适合大规模自动化扫描

如需完整功能，请使用原版 UniMap + ICP-Hunter。

---

**UniMap Light** © 2026
