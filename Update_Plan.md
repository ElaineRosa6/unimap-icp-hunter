# 网页截图重构升级计划（插件桥接方案）

## 一、背景与目标

当前项目网页截图依赖 Chrome CDP 来同步用户登录状态并执行截图。该方案存在浏览器进程耦合强、登录态维护复杂、跨环境稳定性波动等问题。

本次升级目标是将截图执行链路重构为：

- 业务服务（Go）负责任务编排
- 本地桥接服务负责任务派发/回收
- 浏览器插件负责在真实用户浏览器上下文中执行截图

同时保留 CDP 作为可回退引擎，确保升级过程可灰度、可回滚、不影响现有业务。

## 二、改造原则

1. 先解耦再替换：先引入统一截图接口，再接入插件实现。
2. 双引擎并存：CDP 与 Extension 同时保留，配置可切换。
3. 最小行为变更：API 响应字段与错误语义尽量保持不变。
4. 可观测优先：上线前必须具备桥接健康检查与故障诊断。
5. 回退可验证：插件链路异常可快速回退到 CDP。

## 二点五、当前进展（2026-03-30）

### 已完成工作

1. Day 2 解耦已落地
  - 已新增 `screenshot.Provider` 与 `CDPProvider` 适配层。
  - `ScreenshotAppService` 已支持 provider 注入并保留旧调用兼容路径。

2. Day 3 Web 收口已落地
  - 截图 handler 的可用性判断已收口到应用层能力检查。
  - 查询链路已补充 Day 8 迁移 TODO 注释。

3. Day 4 配置与桥接 API 骨架已落地
  - 已新增 `screenshot.engine` 和 `screenshot.extension.*` 配置项、默认值和校验。
  - 已新增桥接 API：`/health`、`/status`、`/pair`。

4. Day 5 桥接服务 MVP 已落地
  - 已新增桥接任务模型、BridgeService（队列/并发/超时/重试）。
  - 已实现任务拉取接口：`GET /api/screenshot/bridge/tasks/next`。
  - 已实现 mock 回传接口：`POST /api/screenshot/bridge/mock/result`。
  - 已实现 `image_data` 落盘并回填真实 `file_path`。
  - 已实现配对发 token 与 Bearer token 鉴权（按 `pairing_required` 生效）。

5. Day 6 插件 MVP 已落地
  - 已创建 MV3 插件工程骨架（manifest/background/pairing/capture/api/storage）。
  - 插件已由“状态心跳”切换为真实任务拉取并执行回传。
  - 已支持自动配对获取 token（开发流程）。

6. 联调结果
  - 已通过一键脚本完成端到端联调：配对 -> 下发任务 -> 拉取任务 -> 回传 image_data -> 服务端落盘 -> 批量 API 成功返回。
  - 全量测试通过：`go test ./...`。

7. Day 7 收口已完成
  - extension 路径已扩展到搜索截图与目标截图，不再仅限批量 URL。
  - `fallback_to_cdp` 已接入单点截图与批量截图统一策略。
  - 全量回归通过：`go test ./...`（2026-03-27）。

8. 前端基线结果判定问题已修复（2026-03-30）
  - 已修复监控页“设置基线”结果判定兼容问题：前端同时兼容 `status` / `Status` 字段。
  - 修复后不再将已保存基线误显示为“全失败”。
  - 回归通过：`go test ./web`。

9. 网络受限场景门禁能力已验证（2026-03-30）
  - 已验证 `/api/url/reachability` 可稳定区分 reachable / unreachable。
  - 已确认可采用“先探活再基线/篡改”的门禁流程提升结果真实性。

10. 当前代码健康度
  - 全量测试通过：`go test ./...`（2026-03-30）。

11. Day 8 查询自动截图改造已完成（2026-03-30）
  - 查询自动截图已切换为通过 `ScreenshotAppService` 执行，支持当前引擎策略。
  - 移除了“CDP 在线性检查失败即中断浏览器联动”的强依赖。
  - 在截图能力不可用时仅记录 `autoCaptureErrors`，不阻断主查询结果返回。
  - 回归通过：`go test ./internal/service ./web`、`go test ./...`。

12. Day 9 Cookie 语义分流已完成（2026-03-30）
  - `handleSaveCookies` 在 extension 模式下改为兼容提示，不再执行 cookie 注入。
  - `applyCookiesFromRequest` 已按引擎分流：cdp 保留 cookie/proxy 写入，extension 仅保留 proxy 更新。
  - `handleVerifyCookies` 已分流：cdp 继续走 `ValidateSearchEngineResult`，extension 走 bridge 任务验证路径。
  - `ValidateSearchEngineResult/loadPageContent` 已补充 CDP-only 作用域注释，避免后续误复用。
  - 回归通过：`go test ./web ./internal/screenshot ./internal/service`、`go test ./...`。

13. Day 10 可观测与诊断补齐已完成（2026-03-30）
  - 已新增 bridge 指标：请求量、耗时、重试、超时、fallback 计数。
  - 已在 bridge 执行路径接入指标打点（single / target / batch urls）。
  - bridge status/health 已改为统一诊断快照输出，新增 `last_error`、`last_error_at`、`paired_clients` 等字段。
  - 已在关键 bridge 错误路径记录最近错误，支持快速定位认证/回调/任务拉取故障。
  - 回归通过：`go test ./internal/metrics ./internal/screenshot ./internal/service ./web`、`go test ./...`。

14. Day 11 灰度与回退演练准备已完成（2026-03-30）
  - 已在配置样例中补充三套 profile 模板：`cdp-only`、`extension-with-fallback`、`extension-only`。
  - 已新增回退脚本：`scripts/rollback_extension_to_cdp.ps1`、`scripts/rollback_extension_to_cdp.sh`。
  - 已新增演练记录模板：`archive/ROLLBACK_DRILL_2026-03-30.md`。
  - 回退脚本支持将配置切换为 `engine=cdp`、`extension.enabled=false`、`fallback_to_cdp=true`，并给出服务重启与健康检查提示。

15. Day 12 发布文档封板已完成（2026-03-30）
  - README 已补充插件安装、配对流程、引擎切换、故障排查、回退触发条件。
  - 已新增运维手册：`docs/OPS_SCREENSHOT_EXTENSION.md`。
  - 已新增发布检查清单：`archive/RELEASE_CHECKLIST_2026-03-30.md`。
  - Day 12 文档交付目标已完成，可进入新增 P0 需求实施阶段。

16. Day 13 代理池能力已完成（2026-03-30）
  - 已新增代理池组件：`internal/proxypool/pool.go`（轮询、失败阈值、冷却、可选直连回退）。
  - 已新增配置：`network.proxy_pool`（`enabled/strategy/proxies/failure_threshold/cooldown_seconds/allow_direct_fallback`）。
  - 已接入 monitor 探活链路：按任务选择代理并回报成功/失败，结果新增 `proxy` 字段。
  - 已接入 screenshot/tamper 链路：通过请求级代理注入，避免全局切换造成并发串扰。
  - 回归通过：`go test ./...`。

17. Day 14 URL->IP 端口扫描 + CDN 排除已完成（2026-03-30）
  - 已新增服务：`internal/service/monitor_port_scan_service.go`。
  - 已实现流程：URL 规范化 -> IPv4 解析 -> CDN 多信号判定 -> 非 CDN 端口扫描。
  - 已实现 CDN 排除策略：命中后返回 `cdn_excluded`，默认不对共享边缘节点执行端口扫描。
  - 已新增 API：`POST /api/url/port-scan`，返回 summary/ports/results。
  - 扫描结果状态覆盖：`invalid_format`、`resolve_failed`、`cdn_excluded`、`scan_failed`、`scanned`。
  - 已新增服务层回归测试：`internal/service/monitor_port_scan_service_test.go`，覆盖端口规范化、CDN 判定、本地可控扫描与 CDN 排除路径。

18. Day 16 文档状态同步 + 生产化加固（2026-03-31）
  - 代码实证同步已完成（按实现文件核对文档状态）：
    - bridge 鉴权与回调路径：`web/screenshot_bridge_handlers.go`
    - bridge 服务初始化与状态缓存：`web/server.go`
    - extension 回调发送路径：`tools/extension-screenshot/src/background.js`、`tools/extension-screenshot/src/api.js`
    - 配置定义与默认/校验：`internal/config/config.go`、`configs/config.yaml.example`
  - 生产化加固已落地：
    - 新增回调签名校验（HMAC-SHA256）+ 时间戳窗口校验。
    - 新增 nonce 防重放缓存校验。
    - 新增 token 轮换接口：`POST /api/screenshot/bridge/token/rotate`（可撤销旧 token）。
    - 新增配置项：`callback_signature_required`、`callback_signature_skew_seconds`、`callback_nonce_ttl_seconds`。
    - extension 回调已默认携带签名头（`X-Bridge-Timestamp`、`X-Bridge-Nonce`、`X-Bridge-Signature`）。
    - extension 已增加 token 临期主动轮换逻辑（失败不阻断任务拉取）。
  - 测试与门禁：
    - 新增回归测试：`web/screenshot_bridge_handlers_test.go`（签名缺失拒绝、签名通过、重放拒绝）。
    - 新增回归测试：`web/screenshot_bridge_handlers_test.go`（token 轮换成功且旧 token 撤销）。
    - 新增 CI 冒烟：`.github/workflows/bridge-smoke.yml`（bridge 测试 + extension 脚本语法检查）。
    - `scripts/bridge_e2e.ps1` 已支持 `-StrictSignature` 与 `-RotateToken` 参数用于严格模式演练。
  - 验证结果：`go test ./web ./internal/config` 通过。

