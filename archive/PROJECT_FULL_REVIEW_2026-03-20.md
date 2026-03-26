# UniMap 项目完整复核报告（2026-03-20）

> 2026-03-23 更新注记：本报告为 2026-03-20 复核基线，后续已完成 Phase B 大部分落地点。请结合 WORK_LOG.md 查看最新状态。

## 1. 复核范围与方法

本次复核覆盖以下范围：

- 代码结构：cmd、internal、web、configs
- 核心链路：查询编排、插件体系、Web 服务、截图与篡改检测
- 文档体系：README、README_LIGHT、QUICKSTART、USAGE、PROJECT_SUMMARY、架构文档
- 工程质量：静态检查、测试覆盖、可维护性

执行的客观检查：

- go test ./...：通过
- go vet ./...：通过
- Go 源文件数：59
- *_test.go 文件数：4

---

## 2. 项目架构现状（按当前代码）

### 2.1 分层与职责

当前项目是“单仓多入口 + 统一服务核心”的架构，不是严格微服务拆分。

- 入口层
  - cmd/unimap-web/main.go：Web 入口
  - cmd/unimap-cli/main.go：CLI 入口
  - cmd/unimap-gui/main.go：GUI 入口
- 业务中台层
  - internal/service/unified_service.go：统一查询服务、缓存、插件钩子、导出入口
