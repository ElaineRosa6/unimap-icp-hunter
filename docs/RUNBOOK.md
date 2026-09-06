# UniMap 运维 Runbook

> 最后按代码核对：2026-09-04。当前发布基线为 `master/e31e03d`，Go 版本为 `1.26.6`。所有业务 API 使用 `/api/v1/...`；旧 `/api/...` 路径已移除。

## 0. 先确认服务与认证

```powershell
Invoke-RestMethod http://127.0.0.1:8448/health
Invoke-RestMethod http://127.0.0.1:8448/health/ready
Invoke-RestMethod http://127.0.0.1:8448/health/live
```

生产环境如启用了认证，按部署方式带上会话、API Key 或管理令牌。Prometheus 指标端点是 `/metrics`：启用认证时必须携带管理令牌；未认证的非 loopback 部署会被拒绝。

```powershell
Invoke-RestMethod http://127.0.0.1:8448/metrics -Headers @{ 'X-Admin-Token' = $env:UNIMAP_ADMIN_TOKEN }
```

不要把管理令牌、Bridge token 或引擎 Key 写入命令历史、工单或日志。

### 启动预检与登录语义（2026-08-17）

`cmd/unimap-web` 在创建服务前执行 `StartupPreflight`：

- 绑定回环：允许空 `admin_token` / `password_hash`，走 `/login` 首管理员注册。
- 绑定 `0.0.0.0` 等非回环（含 Docker 容器内监听）：必须认证开启、`admin_token` 非空、`password_hash` 为合法 bcrypt，且 **`web.auth.username` 不能是 `admin`**。不满足则进程退出，容器不会变为 healthy。
- 容器内监听 `0.0.0.0`、宿主机只发布 `127.0.0.1:8448` 时，预检仍按非回环处理。升级带该预检的镜像前，先把运行配置和 `UNIMAP_ADMIN_USERNAME` 改成非默认用户名；不要把容器绑定改成 `127.0.0.1` 来绕过预检（会破坏 Docker 端口映射）。

登录 API：

- loopback 且用户库为空：409，引导创建首管理员。
- 用户库已有账号、未知用户名或密码错误：401。
- 非 loopback 且用户库为空、配置也无哈希：500 `login not configured`。

`GET /api/v1/config` 不再返回通知渠道的 webhook/密钥。保存 Cookie（`POST` cookies）只落盘，不会自动做 CDP 登录探测；探测使用 `GET /api/v1/cookies/login-status`。

### 发布前审计门槛（2026-07-15）

2026-07-15 审计的 12 项问题已在当前工作区处理；2026-07-16 补修后为 11 项完整修复、1 项产品缓解（未实现的额度趋势/告警入口保持禁用），状态见 [`docs/AUDIT_REMEDIATION_GUIDE.md`](AUDIT_REMEDIATION_GUIDE.md)。发布前仍必须保留以下复验门槛：

- 危险截图 ID 不得在截图根目录外创建文件；同步和异步 handler 必须在任务创建前返回 400；
- 重复批次 ID 必须同时检查内存与 SQLite，内存清理或服务重启后仍返回 409，不得覆盖历史记录；
- 配置保存失败不改变当前内存/运行时配置；
- 调度和备份持久化失败不得报告成功；临近触发的一次性任务在保存失败后也不得执行；
- readiness 必须逐个验证所有已启用引擎，并通过加锁快照读取配置；公开检查响应不得暴露底层数据库错误；
- 未配置 `web.rate_limit.trusted_proxy_cidrs` 时忽略全部转发头；
- Shodan 跨字段 OR 与 ZoomEye 比较操作符返回明确能力错误。

每次修复后保存对应测试输出，并重新运行：

```powershell
go test ./...
go vet ./...
go test -race ./...
```

默认测试层不得启动 Chrome/Edge 或产生可见浏览器窗口。真实浏览器测试使用独立 build tag：

```powershell
$env:UNIMAP_CHROME_PATH = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
go test -tags headless_e2e -run TestHeadlessChromeExecutesJavaScriptAndCapturesPNG ./internal/screenshot
go test -tags headless_e2e -run 'TestRelaxed_|TestStrict_MD5Change|TestNormalDynamic' ./internal/tamper
```

`headless_e2e` 会短暂产生多个 Chrome renderer/GPU/network 子进程，这是 Chrome 的正常多进程模型；测试结束后不得有新增进程残留。

不要把现有审计报告中的 `P0` 自动扫描计数当作已确认漏洞；测试占位密钥、固定文案 `innerHTML` 和历史归档脚本已经在人工审查中去重。

## 1. 服务无法启动