### Day 13/14/15 实施对齐核查（2026-03-30）

核查口径：按“计划目标 -> 已实现代码 -> 文档状态 -> 结论”逐项核对。

1. Day 13（代理池，P0）
  - 计划目标：代理池配置、轮询/熔断、接入探活+截图+篡改。
  - 已实现代码：
    - `internal/proxypool/pool.go`
    - `internal/config/config.go`（`network.proxy_pool`）
    - `internal/service/monitor_app_service.go`（探活按任务选代理）
    - `web/screenshot_handlers.go`、`web/tamper_handlers.go`（请求级代理注入）
  - 文档状态：`Update_Plan.md`、`configs/config.yaml.example`、`docs/OPS_SCREENSHOT_EXTENSION.md` 已覆盖。
  - 结论：已对齐（通过）。

2. Day 14（端口扫描 + CDN 排除，P0）
  - 计划目标：URL->IP、CDN 判定、非 CDN 端口扫描、API 暴露。
  - 已实现代码：
    - `internal/service/monitor_port_scan_service.go`
    - `web/monitor_handlers.go`（`handleURLPortScan`）
    - `web/router.go`（`POST /api/url/port-scan`）
    - `internal/service/monitor_port_scan_service_test.go`（回归测试）
  - 文档状态：`Update_Plan.md`、`docs/OPS_SCREENSHOT_EXTENSION.md` 已覆盖。
  - 结论：已对齐（通过）。

3. Day 15（分布式节点，P1 可选）
  - 计划目标：节点注册/心跳、调度、任务分配回传、网络画像。
  - 当前代码状态：D15-A/B/C/D/E/F 已完成（注册心跳、任务闭环、网络画像、节点/管理侧鉴权、E2E 演练）。
  - 文档状态：`Update_Plan.md`、`README.md`、`archive/DAY15_ACCEPTANCE_RECORD_2026-03-30.md` 已同步。
  - 结论：计划与实现已对齐（通过）。

核查结论汇总：

1. Day 13：代码与文档对齐。
2. Day 14：代码与文档对齐。
3. Day 15：代码与文档对齐，具备灰度发布条件。

### 当前状态结论

1. 目前已形成“后端桥接 + 插件拉取 + 结果回传 + 文件落盘”的可运行闭环。
2. 双引擎并存策略已可用：`engine=extension` 失败时可按配置回退 CDP。
3. 监控页面“基线全失败”误判已消除，前端结果展示与后端返回语义已对齐。
4. 现阶段仍以 mock 回传协议为主，适合开发验证，不建议直接生产上线。

### 下一步计划（发布收口）

1. 发布门禁收口
  - 完成发布清单 Final Decision（GO/NO-GO）、Owner、Timestamp、Notes 归档。
  - 将 Day15 验收记录纳入发布证据链。

2. 文档一致性收口
  - 统一 Day13-15 状态口径，移除历史“未开始”描述。
  - 在本文件维护最终执行状态表（Done/In Progress/Todo）。

3. 生产化安全加固
  - 将 bridge 回调从 mock 语义收口为生产契约（字段、签名、验签失败处理）。
  - 增加 token/session 轮换策略与最小 CI 冒烟校验。

### 提交分组建议（按已完成工作拆分）

1. commit A（Day 2）
  - provider 抽象与 CDP 适配层
  - 建议文件：`internal/screenshot/provider.go`、`internal/screenshot/provider_cdp.go`、`internal/service/screenshot_app_service.go`、`web/server.go`
  - 建议信息：`day2: add screenshot provider abstraction and cdp adapter`

2. commit B（Day 3）
  - web 层截图可用性收口与 query TODO
  - 建议文件：`web/screenshot_handlers.go`、`web/query_handlers.go`
  - 建议信息：`day3: tighten web screenshot dependency to app-layer availability`

3. commit C（Day 4）
  - bridge 配置项与 bridge API 骨架
  - 建议文件：`internal/config/config.go`、`configs/config.yaml.example`、`web/router.go`、`web/screenshot_bridge_handlers.go`
  - 建议信息：`day4: add screenshot bridge config and api skeleton`

4. commit D（Day 5）
  - bridge service、任务下发/回传、mock client、image_data 落盘、token 鉴权
  - 建议文件：`internal/screenshot/bridge_types.go`、`internal/screenshot/bridge_service.go`、`internal/service/screenshot_app_service.go`、`web/screenshot_bridge_mock.go`、`web/screenshot_bridge_handlers.go`、`web/server.go`、`web/router.go`、`scripts/bridge_e2e.ps1`
  - 建议信息：`day5: implement bridge queue auth callback and image_data persistence`

5. commit E（Day 6）
  - 插件 MVP 骨架与任务拉取链路
  - 建议文件：`tools/extension-screenshot/**`
  - 建议信息：`day6: scaffold mv3 extension and integrate bridge task polling`

### 下一阶段立即执行清单（2026-03-30 更新）

1. 完成发布清单最终裁决与责任人确认（GO for controlled gray rollout）。
2. 将 Day15 验收记录与发布检查清单建立互相引用，作为发布证据。
3. 启动 Day16 生产加固任务：回调签名、token/session 轮换、CI 冒烟。

### 最终执行状态表（2026-03-31）

| 范围 | 状态 | 结论 |
| --- | --- | --- |
| Day 2-7（双引擎与桥接闭环） | Done | 已完成并通过回归。 |
| Day 8-12（查询自动截图、Cookie 分流、观测、灰度、发布文档） | Done | 已完成并通过回归。 |
| Day 13（代理池，P0） | Done | 已落地并接入关键链路。 |
| Day 14（URL->IP 端口扫描 + CDN 排除，P0） | Done | 已落地并开放 API。 |
| Day 15（分布式节点，P1 可选） | Done | D15-A~F 完成，验收通过，可灰度。 |
| 发布裁决（Release GO/NO-GO） | In Progress | 清单已补证据，待负责人最终签发。 |
| 生产化安全加固（回调签名/轮换/CI 冒烟） | Done | 回调签名、重放防护、token 轮换与 CI 冒烟已完成；可进入下一轮生产契约细化。 |

### 新增需求补充（2026-03-30）

优先级定义：

1. 必选 P0：代理池能力（用于受限网络环境）。
2. 必选 P0：同 URL 对应 IP 的端口扫描能力（含 CDN 智能识别与排除）。
3. 可选 P1：分布式节点与节点网络信息采集能力（与代理池并行设计，但不阻塞 P0 交付）。

新增执行计划：

1. Day 13（P0）：代理池能力落地
  - 新增代理池配置（静态列表 + 健康检查 + 轮询/失败熔断）。
  - 接入探活、基线、篡改、截图链路的统一出站策略。
  - 输出代理可用率与失败原因统计。

2. Day 14（P0）：端口扫描 + CDN 排除落地
  - 对输入 URL 先解析域名与候选 IP，再执行 CDN 判定。
  - CDN 场景默认不扫共享边缘节点，仅输出“CDN 命中+原因+建议源站排查”。
  - 非 CDN 场景执行端口扫描（默认常见端口，可扩展自定义端口集）。

3. Day 15（P1，可选）：分布式节点与网络信息采集
  - 增加节点注册/心跳/能力上报。
  - 调度侧支持按节点网络区域或出站质量分配任务。
  - 汇总节点公网出口、延迟、成功率等网络画像信息。

Day15 当前进展（2026-03-30 实施中）：

1. D15-A 已落地：节点注册/心跳/状态查询首版完成。
2. 已新增：`/api/nodes/register`、`/api/nodes/heartbeat`、`/api/nodes/status`。
3. 已新增内存节点目录：`internal/distributed/registry.go`。
4. 已新增首版 Web 回归测试：`web/node_handlers_test.go`。
5. D15-B 已落地（基础任务闭环）：
  - 已新增内存任务队列：`internal/distributed/task_queue.go`。
  - 已新增任务 API：`/api/nodes/task/enqueue|claim|result|status`。
  - 已新增回归测试：`internal/distributed/task_queue_test.go`、`web/node_task_handlers_test.go`。
6. D15-C 已落地（网络画像与灰度开关）：
  - 已新增配置：`distributed.enabled`、`distributed.heartbeat_timeout_seconds`、`distributed.max_reassign_attempts`、`distributed.scheduler.strategy`、`distributed.node_auth_tokens`。
  - 已新增网络画像 API：`GET /api/nodes/network/profile`。
  - 分布式节点与任务 API 已受开关控制：关闭时返回 `distributed_disabled`，支持一键回退单机。
  - 已补充回归测试覆盖开关关闭与网络画像输出路径。
7. D15-D 已落地（节点鉴权加固）：
  - 已将 `distributed.node_auth_tokens` 接入节点侧接口鉴权。
  - 已覆盖接口：`/api/nodes/register`、`/api/nodes/heartbeat`、`/api/nodes/task/claim`、`/api/nodes/task/result`。
  - 鉴权规则：当 `node_auth_tokens` 非空时，要求 `Authorization: Bearer <token>`（兼容 `X-Node-Token`）且与 `node_id` 映射匹配。
  - 已新增 Web 回归测试覆盖未鉴权拒绝（401）与鉴权通过路径。
