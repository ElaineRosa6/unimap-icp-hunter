> 历史快照：以下正文保留 2026-09-04 核对时的事实，不代表实时状态。后续审查与发布证据见 [2026-09-06 核验快照](CURRENT_STATUS_2026-09-06.md)。

# 当前项目状态（2026-09-04）

> 本页是当前快照。日期化验收、历史计划和旧审计记录保留原始事实；若与本页冲突，以当前代码、测试、API/Runbook 和最新发布记录为准。

## 代码与发布基线

- 当前工作分支：master。
- 当前提交：e31e03d；本地 HEAD、origin/master 与 GitHub master 在本次核对中一致。
- origin/develop 为 71be507，落后 master 32 个提交，不是当前发布基线。
- Go 版本以 go.mod 为准：1.26.6。
- 工作区保留一个未跟踪的 docs/.claude/settings.local.json，本地权限配置不属于发布代码。

## CI/CD

- 最新 master 提交的 ci workflow：成功。
- 最新 master 提交的 bridge-smoke workflow：成功。
- ci 已完成双平台构建、Race Test、coverage、go vet、gofmt、golangci-lint、govulncheck、headless browser E2E、扩展脚本检查和 Docker Build & Push。
- CI run：[ci](https://github.com/ElaineRosa6/unimap/actions/runs/33347273951)；Bridge run：[bridge-smoke](https://github.com/ElaineRosa6/unimap/actions/runs/33347274009)。
- 当前仓库没有独立的生产环境部署 workflow；CD 范围是 Docker 镜像构建并推送到 GHCR。

## 引擎与浏览器状态

- 稳定 Web UI 覆盖 FOFA、Hunter、ZoomEye、Quake、Shodan、Censys、DayDayMap 七个引擎。
- 七引擎 Bridge 真实结构化采集已有非空证据；Censys challenge 识别与 Extension fallback 已验证。
- 最近一次 CDP 定级：FOFA、Shodan 于 2026-08-21 通过；ZoomEye 因当时站点/SSO 状态受限；Quake、Hunter、DayDayMap 既有证据通过。
- 2026-08-21 后 ExtractJS 有调整，但截至本页日期尚无新的 CollectAndCapture CDP 复跑；插件活页核验不能替代 CDP 闭环。
- 自动巡检证据截图仍以 tamper.evidence_screenshot_enabled=false 为默认门禁，启用前需要受控页面变化与图片送达验收。

## 本地验证

- go test ./...：exit 0。
- go vet ./...：exit 0。
- go build -mod=readonly ./...：exit 0；本机全局 Go 模块缓存有权限警告，CI 构建不受此本机缓存路径影响。

## 文档入口

- 当前 API：docs/API.md。
- 运维：docs/RUNBOOK.md。
- 架构：docs/ARCHITECTURE.md。
- 后续工作矩阵：docs/REMAINING_WORK_2026-07-23.md。
- 历史推进计划：docs/AGENT_CONTINUATION_PLAN_2026-08-20.md。
