# UniMap

多引擎网络空间资产查询与网页监控工具，提供 Web、CLI、GUI 三种入口，支持查询、截图、篡改检测、定时任务与结果导出。

## 当前版本定位

本仓库当前主线以 UniMap 查询与监控能力为主：

- 多引擎统一查询：FOFA、Hunter、ZoomEye、Quake、Shodan
- UQL 查询语言：统一语法，多引擎翻译
- Web 控制台：查询、Cookie 管理、截图、篡改检测、历史记录
- **定时任务系统**：Cron 表达式配置 8 种业务任务，支持创建/启停/执行历史/持久化
- **Admin Token 鉴权**：支持 Web 管理端点认证，保护定时任务、Cookie、篡改检测等敏感操作
- CLI 工具：快速查询与导出
- GUI 工具：桌面交互查询
- 网页篡改检测：专门针对挂马和黄色网站篡改检测，降低普通内容变化敏感度

说明：仓库中仍保留部分历史文档（ICP-Hunter/Docker 全链路描述），请以本 README 与 QUICKSTART 为准。

## 目录结构

```text
cmd/
  unimap-cli/      CLI 入口
  unimap-gui/      GUI 入口
  unimap-web/      Web 入口
internal/
  adapter/         引擎适配与编排
  scheduler/       定时任务调度器（cron + 8种 Runner + 持久化）
  core/unimap/     UQL 解析与结果归并
  service/         统一服务层
  plugin/          插件与处理管道
  screenshot/      截图能力
  tamper/          网页篡改检测
  config/          配置管理
web/
  server.go        Web 服务与路由
  middleware_auth.go  Admin Token 鉴权中间件
  scheduler_handlers.go  定时任务 HTTP 处理
  templates/       页面模板
  static/          前端静态资源
configs/
  config.yaml
  config.yaml.example
```

## 快速启动

详细步骤见 QUICKSTART.md。

### 1. 配置

编辑 configs/config.yaml：

- 启用需要的引擎
- 填写 API Key
- 可选配置 screenshot 相关参数

关键 system 参数：

- `system.max_concurrent`：查询并发上限（会作用于引擎编排器）
- `system.cache_ttl`：查询缓存 TTL（秒）
- `system.cache_max_size`：内存缓存最大条目数
- `system.cache_cleanup_interval`：缓存清理周期（秒）

### 2. 启动 Web

```bash
go run ./cmd/unimap-web
```

访问：http://localhost:8448

**Web 配置**（configs/config.yaml）：

```yaml
web:
    port: 8448              # 监听端口
    bind_address: "0.0.0.0" # 监听地址（改为 127.0.0.1 仅本机访问）
    auth:
        enabled: false      # 设为 true 启用 Admin Token 鉴权
        admin_token: ""     # 管理密钥，支持 ${ADMIN_TOKEN} 环境变量
```

**启用鉴权后**，访问管理页面（定时任务、篡改检测等）需要携带 `X-Admin-Token` 请求头或在 URL 中加 `?admin_token=xxx`。首次访问时浏览器会自动弹出 token 输入框。

**重要提示**：服务启动后，浏览器扩展需要等待大约 1-2 分钟才能正常连接。这是因为服务需要初始化截图管理器、桥接服务等组件，确保所有 API 端点就绪后扩展才能成功配对和通信。

### 3. 使用 CLI

```bash
go run ./cmd/unimap-cli -q 'country="CN" && port="80"' -e fofa,hunter -l 100 -o result.csv
```

如果服务端启用了 Admin Token 鉴权，CLI 请求需要携带 token：

```bash
# 通过请求头
curl -H "X-Admin-Token: your-token" http://localhost:8448/api/query?q='country="CN"'

# 或通过查询参数
curl "http://localhost:8448/api/query?q=country%3D%22CN%22&admin_token=your-token"
```

### 4. 使用 GUI

```bash
go run -tags gui ./cmd/unimap-gui
```

GUI 构建依赖请参考 GUI_BUILD.md。

### 5. 定时任务系统

访问 `/scheduler` 页面或通过首页导航进入 **⏰ 定时任务**。

#### 支持的任务类型

