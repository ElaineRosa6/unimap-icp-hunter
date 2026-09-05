# UniMap 当前剩余工作清单

> 发布与审查入口更新：2026-09-06；已核验代码 `f6b3689`，见 [发布核验快照](CURRENT_STATUS_2026-09-06.md) 与 [项目审查记录](PROJECT_REVIEW_2026-09-06.md)。以下能力矩阵保留原有日期化证据（2026-08-21），本轮未新增真实浏览器或云端业务验收，不据 CI 成功勾选这些待办。
> [2026-08-20 推进计划书](AGENT_CONTINUATION_PLAN_2026-08-20.md)。
> 08-02 清单与 07-23 按旬计划只表达当时事实。历史审计、E2E 和旧计划若与当前代码冲突，以代码、
> API/Runbook 和上述计划书为准。完成项必须同时满足代码、测试和所声明后端的真实验收，
> 不能因存在选择器、接口或单元测试就标记完成。

## 1. 引擎能力事实矩阵

| 引擎 | 稳定 Web UI | API 适配 | Bridge 结构化采集实测 | 插件活页 DOM（2026-08-21） | CDP 结构化采集实测 |
|---|---|---|---|---|---|
| FOFA | 已接入 | 已有适配器；历史另有真实 API 证据 | 2026-08-02 复验通过（10） | 10 条（`.hsxa-meta-data-item`）；host/端口拆分后未再活抽 | **2026-08-21 通过**（10 + PNG）。当天傍晚又改 ExtractJS，**未再跑 CDP** |
| Hunter | 已接入 | 已有适配器 | 2026-08-02 复验通过（10） | 13 条含 ICP 脏行；跳过逻辑后未再活抽 | 2026-07-29 通过（10 + 截图）。ExtractJS 当天有补丁，**未再跑 CDP** |
| ZoomEye | 已接入 | 已有适配器 | 2026-08-02 复验通过（10） | `.org` 今日可开，10 条（等 loading）；host:port 修复后未再活抽 | **2026-08-21 受限**（当时 `.org` 521 / `.ai` SSO）。代码 URL 仍 `.org` |
| Quake | 已接入 | 已有适配器 | 2026-08-02 复验通过（10） | 英文页 10 条；聚合块/`--` 修复后未再活抽 | 2026-07-29 通过（10 + 截图）。ExtractJS 当天有补丁，**未再跑 CDP** |
| Shodan | 已接入 | 已有适配器 | 2026-08-02 通过（10） | 10 条，`.l-search-results .result` | **2026-08-21 通过**（校准后 10 + PNG） |
| Censys | 已接入 | 已完成，2026-06-27 API 实机验证通过 | 2026-08-02 通过（9） | 宽选择器 102 条误伤；已改 `a[href*='/hosts/']`，**新选择器未活抽** | 挑战识别 + Bridge fallback 通过（9）。新 ExtractJS **未再跑 CDP** |
| DayDayMap | 已接入 | 已完成，2026-06-27 API 实机验证通过 | 2026-08-02 重试通过（1） | **0.4.18 复验 10 条**，`tr.ant-table-row`，总数 2,163,417,935 | 08-02 Bridge→CDP 通过（10）。当天改 `ant-table-row` 后 **未再跑 CDP** |

说明：

- Censys、DayDayMap 的 Extension DOM 选择器和 Go DOM `ExtractJS` 已于 2026-08-01 完成；
- Bridge 和 CDP 搜索 URL 构造已支持全部七个引擎（2026-08-01）；
- Censys、DayDayMap 的 L1 Network 解析器已添加（2026-08-01），单元测试通过；
- `live_bridge_e2e` 白名单已扩展为七个引擎（2026-08-01）；
- 2026-08-02 产品范围升级为七引擎稳定 Web UI；Censys、DayDayMap 缺少 API 凭据时注册明确的 Web-only adapter；
- 普通网页 CDP 截图通过不等于测绘引擎 CDP 结构化采集通过。
- 2026-08-21 插件活页与 CDP 分列见 [PLUGIN_CDP_STATUS_2026-08-21.md](PLUGIN_CDP_STATUS_2026-08-21.md)。插件 0.4.18 不是 Bridge 闭环；改完的 ExtractJS 必须再跑 `CollectAndCapture` 才能更新 CDP 列。
- 2026-08-01：七引擎均已有 L1 Network 解析器（Censys/DayDayMap 新增）和 L3 DOM ExtractJS；
  FOFA DOM 选择器已加固（4 级行选择器回退）；Censys/DayDayMap 搜索 URL 构造、
  Extension 选择器、Bridge 白名单和设置页配置均已接通。