8. D15-E 已落地（端到端演练脚本）：
  - 已新增一键演练脚本：`scripts/day15_distributed_e2e.ps1`、`scripts/day15_distributed_e2e.sh`。
  - 覆盖流程：register -> heartbeat -> enqueue -> claim -> result -> task status -> network profile。
  - 可用于灰度环境快速验收与问题复现。
9. D15-F 已落地（管理侧接口鉴权）：
  - 已新增配置：`distributed.admin_token`（可选）。
  - 已覆盖接口：`/api/nodes/status`、`/api/nodes/network/profile`、`/api/nodes/task/enqueue`、`/api/nodes/task/status`。
  - 当 `admin_token` 非空时，要求 `Authorization: Bearer <admin-token>`（兼容 `X-Admin-Token`）。
  - 已新增 Web 回归测试覆盖未鉴权拒绝（401）与鉴权通过路径。

Day13-15 发布证据链（2026-03-31 收口）：

1. 发布检查清单：`archive/RELEASE_CHECKLIST_2026-03-30.md`。
2. Day15 验收记录：`archive/DAY15_ACCEPTANCE_RECORD_2026-03-30.md`。
3. 回退演练记录：`archive/ROLLBACK_DRILL_2026-03-30.md`。

发布评审时需同时确认：

1. 验收结论与发布裁决一致（GO/NO-GO）。
2. 回退步骤可在目标窗口内执行。
3. 风险备注与后续加固任务已登记。

新增验收标准：

1. 代理池（必选）：在受限网络下可切换可用代理继续任务，失败率显著下降且可观测。
2. 端口扫描（必选）：扫描结果可区分 CDN 与非 CDN，默认不对 CDN 共享边缘 IP 误扫。
3. 分布式（可选）：节点离线不影响单机主流程，启用后可获得节点网络信息与调度收益。

### Day 15 详细实施规划（2026-03-30）

目标定义：

1. 在不影响当前单机主流程的前提下，引入“可选分布式节点”能力。
2. 调度层支持按节点状态与网络画像分配任务，具备最小可用容错。
3. 产出可观测与可回滚的交付件，便于灰度启停。

范围边界（本期包含）：

1. 节点注册、心跳、能力上报。
2. 节点调度（基础轮询 + 健康过滤 + 简单负载优先）。
3. 节点任务下发与结果回传协议（HTTP）。
4. 节点网络画像采集与查询（公网出口、延迟、成功率）。

范围边界（本期不包含）：

1. 跨机房强一致队列。
2. 多租户隔离与计费。
3. 复杂流量编排（如按国家/ASN/策略路由）。

总体架构：

1. Controller（现有 Web 服务）：维护节点目录、任务分配、状态聚合。
2. Worker Node（新增轻量节点进程或现有实例 agent 模式）：执行任务并回报结果。
3. Node Registry（内存优先，预留持久化接口）：存储节点元数据与在线状态。
4. Scheduler（内置模块）：按节点健康、并发占用、网络质量选择节点。

数据结构规划：

1. NodeDescriptor
  - node_id, hostname, region, labels, capabilities。
2. NodeHeartbeat
  - node_id, timestamp, current_load, max_concurrency, version。
3. NodeNetworkProfile
  - egress_ip, avg_latency_ms, success_rate_5m, fail_reason_topn。
4. DistributedTaskEnvelope
  - task_id, task_type, payload, priority, timeout_seconds, trace_id。
5. DistributedTaskResult
  - task_id, node_id, status, duration_ms, output, error。

API 规划（Controller 侧）：

1. POST /api/nodes/register
  - 作用：节点注册或刷新节点元数据。
2. POST /api/nodes/heartbeat
  - 作用：更新节点在线状态与负载。
3. GET /api/nodes/status
  - 作用：返回节点在线列表、负载与最近心跳。
4. POST /api/nodes/task/claim
  - 作用：节点主动拉取可执行任务。
5. POST /api/nodes/task/result
  - 作用：节点提交任务执行结果。
6. GET /api/nodes/network/profile
  - 作用：查询节点网络画像汇总。

调度策略（Day15 首版）：

1. 仅选择“心跳未超时 + 能力匹配 + 未达并发上限”的节点。
2. 在候选集中按 `success_rate_5m` 降序、`current_load` 升序选择。
3. 当无可用节点时自动回退本机执行（不阻断现有主流程）。
4. 任务超时或节点失联后触发一次重分配，避免无限重试。

安全与一致性：

1. 节点鉴权：`node_id + token`（首版静态签发，后续可轮换）。
2. 幂等约束：`task_id` 唯一，重复回传只采纳首个成功结果。
3. 时间窗校验：心跳与结果时间戳允许固定漂移窗口。
4. 最小审计：记录注册、心跳、任务分配、结果回传关键事件。

可观测规划：

1. 指标
  - distributed_nodes_online
  - distributed_task_dispatch_total{status}
  - distributed_task_duration_seconds
  - distributed_node_heartbeat_lag_seconds
2. 诊断接口
  - /api/nodes/status
  - /api/nodes/network/profile
3. 关键日志
  - 节点上下线、分配失败、重分配、回退本机执行。

实施拆分（可回滚提交）：

1. D15-A：节点注册与心跳基建
  - 文件建议：
    - internal/distributed/registry.go
    - internal/distributed/types.go
    - web/node_handlers.go
    - web/router.go
  - 交付：register/heartbeat/status 可用，支持在线节点列表。
  - 提交建议：`day15-a: add node registry register heartbeat status`