1. 检查配置路径与环境变量占位符：默认配置是 `configs/config.yaml`，示例为 `configs/config.yaml.example`。
2. 确认端口未被占用：`Get-NetTCPConnection -LocalPort 8448 -ErrorAction SilentlyContinue`。
3. 检查日志中的配置、数据库和浏览器初始化错误。
4. 最小校验：`go test -race ./...`，然后 `go run ./cmd/unimap-web`。

非 loopback 监听且 Web 认证未启用时，服务会 fail-closed；这是预期安全行为，应该修正部署配置而不是绕过认证。

## 2. 查询失败、无结果或某引擎不可用

1. 先确认登录状态：`GET /api/v1/cookies/login-status`。
2. 使用 API 查询时发送表单字段 `query`、可选 `engines` 与 `page_size`；不要发送旧文档中的 JSON `limit/offset/timeout` 请求体。
3. `page_size` 默认 50，最大为 3000；DayDayMap API 单页上限为 2500。查询状态使用 `GET /api/v1/query/status?query_id=...`。
4. 检查目标引擎 API Key、Cookie、额度和网络连通性。Web UI 展示七引擎；无 API 凭据时使用 Web-only adapter。七引擎 Bridge 已有真实非空证据，CDP 的逐引擎限制见 [CURRENT_STATUS_2026-09-04.md](CURRENT_STATUS_2026-09-04.md)。

## 3. Chrome/CDP 或截图失败

检查 CDP：

```powershell
Invoke-RestMethod http://127.0.0.1:8448/api/v1/cdp/status
```

检查截图路由：

```powershell
Invoke-RestMethod http://127.0.0.1:8448/api/v1/screenshot/router/status
```

可将模式切换为 `cdp`、`extension` 或 `auto`：

```powershell
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8448/api/v1/screenshot/set-mode `
  -ContentType application/json -Body '{"mode":"auto"}'