## 1.1 云端试运行（2026-08-20）

日更闭环已在转，不是待办。机内待办（执行见计划书 A/B 波）：

- OPS-1 清旧镜像；**已完成（2026-08-20，盘 73%→43%）**
- OPS-2 对齐 `.env` 与运行配置的 admin token；**已完成（2026-08-20）**
- OPS-3 下次重建写入 git commit
- OPS-5 安全组复核（需控制台）
- BIZ-1 Quake 三任务：用户确认无 key，**保持 disabled**（2026-08-20）
- BIZ-2 ZoomEye / Shodan / Censys：用户确认无可用 key，**不进日更**（2026-08-20）
- BIZ-3 云端截图：**不开**（2026-08-20）
- BIZ-4～BIZ-6 未拍板：FOFA 字段降级、smtp-relay recreate、TLS（默认保持）
- 日更范围冻结：FOFA / Hunter / DayDayMap 查询 → 邮件，ICP → 企微。未补 key 并再次确认前，禁止 enable `quake_*`、禁止新建 ZoomEye/Shodan/Censys 调度、禁止打开 `screenshot.enabled`

## 2. P1：下一阶段必须完成

### RW-01 生产配置持久化与秘密录入

状态：**已完成（2026-07-23）**。

生产 Compose 已改用 `unimap_config:/app/runtime-config` 可写命名卷。入口脚本仅在
`config.yaml` 不存在时从镜像内 `config.prod.yaml` 初始化，权限设为 `0600`；应用通过
`UNIMAP_CONFIG_PATH` 读取运行配置，支持同目录临时文件和原子替换。

完成标准：

- 使用受保护的可写运行配置目录或等价秘密存储；**已完成**
- 不把秘密写入 Git、日志或响应；
- 不可写模式下页面明确提示并禁用保存；**仍待增强**
- 保存、重启、回滚和备份恢复测试通过；**已完成**

阿里云实机已验证：ACR 固定摘要镜像、`unimap_config` 可写卷、`0600` 配置、应用原子保存、
容器重启哈希一致、业务备份、受限配置备份以及“临时改为 auto → 原子恢复 → 重启回到 cdp”。
不可写页面提示属于后续 UX 增强，不再阻断当前可写生产基线。

### RW-02 Quake/Hunter 生产配置与真实 CDP 验收

状态：**已完成（2026-08-02）**。Quake/Hunter CDP、七引擎 Bridge 非空采集、Censys/DayDayMap collect-and-capture、SQLite 单历史和云端飞书通知均已通过。

2026-07-29 验收结果（详见 [CDP_VERIFICATION_2026-07-29.md](CDP_VERIFICATION_2026-07-29.md)）：

- Quake：Cookie 登录态有效，CDP 搜索返回 2.06 亿条结果，
  应用代码路径采集 10 条结构化资产，PNG 截图 375KB，人工确认非登录页；
- Hunter：Cookie 登录态有效，CDP 搜索返回 2696 万条资产，
  应用代码路径采集 10 条结构化资产，历史验收截图为 JPEG 内容，人工确认非登录页；
- 截图回退已修复为统一 PNG 契约：PNG 首次捕获失败时使用 JPEG 捕获，再转码为 PNG 后保存和返回；
- Cookie 已写入 `configs/config.yaml`，Chrome 用户数据目录持久化会话。

2026-07-29 追加：

- Hunter DOM 选择器已校准（列映射修正 + checkbox 自动检测 + status_code/region 提取）；
- 历史 Web 服务 E2E：POST /api/v1/query browser_query=true 返回 200，
  Hunter bridge-collect 采集 2 条资产；当时 Quake Extension DOM 为 0 条；