2. D15-B：任务调度与节点拉取执行
  - 文件建议：
    - internal/distributed/scheduler.go
    - internal/distributed/task_queue.go
    - web/node_task_handlers.go
    - internal/service/*（按现有任务入口接入调度）
  - 交付：task/claim 与 task/result 闭环，超时后单次重分配。
  - 提交建议：`day15-b: implement distributed task claim result and scheduler`

3. D15-C：网络画像与灰度开关
  - 文件建议：
    - internal/distributed/network_profile.go
    - internal/config/config.go（distributed 开关与阈值）
    - configs/config.yaml.example
    - docs/OPS_SCREENSHOT_EXTENSION.md（新增 Day15 运维段落）
  - 交付：网络画像接口与配置开关，支持一键关闭分布式回退单机。
  - 提交建议：`day15-c: add node network profile metrics and rollout controls`

配置规划（新增块建议）：

1. distributed.enabled: false（默认关闭）。
2. distributed.heartbeat_timeout_seconds: 30。
3. distributed.max_reassign_attempts: 1。
4. distributed.node_auth_tokens: map[node_id]token。
5. distributed.scheduler.strategy: health_load。

测试计划：

1. 单元测试
  - registry 增删改查、心跳超时摘除。
  - scheduler 候选过滤与排序正确性。
  - task 幂等与重复结果处理。
2. 集成测试
  - 2 节点 + 1 控制器：注册、心跳、拉取、回传全流程。
  - 节点失联场景：任务重分配与本机回退。
3. 回归门禁
  - `go test ./...`
  - 分布式开关关闭时行为与当前版本一致。

验收标准（Day15 交付判定）：

1. 在 `distributed.enabled=true` 下，至少 2 节点可稳定注册并维持心跳。
2. 任务可成功分配到节点执行并回传结果，成功率达到基线目标。
3. 节点离线后，任务可在重分配后完成或回退本机，不出现任务丢失。
4. 指标与诊断接口可用于定位节点与调度异常。
5. 在 `distributed.enabled=false` 下，系统完全回退单机模式且通过全量回归。

回滚策略：

1. 配置回滚：`distributed.enabled=false` 并重启服务。
2. 行为回滚：调度失败统一回退本机执行。
3. 代码回滚：按 D15-A/B/C 提交粒度逐步回退。

## 三、按天可执行任务清单（文件/函数级别）

### Day 1：冻结当前截图调用边界（不改行为）

目标：固定现有调用链，形成改造基线。

涉及文件与函数：

- web/router.go
  - RegisterRoutes
- web/screenshot_handlers.go
  - handleScreenshot
  - handleSearchEngineScreenshot
  - handleTargetScreenshot
  - handleBatchScreenshot
  - handleBatchURLsScreenshot
- web/query_handlers.go
  - runBrowserQueryMode

执行项：

1. 梳理 HTTP 入口到应用层、管理器层调用链。
2. 记录所有截图相关 API 入参与返回结构。
3. 记录 auto capture 在查询链路中的触发条件。

当日产出：

- 一份截图调用链基线说明（供后续回归对照）。

验收标准：

- 不改任何逻辑。
- 项目回归通过：go test ./...。

---

### Day 2：引入截图引擎抽象层（接口层）

目标：业务层不直接依赖 CDP 具体实现。

涉及文件与函数：

- internal/service/screenshot_app_service.go
  - CaptureSearchEngineResult
  - CaptureTargetWebsite
  - CaptureBatch
  - CaptureBatchURLs
- internal/screenshot/manager.go
  - 现有 Manager 作为 CDP 适配实现来源

执行项：

1. 设计 ScreenshotProvider 接口（单截图、搜索页截图、目标站截图、批量截图）。
2. 增加 CDPProvider（由现有 Manager 适配实现）。
3. ScreenshotAppService 改为依赖接口而非具体 Manager。

当日产出：

- 截图接口定义。
- CDP 适配器接入并编译通过。

验收标准：

- 截图 API 行为保持一致。
- go test ./... 通过。

#### Day 2 函数级施工单

目标：只做依赖解耦，不改变外部行为。

1. 新增接口定义文件：internal/screenshot/provider.go

建议新增类型与函数签名：

```go
package screenshot

import "context"

type Provider interface {
   CaptureSearchEngineResult(ctx context.Context, engine, query, queryID string) (string, error)
   CaptureTargetWebsite(ctx context.Context, targetURL, ip, port, protocol, queryID string) (string, error)
   CaptureBatchURLs(ctx context.Context, urls []string, batchID string, concurrency int) ([]BatchScreenshotResult, error)
   GetScreenshotDirectory() string
}
```

说明：

1. 接口先覆盖 ScreenshotAppService 真实使用的最小集合。
2. 先不引入 Extension 细节，避免 Day 2 范围失控。

2. 新增 CDP 适配器文件：internal/screenshot/provider_cdp.go

建议新增结构体与构造函数：

```go
package screenshot

type CDPProvider struct {
   mgr *Manager
}

func NewCDPProvider(mgr *Manager) *CDPProvider {
   return &CDPProvider{mgr: mgr}
}
```

建议实现方法：

1. CaptureSearchEngineResult: 直接委托给 m.mgr.CaptureSearchEngineResult。
2. CaptureTargetWebsite: 直接委托给 m.mgr.CaptureTargetWebsite。
3. CaptureBatchURLs: 直接委托给 m.mgr.CaptureBatchURLs。
4. GetScreenshotDirectory: 直接委托给 m.mgr.GetScreenshotDirectory。

3. 改造应用层依赖：internal/service/screenshot_app_service.go

改造点 A：结构体字段

1. 在 ScreenshotAppService 中新增 provider 字段（类型 screenshot.Provider）。
2. 保留 baseDir 字段用于批次文件管理逻辑。

改造点 B：构造函数

1. 维持现有 NewScreenshotAppService(baseDir string) 以兼容旧调用。
2. 新增 NewScreenshotAppServiceWithProvider(baseDir string, provider screenshot.Provider)。
3. 在旧构造函数中仅初始化 baseDir，不绑定 provider（保持兼容，便于渐进迁移）。

改造点 C：函数级替换

1. CaptureSearchEngineResult
  - 现状：参数依赖 mgr *screenshot.Manager，内部调用 mgr.CaptureSearchEngineResult。
  - 目标：优先调用 s.provider.CaptureSearchEngineResult。
  - 兼容：若 provider 为空则回退当前 mgr 路径（Day 2 不破坏旧调用）。

2. CaptureTargetWebsite
  - 现状：内部调用 mgr.CaptureTargetWebsite。
  - 目标：优先调用 s.provider.CaptureTargetWebsite。
  - 兼容：provider 为空时回退 mgr。

3. CaptureBatch
  - 现状：并发中直接调用 mgr.CaptureSearchEngineResult / mgr.CaptureTargetWebsite。
  - 目标：并发任务中优先改为调用 provider 对应方法。
  - 兼容：provider 为空时保留 mgr 调用路径。

4. CaptureBatchURLs
  - 现状：调用 mgr.CaptureBatchURLs 与 mgr.GetScreenshotDirectory。
  - 目标：优先改为 provider.CaptureBatchURLs 与 provider.GetScreenshotDirectory。
  - 兼容：provider 为空时回退 mgr。

4. 接入点最小改动：cmd/unimap-gui/main.go

目标：让现有初始化路径可以构造 provider，但不强制全链路切换。

执行项：

1. 在 buildScreenshotManager(cfg) 之后，创建 cdpProvider := screenshot.NewCDPProvider(mgr)。
2. 将 screenshot app service 初始化改为 NewScreenshotAppServiceWithProvider(baseDir, cdpProvider)。
3. 若当前初始化链条暂不便调整，可先保留原构造，Day 3 再统一切换。

5. Day 2 当日回归命令

```bash
go test ./...
go run ./cmd/unimap-web
```

6. Day 2 提交建议

```bash
git add internal/screenshot/provider.go internal/screenshot/provider_cdp.go internal/service/screenshot_app_service.go cmd/unimap-gui/main.go
git commit -m "day2: introduce screenshot provider abstraction with cdp adapter"
```

7. Day 2 完成判定

1. ScreenshotAppService 编译通过且对外行为不变。
2. 现有截图 API 回归通过（含 batch 与 target）。
3. 未引入 extension 运行时依赖。

---

### Day 3：Web 层切换到抽象接口（不改响应）

目标：处理器层完成依赖倒置。

涉及文件与函数：

- web/screenshot_handlers.go
  - handleSearchEngineScreenshot
  - handleTargetScreenshot
  - handleBatchScreenshot
  - handleBatchURLsScreenshot
- web/query_handlers.go
  - runBrowserQueryMode

执行项：

1. 所有 handler 调用统一截图接口。
2. 保留既有错误码与响应字段。
3. 保留 metrics 统计口径。

当日产出：

- Web 层与 CDP 实现脱钩。

验收标准：

- /api/screenshot、/api/screenshot/target、/api/screenshot/batch-urls 回归通过。

#### Day 3 函数级施工单

目标：Web 层只依赖应用层抽象，不直接依赖 screenshot.Manager 具体实现。

1. Server 结构体依赖梳理与收口

涉及文件：web/server.go（若 Server 结构体在此定义）

执行项：

1. 确认 Server 中已有 screenshotApp 字段（*service.ScreenshotAppService）。
2. 检查 screenshotMgr 字段是否仍被 handlers 直接调用。
3. Day 3 原则：handler 优先走 screenshotApp；screenshotMgr 仅保留兼容兜底，不新增直接依赖。

2. 处理器函数逐个替换调用路径

涉及文件：web/screenshot_handlers.go

函数 A：handleSearchEngineScreenshot

1. 保留前置校验（方法、参数、manager 初始化报错语义）。
2. 将核心调用统一为：
  - s.screenshotApp.CaptureSearchEngineResult(...)
3. 保留 metrics 与返回字段：path、engine、query、query_id。

函数 B：handleTargetScreenshot

1. 保留 trusted request 校验。
2. 保留原请求体字段：url、ip、port、protocol、query_id。
3. 核心调用保持通过 screenshotApp：
  - s.screenshotApp.CaptureTargetWebsite(...)
4. 错误语义保持不变：missing_parameters / screenshot_failed。

函数 C：handleBatchScreenshot

1. 保留请求体组装逻辑（engines、targets -> appReq）。
2. 核心调用保持通过 screenshotApp：
  - s.screenshotApp.CaptureBatch(...)
3. 保持当前响应结构与错误码 batch_screenshot_failed。

函数 D：handleBatchURLsScreenshot

1. 保留并发参数、URL 数量限制对应错误码映射。
2. 核心调用保持通过 screenshotApp：
  - s.screenshotApp.CaptureBatchURLs(...)
3. 保持结果字段兼容（batch_id、total、success、failed、results、screenshot_dir）。

函数 E：handleScreenshot（即时返回图片流的接口）

1. 该函数当前内联 chromedp 执行，不在 ScreenshotAppService 调用链上。
2. Day 3 建议最小改动策略：
  - 暂不重写业务行为；
  - 在函数注释中标注 Day 7 后迁移到 provider 统一链路；
  - 避免当日引入行为回归。

3. 查询链路的自动截图入口切面

涉及文件：web/query_handlers.go

函数：runBrowserQueryMode

执行项：

1. 保留 query 模式现有参数与返回结构。
2. 暂维持当前参数传递（含 s.screenshotMgr 与 s.resolveCDPURL）。
3. 在函数内增加 TODO 注释（Day 8 执行）：
  - 将 auto capture 从 cdpBaseURL 依赖迁移至 screenshot provider 可用性检查。

说明：

1. Day 3 只做 handler 层依赖收口，不做 query_app_service 的 CDP 判断重写。
2. query_app_service 的重构放在 Day 8，避免跨天任务耦合。

4. 兼容性检查点（函数级）

1. handleSearchEngineScreenshot：输出 query_id 逻辑不变（空时自动生成）。
2. handleTargetScreenshot：当 URL 与 IP 同时为空时，仍返回 missing_parameters。
3. handleBatchURLsScreenshot：no_urls / too_many 错误码映射保持不变。
4. handleScreenshotFile：静态文件预览路径解析逻辑不动。

5. Day 3 回归脚本

```bash
go test ./...
go run ./cmd/unimap-web
```

接口冒烟建议：

```bash
curl -X POST http://127.0.0.1:8448/api/screenshot/target -H "Content-Type: application/json" -d '{"url":"https://example.com"}'
curl -X POST http://127.0.0.1:8448/api/screenshot/batch-urls -H "Content-Type: application/json" -d '{"urls":["https://example.com"],"concurrency":2}'
```

6. Day 3 提交建议

```bash
git add web/screenshot_handlers.go web/query_handlers.go web/server.go
git commit -m "day3: route web screenshot handlers through app abstraction"
```

7. Day 3 完成判定

1. screenshot handlers 不新增对 screenshot.Manager 的直接调用。
2. Web 层截图功能与错误语义与 Day 2 前保持一致。
3. 编译通过且基础冒烟通过。

---

### Day 4：定义插件桥接协议与配置（服务端）

目标：确定插件通信契约，搭建可扩展配置。

涉及文件与函数：

- internal/config/config.go
- configs/config.yaml.example
- web/router.go

执行项：

1. 新增 screenshot.engine（cdp/extension）。
2. 新增 screenshot.extension 配置块：
   - enabled
   - listen_addr
   - auth（pairing_required, token_ttl）
   - task（timeout, max_concurrency）
   - fallback_to_cdp
3. 增加桥接管理路由（健康检查、配对、状态）。
4. 约定统一错误码：bridge_unavailable、pair_required、plugin_timeout、plugin_rejected。

当日产出：

- 桥接协议文档 v1。
- 配置项与路由骨架。

验收标准：

- 配置可解析。
- 不影响现有 CDP 路径。

#### Day 4 函数级施工单

目标：完成“插件桥接协议 + 配置骨架 + 路由入口”的最小闭环，不接入真实插件执行。

1. 配置结构体扩展

涉及文件：internal/config/config.go

施工点：

1. 在现有 screenshot 配置下新增字段：
  - engine（string，默认 cdp）
  - extension（结构体）
2. extension 子结构建议字段：
  - enabled bool
  - listen_addr string
  - pairing_required bool
  - token_ttl_seconds int
  - task_timeout_seconds int
  - max_concurrency int
  - fallback_to_cdp bool
3. 在 ResolveEnv 对应流程中，补齐 listen_addr 等字符串字段解析。
4. 在默认值装配逻辑中补齐兜底：
  - engine 默认 cdp
  - listen_addr 默认 127.0.0.1:19451
  - token_ttl_seconds 默认 600
  - task_timeout_seconds 默认 30
  - max_concurrency 默认 5

函数级检查点：

1. Load/Init 后，空配置也可得到完整默认值。
2. 不影响既有字段：chrome_path、chrome_remote_debug_url、headless 等读取行为。

2. 配置样例同步

涉及文件：configs/config.yaml.example

施工点：

1. 在 screenshot 下新增：
  - engine: cdp
  - extension:
    - enabled
    - listen_addr
    - auth（或平铺 pairing_required/token_ttl_seconds）
    - task_timeout_seconds
    - max_concurrency
    - fallback_to_cdp
2. 在注释中明确：
  - engine=cdp 时沿用当前逻辑
  - engine=extension 时走本地桥接
  - fallback_to_cdp 仅在 extension 失败时生效

3. 路由注册骨架

涉及文件：web/router.go

施工点：

1. 新增桥接管理 API 路由（建议统一前缀 /api/screenshot/bridge）：
  - GET /api/screenshot/bridge/health
  - GET /api/screenshot/bridge/status
  - POST /api/screenshot/bridge/pair
2. 先只注册路由和 handler 函数，不接真实插件逻辑。
3. 路由默认不加限流，沿用已有 trusted request 校验机制。

4. 处理器最小实现（占位可观测）

涉及文件：web/screenshot_bridge_handlers.go（建议新建）

建议新增函数：

1. handleScreenshotBridgeHealth
  - 返回进程内 bridge 服务健康状态（先返回 enabled/listen_addr/engine）。
2. handleScreenshotBridgeStatus
  - 返回配对开关、当前连接数、最近错误（首版可为空）。
3. handleScreenshotBridgePair
  - 首版仅验证请求格式并返回占位 token（或 not_implemented）。

函数级约束：

1. 响应统一 JSON。
2. 错误码先固化：
  - bridge_unavailable
  - pair_required
  - invalid_pair_request
  - not_implemented
3. 不影响现有截图 API 行为。

5. Server 装配点

涉及文件：web/server.go（或实际 Server 初始化文件）

施工点：

1. 在 Server 结构体中新增 bridge 配置快照字段（可选）。
2. 启动时根据 config.Screenshot.Engine 和 config.Screenshot.Extension.Enabled 初始化 bridge 占位状态。
3. 首版不创建后台 goroutine，仅提供状态读取能力。

6. 协议文档落盘

涉及文件：Update_Plan.md（本文件）或 docs 新文档

Day 4 协议最小请求/响应约定：

1. Pair 请求
  - request: client_id, pair_code
  - response: success, token, expires_in
2. Health 响应
  - engine, extension_enabled, listen_addr, ready
3. Status 响应
  - paired_clients, in_flight_tasks, last_error

7. Day 4 回归命令

```bash
go test ./...
go run ./cmd/unimap-web
curl http://127.0.0.1:8448/api/screenshot/bridge/health
```

8. Day 4 提交建议

```bash
git add internal/config/config.go configs/config.yaml.example web/router.go web/screenshot_bridge_handlers.go web/server.go Update_Plan.md
git commit -m "day4: add screenshot bridge config and api skeleton"
```

9. Day 4 完成判定

1. 新配置项可读取且有默认值。
2. bridge 三个 API 可访问并返回结构化 JSON。
3. 现有 CDP 截图链路回归通过。

---

### Day 5：实现本地桥接服务 MVP（Go 侧）

目标：服务端具备“发任务、等结果、超时处理”的闭环能力。

涉及文件与函数：

- internal/service/screenshot_app_service.go
- internal/util/workerpool/workerpool.go

执行项：

1. 定义任务模型（request_id、url、viewport、wait 策略、timeout）。
2. 实现请求-响应关联、超时与重试。
3. 实现并发控制与队列管理。

当日产出：

- Bridge MVP（可与模拟客户端联调）。

验收标准：

- 本地模拟客户端可完成单任务请求/回传。

#### Day 5 函数级施工单

目标：完成桥接服务端 MVP，实现任务入队、执行、回传、超时与重试，不接真实浏览器插件也可联调。

1. 新增桥接域模型与接口

建议新建文件：internal/screenshot/bridge_types.go

建议新增类型：

1. BridgeTask
  - RequestID string
  - URL string
  - BatchID string
  - ViewportWidth int
  - ViewportHeight int
  - WaitStrategy string
  - Timeout time.Duration
2. BridgeResult
  - RequestID string
  - Success bool
  - ImagePath string
  - Error string
  - DurationMs int64
3. BridgeClient（抽象对端通信）
  - SubmitTask(ctx, task) error
  - AwaitResult(ctx, requestID) (BridgeResult, error)

说明：

1. Day 5 先定义最小字段集，避免提前引入复杂协议。
2. RequestID 必须全局唯一，建议使用 time.Now().UnixNano + 随机后缀。

2. 新增桥接服务执行器

建议新建文件：internal/screenshot/bridge_service.go

建议新增结构体：

1. BridgeService
  - client BridgeClient
  - queue chan BridgeTask
  - results sync.Map（requestID -> chan BridgeResult）
  - maxConcurrency int
  - retry int
  - taskTimeout time.Duration

建议新增函数：

1. NewBridgeService(client BridgeClient, maxConcurrency int, taskTimeout time.Duration) *BridgeService
2. Start(ctx context.Context)
3. Stop()
4. Submit(ctx context.Context, task BridgeTask) (BridgeResult, error)
5. executeTask(ctx context.Context, task BridgeTask) (BridgeResult, error)

函数级要求：

1. Submit 必须具备 context 取消能力。
2. executeTask 内部统一处理超时、重试与错误归一化。
3. Start 启动固定 worker 数量，worker 从 queue 消费任务。

3. 与现有 workerpool 的接入策略

涉及文件：internal/util/workerpool/workerpool.go

执行策略（二选一，推荐 A）：

1. A 方案：BridgeService 自建 worker（推荐）
  - 不修改现有 workerpool，避免影响搜索任务。
2. B 方案：复用 workerpool.Task
  - 若复用，新增 BridgeTaskAdapter 实现 Execute() error。

Day 5 推荐结论：

1. 采用 A 方案，隔离风险。
2. 保持 workerpool.go 无改动或仅新增注释。

4. 应用层接入点

涉及文件：internal/service/screenshot_app_service.go

函数级改造：

1. CaptureBatchURLs
  - 当 engine=extension 时，调用 bridge service 执行批量任务。
  - 返回结构继续沿用 BatchURLsResponse。
2. CaptureSearchEngineResult
  - Day 5 可先保留 CDP 路径，或仅加 extension 分支占位返回 not_implemented。
3. CaptureTargetWebsite
  - 与上同，先完成批量路径闭环优先。

说明：

1. Day 5 重点是桥接执行能力，不要求覆盖全部截图入口。
2. 优先打通 batch URLs，因为最容易做并发与超时验证。

5. 新增桥接联调处理器（模拟插件）

涉及文件：web/screenshot_bridge_handlers.go

建议新增函数：

1. handleScreenshotBridgeMockResult（仅开发环境）
  - 接收 request_id 与结果，写回 bridge results map。
2. handleScreenshotBridgeStatus
  - 增加 in_flight_tasks、queue_len、worker_count 字段。

安全要求：

1. 仅允许 loopback 来源调用 mock 接口。
2. 可通过配置开关启用 mock 接口，默认关闭。

6. 错误码与重试策略固化

建议统一错误码：

1. bridge_submit_failed
2. bridge_timeout
3. bridge_result_not_found
4. bridge_task_canceled
5. bridge_internal_error

重试策略建议：

1. 仅对可重试错误重试（超时、瞬时连接失败）。
2. 默认重试 1 次，间隔 200ms。
3. 业务错误（参数非法、拒绝执行）不重试。

7. Day 5 函数级验收清单

1. Submit 在超时场景可返回 bridge_timeout。
2. 并发 20 个 URL 时队列可稳定消费，不 panic。
3. 上下文取消可中断等待中的任务。
4. 批量响应 success/failed 统计正确。

8. Day 5 回归与压测命令

```bash
go test ./...
go run ./cmd/unimap-web
curl -X POST http://127.0.0.1:8448/api/screenshot/batch-urls -H "Content-Type: application/json" -d '{"urls":["https://example.com","https://example.org"],"batch_id":"bridge_day5","concurrency":5}'
```

9. Day 5 提交建议

```bash
git add internal/screenshot/bridge_types.go internal/screenshot/bridge_service.go internal/service/screenshot_app_service.go web/screenshot_bridge_handlers.go Update_Plan.md
git commit -m "day5: implement screenshot bridge service mvp with queue timeout retry"
```

10. Day 5 完成判定

1. BridgeService 可独立运行并处理任务生命周期。
2. batch-urls 在 extension 模式下可通过 mock 闭环返回结果。
3. 不影响 cdp 模式既有截图结果。

---

### Day 6：开发浏览器插件 MVP（Chrome/Edge MV3）

目标：插件可执行最小可用截图能力。

涉及内容：

- 新建插件工程目录（建议：web/extension-screenshot 或 tools/extension-screenshot）。

执行项：

1. 实现配对握手与会话令牌。
2. 实现任务消费：打开/复用标签页、等待稳定、截图、回传。
3. 首版实现可视区截图（全页拼接放二期）。

当日产出：

- 开发者模式可加载插件。
- 与桥接服务完成一次端到端截图。

验收标准：

- 输入 URL 可成功回传截图结果。

#### Day 6 函数级施工单

目标：完成浏览器插件 MVP（MV3），具备配对、任务执行、截图回传能力，并可与 Day 5 桥接服务联调。

1. 插件工程目录与文件骨架

建议目录：tools/extension-screenshot/

建议文件：

1. manifest.json
2. src/background.js
3. src/content.js（可选，首版可不依赖）
4. src/pairing.js
5. src/capture.js
6. src/api.js
7. src/storage.js
8. README.md

2. manifest.json 施工点（MV3）

建议关键项：

1. manifest_version: 3
2. background.service_worker: src/background.js
3. permissions:
  - tabs
  - scripting
  - storage
  - activeTab
4. host_permissions:
  - http://127.0.0.1/*
  - http://localhost/*
5. action.default_title: Unimap Screenshot Bridge

最小权限原则：

1. 首版不申请不必要权限（downloads、webRequest 等暂不加）。
2. host_permissions 仅限本地桥接地址。

3. 配对流程函数级拆分

涉及文件：tools/extension-screenshot/src/pairing.js

建议函数：

1. requestPairToken(clientId, pairCode)
  - 调用 POST /api/screenshot/bridge/pair。
2. saveSessionToken(token, expiresIn)
  - token 与过期时间写入 chrome.storage.local。
3. loadSessionToken()
  - 启动时读取 token。
4. isTokenExpired(expireAt)
  - 判定 token 是否过期。

函数级要求：

1. token 过期时自动触发重新配对。
2. 所有配对失败返回标准错误对象：code/message。

4. 任务拉取与执行主循环

涉及文件：tools/extension-screenshot/src/background.js

建议函数：

1. startBridgeLoop()
  - 周期轮询桥接任务（例如每 500ms 一次）。
2. pollTaskOnce()
  - 调用桥接服务获取待执行任务（如 GET /status 或 /tasks/next）。
3. handleTask(task)
  - 统一封装任务生命周期。
4. reportTaskResult(result)
  - 回传执行结果。

执行顺序建议：

1. 读取 token。
2. 拉取任务。
3. 校验任务参数。
4. 执行截图。
5. 回传结果。

5. 截图执行函数级拆分

涉及文件：tools/extension-screenshot/src/capture.js

建议函数：

1. ensureTab(url)
  - 复用同 URL 标签页或新建标签页。
2. waitForPageReady(tabId, strategy, timeoutMs)
  - 支持 load / delay 两种策略（首版）。
3. captureVisible(tabId)
  - 使用 chrome.tabs.captureVisibleTab 捕获可视区。
4. normalizeImagePayload(dataUrl, requestId)
  - 转换成桥接服务可接受的返回格式。

函数级要求：

1. waitForPageReady 超时必须返回 plugin_timeout。
2. captureVisible 失败返回 plugin_capture_failed。
3. 结果必须包含 request_id 与 duration_ms。

6. API 访问封装

涉及文件：tools/extension-screenshot/src/api.js

建议函数：

1. apiGet(path, token)
2. apiPost(path, body, token)
3. buildHeaders(token)

要求：

1. 统一附带 Authorization: Bearer <token>。
2. 非 2xx 响应统一解析为 {code, message, status}。

7. 状态持久化与诊断

涉及文件：tools/extension-screenshot/src/storage.js

建议函数：

1. saveRuntimeState(state)
2. loadRuntimeState()
3. saveLastError(err)

建议状态字段：

1. paired_at
2. token_expire_at
3. last_task_id
4. last_success_at
5. last_error

8. 联调协议字段最小集

任务下发字段（bridge -> extension）：

1. request_id
2. url
3. timeout_ms
4. wait_strategy
5. viewport（可选，首版可忽略）

结果回传字段（extension -> bridge）：

1. request_id
2. success
3. image_data（data URL 或临时文件引用）
4. error_code
5. error_message
6. duration_ms

9. Day 6 本地联调步骤

1. 浏览器加载 unpacked 插件目录。
2. 调用配对接口获取 token。
3. 触发一条 batch-urls 任务。
4. 观察插件后台日志与 bridge status。
5. 校验结果是否写入 screenshots 目录并返回 API。

10. Day 6 回归命令

```bash
go run ./cmd/unimap-web
curl http://127.0.0.1:8448/api/screenshot/bridge/status
curl -X POST http://127.0.0.1:8448/api/screenshot/batch-urls -H "Content-Type: application/json" -d '{"urls":["https://example.com"],"batch_id":"ext_day6","concurrency":1}'
```

11. Day 6 提交建议

```bash
git add tools/extension-screenshot/ Update_Plan.md
git commit -m "day6: add mv3 extension mvp for screenshot bridge"
```

12. Day 6 完成判定

1. 插件可完成配对并持久化 token。
2. 插件可消费至少 1 条任务并回传结果。
3. 与 Day 5 bridge MVP 联调通过。
4. 不影响 cdp 模式截图能力。

---

### Day 7：接入 ExtensionProvider（双引擎并存）

目标：将插件链路正式接入统一截图接口。

涉及文件与函数：

- internal/service/screenshot_app_service.go
- internal/screenshot/manager.go（保留 CDP 实现）
- cmd/unimap-gui/main.go

执行项：

1. 新增 ExtensionProvider。
2. 增加引擎选择器：engine=extension 时走桥接服务。
3. 配置 fallback_to_cdp 时失败自动降级。

当日产出：

- 双引擎可切换。

验收标准：

- 同一请求在 cdp/extension 下均可执行。

#### Day 7 函数级施工单

目标：将 ExtensionProvider 正式接入统一截图接口，完成引擎选择与失败回退（fallback_to_cdp）。

1. ExtensionProvider 实现落地

涉及文件：internal/screenshot/provider_extension.go（建议新建）

建议结构体：

1. type ExtensionProvider struct
  - bridge *BridgeService
  - baseDir string
2. 构造函数：NewExtensionProvider(bridge *BridgeService, baseDir string)

建议实现方法（对齐 Provider 接口）：

1. CaptureSearchEngineResult
2. CaptureTargetWebsite
3. CaptureBatchURLs
4. GetScreenshotDirectory

函数级要求：

1. 所有方法统一将 bridge 返回结果转换为现有路径语义。
2. 错误统一归一为可判定错误码文本（用于 fallback 判定）。

2. Provider 选择器（引擎路由）

涉及文件：internal/service/screenshot_app_service.go

建议新增字段：

1. cdpProvider screenshot.Provider
2. extProvider screenshot.Provider
3. engine string
4. fallbackToCDP bool

建议新增函数：

1. selectProvider() screenshot.Provider
  - engine=extension 时优先返回 extProvider
  - 其他情况返回 cdpProvider
2. withFallback(exec func(p screenshot.Provider) error) error
  - extension 失败且 fallbackToCDP=true 时自动改走 cdpProvider

函数级改造点：

1. CaptureSearchEngineResult
  - 先 selectProvider 执行
  - 若 extension 失败且允许回退，重试 cdpProvider
2. CaptureTargetWebsite
  - 同上
3. CaptureBatchURLs
  - 对每个任务执行 provider
  - 可选策略：批次级回退或单任务回退（推荐单任务回退）

3. fallback 判定规则固化

涉及文件：internal/service/screenshot_app_service.go

建议新增函数：

1. isFallbackEligible(err error) bool

建议可回退错误：

1. bridge_unavailable
2. bridge_timeout
3. bridge_submit_failed
4. plugin_offline

建议不可回退错误：

1. 参数错误（invalid_url / missing_parameters）
2. 业务拒绝错误（plugin_rejected）

4. 初始化装配修改

涉及文件：cmd/unimap-gui/main.go

施工点：

1. 保留 buildScreenshotManager(cfg) 结果作为 cdpProvider 输入。
2. 在 bridge 可用时创建 extProvider。
3. 调用新的应用层构造函数（或 setter）注入：
  - cdpProvider
  - extProvider
  - engine
  - fallbackToCDP

装配顺序建议：

1. 先初始化 cdpProvider（保证兜底）。
2. 再尝试初始化 extProvider（失败不阻断启动）。

5. Web 层行为保持稳定

涉及文件：web/screenshot_handlers.go

执行原则：

1. 不新增 handler 参数。
2. 不修改已有响应字段结构。
3. 在日志中新增 engine_used 与 fallback_used 字段（便于排障）。

6. 配置读取与默认行为

涉及文件：internal/config/config.go

函数级要求：

1. engine 为空时默认 cdp。
2. engine=extension 且 extProvider 未就绪时：
  - fallback_to_cdp=true：自动走 cdp 并记录 warn。
  - fallback_to_cdp=false：返回 bridge_unavailable。

7. Day 7 回归验证矩阵

1. 配置 engine=cdp：所有截图接口正常。
2. 配置 engine=extension 且 bridge 正常：走 extension。
3. 配置 engine=extension 且 bridge 故障 + fallback=true：自动回退 cdp。
4. 配置 engine=extension 且 bridge 故障 + fallback=false：返回 bridge_unavailable。

8. Day 7 回归命令

```bash
go test ./...
go run ./cmd/unimap-web
curl -X POST http://127.0.0.1:8448/api/screenshot/target -H "Content-Type: application/json" -d '{"url":"https://example.com"}'
curl -X POST http://127.0.0.1:8448/api/screenshot/batch-urls -H "Content-Type: application/json" -d '{"urls":["https://example.com"],"concurrency":2}'
```

9. Day 7 提交建议

```bash
git add internal/screenshot/provider_extension.go internal/service/screenshot_app_service.go cmd/unimap-gui/main.go web/screenshot_handlers.go internal/config/config.go Update_Plan.md
git commit -m "day7: integrate extension provider with engine routing and cdp fallback"
```

10. Day 7 完成判定

1. 引擎选择逻辑可按配置生效。
2. fallback_to_cdp 行为符合预期。
3. 现有 API 响应结构无破坏性变化。
4. cdp 路径在 extension 不可用时仍可稳定运行。

---

### Day 8：改造查询自动截图链路

目标：auto capture 不再强依赖 CDP 在线。

涉及文件与函数：

- web/query_handlers.go
  - runBrowserQueryMode
- internal/service/query_app_service.go
  - RunBrowserQueryAsync
  - checkCDPStatus（改造成 engine 可用性检查）

执行项：

1. 将 auto capture 调用改为统一截图接口。
2. 将 CDP 在线性校验改成“当前引擎可用性校验”。
3. 保留 AutoCapturedPaths 输出结构。

当日产出：

- extension 模式可支持查询自动截图。

验收标准：

- autoCapture 打开时，extension 模式可产出截图路径。

#### Day 8 函数级施工单

目标：将查询自动截图从 CDP 在线强依赖改为引擎可用性驱动。

1. 查询入口参数收敛

涉及文件：web/query_handlers.go

函数：runBrowserQueryMode

施工点：

1. 保留原有返回结构字段：autoCapture、autoCaptureQueryID、autoCapturedPaths、autoCaptureErrors。
2. 新增引擎参数透传：
  - screenshotEngine
  - fallbackToCDP
3. 移除对 cdpBaseURL 的硬绑定心智，改为传递截图应用服务能力对象。

2. 应用层异步查询改造

涉及文件：internal/service/query_app_service.go

函数：RunBrowserQueryAsync

施工点：

1. 将签名中的 cdpBaseURL 字段替换为引擎可用性检查函数或截图应用服务依赖。
2. autoCaptureEnabled=true 时，调用统一截图接口执行自动截图。
3. 将截图失败信息写入 outcome.AutoCaptureErrors，不阻断主查询结果返回。

函数：checkCDPStatus

处理策略：

1. 保留函数用于 cdp 模式探活。
2. 新增 checkScreenshotEngineStatus(engine) 统一入口。
3. engine=extension 时调用 bridge 状态检查，engine=cdp 时复用 checkCDPStatus。

3. 自动截图任务执行策略

涉及文件：internal/service/query_app_service.go

建议新增函数：

1. captureSearchResultsForOutcome(...)
2. appendAutoCaptureError(outcome, engine, err)

函数级要求：

1. 单引擎截图失败不影响其他引擎继续执行。
2. 错误记录包含 engine 名称与错误码。
3. 路径映射仍写入 AutoCapturedPaths[engine]。

4. 回退行为对齐 Day 7

涉及文件：internal/service/screenshot_app_service.go

要求：

1. 若 extension 引擎失败且 fallback_to_cdp=true，则自动截图分支可回退。
2. 回退后的产物路径仍按原 engine 键回填，避免前端展示兼容问题。

5. Day 8 回归命令

```bash
go test ./...
go run ./cmd/unimap-web
curl -X GET "http://127.0.0.1:8448/api/query?query=app%3D%22nginx%22&engine=fofa"
```

6. Day 8 提交建议

```bash
git add web/query_handlers.go internal/service/query_app_service.go internal/service/screenshot_app_service.go Update_Plan.md
git commit -m "day8: decouple auto-capture from cdp health and use engine-aware checks"
```

7. Day 8 完成判定

1. auto capture 在 extension 模式下可工作。
2. 主查询流程不因截图失败中断。
3. cdp 模式行为保持兼容。

---

### Day 9：Cookie/登录态语义重构

目标：插件模式优先使用浏览器真实登录态，减少后端 Cookie 注入。

涉及文件与函数：

- web/cookie_handlers.go
  - handleSaveCookies
  - handleVerifyCookies
  - applyCookiesFromRequest
- internal/screenshot/manager.go
  - ValidateSearchEngineResult
  - loadPageContent

执行项：

1. extension 模式下，Cookie 接口改为“提示与兼容”，不再作为强依赖。
2. cdp 模式继续保留现有 Cookie 行为。
3. 验证逻辑按引擎分支处理（避免误报）。

当日产出：

- 双模式 Cookie 语义清晰。

验收标准：

- extension 模式在用户已登录浏览器中可直接截图受登录保护页面。

#### Day 9 函数级施工单

目标：将 Cookie 与登录态语义改造成按引擎分支处理，extension 以浏览器真实会话为主。

1. Cookie 保存接口语义调整

涉及文件：web/cookie_handlers.go

函数：handleSaveCookies

施工点：

1. engine=extension 时，保存操作改为兼容提示模式：
  - 返回 success=true
  - 增加 message: extension mode uses browser session
2. engine=cdp 时维持现有 SetCookies 行为。

2. Cookie 验证接口分支

涉及文件：web/cookie_handlers.go

函数：handleVerifyCookies

施工点：

1. cdp 模式：沿用 screenshotMgr.ValidateSearchEngineResult。
2. extension 模式：调用 bridge 健康状态 + 目标页访问检查（通过 extension provider）。
3. 输出结构保持兼容：ok、title、hint、error。

3. 请求参数应用逻辑分支

涉及文件：web/cookie_handlers.go

函数：applyCookiesFromRequest

施工点：

1. cdp 模式允许继续写入 config cookies/proxy。
2. extension 模式忽略 cookie 注入，仅保留 proxy 参数作为可选桥接配置。
3. 增加日志字段：engine、cookie_apply_mode。

4. 管理器验证函数边界收口

涉及文件：internal/screenshot/manager.go

函数：ValidateSearchEngineResult、loadPageContent

处理原则：

1. 仅服务 cdp 模式，不再承担 extension 验证。
2. 在函数注释中明确 scope，防止后续误复用。

5. 错误提示统一

建议错误码：

1. extension_session_required
2. extension_not_paired
3. cdp_cookie_missing

6. Day 9 回归命令

```bash
go test ./...
go run ./cmd/unimap-web
curl -X POST http://127.0.0.1:8448/api/cookies/verify -d "query=test&engines=fofa"
```

7. Day 9 提交建议

```bash
git add web/cookie_handlers.go internal/screenshot/manager.go Update_Plan.md
git commit -m "day9: split cookie semantics by engine and prefer extension session"
```

8. Day 9 完成判定

1. extension 模式下无需后端 Cookie 注入也可验证登录态能力。
2. cdp 模式 Cookie 功能无回归。
3. 接口响应结构保持兼容。

---

### Day 10：可观测性与诊断能力补齐

目标：上线前具备可定位问题的观测能力。

涉及文件与函数：

- web/screenshot_handlers.go
- web/cdp_handlers.go（复用状态接口风格）

执行项：

1. 增加 bridge metrics：成功率、耗时、超时率、重试率。
2. 增加桥接状态 API：插件在线、最近错误、队列深度、配对状态。
3. 日志加入 request_id 全链路追踪。

当日产出：

- 可诊断 API + 观测指标。

验收标准：

- 能明确区分：插件离线、认证失败、任务超时、页面异常。

#### Day 10 函数级施工单

目标：补齐可观测与诊断闭环，做到问题可识别、可定位、可复现。

1. Metrics 打点扩展

涉及文件：internal/metrics（实际实现文件）

建议新增指标：

1. screenshot_bridge_requests_total{engine,status}
2. screenshot_bridge_duration_seconds{engine}
3. screenshot_bridge_retries_total
4. screenshot_bridge_timeouts_total

函数级要求：

1. 在 extension provider 每次执行路径打点。
2. fallback 发生时额外计数 screenshot_bridge_fallback_total。

2. Handler 侧诊断接口增强

涉及文件：web/screenshot_bridge_handlers.go

函数：handleScreenshotBridgeStatus

新增返回字段：

1. paired_clients
2. in_flight_tasks
3. queue_len
4. worker_count
5. last_error
6. last_error_at

函数：handleScreenshotBridgeHealth

新增返回字段：

1. ready
2. engine
3. extension_enabled
4. bridge_connected

3. 全链路日志字段统一

涉及文件：web/screenshot_handlers.go、internal/service/screenshot_app_service.go

要求：

1. 每次截图日志带 request_id。
2. 记录 engine_selected、fallback_used、duration_ms。
3. 错误日志统一输出 error_code。

4. 诊断聚合函数

建议新增函数：

1. BuildBridgeDiagnosticSnapshot() map[string]interface{}

用途：

1. 提供给 status/health handler 共用，避免重复拼装。

5. Day 10 回归命令

```bash
go test ./...
go run ./cmd/unimap-web
curl http://127.0.0.1:8448/api/screenshot/bridge/status
curl http://127.0.0.1:8448/api/screenshot/bridge/health
```

6. Day 10 提交建议

```bash
git add internal/metrics/ web/screenshot_bridge_handlers.go web/screenshot_handlers.go internal/service/screenshot_app_service.go Update_Plan.md
git commit -m "day10: add bridge observability and diagnostic endpoints"
```

7. Day 10 完成判定

1. 可以通过 API 快速判定桥接当前状态。
2. 关键故障类型可区分并定位。
3. 回退行为在指标与日志中可观测。

---

### Day 11：灰度切换与回退演练

目标：确保插件链路可控上线。

涉及文件与函数：

- configs/config.yaml.example
- internal/config/config.go

执行项：

1. 准备三套配置模板：
   - cdp-only
   - extension-with-fallback
   - extension-only
2. 演练回退：插件停机后自动/手动切回 cdp。
3. 记录切换时延与失败场景。

当日产出：

- 灰度与回退操作手册。

验收标准：

- 可在 15 分钟内完成从 extension 回退到 cdp。

#### Day 11 函数级施工单

目标：形成可执行的灰度切换与回退演练手册，并完成一次完整演练。

1. 配置模板固化

涉及文件：configs/config.yaml.example、configs/config.yaml（本地环境）

新增模板片段：

1. cdp-only
2. extension-with-fallback
3. extension-only

要求：

1. 每个模板标注适用场景与风险。
2. 模板内必须包含 engine 与 fallback_to_cdp。

2. 配置热切换流程

涉及文件：internal/config/config.go、web/server.go（若支持 reload）

施工点：

1. 明确是否支持运行时 reload。
2. 若支持 reload，提供函数级步骤：
  - reload config
  - re-init provider
  - health verify
3. 若不支持 reload，提供零停机最短重启路径。

3. 回退演练脚本

建议新增文件：scripts/rollback_extension_to_cdp.sh 与 scripts/rollback_extension_to_cdp.ps1

脚本步骤：

1. 校验当前 engine=extension。
2. 修改配置切回 cdp。
3. 重启服务或触发 reload。
4. 调用 health/status 验证。

4. 演练记录项

建议新增文件：archive/ROLLBACK_DRILL_YYYY-MM-DD.md

记录：

1. 开始时间
2. 完成时间
3. 演练结果
4. 问题与修正项

5. Day 11 回归命令

```bash
go run ./cmd/unimap-web
curl http://127.0.0.1:8448/api/screenshot/bridge/health
curl -X POST http://127.0.0.1:8448/api/screenshot/target -H "Content-Type: application/json" -d '{"url":"https://example.com"}'
```

6. Day 11 提交建议

```bash
git add configs/config.yaml.example internal/config/config.go scripts/rollback_extension_to_cdp.sh scripts/rollback_extension_to_cdp.ps1 archive/ROLLBACK_DRILL_*.md Update_Plan.md
git commit -m "day11: finalize gray release and rollback drill playbook"
```

7. Day 11 完成判定

1. 三套配置模板可直接使用。
2. 回退演练在 15 分钟内完成。
3. 演练记录可追溯。

---

### Day 12：发布准备与封板

目标：形成可发布版本与运维资料。

涉及文件：

- README.md
- Update_Plan.md（本文件持续更新状态）

执行项：

1. 补充部署文档：插件安装、配对、故障排查、引擎切换。
2. 输出发布检查清单与回滚检查清单。
3. 完成最终回归与冒烟。

当日产出：

- 发布说明。
- 运维手册。
- 回滚手册。

验收标准：

- 新人可按文档独立完成安装、配对、截图、回退。

#### Day 12 函数级施工单

目标：完成发布封板，交付可运行文档与最终验收记录。

1. 文档收敛

涉及文件：README.md、Update_Plan.md

施工点：

1. README 新增章节：
  - 插件安装
  - 配对流程
  - 引擎切换
  - 常见故障与回退
2. Update_Plan.md 增加最终执行状态表（Done/In Progress/Todo）。

2. 运行手册与故障手册

建议新增文件：docs/OPS_SCREENSHOT_EXTENSION.md

必须包含：

1. 日常巡检项
2. 故障定位顺序
3. 关键 API 检查命令
4. 回退触发条件

3. 最终回归矩阵

建议新增文件：archive/RELEASE_CHECKLIST_YYYY-MM-DD.md

回归项：

1. cdp-only 全量通过
2. extension-with-fallback 全量通过
3. extension-only 关键路径通过
4. fallback 场景通过

4. 封板规则

要求：

1. 封板后仅允许 P0 缺陷修复。
2. 任何代码改动需附回归结论。

5. Day 12 回归命令

```bash
go test ./...
go run ./cmd/unimap-web
curl http://127.0.0.1:8448/health
curl http://127.0.0.1:8448/api/screenshot/bridge/status
```

6. Day 12 提交建议

```bash
git add README.md docs/OPS_SCREENSHOT_EXTENSION.md archive/RELEASE_CHECKLIST_*.md Update_Plan.md
git commit -m "day12: release hardening docs runbook and final checklist"
```

7. Day 12 完成判定

1. 新成员可按文档独立完成端到端流程。
2. 发布与回退路径文档化且可执行。
3. 项目达到可发布状态。

## 四、每日固定执行规范

1. 每天至少一个可回滚提交，建议格式：
   - dayX: screenshot refactor - <topic>
2. 每天结束执行：
   - go test ./...
   - go run ./cmd/unimap-web
3. 每天记录指标：
   - 成功率
   - 平均耗时与 P95
   - 失败原因分布（超时/认证/页面异常）

## 五、里程碑检查点

1. M1（Day 3 结束）：接口解耦完成，行为不变。
2. M2（Day 7 结束）：双引擎并存可切换。
3. M3（Day 10 结束）：观测与诊断能力可用。
4. M4（Day 12 结束）：可灰度发布、可快速回退。
5. M5（Day 13 结束）：代理池能力上线（P0）。
6. M6（Day 14 结束）：端口扫描 + CDN 排除能力上线（P0）。
7. M7（Day 15 结束，可选）：分布式节点能力可用（P1）。

## 六、风险与对策

1. 插件生命周期波动
   - 对策：心跳、自动重连、状态诊断接口。
2. 页面复杂导致截图不稳定
   - 对策：等待策略可配置，首版仅可视区截图。
3. 浏览器版本差异
   - 对策：启动时做能力检测并输出提示。
4. 插件未安装/被禁用
   - 对策：桥接健康检查 + fallback_to_cdp。
5. 代理源质量波动导致结果不稳定
  - 对策：代理健康评分、失败熔断、按场景回退直连或更换代理。
6. CDN 识别误判导致漏扫或误扫
  - 对策：多信号判定（ASN/CNAME/响应头/IP段），支持人工覆写白名单。
7. 分布式节点不稳定或数据不一致（可选模块）
  - 对策：节点心跳超时摘除、任务幂等、结果签名与时间窗校验。

## 七、建议实施顺序（最小风险）

1. Day 2-3：先做接口解耦。
2. Day 4-5：再做桥接协议与服务端闭环。
3. Day 6-7：接入插件并完成双引擎。
4. Day 8-12：完成自动截图、观测、灰度、发布。
5. Day 13-14（必选 P0）：先交付代理池与端口扫描（含 CDN 排除）。
6. Day 15（可选 P1）：按资源投入推进分布式节点与网络信息采集。
