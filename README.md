# UniMap

多引擎网络空间资产查询与网页监控工具，提供 Web、CLI、GUI 三种入口，支持查询、截图、篡改检测、定时任务、告警与结果导出。

## 当前版本定位

本仓库当前主线以 UniMap 查询与监控能力为主，核心功能完成度约 **98%**，33 个测试包通过 `-race` 检测。

- **多引擎统一查询**：FOFA、Hunter、ZoomEye、Quake、Shodan
- **UQL 查询语言**：统一语法，多引擎翻译
- **Web 控制台**：查询、Cookie 管理、截图、篡改检测、监控、定时任务
- **定时任务系统**：20 种业务任务类型，支持 Cron 创建/启停/执行历史/持久化/编辑
- **截图高可用**：CDP + Extension 双模式共存，ScreenshotRouter 自动健康探测 + 降级
- **网页篡改检测**：恶意脚本/可疑内容检测 + 内容哈希对比，5 种检测模式
- **URL 监控**：可达性检测 + 端口扫描
- **告警系统**：Webhook + Log 通知渠道，去重/静默/频率控制
- **数据备份**：篡改基线、配置、Cookie 数据定时备份
- **分布式任务**：节点注册/心跳/任务领取/故障转移
- **Admin Token 鉴权**：保护管理端点，前端自动 token 输入
- **API 限流**：滑动窗口限流，X-RateLimit-* 响应头
- **Prometheus 指标**：查询延迟、缓存命中率、截图成功率、节点健康度等

## 快速启动

### 1. 配置

```bash
cp configs/config.yaml.example configs/config.yaml
```

编辑 `configs/config.yaml`：
- 启用需要的引擎
- 填写 API Key（支持 `${ENV_VAR}` 环境变量注入）
- 配置截图相关参数（Chrome 路径、截图目录等）

关键 system 参数：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `system.max_concurrent` | 查询并发上限 | 10 |
| `system.cache_ttl` | 查询缓存 TTL（秒） | 3600 |
| `system.cache_max_size` | 内存缓存最大条目数 | 1000 |
| `system.cache_cleanup_interval` | 缓存清理周期（秒） | 300 |

### 2. 启动 Web

```bash
go run ./cmd/unimap-web
```

访问：http://localhost:8448

**Web 配置**：

```yaml
web:
    port: 8448
    bind_address: "0.0.0.0"
    auth:
        enabled: false      # 设为 true 启用 Admin Token 鉴权
        admin_token: ""     # 管理密钥，支持 ${ADMIN_TOKEN} 环境变量
```

**启用鉴权后**，访问管理页面需要携带 `X-Admin-Token` 请求头或 `?admin_token=xxx` 查询参数。浏览器遇到 401 会自动弹出 token 输入框。

### 3. 使用 CLI

```bash
go run ./cmd/unimap-cli -q 'country="CN" && port="80"' -e fofa,hunter -l 100 -o result.csv
```

### 4. 使用 GUI

```bash
go run -tags gui ./cmd/unimap-gui
```

## 功能页面

| 页面 | 路径 | 说明 |
|------|------|------|
| 首页/查询 | `/` | UQL 查询、Cookie 管理、浏览器连接 |
| 网页监控 | `/monitor` | URL 可达性、篡改检测基线/历史、端口扫描 |
| 定时任务 | `/scheduler` | 创建/编辑/启停定时任务，查看执行历史 |
| 批量截图 | `/batch-screenshot` | 批量 URL 截图任务管理 |
| 查看配额 | `/quota` | 各引擎 API 配额查询 |

## 定时任务系统

访问 `/scheduler` 页面或通过 API 管理。

### 支持的任务类型（20 种）

#### 高优先级（核心业务）

| 类型 | 说明 | Payload 示例 |
|------|------|-------------|
| UQL 查询 | 执行统一查询语言搜索 | `{"query": "port=\"80\"", "engines": ["fofa"]}` |
| 搜索引擎截图 | 打开搜索引擎结果页截图 | `{"engine": "fofa", "query": "port=80"}` |
| 批量截图 | 批量对 URL 列表截图 | `{"urls": ["https://a.com"], "concurrency": 5}` |
| 篡改检测 | 对指定 URL 做篡改对比 | `{"urls": ["https://example.com"], "concurrency": 5}` |
| URL 可达性检测 | 检测 URL 是否可访问 | `{"urls": ["https://example.com"]}` |
| Cookie 验证 | 验证各引擎 Cookie 是否有效 | `{"engines": ["fofa","hunter"]}` |
| 登录状态检测 | 检测搜索引擎是否已登录 | `{"engines": ["fofa","hunter"]}` |
| 分布式任务提交 | 向分布式队列提交任务 | `{"task_type": "screenshot", "priority": 0}` |

#### 中优先级（运维辅助）