- Quake L1 已校准到 `/api/search/query_string/quake_service`，解析器兼容当前 `data` 数组响应，
  并于 2026-07-29 使用持久化登录态获得非空真实结果；
- Quake Extension DOM 已修复 `data-clipboard-text` 的 IP 提取并通过真实 Chrome 固定夹具；
- 2026-08-02 使用 Extension 0.4.15 复验，真实 Bridge 在线，Quake `port="80"` 取得
  10 条结构化资产；API 失败但 Browser 非空时现返回 HTTP 200 + `status=partial`；
- SQLite 合并路径由单元测试覆盖（TestRunBrowserQueryAsync_CollectsStructuredAssets）；
- 通知管线已由 Scheduler 后置管线覆盖；云端 `feishu_app` 两次真实测试均返回 HTTP 200。

本项的 Router partial 语义、collect-and-capture、SQLite 合并、通知发送和云端容器重启均已形成验证记录。

`configs/config.prod.yaml` 已显式启用 Quake/Hunter，使用正确 Base URL、QPS 1、30 秒超时；
Compose 已透传可选 `QUAKE_API_KEY`、`HUNTER_API_KEY`。Key 留空时只表示允许进入 Web-only
路径，不代表登录态或 CDP 采集已通过。

逐引擎完成：

1. API 最小查询；
2. Cookie 登录状态；
3. CDP 打开真实结果页；
4. 非空结构化资产；
5. 结果页 PNG 人工检查；
6. SQLite 合并持久化；
7. 图片通知；
8. 容器重启后会话和结果恢复。

### RW-03 七个稳定引擎 CDP 能力定级

状态：**FOFA/Shodan 08-21 已定级通过；ZoomEye 08-21 受限。** 当天傍晚按活页改了 ExtractJS（含 DayDayMap `ant-table-row`、Censys `/hosts/` 等），**CDP 尚未用新脚本复跑**。插件活页不能替代本项。下一步顺序见 [PLUGIN_CDP_STATUS_2026-08-21.md](PLUGIN_CDP_STATUS_2026-08-21.md) 第 4 节。

现有 CDP URL、Cookie、Network/DOM 和 `collect_and_capture` 代码不能替代真实验证。

完成标准：

- FOFA、Hunter、ZoomEye、Quake、Shodan、Censys、DayDayMap 分别记录通过、受限或不支持；
- 登录页、验证码页、空资产不能报告成功；
- 页面改版导致的 selector/network 失败有明确诊断；
- 形成独立于 Bridge 的 CDP live E2E 和日期化证据。

### RW-04 Censys/DayDayMap 浏览器能力与 UI 决策

状态：**稳定 Web UI 与 Bridge 已完成；DayDayMap CDP 通过，Censys challenge 识别和自动 Bridge fallback 通过（2026-08-02）。**

已完成查询 URL、UQL Web 翻译、Cookie/同源 Web Storage 交接、Bridge/CDP L1/L3、截图、前端选择、设置页、调度、通知管线和 live E2E。缺少 API 凭据时注册明确的 Web-only adapter，其可执行性由 ScreenshotRouter 健康状态决定。

当前结果：DayDayMap 经 Bridge Web Storage 交接与受控 loopback SOCKS5 出口取得 10 条 CDP 资产。Censys CDP 到达 Cloudflare 挑战页；系统已结构化识别 `browser_challenge` 并在 `auto`/fallback 下切换 Bridge，取得 9 条资产。详见 [七引擎浏览器验收记录](BROWSER_SEVEN_ENGINE_VERIFICATION_2026-08-02.md)。

### RW-05 Cookie/Profile 无人值守闭环

状态：**核心框架已完成（2026-07-25），真实引擎验收待执行**。

已完成：

- `internal/scheduler/session_health.go`：`SessionHealthTracker` 实现 per-engine 熔断器（closed/open/half-open），连续失败阈值和冷却时间可配置；
- `ClassifyFailureReason()`：将失败分为 6 类（cookie_missing / cookie_expired / login_wall / captcha / page_changed / network）；
- `RecoveryHint()`：每类失败返回中文恢复操作指引；
- `LoginStatusCheckRunner` 已集成 health tracker：记录成功/失败、熔断跳过、输出健康摘要和恢复提示；
- `AllowBrowserTask()`：熔断打开时阻止对应引擎的浏览器任务，但允许登录检查（探测恢复）；
- 单元测试覆盖熔断、冷却、分类、摘要和恢复提示。