```

单站截图使用 `POST /api/v1/screenshot`，请求体仅需 `{"url":"https://example.com"}`。目标 URL 被校验为公网 HTTP/HTTPS 地址；被拒绝的内网、loopback 或私有地址不是截图服务故障。

### 无图形界面的 Linux / 云主机

CDP 模式使用 Chrome/Chromium 自带的新版 headless，不依赖桌面环境、`DISPLAY`、X11 或 VNC。

> **B-01 已修复（2026-07-20）**：`Dockerfile` 和 CI 均已改为 `CGO_ENABLED=1` 并安装 Alpine `build-base`。本地 `go test ./internal/auth ./internal/history ./internal/screenshot/batchdb` 验证通过。容器镜像内 SQLite 与 headless 闭环仍需在正式云机执行验收（见 [云服务器部署评估](CLOUD_DEPLOYMENT_ASSESSMENT_2026-07-20.md)）。

修复该阻断后的容器启动基线是：

```bash
# 生产覆盖首次启动从镜像内 configs/config.prod.yaml 初始化可写运行卷
# 云主机建议 screenshot.mode=cdp、headless=true、max_sessions=1
export UNIMAP_BOOTSTRAP_PASSWORD='使用密码管理器生成的随机长密码'
export UNIMAP_ADMIN_USERNAME='非默认管理员名'
export UNIMAP_ADMIN_TOKEN='使用密码管理器生成的随机令牌'
export UNIMAP_DISTRIBUTED_ADMIN_TOKEN='另一枚独立随机令牌'
export UNIMAP_NOTIFY_PEPPER='独立于管理令牌的随机 pepper'
# 可选：留空时 Quake/Hunter 使用 Web-only/CDP 路径，仍必须另行准备登录态
export HUNTER_API_KEY=''
export QUAKE_API_KEY=''
# 二选一：
# A. 构建机可访问基础镜像时，本机构建
docker compose -f docker-compose.yml -f docker-compose.prod.yaml up -d --build
# B. 运行机只拉取组织 ACR 中已验签/固定摘要的镜像
export UNIMAP_IMAGE='registry.example.com/team/unimap@sha256:<digest>'
docker compose -f docker-compose.yml -f docker-compose.prod.yaml pull unimap
docker compose -f docker-compose.yml -f docker-compose.prod.yaml up -d --no-build
curl --fail http://127.0.0.1:8448/health/ready
```

生产覆盖使用 Compose 的 `!override` 完整替换基础端口和卷，因此要求 Docker Compose **2.24.4 或更高版本**。部署前先运行 `docker compose version`；合并结果必须只有 `127.0.0.1:8448:8448`，且不得出现 `./web:/app/web`。检查 `docker compose config` 输出时先脱敏，禁止把展开后的令牌保存到日志或工单。

Compose 通过专用的 `UNIMAP_CONTAINER_BIND_ADDRESS` 显式切换为容器内 `0.0.0.0`，并要求 `UNIMAP_BOOTSTRAP_PASSWORD`；后者只在启动配置阶段用于生成 bcrypt 哈希，不写回配置或日志。生产环境可通过 `UNIMAP_IMAGE` 指定预构建镜像，推荐使用 `仓库@sha256:摘要` 而不是可变 `latest`，并以 `pull` + `up --no-build` 部署。生产入口仅在 `unimap_config` 卷中不存在 `config.yaml` 时，从镜像内 `configs/config.prod.yaml` 初始化并设为 `0600`；应用通过 `UNIMAP_CONFIG_PATH=/app/runtime-config/config.yaml` 读取。更新镜像不会覆盖已存在的运行配置。环境变量占位符在加载时解析；设置页保存会把解析后的候选配置写入运行卷，因此 Key 轮换不能依赖修改环境变量永久覆盖已经持久化的值。若不使用 Compose，镜像基线保持 loopback，仅可通过容器内检查或自行提供安全的公开监听配置访问。

生产部署还必须设置固定管理令牌和非默认管理员用户名。生产模板显式启用 Quake、Hunter，QPS 为 1；缺少 API Key 时注册的 Web-only adapter 并不代表真实查询已经可用，必须执行一次真实查询或浏览器采集验收。未配置的其他引擎默认禁用；启用 Censys、DayDayMap 但缺少完整 API 凭据时注册明确的 Web-only adapter，查询必须同时设置 `browser_query=true`。

当前阿里云试运行机的 SSH 隧道登录、Quake/Hunter API Key 与 Cookie 准备、秘密录入限制和验证顺序见 [云服务器常态化运行准备与协作清单的操作章节](CLOUD_STEADY_STATE_PLAN_2026-07-23.md#10-管理登录与凭据录入操作)。首次录入后必须完成保存、容器重启和恢复验证；本地代码测试不能替代该云机证据。2026-08-20 机内状态与后续运维条目见 [CLOUD_DEPLOYMENT_TENCENT_2026-08-06.md](CLOUD_DEPLOYMENT_TENCENT_2026-08-06.md) 第 0 节和 [推进计划书](AGENT_CONTINUATION_PLAN_2026-08-20.md)。

镜像内置 Chromium 和中日韩字体，固定 `UNIMAP_CHROME_PATH=/usr/bin/chromium`，并持久化 `/app/data`、`/app/screenshots`、`/app/chrome-profile`。Compose 把 `/dev/shm` 提高到 256 MiB；容器基线显式设置 `no_sandbox: true`，普通主机应保持 false 以使用 Chrome sandbox。不得删掉 `--disable-dev-shm-usage` 或独立 `user-data-dir`。持久化 Chrome profile 有独占锁，程序会把其并发会话自动限制为 1；不使用固定 profile 时可按内存逐步提高 `screenshot.max_sessions`。

基础 Compose 的 2 CPU / 1 GiB 限制不是已验收的生产容量；生产覆盖默认 4 CPU / 6 GiB。可通过 `UNIMAP_CPU_LIMIT`、`UNIMAP_MEMORY_LIMIT`、`UNIMAP_CPU_RESERVATION`、`UNIMAP_MEMORY_RESERVATION` 适配验收环境，但降低参数不代表达到生产容量。单机完整功能建议从 4 vCPU、8 GiB RAM、80–100 GiB SSD 和 2–4 GiB swap 起步，并按真实批量负载调整。生产覆盖已移除 `./web:/app/web`、仅向 loopback 暴露 8448，并以命名卷持久化 `/app/logs` 和 `/app/backups`；异机备份和 TLS 反向代理仍需部署方完成。2026-07-23 实测证据见 [阿里云真实环境验收](CLOUD_ACCEPTANCE_2026-07-23.md)。

非容器部署至少确认：

```bash
command -v google-chrome || command -v chromium || command -v chromium-browser
export UNIMAP_CHROME_PATH=/usr/bin/chromium
export UNIMAP_DATA_DIR=/var/lib/unimap
export UNIMAP_CHROME_USER_DATA_DIR=/var/lib/unimap/chrome-profile
unset DISPLAY
```

`/health/live` 只证明进程存活；部署流量必须以 `/health/ready` 为准。截图启用时，readiness 会按配置模式判断真正可用的后端：强制 `extension` 且没有在线扩展时不会因为本机有 Chrome 就误报就绪，除非明确开启 fallback。`/api/v1/screenshot/router/status` 同时返回 `configured_mode`、`current_mode`、`ready` 和两个后端健康状态。

Windows 上 readiness 对 Chrome/Edge 使用 PE 静态验证，不执行 `chrome.exe --version`。后者会被已运行的 Chrome 转交给用户会话，可能打开或激活可见窗口。Router 未配置 CDP provider 时完全跳过 CDP 探针。

本地无法替代真实云机的最终验收项包括：目标发行版的 Chromium 包、容器运行时、出站 DNS/TLS、机器内存和供应商安全组。拿到正式测试机后，再执行镜像启动、ready、单 URL PNG、批量任务、浏览器采集、巡检和重启后持久化验收。

## 4. 浏览器扩展无法配对或截图

1. 扩展和服务应运行在同一台机器的 loopback 环境。
2. 在服务本机检查：

```powershell
Invoke-RestMethod http://127.0.0.1:8448/api/v1/screenshot/bridge/health
Invoke-RestMethod http://127.0.0.1:8448/api/v1/screenshot/bridge/status
```

3. 用 `POST /api/v1/screenshot/bridge/pair` 配对，JSON 为 `client_id` 和 `pair_code`。如果开启 pairing，后续 task/result/rotate 请求需 `Authorization: Bearer <bridge-token>`。
4. bridge token 过期或服务重启后，在 loopback 下重新配对；管理令牌只可作为本机恢复路径使用，不应配置到远程扩展。
5. 不要探测不存在的 `/diagnostic` 端点。完整协议见 [截图扩展运维说明](OPS_SCREENSHOT_EXTENSION.md)。

## 5. 批量截图、文件或任务进度异常

- 异步 URL 批量截图：`POST /api/v1/screenshot/batch-urls`，JSON：`urls`、可选 `batch_id`、`concurrency`。
- 进度：`GET /api/v1/screenshot/batch/progress?job_id=...`。
- 文件：`GET /api/v1/screenshot/batches/files?batch=...`。
- 删除批次：`DELETE /api/v1/screenshot/batches/delete?batch=...`。

`batch_id` 是创建请求中的可选 JSON 字段；列文件和删除时的查询参数叫 `batch`。

POST 返回 202 只表示任务已接受。CLI/GUI 会继续轮询；手工调用时必须保存 `job_id` 并查询到 `completed`/`failed`。浏览器端等待超时会显示明确的超时终态和 `job_id`，但不会取消服务端任务；拿到 `job_id` 后遇到断网或超时，不要本地重跑同一批 URL，以免产生重复截图，恢复后应继续查询原任务进度。`persistence_error` 表示截图终态完成但持久化降级。

## 6. 巡检、篡改检测或基线异常

支持的模式只有：`strict`、`relaxed`、`security`、`balanced`、`precise`。`malicious`、`performance`、`full` 是历史名称，不能再用于新请求。

- 检测：`POST /api/v1/tamper/check`，JSON：`urls`、可选 `concurrency`、`mode`。
- 设置基线：`POST /api/v1/tamper/baseline`。
- 删除基线：`DELETE /api/v1/tamper/baseline/delete?url=...`。
- 历史：`GET /api/v1/tamper/history?limit=...&offset=...&url=...&type=...&mode=...&q=...&start_time=...&end_time=...`；`limit` 最大 1000，`offset` 最大 100000，时间接受 Unix 秒或 RFC3339。
- 导出：`GET /api/v1/tamper/history/export`，支持同样的过滤参数。

历史响应的 `count` 是过滤后总数。巡检的手动检查、定时检查和基线刷新复用当前 Tamper
配置；定时基线刷新也复用截图管理器的 allocator，不应再把 SPA 的 HTTP 空内容误当作有效基线。

这些 URL 也会进行 SSRF 防护。对于 SPA 目标，先确认截图/浏览器能力可用，再判断空 hash 或不可达结果。

## 7. 调度器与通知

```powershell
Invoke-RestMethod http://127.0.0.1:8448/api/v1/scheduler/tasks
Invoke-RestMethod http://127.0.0.1:8448/api/v1/scheduler/history
```

创建任务的端点是 `POST /api/v1/scheduler/tasks/create`，不是旧的 `/api/scheduler/tasks`。一次性、延迟和 cron 任务分别通过 `schedule_type` 的 `once`、`delay`、`cron` 表示。通知通道从 `GET /api/v1/notifications/channels` 查看，并可用 `/api/v1/notifications/channels/test` 验证。

通知通道列表只返回编辑所需的非凭据字段，不回传 Webhook URL、签名 secret 或 app secret。编辑既有通道时应提交 `preserve_existing=true`，并将不修改的凭据留空；不要把页面掩码值重新提交。该模式只允许编辑同 ID、同类型通道，渠道类型变化必须删除旧通道后重新创建。保存后再执行测试接口，确认服务端实际配置可投递。

查询日更的邮件通道是 `email_agent`（webhook → smtp-relay → SMTP）。发件箱 `SMTP_USER`、收件箱 `MAIL_TO` 只写在 gitignore 的 `smtp-relay/.env`，改完重启 relay。云端是 compose 服务 `unimap-smtp-relay`（容器内 URL `http://smtp-relay:8099/webhook`）。本机不跑 Docker 时：`python smtp-relay/relay.py`，监听 `127.0.0.1:8099`，UniMap 渠道指 `http://127.0.0.1:8099/webhook` 且 `allow_private_ip=true`。2026-08-25 本机用与云端相同的 SMTP 配置直发测试信，用户确认收件；细节见 [smtp-relay/README.md](../smtp-relay/README.md)。