- 引擎抽象层
  - internal/adapter/*.go：各引擎 API/Web-only 适配
  - internal/adapter/orchestrator.go：多引擎并发编排、查询转换、缓存联动
- 插件扩展层
  - internal/plugin/*.go：插件生命周期、Hook、处理管道
- 能力模块层
  - internal/tamper/detector.go：网页篡改检测
  - internal/screenshot/manager.go：截图与 CDP 管理
  - internal/core/unimap/*：UQL 解析与结果归并
- Web 表现层
  - web/server.go：HTTP 路由、页面渲染、WebSocket、API 编排
  - web/templates、web/static：页面模板与静态资源

### 2.2 实际请求链路（查询）

1. CLI/GUI/Web 接收查询参数
2. 进入 UnifiedService.Query
3. UQL 解析为 AST
4. orchestrator.TranslateQuery 转引擎 DSL
5. orchestrator.SearchEngines 并发查询
6. 各 adapter.Normalize 统一资产模型
7. 可选处理管道（processor plugins）
8. 返回并可导出

### 2.3 架构优势

- 统一服务接口可复用，CLI/GUI/Web 复用核心逻辑
- 适配器与编排器边界清晰，新增引擎成本可控
- 插件机制和 Hook 为扩展保留了接口
- 篡改检测与截图独立为子模块，功能边界相对明确

---

## 3. 关键问题与优化点（分优先级）

## 3.1 高优先级（建议 1-2 周内）

### H1. Web 层单文件过大，维护风险高 ✅ 已完成

- **现状**：`web/server.go` 已由约 2780 行降到约 510 行，query/ws/cookie/cdp/monitor 等处理器已完成分拆。
- **新增**：2026-03-20 创建 `web/router.go` 统一路由注册，创建 `web/middleware_ratelimit.go` 限流中间件。
- **收益**：降低耦合，提升可读性和变更可控性，API 高频入口已实现限流保护。

### H2. 文档与实现存在结构性偏差，影响上手与协作 ✅ 已校准

- **现状**：README、QUICKSTART、PROJECT_SUMMARY 已完成第一轮现状校准。
- **新增**：2026-03-20 创建 `WORK_LOG.md` 工作日志，记录进展与规划。

### H3. 测试覆盖缺失 ✅ 已扩展

- **现状**：已新增 parser/orchestrator/tamper/service 共 4 个测试文件，约 70+ 测试用例。
- **新增**：2026-03-20 扩展表驱动测试、并发安全测试、基准测试。

### H4. API 限流保护 ✅ 已完成

- **现状**：已实现 `RateLimiter` 限流中间件，对查询、截图、导入、篡改检测等高频接口应用限流。
- **配置**：默认每分钟 60 次请求，支持 `SetRateLimitConfig()` 调整。

## 3.2 中优先级（建议 2-4 周）

### M1. 缓存能力配置化不足

- 现状：已完成第一轮配置化，`system.max_concurrent`、`system.cache_ttl`、`system.cache_max_size`、`system.cache_cleanup_interval` 已接入统一服务构造；Redis 策略仍保持禁用。
- 建议：
  - 抽象统一 CacheConfig，来源于 config.yaml
  - 统一 cache key 规范与 TTL 策略
  - 支持按引擎维度设置 TTL/QPS 和缓存开关

### M2. 并发模型可统一

- 现状：orchestrator 与 tamper 批处理均已使用统一 workerpool（已完成）
- 建议：
  - 将 monitor 等批处理能力统一采用相同任务抽象
  - 统一超时、取消、重试策略
  - 建立并发参数的全局配置（system.max_concurrent 的实际贯通）

### M3. API 分层可再清晰

- 现状：部分业务逻辑仍在 Web handler 内部拼装
- 建议：
  - 增加 application service 层（例如 QueryAppService、TamperAppService）
  - handler 仅做参数校验与响应编码

### M4. 安全基线可进一步加固

- 现状：已有基础安全头与 Origin 校验
- 建议：
  - 引入可配置 CORS 白名单
  - 对导入、截图、历史查询等高频入口增加限流
  - 对关键 API 增加请求大小限制、错误码标准化

## 3.3 低优先级（持续改进）

### L1. 观测性增强

- 增加 Prometheus 指标：请求耗时分位、引擎错误率、缓存命中率、篡改误报率
- 统一 request_id 贯穿日志

### L2. 性能细节优化

- 复用 regexp 编译对象（篡改 HTML 清洗）
- 减少热路径对象分配（例如字符串拼接与切片扩容）

### L3. 文档自动化

- 增加 docs/index.md 作为文档总入口
- 使用脚本自动校验 README 中“目录树/命令示例”与真实代码一致性

---

## 4. 项目说明（文档体系）现状复核

## 4.1 当前文档优点

- 文档数量充足，覆盖 README、快速开始、架构、插件、排障
- 轻量版 README_LIGHT 对 GUI 用户较友好
- 多个专题文档对历史修复有记录价值

## 4.2 主要不一致点

1. README 中部分目录和组件描述偏旧，和当前目录不完全对齐
2. QUICKSTART 仍强调历史容器化 ICP 流程，和当前常见使用路径（unimap-web / CLI / GUI）混杂
3. PROJECT_SUMMARY 的部分统计与“完成度”表述偏宣传化，缺少可验证指标（测试覆盖、SLO、性能基线）
4. 架构文档描述了插件化优势，但对应示例与当前实际落地程度未完全同步

## 4.3 建议的文档重构结构

建议在仓库维持一套“单一事实来源”文档体系：

- docs/01-overview.md：项目定位、能力边界
- docs/02-architecture.md：当前架构（仅反映现状）
- docs/03-quickstart.md：按 Web/CLI/GUI 三路径
- docs/04-configuration.md：配置项与环境变量
- docs/05-operations.md：部署、监控、故障排查
- docs/06-roadmap.md：计划能力与状态

并在 README 中仅保留导航和最短启动路径，避免 README 过长导致失真。

---

## 5. 建议的落地路线图

## Phase A（本周）✅ 已完成

- 已完成 README、QUICKSTART、PROJECT_SUMMARY 的现状校准
- 已完成基础测试骨架（parser/orchestrator/tamper）并补充 service 构造测试
- 已完成 web/server.go 多批拆分（tamper + screenshot + query + websocket + cookie + cdp + monitor handler）
- ✅ 新增：创建 `web/router.go` 统一路由注册
- ✅ 新增：创建 `web/middleware_ratelimit.go` API 限流中间件
- ✅ 新增：扩展测试覆盖（表驱动测试、并发安全测试、基准测试）
- ✅ 新增：创建 `WORK_LOG.md` 工作日志

## Phase B（2-4 周）

- ✅ CORS 白名单配置化
- ✅ 请求大小限制与错误码标准化
- ✅ 限流参数配置化（移至 config.yaml）
- 🟡 持续收敛缓存与并发配置（含 Redis 方案评估）
- 🟡 接入 Prometheus 指标与关键日志字段（基础指标已接入）

## Phase C（4-8 周）

- 🟡 完善安全基线（限流、CORS 白名单、输入大小控制）
- ⏳ 建立文档一致性检查脚本与 CI 质量门禁

---

## 6. 结论

项目当前已具备较完整的功能骨架与可用能力，尤其在多引擎适配、统一服务、截图与篡改检测方面已经形成体系。当前最主要的短板不在“功能缺失”，而在“工程化成熟度”：

- Web 层过度集中导致维护风险
- 文档与实现不同步影响协作效率
- 自动化测试缺失限制长期演进

建议优先投入在“结构拆分 + 文档校准 + 测试补齐”三件事，收益最大、风险最低，可显著提升后续迭代效率与稳定性。

---

## 7. 本报告引用的关键文件

- cmd/unimap-web/main.go
- cmd/unimap-cli/main.go
- cmd/unimap-gui/main.go
- internal/service/unified_service.go
- internal/adapter/orchestrator.go
- internal/plugin/manager.go
- internal/config/config.go
- internal/tamper/detector.go
- web/server.go
- web/router.go
- web/middleware_ratelimit.go
- web/query_handlers.go
- web/websocket_handlers.go
- web/cookie_handlers.go
- web/cdp_handlers.go
- web/monitor_handlers.go
- README.md
- README_LIGHT.md
- QUICKSTART.md
- USAGE.md
- PROJECT_SUMMARY.md
- ARCHITECTURE_IMPROVEMENTS.md
- PLUGIN_ARCHITECTURE.md
- WORK_LOG.md
