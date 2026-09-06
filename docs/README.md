# UniMap 文档索引

## 当前操作文档
- [发布核验快照（2026-09-06）](CURRENT_STATUS_2026-09-06.md)：已核验代码、CI 与镜像结果；明确 Bridge 和真实业务验收边界，不代表实时 HEAD。
- [项目审查与修复记录（2026-09-06）](PROJECT_REVIEW_2026-09-06.md)：复现、修复与回归范围。

- [快速开始](QUICKSTART.md)：本地配置、Web 与 CLI 启动。
- [使用指南](USAGE.md)：查询、配置、截图和 Web/CLI 操作。
- [API](API.md)：当前 `/api/v1` HTTP 契约。
- [运维 Runbook](RUNBOOK.md)：服务、认证、截图、Bridge、调度和节点排障。
- [云服务器部署评估（2026-07-20）](CLOUD_DEPLOYMENT_ASSESSMENT_2026-07-20.md)：历史部署评估快照；当前发布状态见 [当前项目状态](CURRENT_STATUS_2026-09-06.md)。
- [阿里云真实环境验收（2026-07-23）](CLOUD_ACCEPTANCE_2026-07-23.md)：历史条件验收快照；真实浏览器与发布边界以 [当前项目状态](CURRENT_STATUS_2026-09-06.md) 为准。
- [云服务器常态化运行准备与协作清单（2026-07-23）](CLOUD_STEADY_STATE_PLAN_2026-07-23.md)：历史准备计划快照；当前能力边界见 [当前项目状态](CURRENT_STATUS_2026-09-06.md)。
- [后续推进计划书（2026-08-20）](AGENT_CONTINUATION_PLAN_2026-08-20.md)：历史推进快照；当前基线与状态见 [当前项目状态](CURRENT_STATUS_2026-09-06.md)。
- [当前剩余工作清单（能力矩阵）](REMAINING_WORK_2026-07-23.md)：最近一次浏览器证据与完成标准；当前快照见 [CURRENT_STATUS_2026-09-06.md](CURRENT_STATUS_2026-09-06.md)。
- [实施收口状态（2026-08-02）](FINAL_IMPLEMENTATION_STATUS_2026-08-02.md)：2026-08-02 收口快照；当前基线与待办见 [当前项目状态](CURRENT_STATUS_2026-09-06.md)。
- [后续实施清单（2026-08-02）](NEXT_STEPS_20260802.md)：08-02 收口快照，执行顺序已移交 08-20 计划书。
- [安全发布收口修复计划（2026-07-29）](SECURITY_RELEASE_CLOSURE_REPAIR_PLAN_2026-07-29.md)：SSRF、截图格式、Bridge 取消、Quake 双链路、依赖漏洞和文档同步的分阶段修复与发布门槛。
- [安全发布收口修复记录（2026-07-29）](SECURITY_RELEASE_CLOSURE_REPAIR_REPORT_2026-07-29.md)：本轮代码修复、专项验证、Quake 分链路证据和仍待外部验收的发布边界。
- [云端安全收口验收 Runbook（2026-07-29）](CLOUD_SECURITY_ACCEPTANCE_RUNBOOK_2026-07-29.md)：DNS 动态变化、页面篡改证据截图、图片通知和重启恢复的受控 live E2E 契约。
- [后续实施计划（2026-07-23）](IMPLEMENTATION_PLAN_2026-07-23.md)：按旬排期的开发阶段、当前本地提交状态、依赖、验收门槛和用户配合事项。
- [截图扩展运维](OPS_SCREENSHOT_EXTENSION.md)：本机配对、Bridge token 与回调协议。
- [2026-08-02 七引擎浏览器验收](BROWSER_SEVEN_ENGINE_VERIFICATION_2026-08-02.md)：历史 Bridge/CDP 验收快照；当前逐引擎结论见 [当前项目状态](CURRENT_STATUS_2026-09-06.md)。
- [2026-08-21 插件/CDP 现状与下一步](PLUGIN_CDP_STATUS_2026-08-21.md)：最近一次插件/CDP 人工核验；改 ExtractJS 后的复跑状态见 [当前项目状态](CURRENT_STATUS_2026-09-06.md)。
- [2026-08-21 FOFA/Shodan/ZoomEye 原生 CDP 定级](CDP_VERIFICATION_FOFA_SHODAN_2026-08-21.md)：日期化 CDP 定级快照；FOFA/Shodan 通过、ZoomEye 当时受限，当前入口见 [当前项目状态](CURRENT_STATUS_2026-09-06.md)。
- [2026-08-21 中国电信上海分公司 ICP 与资产测绘结果](ICP_ASSET_SURVEY_SHANGHAI_TELECOM_2026-08-21.md)：云端查询条件、22 条 ICP 明细、31 条合并资产、归组判断与后续建议。
- [无图形浏览器运行时决策](DECISIONS/0006-headless-browser-runtime.md)：云主机 CDP、统一路由、会话限制和容器边界。
- [架构](ARCHITECTURE.md) 与 [业务架构](BUSINESS_AND_LOGIC_ARCHITECTURE.md)。
- [UQL 指南](UQL_GUIDE.md) 与 [搜索引擎语法快照](SEARCH_ENGINE_SYNTAX.md)。
- [七引擎查询语法及 API 参考（2026-08-10）](网络空间测绘平台_查询语法及API文档_2026-08-10/)：FOFA/Hunter/ZoomEye/Quake/Shodan/Censys/DayDayMap 官方查询语法与 API 示例，提交于 2026-08-11。
- [插件架构](PLUGIN_ARCHITECTURE.md) 与 [插件开发](PLUGIN_DEVELOPMENT_GUIDE.md)。
- GUI 入口仍为 `cmd/unimap-gui`（需 `go run -tags gui`）；默认产品入口是 Web 与 CLI。
- [变更日志](CHANGELOG.md)：按日期记录已完成的功能、兼容性与验证结果。

