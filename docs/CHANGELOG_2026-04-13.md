# 变更日志 — 2026-04-13

> **分支**: `release/major-upgrade-vNEXT`
> **变更类型**: 缺陷修复 + 跨平台适配增强
> **涉及模块**: Web 前端、定时任务、截图管理器、信号处理、CI/CD、Docker

---

## 一、缺陷修复

### 1.1 页面打开自动执行示例查询检查

**问题描述**: 打开首页时自动触发 CDP 浏览器打开各引擎页面检测登录状态，相当于自动执行了示例查询。

**根因**: `web/static/js/main.js` 中 `initLoginStatusPoll()` 函数在页面加载时立即调用 `refreshLoginStatus()`，该 API 会通过 CDP 打开浏览器页面检测各引擎登录状态。

**修复**: 移除 `initLoginStatusPoll()` 中的立即调用，仅保留 15 秒间隔轮询。用户可通过手动点击"刷新"按钮主动检测。

| 文件 | 变更 |
|------|------|
| `web/static/js/main.js:766-772` | 移除 `refreshLoginStatus()` 立即调用 |

### 1.2 定时任务页面 "failed to render page"

**问题描述**: 访问 `/scheduler` 页面时返回 500 错误，页面显示 "Failed to render page"。

**根因**: `scheduler.html` 模板使用 `{{$.TaskTypeLabel .}}` 调用 Go 函数，但 `html/template` 中 `range` 遍历出的 `.` 是 `interface{}` 类型，无法自动转换为 `scheduler.TaskType` 参数。

**修复**:
1. `handleSchedulerPage` 中预先将 `TaskType` 枚举转为 `[]string` + `map[string]string` labels
2. 模板改用 Go 内置 `{{index $.TaskTypeLabels .}}` 进行 map 查找

| 文件 | 变更 |
|------|------|
| `web/scheduler_handlers.go` | 预计算 `taskTypes []string` + `taskTypeLabels map[string]string` |
| `web/templates/scheduler.html` | `{{$.TaskTypeLabel .}}` → `{{index $.TaskTypeLabels .}}`（两处） |

---

## 二、跨平台适配增强

### 2.1 Dockerfile Go 版本对齐

**问题描述**: `Dockerfile` 使用 `golang:1.23-alpine`，但 `go.mod` 声明 `go 1.26`，版本不一致导致构建失败。

**修复**: `Dockerfile:2` — `golang:1.23-alpine` → `golang:1.26-alpine`

### 2.2 信号处理跨平台兼容

**问题描述**: `internal/utils/shutdown.go` 使用 `syscall.SIGHUP`，该信号在 Windows 上未定义，会导致编译失败。

**修复**: 拆分平台相关代码，`SIGHUP` 仅在 Unix 平台注册。

| 文件 | 变更 |
|------|------|
| `internal/utils/shutdown.go` | 移除 `syscall.SIGHUP`，改为 `registerSIGHUP` 函数变量 |
| `internal/utils/shutdown_unix.go` (新) | `//go:build unix` 约束，`init()` 中注册 `SIGHUP` |

### 2.3 Chrome 路径检测统一

**问题描述**: `screenshot/manager.go` 使用 `os.PathSeparator` 判断平台，与 `cdp_handlers.go` 中 `runtime.GOOS` 方式不一致，且路径覆盖不足。

**修复**: 统一为 `runtime.GOOS` 判断，补全 Edge/beta/snap 路径。

| 文件 | 变更 |
|------|------|
| `internal/screenshot/manager.go` | `os.PathSeparator` → `runtime.GOOS` switch；新增 Edge/macOS 用户目录路径 |

### 2.4 CI 增加 macOS 覆盖

**问题描述**: CI 仅在 `ubuntu-latest` 运行，无 macOS 构建/测试覆盖。

**修复**: build/test/lint 三个 job 加入 `macos-latest` matrix。

| 文件 | 变更 |
|------|------|
| `.github/workflows/ci.yml` | build/test/lint 加入 `os: [ubuntu-latest, macos-latest]` |

---

## 三、变更文件清单

| 文件 | 类型 | 说明 |
|------|------|------|
| `Dockerfile` | 修改 | Go 版本 1.23 → 1.26 |
| `internal/utils/shutdown.go` | 修改 | 移除 SIGHUP |
| `internal/utils/shutdown_unix.go` | 新增 | Unix-only SIGHUP 注册 |
| `internal/screenshot/manager.go` | 修改 | 统一 runtime.GOOS，补全路径 |
| `.github/workflows/ci.yml` | 修改 | 加入 macOS runner |
| `web/static/js/main.js` | 修改 | 移除登录状态立即检测 |
| `web/scheduler_handlers.go` | 修改 | 预计算 task type labels |
| `web/templates/scheduler.html` | 修改 | 使用 index 函数查找 label |

---

## 四、服务器部署检查结果

> 以下问题已通过本地代码检查与配置核对确认，属于当前版本在服务器部署时需要优先处理的遗留风险。

### 4.1 容器端口与程序默认端口不一致

**现象**: 程序默认监听 `8448`，但 Docker 相关配置仍按 `8080` 暴露与健康检查。

**涉及文件**:

| 文件 | 位置 | 说明 |
|------|------|------|
| `configs/config.yaml` | `web.port: 8448` | 程序默认 Web 端口 |
| `Dockerfile` | `EXPOSE 8080` / healthcheck `localhost:8080` | 容器暴露端口仍为 8080 |
| `docker-compose.yml` | `8080:8080` / healthcheck `localhost:8080` | compose 端口映射仍为 8080 |

**影响**: 直接使用容器部署时，容易出现“服务已启动但外部访问不到”或健康检查误判。

### 4.2 分布式管理接口默认无 token 保护

**现象**: 当前默认配置启用了 `distributed.enabled`，但 `admin_token` 和 `node_auth_tokens` 为空。

**涉及文件**:

| 文件 | 位置 | 说明 |
|------|------|------|
| `configs/config.yaml` | `distributed.enabled: true` | 分布式功能默认开启 |
| `configs/config.yaml` | `distributed.admin_token: ""` | 管理 token 默认为空 |
| `configs/config.yaml` | `distributed.node_auth_tokens: {}` | 节点 token 默认为空 |
| `web/node_auth.go` | `requireDistributedAdminToken()` | `admin_token` 为空时直接放行 |

**影响**: 在公网服务器上运行时，分布式相关管理接口可能处于无鉴权状态，不适合直接暴露到外网。

### 4.3 浏览器侧跨域默认仅面向本机

**现象**: 默认 CORS 允许来源仅包含 `localhost` 与 `127.0.0.1`。

**影响**: 如果服务器通过域名、反向代理或非默认端口访问 Web 页面，前端请求和 WebSocket 可能需要同步调整 CORS 配置。

---

**更新日期**: 2026-04-13
**版本号**: v2.1.6
**更新者**: Cross-platform & Bugfix Sprint