任务列表的 `enabled=true` 表示期望启用；还要检查 `runtime_status`。`schedule_error` 表示加载或布置失败，具体诊断在同名错误字段中。删除、停用持久化失败会返回 500 并回滚内存调度状态，不应按 404 处理。

需要定时查询的完整浏览器采集与截图闭环时，`query` 任务 payload 必须包含：

```json
{
  "query": "port=\"443\"",
  "engines": ["fofa"],
  "page_size": 10,
  "notification_detail_limit": 50,
  "browser_query": true,
  "browser_action": "collect_and_capture"
}
```

该工作流通过当前 ScreenshotRouter 后端执行：`cdp` 模式使用 headless Chromium，`extension` 模式使用在线扩展，`auto` 模式按健康状态和 fallback 配置选择。Bridge 不是该 payload 的必需条件。为任务启用成功通知及至少一个支持图片的渠道后，按实际后端排障：

不要用 Bridge E2E 代替 CDP 验收。当前七个稳定引擎的真实结构化采集证据来自 Bridge；CDP 必须逐引擎检查真实结果页、非空结构化资产和截图。DayDayMap 已通过凭据交接与受控 SOCKS5 出口取得 10 条 CDP 资产；Censys 交接后仍命中 Cloudflare 挑战，系统会标记 `browser_challenge`，并在 `auto`/fallback 模式对该任务切换到 Extension（实测 9 条）。

