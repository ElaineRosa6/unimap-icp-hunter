# Screenshot Extension Ops Runbook

> 最后按代码核对：2026-08-02。适用于 `tools/extension-screenshot` manifest 0.4.15。

## 运行边界

- 扩展 Bridge 面向本机浏览器；配对、令牌轮换、任务拉取和结果回调均限制为 loopback 请求。
- API 路径必须使用 `/api/v1/screenshot/bridge/...`。旧 `/api/screenshot/bridge/...` 已移除。
- `health` 和 `status` 对远程请求仅返回最小信息；本机请求可获得诊断快照。
- 未实现 `/diagnostic` 端点；不要将它加入探活、监控或自动化脚本。

## 启动与诊断

启动服务和浏览器扩展后，在服务所在机器执行：

```powershell
Invoke-RestMethod http://127.0.0.1:8448/api/v1/screenshot/bridge/health
Invoke-RestMethod http://127.0.0.1:8448/api/v1/screenshot/bridge/status
Invoke-RestMethod http://127.0.0.1:8448/api/v1/screenshot/router/status
```

扩展连接失败时，依次确认：

1. 扩展配置中的服务地址指向正确的本机 UniMap 服务。
2. 服务和扩展处于同一 host，且本机可访问 8448 端口。
3. 配对码与 `screenshot.extension.pair_code` 一致。
4. pairing 开启时，扩展保存的 bridge token 未过期；必要时重新配对。
5. CDP 或 Extension 引擎健康状态是否可用。可通过 `POST /api/v1/screenshot/set-mode` 在 `cdp`、`extension`、`auto` 间切换。

## 配对与令牌

配对只允许 loopback 请求：

```powershell
$body = @{ client_id = 'chrome-extension'; pair_code = $env:UNIMAP_EXTENSION_PAIR_CODE } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8448/api/v1/screenshot/bridge/pair `
  -ContentType application/json -Body $body
```

成功后服务返回 `token`、`expires_in` 和 `expire_at`。将 token 仅保存在扩展本地存储。任务拉取、回调与轮换使用：

```text
Authorization: Bearer <bridge-token>
```

令牌轮换也仅允许 loopback：

```powershell
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8448/api/v1/screenshot/bridge/token/rotate `
  -Headers @{ Authorization = "Bearer $env:UNIMAP_BRIDGE_TOKEN" } `
  -ContentType application/json -Body '{"revoke_old":true}'
```

如果服务重启导致短期 token 失效，重新配对即可。管理令牌可在 loopback 本机恢复流程中被接受，但不得暴露给远程扩展或记录到日志。

## 任务协议

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/api/v1/screenshot/bridge/tasks/next` | 获取一个待执行任务；无任务时返回 `task: null` |
| POST | `/api/v1/screenshot/bridge/mock/result` | 回传任务结果 |

任务可使用 `collect`、`screenshot` 或 `collect_and_capture` 动作。扩展对三种动作执行一致的等待、懒加载与稳定性处理；不要将 `collect_and_capture` 当作未支持的私有动作。

当启用回调签名时，回传还必须包含 `X-Bridge-Timestamp`、`X-Bridge-Nonce` 与 `X-Bridge-Signature`。服务会验证时间窗口、nonce 重放和 body HMAC。

## 受支持的采集目标

Extension 0.4.15 与服务端 Bridge 支持 FOFA、Hunter、ZoomEye、Quake、Shodan、Censys、DayDayMap。2026-08-02 七引擎均取得真实非空结构化资产；Censys 为 9 条，DayDayMap 为 1 条（首次页面抖动后重试通过）。任务 DTO 必须携带 `query`，DayDayMap 使用 `/searchResult?keyword=...`。

## 常用验证

搜索页截图：

```powershell
Invoke-RestMethod 'http://127.0.0.1:8448/api/v1/screenshot/search-engine?engine=fofa&query=port%3D%2280%22'
```

异步 URL 批量截图：

```powershell
$body = @{ urls = @('https://example.com'); concurrency = 1 } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8448/api/v1/screenshot/batch-urls `
  -ContentType application/json -Body $body
```

随后用返回的 `job_id` 调用：

```text
GET /api/v1/screenshot/batch/progress?job_id=<job_id>
```

服务会拒绝私有、回环或内部目标；这是 SSRF 防护，不能通过重试或切换扩展绕过。

### Bridge 到 CDP 的凭据交接

CDP 登录态缺失时，服务向同源 Extension 标签页提交 `get_browser_credentials`：扩展回传类型化 Cookie、`localStorage`、`sessionStorage` 与最终 URL。服务只持久化 Cookie；Web Storage 仅保存在进程内，CDP 导航到同源页面后注入并刷新。整个动作继续受 loopback Bridge、引擎域名白名单、最终 URL 同源校验和浏览器 SSRF fail-closed 约束。

扩展选项允许修改 Loopback API Base URL，但只接受 `http://127.0.0.1|localhost|[::1]:显式端口`，拒绝凭据、查询和片段。扩展源码更新后在 `chrome://extensions` 对 **UniMap Screenshot Bridge** 执行一次重新加载，并确认版本为 0.4.15，再运行 live E2E。

## Bridge 查询、截图与通知联调