| 类型 | 说明 | Payload 示例 |
|------|------|-------------|
| UQL 查询 | 执行统一查询语言搜索 | `{"query": "port=\"80\"", "engines": ["fofa"]}` |
| 搜索引擎截图 | 打开搜索引擎结果页截图 | `{"engine": "fofa", "query": "port=80"}` |
| 批量截图 | 批量对 URL 列表截图 | `{"urls": ["https://a.com"], "concurrency": 5}` |
| 篡改检测 | 对指定 URL 做篡改对比 | `{"urls": ["https://example.com"], "detection_mode": "relaxed"}` |
| URL 可达性检测 | 检测 URL 是否可访问 | `{"urls": ["https://example.com"]}` |
| Cookie 验证 | 验证各引擎 Cookie 是否有效 | `{"engines": ["fofa","hunter"]}` |
| 登录状态检测 | 检测是否已登录 | `{"engines": ["fofa","hunter"]}` |
| 分布式任务提交 | 向分布式队列提交任务 | `{"task_type": "screenshot", "task_payload": {...}}` |

#### 创建任务

1. 展开 **+ 创建新任务** 表单
2. 填写任务名称、选择任务类型
3. 设置 Cron 表达式（秒 分 时 日 月 周，如 `0 0 2 * * *` = 每天凌晨2点）
4. 配置超时、重试次数和任务参数（JSON 格式）
5. 点击 **创建任务**

#### 管理任务

- **立即执行**：点击 ▶ 按钮，不等待 cron 触发
- **启用/禁用**：点击 ⏸/▶️ 切换任务状态
- **删除**：点击 ✕ 按钮永久删除
- **执行历史**：切换到「执行历史」Tab，可按任务类型和状态筛选

## 网页篡改检测说明

### 检测目标

本系统的篡改检测专门针对以下场景设计：
- **挂马检测**：检测网页是否被植入恶意脚本
- **黄色网站篡改**：检测网页是否被篡改添加色情、赌博等违法内容

### 检测优先级

1. **最高优先级**：恶意内容检测
   - 恶意脚本关键字检测
   - 可疑域名/内容检测
   - 隐藏 iframe 检测
   - 可疑事件处理器检测

2. **次要优先级**：普通内容变化检测
   - 仅在未发现恶意内容时执行
   - 用于跟踪网页正常内容更新

### 检测规则

#### 恶意脚本检测
- `eval(`、`document.write(`、`innerHTML`、`outerHTML`
- `setInterval`、`setTimeout`、`Function(`
- `atob(`、`btoa(`、`unescape(`、`decodeURIComponent(`
- `String.fromCharCode`、`createElement`、`appendChild`、`insertBefore`
- `document.cookie`、`window.location`、`document.location` 等

#### 可疑域名/内容检测
- 色情相关：`xxx`、`porn`、`sex`、`adult`
- 赌博相关：`casino`、`gambling`、`bet`、`lottery`
- 加密货币/挖矿：`crypto`、`bitcoin`、`mining`、`coin-hive`、`coinhive`、`cryptonight`

#### 可疑路径检测
- 后门相关：`shell`、`backdoor`、`webshell`、`hacked`、`deface`
- 钓鱼相关：`phishing`、`fake`、`login`、`admin`

#### 特殊特征检测
- 隐藏 iframe：`<iframe>` 配合 `display:none`、`visibility:hidden`、`width:0`、`height:0`
- 可疑事件处理器：`onerror=`、`onload=`、`onmouseover=`

### 检测状态说明

- `normal`：未检测到任何异常
- `suspicious`：检测到恶意内容或可疑特征
- `tampered`：普通内容发生显著变化（仅在未发现恶意内容时触发）
- `no_baseline`：首次检测，无基线数据
- `unreachable`：网页无法访问

### 性能优化

- Fast 模式下优先使用 HTTP GET 获取页面（比 chromedp 快 10 倍+）
- 使用 `json.Marshal` 替代 `json.MarshalIndent` 提升序列化速度
- 新增 SimpleMD5Hash 算法：`hash = md5(header + "\r\n\r\n" + body)`

## 常用命令

```bash
# 代码检查
go vet ./...
go test ./...

# 运行 Web
go run ./cmd/unimap-web

# 运行 CLI
go run ./cmd/unimap-cli --help
```

## 插件桥接运行说明

### 鉴权说明

如果启用了 Admin Token 鉴权（`web.auth.enabled: true`），所有管理端点（定时任务、篡改检测、Cookie 管理等）需要在请求中携带 token：