1. `/api/v1/screenshot/router/status` 的 `current_mode` 与预期一致且 `ready=true`；CDP 再检查 `/api/v1/cdp/status`，Extension 再检查 `/api/v1/screenshot/bridge/status` 的近期拉取或回调活动。
2. 调度执行结果包含浏览器截图保存以及“采集结果已合并并持久化”；不能只根据 PNG 文件存在判断结构化采集成功。
3. `/api/v1/history?type=query` 只有一条对应查询历史，并能读取结果明细。
4. `/api/v1/scheduler/history` 中该次执行为 `success`，且 `result` 含“| 资产 | 标题 | 状态 |”表格头和至少一条已持久化资产；不能只核对总数。
5. 通知接收端确认文字明细与图片均送达。`notification_detail_limit` 默认 50、最大 100；查询明细默认按 3800 字节预算生成紧凑表格（企业微信 markdown 正文上限 4096），可通过 payload `notification_detail_bytes` 覆盖（最大 40000，供 smtp-relay 邮件等无企微大小限制的通道携带完整表格），超过部分仍持久化并在通知中提示。通知投递仍是任务完成后的异步阶段，投递失败会记录日志和指标，不会回滚已经持久化的查询结果。

云服务器常态化运行的输入、Cookie/Profile 约束和完全通过门槛见 [云服务器常态化运行准备与协作清单](CLOUD_STEADY_STATE_PLAN_2026-07-23.md)。

普通 API 定时查询不设置 `browser_query`，但通知明细规则相同。真实 API 配置可用时可执行显式联调：

```powershell
$env:UNIMAP_LIVE_API_ENGINE = 'fofa'
go test -tags live_bridge_e2e ./web -run '^TestLiveAPIScheduledQueryNotificationDetails$' -count=1 -v
```

### 篡改证据截图门禁

`tamper.evidence_screenshot_enabled` 默认为 `false`。关闭时，`tamper_check` 只执行检测和
文字结果；兼容入口 `CaptureBatchURLsWithTamper(..., true, ...)` 会显式失败，不会把普通
截图冒充篡改证据。调度任务每次执行前读取最新已提交配置，把开关从 `true` 改回 `false`
会立即停止后续证据截图，不需要等待服务重启。