以下显式标签测试会启动一个仅监听 loopback 的临时服务，创建一次
搜索截图任务，并验证 Bridge 回调、截图文件及飞书应用图片通知：

```powershell
go test -tags live_bridge_e2e ./web -run '^TestLiveBridgeSearchScreenshotNotification$' -count=1 -v
```

完整定时查询闭环（Bridge 结构化采集 + 截图 + SQLite 查询结果 + 图片通知）使用：

```powershell
go test -tags live_bridge_e2e ./web -run '^TestLiveBridgeScheduledQueryClosedLoop$' -count=1 -v
```

默认引擎为 FOFA；可在 PowerShell 设置 `UNIMAP_LIVE_BRIDGE_ENGINE` 为 `hunter`、`zoomeye`、`quake` 或 `shodan` 后执行同一命令。

如果日常 Chrome 中已有另一个 UniMap Extension，它也可能轮询默认 8448 并与测试实例竞争任务。
实机验收应给测试 Extension 设置独立端口，并让测试服务使用同一端口。例如先在测试
Extension 的 service worker 控制台执行：

```javascript
chrome.storage.local.set({ apiBaseURL: "http://127.0.0.1:18448" })
```

再运行：

```powershell
$env:UNIMAP_LIVE_BRIDGE_PORT = '18448'
$env:UNIMAP_LIVE_BRIDGE_ENGINE = 'quake'
go test -tags live_bridge_e2e ./web -run '^TestLiveBridgeScheduledQueryClosedLoop$' -count=1 -v
```

测试端口必须在 1024—65535 范围内且未被占用。验收结束后，如该测试 Extension 还要连接
默认服务，应把 `apiBaseURL` 恢复为 `http://127.0.0.1:8448`。生产配对码和管理令牌不得
复制到测试代码、命令历史或验收文档。

测试除非空截图外，还要求 Bridge 结构化结果非空、SQLite 明细非空、调度通知正文包含已持久化资产，并验证真实通知成功指标。若截图是登录页，即使 PNG 非空也必须判为失败；先恢复对应引擎登录态再重跑。

普通 API 调度查询的“API 结果 → SQLite 明细 → 文字明细通知”使用：

```powershell
$env:UNIMAP_LIVE_API_ENGINE = 'fofa'
go test -tags live_bridge_e2e ./web -run '^TestLiveAPIScheduledQueryNotificationDetails$' -count=1 -v
```

需要人工核验截图内容时，可将 `UNIMAP_LIVE_BRIDGE_ARTIFACT_DIR` 设为一个绝对路径。测试会以仅当前用户可读的权限保留一份 PNG 副本；未设置时，截图随临时测试目录清理。

它会使用本机 `configs/config.yaml`、已配对的扩展和当前登录的搜索引擎会话，向已启用的
`feishu_app` 渠道发送一条真实测试消息。该测试不适合 CI，也不得在未获通知接收方同意时执行；
它不会打印凭证，且不会修改原配置或持久化任务。

## 变更与回滚

- 修改 Bridge API、token 或回调协议前，先更新扩展与服务端，再执行 `go test -race ./...`。
- 将模式切回 CDP：`POST /api/v1/screenshot/set-mode`，JSON 为 `{"mode":"cdp"}`。
- 扩展问题不能通过恢复旧 `/api/...` shim 解决；调用方必须迁移到 `/api/v1/...`。

### 0.4.20 本地修复及验收边界

识别专用登录标题与 `Just a moment...` / Cloudflare 挑战标题；collect 和 collect_and_capture 都通过失败回调报告 login_required 或 browser_challenge，不把验证页当空成功。重新加载后才能使用更新；源码回归42项通过不替代活页验证。2026-09-06 的真实0.4.19七引擎结果见 `CDP_EXTENSION_RECHECK_2026-09-06.md`，其中Quake登录、Shodan验证仍待处理。

### 0.4.21 Quake 域名卡片修复（本地待加载）

2026-09-06 已登录活页 DOM 的10张结果卡片，其 div.ip span.copy_btn 的 data-clipboard-text 为完整域名，而IP被站点遮蔽。旧提取器仅识别该字段的IPv4，导致无Hostname详情的域名卡片丢失。Extension与原生CDP脚本现在保留合法可见域名为host，ip保持空，不解析DNS、不推测遮蔽IP、不请求资产链接。原有IPv4路径与Hostname详情保留。

回归先失败后通过，Node共44项通过（包含直接执行Go内嵌Quake提取脚本的测试）。这是活页DOM定位与固定fixture验证；0.4.21仍待浏览器重新加载后的任务回传验收，不等同于独立CDP已登录成功。

### 0.4.22 设置页连接诊断

重新加载扩展后，点击工具栏 UniMap 图标打开设置页，顶部提供运行版本、最近成功轮询时间与手动刷新按钮。无需寻找另一个弹窗或共享 token。仅成功拉取任务接口后记录 last_poll_at；30秒内标记最近轮询成功，超时标记状态过期（可能执行长任务、后台休眠或断连），不单凭 paired 声明在线。显示采用固定诊断文案，不回显原始错误或凭据。该功能不替代各引擎实测与服务端 readiness。0.4.22 源码与浏览器实际加载版本仍须分别确认。