2026-07-29 新增：

- AllowBrowserTask() 已接入 QueryRunner.Execute() 浏览器任务路径：熔断打开时跳过对应引擎的浏览器任务，仅允许登录检查探测恢复；
- SessionHealthTracker 改为 web/server.go 中创建的共享实例，LoginStatusCheckRunner 和 QueryRunner 共用同一熔断状态；
- 通知集成已由 Scheduler 后置管线覆盖：LoginStatusCheckRunner 结果文本包含健康摘要和恢复提示，任务启用通知后自动发送到飞书/Webhook。

仍待完成：

- 周期性打开真实结果页验证登录（需要真实引擎账号和 CDP 环境）；
- 更新 Cookie 后的低额度恢复验收。2026-08-17 明确不在 `handleSaveCookies` 内自动跑 CDP：保存只落盘；探测继续用独立的 `GET /api/v1/cookies/login-status`，且仅在操作者主动验证时调用。

## 3. P2：应当收尾

### RW-06 设置页 Base URL 纠正

状态：**已完成（2026-07-23）**。

- Hunter 占位值已与默认 `https://hunter.qianxin.com` 一致；
- Quake 占位值已与默认 `https://quake.360.net/api` 一致；
- 已增加前端契约测试，防止再次偏离 adapter 默认值。

### RW-07 配额功能

当前明确未实现：

- 配额趋势图；
- 自动刷新设置；
- 阈值告警。

入口继续保持禁用，直到服务端存储、调度、告警和 UI 全部完成。

### RW-08 发布与工作区清理

- 状态：**本地门禁、云端发布、回滚和飞书通知已通过（2026-08-02）；仅受控 DNS/页面变化 fixture 待外部输入。**
- 当前代码与第一轮文档变更已审查并提交为 `f9317a3`；**已完成**
- `CLAUDE.md` 已按用户要求保留并同步当前事实；**已完成**
- 根目录 `tmp_speedtest.go` 已迁移为带 `manual_browser_test` 标签的
  `tools/chrome-speedtest` 手动工具；**已完成**
- 推送 `develop` 当前领先远端的本地提交；**已完成（2026-08-04）**
- 发布门禁 `go test -race ./...`、`go vet ./...`、`go build ./...`、Extension 9/9、
  headless SSRF、前端语法与 `git diff --check` 已通过。**已完成**
- 云端安全验收 fixture 已构建（`tools/acceptance-fixture/`）：DNS rebinding 控制 API +
  可变页面 + 私网 sink + Cloudflare DNS 翻转 + Caddy HTTPS + Docker Compose。
  部署到受控服务器后即可运行 `live_dns_e2e` 和 `live_tamper_e2e`。**部署计划：2026-08-02。**

### RW-09 网页巡检确定性收尾

状态：**部分完成（2026-07-24），安全证据截图与云端真实闭环待完成**。

- 手动巡检、定时巡检与基线刷新读取同一套实时 Tamper 配置，并统一使用 Screenshot Manager 的受保护页面加载接口；
- 基线刷新按实际 `saved` 结果统计，不再把业务失败计为成功；
- 历史列表和导出支持 `start_time`、`end_time`，`count` 返回过滤后总数；
- “发现变化 → 证据截图 → 图片通知”仍未启用：逐跳和连接级 SSRF 代码及本地验收已完成，
  但受控云端真实变化、图片送达和重启复验尚未完成；
- 尚需在云端用受控页面完成基线、真实变化、历史、飞书图片送达、恢复和重启后复验。

## 4. P3：可选增强