开启后，只有状态为 `tampered` 的 URL 会通过当前 ScreenshotRouter 生成证据图；截图文件
不存在、截图后端失败或 SSRF 门禁拒绝时，调度任务整体失败。任务成功通知会自动提取证据图
路径并交给支持图片的通知渠道。生产启用前必须完整执行
[云端安全收口验收 Runbook](CLOUD_SECURITY_ACCEPTANCE_RUNBOOK_2026-07-29.md)，不能用普通
搜索结果截图替代页面变化验收。

## 8. 分布式节点不可用

分布式接口只在 `distributed.enabled=true` 时可用；否则返回 `distributed_disabled`。没有单独的 `unimap-node` 可执行文件，节点应通过现有 HTTP 协议集成。

```powershell
Invoke-RestMethod http://127.0.0.1:8448/api/v1/nodes/status -Headers @{ 'X-Admin-Token' = $env:UNIMAP_DISTRIBUTED_ADMIN_TOKEN }
Invoke-RestMethod http://127.0.0.1:8448/api/v1/nodes/network/profile -Headers @{ 'X-Admin-Token' = $env:UNIMAP_DISTRIBUTED_ADMIN_TOKEN }
```

注册、心跳、领取和结果回传应使用对应节点令牌；状态、任务队列管理使用分布式管理令牌。

`max_reassign=N` 表示首次分配之外最多重新分配 N 次；离线、租约过期和 retryable 失败都会消耗次数。队列快照写失败时入队、认领、结果提交和删除不会返回成功，后台回收会恢复旧状态并记录错误。

## 9. 备份、配置或历史记录

- 配置读取/保存：`GET`/`POST /api/v1/config`，需要管理员。
- 备份：`POST /api/v1/backup/create`、`GET /api/v1/backup/list`。
- 操作历史：`POST /api/v1/history/save`、`GET`/`DELETE /api/v1/history`，需要管理员。

配置保存响应包含 `persisted`、`applied` 和 `restart_required`。查询响应的 `persistence.status` 为 `persisted`、`failed` 或 `disabled`；批量截图任务可能包含 `persistence_error`。反向代理部署必须把直接代理网段加入 `web.rate_limit.trusted_proxy_cidrs`，直连部署保持空列表。

所有配置写入口采用候选副本提交：只有 `SaveConfig` 成功后才发布并执行运行态刷新。保存失败时当前 Manager 和运行态保持旧值。`restart_required=true` 时不要仅凭“保存成功”判断已热生效。

多用户数据库启动时会幂等迁移 `session_version`。禁用、删除或改密后旧会话下一请求应为 401；用户库故障时会话请求为 503，但管理令牌仍可作为运维恢复入口。首次管理员使用数据库条件写入，多个并发公开注册最多一个成功。

备份文件和本地配置可能包含敏感数据；限制文件系统权限，并通过受控部署流程恢复。

备份取消：Web 请求 context 和定时任务 context 已传入归档流程。发布前观察到取消/超时会返回失败、清理临时归档并保留旧恢复点；已经发布的归档不因随后到达的取消回滚。取消在目录遍历与分块读取之间检查，不强制中断正在进行的文件系统调用。Web 与定时备份已绑定 history、users、batch、ICP 和 check_records 五类应用数据库，按现有连接生成独立 SQLite 快照，单次快照最多两分钟并受调用方更短的截止时间约束。未绑定或初始化失败的 SQLite 来源会使整次备份失败，保留旧恢复点；不重新按来源路径打开数据库，也不回退裸文件。多库归档不是多库联合事务快照。

归档路径冲突会在快照与发布前被拒绝：不同来源映射到同一条目，或某个文件条目同时成为另一个条目的父目录时，整次备份失败并保留旧恢复点。不会自动重命名条目。需要保留不同子目录内的同名文件时，选择其共同父目录作为来源，或调整为互不冲突的来源；归档仍使用各来源内的相对路径。

输出目录是专用归档区域。若位于某个来源目录之下，收集时排除整个输出子树（包括旧归档、当前临时文件及 SQLite 暂存目录），目录身份匹配也覆盖输出目录的链接别名。显式来源若等于输出目录或位于其内部，整次备份报错并保留旧恢复点，不会静默省略该必需来源。请勿把需要备份的业务文件放入输出目录。

备份列表与保留清理只匹配普通文件和完整命名格式：`<prefix>_backup_YYYYMMDD_HHMMSS.tar.gz`（旧版）或 `<prefix>_backup_YYYYMMDD_HHMMSS.nnnnnnnnn_<随机数字>.tar.gz`（当前版）。相似前缀、无效时间戳、普通同前缀压缩包及符号链接不会被列入自动轮换。文件名匹配不是归档内容完整性校验；输出目录中的标准命名空间仍应留给备份管理器。