```bash
# 方式一：请求头（推荐）
curl -H "X-Admin-Token: your-secret" http://localhost:8448/api/scheduler/tasks

# 方式二：查询参数
curl "http://localhost:8448/api/scheduler/tasks?admin_token=your-secret"

# /health 和 /static/ 路径不需要鉴权
curl http://localhost:8448/health
```

前端页面（`/scheduler` 等）在检测到 401 响应时会自动弹出 token 输入框，token 存储在 `sessionStorage` 中，关闭浏览器后自动清除。

### 插件安装

1. 打开 Chrome/Edge 扩展管理页面。
2. 开启开发者模式。
3. 选择加载已解压扩展，目录为 tools/extension-screenshot。

### 配对流程

1. 启动 Web 服务后，调用配对接口：POST /api/screenshot/bridge/pair。
2. 获取 token 后由扩展带 Authorization Bearer 拉取任务与回传结果。
3. token 临近过期可调用轮换接口：POST /api/screenshot/bridge/token/rotate。
4. 可通过以下接口检查状态：
  - GET /api/screenshot/bridge/health
  - GET /api/screenshot/bridge/status

### 引擎切换

截图引擎由 configs/config.yaml 的 screenshot.engine 控制：

1. cdp：使用 chromedp 路径。
2. extension：使用浏览器插件桥接路径。

建议结合 screenshot.extension.fallback_to_cdp 使用：

1. true：extension 失败自动回退 cdp。
2. false：严格使用 extension，不自动回退。

生产环境建议同时开启回调签名校验：

1. `screenshot.extension.callback_signature_required=true`
2. `screenshot.extension.callback_signature_skew_seconds=300`
3. `screenshot.extension.callback_nonce_ttl_seconds=600`

签名头说明（extension -> bridge callback）：

1. `X-Bridge-Timestamp`
2. `X-Bridge-Nonce`
3. `X-Bridge-Signature`

### 常见故障与排查

1. bridge health ready=false：优先检查 extension.enabled、engine、服务重启状态。
2. unauthorized_bridge：检查 pairing_required 与 Bearer token 是否过期。
3. invalid callback signature：检查时间偏差、nonce 重放、签名头是否完整。
4. pending_tasks 持续增长：检查扩展是否在线、是否可拉取 tasks/next。
5. autoCaptureErrors 增多：检查当前引擎可用性与 fallback 配置。

### 回退触发条件

建议触发 extension -> cdp 回退的条件：

1. bridge 不可用持续超过 5 分钟。
2. bridge 超时或重试错误持续升高。
3. 配对与鉴权错误持续且无法通过重新配对恢复。

回退脚本：

1. Windows: scripts/rollback_extension_to_cdp.ps1
2. Linux/macOS: scripts/rollback_extension_to_cdp.sh

bridge 冒烟脚本：

1. 基础模式：`scripts/bridge_e2e.ps1`
2. 严格签名 + 轮换验证：`scripts/bridge_e2e.ps1 -StrictSignature -RotateToken`

## Day13-15 状态与计划

Day13-15 的实施状态、验收结论和后续计划已统一收口在 `Update_Plan.md`，README 仅保留入口索引，避免状态口径分散。

建议直接查看：

1. `Update_Plan.md`（Day13/14/15 执行状态与最终状态表）
2. `archive/RELEASE_CHECKLIST_2026-03-30.md`（发布门禁与最终裁决）
3. `archive/DAY15_ACCEPTANCE_RECORD_2026-03-30.md`（Day15 验收证据）
4. `archive/ROLLBACK_DRILL_2026-03-30.md`（回退演练证据）

## 技术栈

- Go 1.24.0
- Web: net/http
- GUI: Fyne
- HTML 解析: goquery
- 截图/CDP: chromedp
- 导出: excelize
- 配置: yaml.v3

## 相关文档

- QUICKSTART.md：当前推荐启动路径
- USAGE.md：使用说明
- README_LIGHT.md：轻量/GUI 视角说明
- PROJECT_SUMMARY.md：项目现状总结
- PROJECT_FULL_REVIEW_2026-03-20.md：完整复核与优化建议
- docs/OPS_SCREENSHOT_EXTENSION.md：插件桥接运维手册
- archive/RELEASE_CHECKLIST_2026-03-30.md：发布检查清单
- archive/DAY15_ACCEPTANCE_RECORD_2026-03-30.md：Day15 分布式节点验收记录
- archive/ROLLBACK_DRILL_2026-03-30.md：回退演练记录

## 许可证

MIT