- ~~历史 API 增加 `start_time`、`end_time`；~~ **已完成（2026-07-24）**
- 提供受控恢复命令或恢复 API，而不只提供备份创建/列表；
- 完成配额趋势与告警后增加长期数据保留策略；
- ARM64 镜像与 Chromium 验收。
- 调度任务通知按事件类型区分渠道（**2026-08-07 记录，暂缓实施**）：
  - 需求：`NotificationConfig` 当前只有单一 `channel_ids`，成功/失败/超时共用同一渠道列表
    （`internal/scheduler/scheduler_types.go`、`scheduler_notify.go` 对所有事件用 `migrateChannelIDs`）。
    期望支持 `success_channel_ids` / `failure_channel_ids` / `timeout_channel_ids` 覆盖字段（为空时回退
    `channel_ids`），`sendNotification` 按 `record.Status` 选择渠道列表；
  - 云端应用（阿里云 12 个任务）：成功推 `['dijia_01_file','dijia_01']`（Excel 文件 + 文本），失败/超时
    只推 `dijia_01`（文本告警，失败无结果文件不必发 file 渠道）；
  - 涉及改动：`internal/scheduler/scheduler_types.go`（字段）、`internal/scheduler/scheduler_notify.go`
    （按事件选渠道）、`web/scheduler_handlers.go`（create/update 校验新字段渠道存在）、
    `web/templates/scheduler.html`（任务表单加每事件渠道多选 + JS 收集/回显）、docs/API.md 同步与单元测试；
  - 状态：已评估改动范围，2026-08-07 用户决定先记录、暂缓实施。

## 5. 本轮已纠正的记录

- GUI 入口 `cmd/unimap-gui` 仍在（`gui` build tag + Fyne）；默认入口为 unimap-web 与 unimap-cli；
- CLI 已完成 Agent 友好化改造：JSON 信封、语义退出码、分页、--format json、quota/config show/help --json；
- CLI 不直接集成 CDP/Bridge，截图通过 Web API 代理；

- Censys、DayDayMap Extension/Bridge 已于 2026-08-02 实测通过，并已更新当前能力矩阵；
- 不再把历史 Bridge 抓取测试当作 CDP 证据；
- 将备份任务改为已提交的第 23 种任务；
- 生产运行配置改为首次初始化的可写命名卷；
- 未配置引擎不再被默认强制启用；Censys/DayDayMap 缺 API 凭据时使用明确的 Web-only adapter；
- Quake/Hunter 生产模板和设置页 Base URL 已统一；
- 当前权威文档统一使用本文件的支持矩阵和待办编号。
- 巡检运行时配置、受保护页面加载接口、时间筛选和真实总数已完成代码收口；变化证据截图因
  云端真实变化与图片送达尚未验收继续保留待办。
- 本地代码基线已提交为 `f9317a3`，2026-08-04 已推送到 `origin/develop`。
- 2026-08-17：空 Key fail-fast、备用 Key 跳过空主键、登录 401、`GET /api/v1/config` 渠道脱敏已合入 `master` `71371f1` 并推送；阿里云试运行机镜像已升到该提交。未做项见 [变更日志 2026-08-17](CHANGELOG.md)。
- 2026-08-20 SSH 机内核查：容器 healthy，`71371f1` 仍在跑；9 个启用任务 08-17 后至 08-20 15:00 共 52 次 success、0 failed；smtp-relay 仍 healthy。执行队列改为 [推进计划书](AGENT_CONTINUATION_PLAN_2026-08-20.md) 的 A–D 波（清镜像、token 对齐、试运行拍板、CDP 定级、证据截图外部输入）。`develop` 落后 `master`，发布基线是 `master`。

## 6. 最终验证（2026-08-02）

- `go test -race ./...`：通过；
- `go vet ./...`：通过；
- `go build ./...`：通过；
- `node --test tools/extension-screenshot/test/*.test.mjs`：9/9 通过；
- `node --check web/static/js/main.js`：通过；
- `git diff --check`：通过。

本轮 Excelize 已升级到 v2.11.0，`govulncheck ./...` 返回 0 个可达漏洞；Quake L1 真实测试、
Quake Extension DOM 固定夹具、headless 私网重定向/子资源和连接前阻断均已通过。全量 race、
vet、默认 build 和前端语法以本轮最终门禁记录为准。七引擎 Bridge、云端发布/回滚和飞书
通知均已通过；云端受控 DNS rebinding 与真实页面变化仍是唯一外部验收边界。