显式配置的 `backup.sources`（目录或文件）均为必需来源，不会静默忽略缺失项。来源收集或文件读取失败时，备份返回错误，不发布不完整归档，也不触发旧备份保留清理；HTTP 创建接口返回失败而非 201。只有完整归档发布成功后才执行 `max_backups` 清理。默认自动发现目录仍仅包含当前存在的默认目录。备份读取通过来源根目录句柄进行，只接受普通文件；根目录内的相对文件链接可读取，越界链接会使整次备份失败并保留旧归档。tar 条目统一使用 `/`，便于跨平台恢复；目录整体不提供原子快照；SQLite 来源按已绑定连接生成单库快照。

## 10. 端口扫描异常或全端口未完成

- Web 页支持“常用端口”“自定义端口”“全端口”三种模式。自定义表达式示例：`22,80,443,8000-8100`；调度任务使用同样的 `port_spec`，或以 `scan_mode: full` 扫描 1-65535。
- 多个目标会先解析并按公网 IPv4 全局去重，再将 `唯一 IP × 端口` 笛卡尔积随机打乱。选择多个 `probe_methods` 时，进度按 `IP × 端口 × 方法` 计数；多个域名指向同一 IP 不会重复探测。
- 如需强制限定授权范围，在 Web 或调度任务填写 `authorized_targets`（IPv4/CIDR 列表）。任一解析 IP 超出清单时，整个目标标记为 `not_authorized`，不进行部分扫描。留空仅表示操作者自行确认授权，不代表系统能够证明资产所有权。
- 全端口扫描默认全局端口并发 256、TCP 连接超时 800ms、扫描计划总超时 300 秒。高丢包网络可适当增大连接/总超时；目标较多时不要同时把目标解析并发和端口并发调到最大。
- 结果中的 `attempted_connections` 小于 `expected_connections` 表示扫描被总超时或请求取消中断；已发现的开放端口仍会保留。若经常超时，先降低目标并发，再提高 `scan_timeout_seconds`（最大 900 秒）。
- `blocked` 表示目标解析为 loopback、私有或内部地址，属于 SSRF 防护；`cdn_excluded` 表示检测到 CDN，属于避免扫描共享边缘节点的安全策略。不要通过关闭这些检查来排障。
- `connect` 执行完整 TCP 握手；`telnet` 在连接后发送 IAC 协商；`udp` 使用常见服务载荷并区分 `open` 与 `open_filtered`。`fin`、`null`、`xmas` 分别发送 FIN、无标志、FIN/PSH/URG TCP 段，必须填写授权范围并以管理员/root 或 `CAP_NET_RAW` 权限运行。
- UDP/FIN/NULL/Xmas 的“无响应”不能证明端口开放，结果会显示为 `open_filtered`，只有确定响应才进入 `open_ports`。防火墙、NAT、主机 TCP 栈差异都会影响原始扫描结果，建议与 connect/Telnet 混合复核。
- `jitter_min_ms` / `jitter_max_ms` 控制每次探测前的随机延迟（最大 5000ms）。抖动会显著增加全端口扫描耗时，需同步增大 `scan_timeout_seconds`。

## 11. 变更后检查清单

```powershell
go test -race ./...
```

随后至少验证 `/health`、受影响的 `/api/v1/...` 路由，以及相关 UI 流程。修改路由、认证或 Bridge 协议时，同步更新 [API 文档](API.md) 和本 Runbook。

## 历史库数据完整性（2026-09-06）

历史库连接现在通过 DSN 为每个池连接开启外键。删除单条历史、按类型清空及全部清空会同时删除对应结果明细；新结果必须引用已有历史。既有孤儿明细不自动清理，先备份并按 [检查记录](PROJECT_REVIEW_2026-09-06.md) 的只读 SQL 核查。删除行不保证数据库文件立即缩小。

镜像发布需等待 test、lint、security、headless-browser、extension-scripts；覆盖率附件按操作系统分别命名。本地 YAML 检查不替代 GitHub Actions 实跑。


### 截图文件管理目录边界（2026-09-06）

批次列举、文件删除和批次删除通过 Go `os.Root` 执行根目录内操作。指向截图根目录外的批次符号链接会报错，文件列表与计数只包含普通文件。正常批次内的链接不会使递归删除跟随到外部目标。不要用外部目录链接作为截图批次；配置的截图根目录及其父目录应由管理员管理，避免非受信任进程改动或挂载。此约束不替代现有下载路径校验、认证或浏览器 SSRF 防护。


### Docker 构建依赖代理（2026-09-06）