| 类型 | 说明 | Payload 示例 |
|------|------|-------------|
| 数据导出 | 执行查询并导出 JSON | `{"query": "port=80", "engines": ["fofa"], "format": "json"}` |
| 端口扫描 | 对 URL 做端口扫描检测 | `{"urls": ["https://example.com"], "ports": ["80","443"]}` |
| 截图清理 | 清理过期截图批次 | `{"max_age_days": 30}` |
| 篡改记录清理 | 清理过期篡改检测记录 | `{"max_age_days": 90}` |
| 配额监控 | 监控各引擎 API 配额 | `{"low_threshold": 10}` |
| 告警汇总 | 生成近期告警统计 | `{"max_age_days": 7}` |
| 基线刷新 | 刷新篡改检测基线 | `{"urls": ["https://example.com"]}` |
| URL 导入 | 从文件导入 URL 列表 | `{"file_pattern": "*.txt"}` |

#### 低优先级（安全与运维）

| 类型 | 说明 | Payload 示例 |
|------|------|-------------|
| 插件健康检查 | 检查插件健康状态 | `{}` |
| Bridge 状态检查 | 检查 Bridge 服务状态 | `{}` |
| 告警静默窗口 | 设置告警静默或清理旧记录 | `{"alert_type": "tamper", "duration_minutes": 60}` |
| 缓存预热 | 预热常用 URL 缓存 | `{"warmup_urls": ["https://example.com"]}` |

### 管理任务

- **创建任务**：展开表单 → 填写名称/类型/Cron/参数 → 创建
- **编辑任务**：点击 ✎ 按钮 → 修改参数 → 保存
- **立即执行**：点击 ▶ 按钮，不等 cron 触发
- **启用/禁用**：点击 ⏸/▶️ 切换状态
- **删除**：点击 ✕ 永久删除
- **执行历史**：切换到「执行历史」Tab，按类型/状态筛选

### API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/scheduler/tasks` | GET | 任务列表 |
| `/api/scheduler/tasks/get` | GET | 单个任务详情 |
| `/api/scheduler/tasks/create` | POST | 创建任务 |
| `/api/scheduler/tasks/update` | POST | 更新任务 |
| `/api/scheduler/tasks/delete` | POST | 删除任务 |
| `/api/scheduler/tasks/run` | POST | 立即执行 |
| `/api/scheduler/tasks/enable` | POST | 启用任务 |
| `/api/scheduler/tasks/disable` | POST | 禁用任务 |
| `/api/scheduler/history` | GET | 执行历史 |

## 截图系统

### 双模式高可用

```
请求 → ScreenshotRouter
       ├── CDP 模式（健康 → 使用 CDP，失败 → 降级到 Extension）
       └── Extension 模式（健康 → 使用扩展，失败 → 降级到 CDP）
```

- **ScreenshotRouter**：统一入口，根据健康状态自动路由
- **HealthChecker**：定期探测 CDP（`/json/version`）和 Extension（Bridge 状态）
- **自动降级**：主模式失败后自动切换，无需人工干预
- **状态查询**：`GET /api/screenshot/router/status`

### 引擎切换

配置 `screenshot.engine`：`cdp` 或 `extension`。建议结合 `screenshot.extension.fallback_to_cdp` 使用。

### 插件桥接