## 决策与历史资料

- [2026-09-04 状态快照](CURRENT_STATUS_2026-09-04.md)：保留当日提交与验收事实，不作为实时发布状态。

> 所有带日期的验收、计划、报告和提交记录保留当时事实；当前结论以 [当前项目状态](CURRENT_STATUS_2026-09-06.md) 为准。

- [2026-07-15 → 08-04 工作与提交记录](WORK_LOG_2026-07-15_to_2026-08-04.md)：2026-07-15 至今 51 个本地提交按 9 阶段整理，含时间线、阶段明细与关键交付物；另提供同内容 Word 版 `Unimap_WORK_LOG_2026-07-15_to_2026-08-04.docx`（纯黑白样式，2026-08-04 生成，未提交）。
- [2026-07-17 代码逻辑、API 适配与用户体验问题报告](CODE_LOGIC_API_UX_REVIEW_2026-07-17.md)：14 项初始问题及后续截图、通知、账号、ICP、调度前后端交互复核记录。
- [2026-07-17 完整修复与回滚指南](REMEDIATION_GUIDE_2026-07-17.md)：实施结果、两轮交互闭环、兼容性、验证门槛以及上线回滚清单。
- [决策记录](DECISIONS/)：保留当时的背景与结论；若与当前代码冲突，以当前 API/架构文档和代码为准。
- [archive](archive/)：历史计划、审计、测试与提交资料，不是当前操作指引。
- [API 版本化实施方案](API_VERSIONING.md)：已完成的历史设计；旧 `/api` shim 已移除。
- [生产就绪计划](PRODUCTION_READINESS_PLAN.md)：历史计划快照，不是当前发布门禁。
- [2026-07-14 Bridge 截图与通知验收](E2E_BRIDGE_SCREENSHOT_NOTIFICATION_2026-07-14.md)：稳定引擎的受控真实联调快照。
- [2026-07-15 查询通知与 Bridge 定时闭环验收](E2E_BRIDGE_SCHEDULED_QUERY_CLOSED_LOOP_2026-07-15.md)：API/Bridge 查询、资产明细通知、截图、SQLite 结果和五引擎复测状态。
- [2026-07-14 持久化与前后端终检](FINAL_PERSISTENCE_FRONTEND_AUDIT_2026-07-14.md)：持久化重载、API 契约和前端渲染的日期化验收。
- [2026-07-15 逻辑可用性三项完善闭环](LOGIC_USABILITY_CLOSEOUT_2026-07-15.md)：Alert 原子持久化回归、ICP history 部分匹配与 scheduler payload 兼容记录。

浏览器运行策略和查询降级计划已归档至 [archive/plans](archive/plans/)。

## 安全与隐私

部分历史资料仍在仓库内，用于追溯决策和验证；它们不应被当作当前事实或操作步骤。所有文档、测试记录和 issue 中都不得新增真实 API Key、Cookie、管理令牌、Bridge token、通知凭证或未授权资产信息。

| [CLI_AGENT_GUIDE.md](CLI_AGENT_GUIDE.md) | CLI Agent 友好化接口规范 |

- [CDP / Extension 分路径复验（2026-09-06）](CDP_EXTENSION_RECHECK_2026-09-06.md)：DayDayMap卡片修复；扩展实机与Bridge尚待验收。