默认依次使用官方 Go 代理、goproxy.cn 和 direct；使用 `|` 允许在 5xx/网络错误时回退，`,` 则仅在 404/410 时回退。可针对国内网络覆盖顺序（参数整体加引号，避免 shell 把竖线当管道）：

```sh
docker build --build-arg 'GOPROXY=https://goproxy.cn|https://proxy.golang.org|direct' -t unimap:local .
```

这是构建阶段参数，不是运行容器环境配置。企业私有模块应使用组织批准的 GOPROXY/GOPRIVATE 策略，避免把私有模块路径发送给公共后备服务。不要用关闭 GOSUMDB 或删除 go.sum 处理网络故障。全部代理不可用时构建仍失败；失败日志须区分依赖下载、编译和镜像推送阶段。


### 备份保留与文件时间

成功发布后，本次归档优先占用一个保留名额，其余名额按已有归档的修改时间选择，避免旧文件时间超前或相同导致本次清理删除新归档。`MaxBackups=0` 仍表示不限量。此保护针对单次调用自身的清理，不是跨进程备份锁，也不保证归档在后续备份轮换或外部删除后仍可访问。


## 查询缓存键升级（2026-09-06）

引擎编排器的单页、分页缓存使用版本化键，包含引擎名、完整查询字节、页码与页大小。缓存层不再转小写或折叠空白；完全相同请求仍可命中，格式不同但语义相同的请求也可能分别占用缓存。

升级后旧 Redis 查询结果键不再命中，按原 TTL 自然过期，无需清空共享 Redis。首次查询可能增加上游请求量，应观察配额与命中率。回退旧二进制会恢复旧键算法及其误命中风险。此变更不修改统一查询服务自己的缓存键协议，也不是 Redis 实机验收。


## 完整查询快照缓存（2026-09-06）

统一查询服务通过 QuerySnapshotCache 一次发布/读取资产及统计、错误信息。内存快照同属一个条目及有效期；Redis 用 `query-snapshot:v1:` 命名空间下单个 JSON envelope 和单次带 TTL 的 SET，读取只执行一次 GET。旧分离资产/元数据键不参与新协议，按原 TTL 回收；首次命中率下降可能增加上游配额消耗，无需清空共享 Redis。

仅支持旧 QueryCache/QueryCacheMetadataCache 的自定义后端跳过服务级查询缓存，查询本身继续执行；引擎资产缓存接口保留。Delete/Clear 清理新快照。Redis 协议本轮以客户端命令拦截 fixture 验证，不代表真实 Redis 服务、网络故障或集群验收。Redis JSON 数值解码沿用原后端语义；内存快照沿用资产类型保留的深复制。回退旧二进制会恢复旧双键读写及其跨代风险。


### Redis 实例持续回归门禁

CI 的 Redis Snapshot Integration 使用独立 `redis:7.4` 服务容器及动态主机端口，健康检查通过后运行三轮竞态实例测试；必须出现三条父测试 PASS，缺失地址时失败而非 Skip。Docker 发布依赖该任务，日志作为 redis-integration artifact 保留七天。标签跟随 7.4 补丁版，实际服务版本由测试 INFO 输出记录，不将 YAML 中的版本标签等同于一次成功验收。

本地仅在独立临时实例上设置 UNIMAP_REDIS_FIXTURE_ADDR=127.0.0.1:PORT 后执行 `go test -race ./internal/utils -run '^TestRedisInstanceQuerySnapshot$' -count=3 -v`。可设 UNIMAP_REDIS_FIXTURE_REQUIRED=1 强制地址存在。该测试写入唯一前缀，并测试 Clear/Delete，勿使用共享生产实例；普通开发环境未配置时仍显式 Skip。测试覆盖并发代际、真实 TTL、旧键及无效 envelope 隔离、删除和前缀外 sentinel 保留，不覆盖 TLS/集群/故障切换。


### 引擎缓存开关一致性（第四十五轮）

`cache.engines.<engine>.enabled=false` 现在分别阻止该引擎单页/分页缓存读写，以及包含该引擎的服务级组合快照读写。初始化不再因 TTL=0 忽略关闭标志，缺省 TTL 单独使用服务默认值；未配置引擎保持默认启用。策略在缓存访问处检查，查询完成准备写入时再检查，不承诺原子撤销已进入缓存调用的在途请求。

关闭不删除已有条目，仍由原 TTL/容量淘汰；重新开启可复用未过期结果。它不是清空缓存操作。修复基于 06ecb79 的单页/分页冷热缓存及配置初始化反例，并增加组合引擎、服务快照写入和重新开启回归。未更改引擎 TTL 返回接口，也未声称 max_size 的逐引擎容量限制已实现。