详见 [插件桥接运行说明](#插件桥接运行说明) 章节。

## 篡改检测

### 检测模式

| 模式 | 说明 | 适用场景 |
|------|------|---------|
| 严格（strict） | 任何内容变化都告警 | 静态页面、关键配置页 |
| 宽松（relaxed） | 忽略非关键区域变化 | 动态内容页面 |
| 恶意内容（malicious） | 仅检测恶意脚本/可疑内容 | 挂马/黄色网站检测 |
| 性能（performance） | 快速 HTTP 模式，跳过渲染 | 大批量快速扫描 |
| 完整（full） | 完整浏览器渲染 + 所有检测 | 深度审计 |

### 恶意内容检测

检测类型：`eval(`、`document.write(`、隐藏 iframe、可疑事件处理器、色情/赌博/挖矿关键字等。

### 常用操作

| 操作 | 端点 | 说明 |
|------|------|------|
| 设置基线 | `POST /api/tamper/baseline/set` | 为 URL 建立当前内容基线 |
| 查看基线 | `GET /api/tamper/baseline/list` | 列出所有已设置的基线 |
| 删除基线 | `DELETE /api/tamper/baseline/delete` | 删除指定基线 |
| 查询历史 | `GET /api/tamper/history` | 按 URL/类型/模式查询 |
| 删除记录 | `DELETE /api/tamper/history/delete` | 清理历史记录 |

## 监控与告警

### URL 监控

| 功能 | 说明 |
|------|------|
| 可达性检测 | 分类错误（DNS/TLS/超时/连接拒绝） |
| 端口扫描 | 解析 IP、排除 CDN、扫描常用 TCP 端口 |
| 系统资源 | CPU/内存/Goroutine/文件描述符/网络连接 |

### 告警系统

- **通知渠道**：Webhook（支持自定义 URL + Auth Token）、Log
- **告警功能**：阈值检测、去重、静默窗口、频率控制、确认
- **错误日志告警**：ERROR 级别日志自动计数并触发告警

## 运维支持

| 文档 | 路径 | 说明 |
|------|------|------|
| 运维 Runbook | `docs/RUNBOOK.md` | 6 个故障场景：Chrome 崩溃、Bridge 断连、Cookie 失效、节点失联、磁盘满、Redis 不可用 |
| Grafana 面板 | `docs/grafana-dashboard.json` | 7 个面板：查询延迟、缓存命中率、截图成功率、篡改检测率、节点健康度、Goroutine、内存 |
| 架构文档 | `docs/ARCHITECTURE.md` | 分层架构、核心模块、数据流向 |
| 生产就绪计划 | `docs/PRODUCTION_READINESS_PLAN.md` | 生产发布检查清单 |

### 关键指标告警阈值

| 指标 | 警告 | 严重 |
|------|------|------|
| 查询 P95 延迟 | > 30s | > 60s |
| 缓存命中率 | < 50% | < 20% |
| 截图成功率 | < 90% | < 70% |
| 节点在线率 | < 80% | < 50% |
| Goroutine 数 | > 1000 | > 5000 |
| 磁盘使用 | > 80% | > 90% |

## 常用命令

```bash
# 代码检查
go vet ./...
go test -race ./...

# 运行 Web
go run ./cmd/unimap-web

# 运行 CLI
go run ./cmd/unimap-cli --help
```

## 插件桥接运行说明

### 鉴权说明

如果启用了 Admin Token 鉴权，管理端点需要在请求中携带 token：

```bash
# 请求头（推荐）
curl -H "X-Admin-Token: your-secret" http://localhost:8448/api/scheduler/tasks

# 查询参数
curl "http://localhost:8448/api/scheduler/tasks?admin_token=your-secret"

# /health 和 /static/ 不需要鉴权
curl http://localhost:8448/health
```

### 插件安装

1. 打开 Chrome/Edge 扩展管理页面，开启开发者模式
2. 加载已解压扩展，目录为 `tools/extension-screenshot`

### 配对与运行

1. 启动 Web 服务后调用：`POST /api/screenshot/bridge/pair`
2. 获取 token 后由扩展通过 `Authorization: Bearer` 拉取任务
3. Token 轮换：`POST /api/screenshot/bridge/token/rotate`
4. 状态检查：`GET /api/screenshot/bridge/health` 或 `/status`

### 常见故障

| 症状 | 排查 |
|------|------|
| bridge health ready=false | 检查 extension.enabled、engine 配置、服务重启状态 |
| unauthorized_bridge | 检查 pairing_required 与 Bearer token 是否过期 |
| pending_tasks 持续增长 | 检查扩展是否在线、是否可拉取 tasks/next |

## 目录结构

```text
cmd/
  unimap-cli/      CLI 入口
  unimap-gui/      GUI 入口
  unimap-web/      Web 入口
internal/
  adapter/         引擎适配与编排
  scheduler/       定时任务调度器（cron + 20 种 Runner + 持久化）
  alerting/        告警管理（Webhook/Log 渠道、去重/静默/频率）
  auth/            API Key + 权限管理
  backup/          数据备份（基线/配置/Cookie）
  core/unimap/     UQL 解析与结果归并
  service/         统一服务层
  plugin/          插件与处理管道
  screenshot/      截图能力（CDP/Extension/Router）
  tamper/          网页篡改检测
  distributed/     分布式节点注册/心跳/任务队列/故障转移
  config/          配置管理 + 热更新
  logger/          日志系统（动态级别、异步写入）
  monitoring/      资源监控 + 泄漏检测
web/
  server.go        Web 服务与路由（69 个路由）
  templates/       页面模板
  static/          前端静态资源
configs/
  config.yaml      当前配置
  config.yaml.example  示例配置
docs/
  ARCHITECTURE.md          架构文档
  RUNBOOK.md               运维故障处理手册
  grafana-dashboard.json   Grafana 监控面板
  PRODUCTION_READINESS_PLAN.md  生产就绪清单
```

## 技术栈

| 类别 | 技术 |
|------|------|
| 语言 | Go 1.24+ |
| Web | 标准库 `net/http` |
| GUI | Fyne |
| CLI | Cobra |
| 浏览器自动化 | chromedp |
| 定时任务 | robfig/cron/v3 |
| 缓存 | 内存 + Redis |
| HTML 解析 | goquery |
| 导出 | excelize |
| 配置 | yaml.v3 |
| 监控 | Prometheus metrics |
| CI/CD | GitHub Actions (test + lint + race + security + Docker) |

## 相关文档

- `QUICKSTART.md`：快速启动指南
- `USAGE.md`：使用说明
- `docs/ARCHITECTURE.md`：架构文档
- `docs/RUNBOOK.md`：运维 Runbook（6 个故障场景）
- `docs/grafana-dashboard.json`：Grafana 监控面板
- `docs/PRODUCTION_READINESS_PLAN.md`：生产就绪清单
- `docs/OPS_SCREENSHOT_EXTENSION.md`：插件桥接运维手册

## 许可证

MIT
