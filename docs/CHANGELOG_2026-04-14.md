# 变更日志 — 2026-04-14

> **分支**: `release/major-upgrade-vNEXT`
> **变更类型**: 生产就绪 Phase 2/3 功能增强
> **涉及模块**: 限流中间件、备份系统、审计中间件、健康检查、熔断器、日志告警、CI/CD

---

## 一、速率限制中间件增强（Phase 2.1）

### 1.1 固定窗口 → 滑动窗口

**问题描述**: 原限流器使用固定时间窗口（`lastReset`），窗口边界处可能出现 2 倍流量突发。

**修复**: 改为滑动窗口实现，每个请求记录精确时间戳，窗口内累计计数。

| 文件 | 变更 |
|------|------|
| `web/middleware_ratelimit.go` | `clientInfo{count, lastReset}` → `map[string][]time.Time` |
| `web/middleware_ratelimit.go` | 新增 `GetRemaining()` 方法返回剩余请求数和重置时间 |

### 1.2 X-RateLimit-* 响应头

**新增响应头**:
- `X-RateLimit-Limit` — 窗口内最大请求数
- `X-RateLimit-Remaining` — 当前窗口剩余请求数
- `X-RateLimit-Reset` — 窗口重置时间戳（Unix 毫秒）
- `Retry-After` — 被限流后的重试等待秒数（仅 429 响应）

### 1.3 测试覆盖

| 文件 | 用例数 | 覆盖范围 |
|------|-------|---------|
| `web/middleware_ratelimit_test.go` (新) | 9 | 滑动窗口允许/拒绝、窗口过期、客户端隔离、剩余查询、并发安全、响应头验证 |

---

## 二、数据备份模块（Phase 2.3）

### 2.1 备份核心模块

**新建 `internal/backup/backup.go`**:
- `Backup(cfg BackupConfig)` — 创建 tar.gz 归档备份
- `ListBackups(dir, prefix)` — 列出已有备份（按时间倒序）
- `collectFiles()` — 递归收集目录文件
- `cleanupOldBackups()` — 保留策略，自动删除过期备份
- 支持多源备份、自定义前缀、最大保留数量

### 2.2 备份 API 端点

| 路由 | 方法 | Handler | 说明 |
|------|------|---------|------|
| `/api/backup/create` | POST | `handleCreateBackup` | 立即创建备份 |
| `/api/backup/list` | GET | `handleListBackups` | 列出所有备份文件 |

**新增文件**:
- `web/backup_handlers.go`
- `web/backup_handlers_test.go`

### 2.3 备份配置

**新增 `config.Backup` 配置段**:

```yaml
backup:
  enabled: true
  output_dir: "./backups"
  prefix: "unimap"
  max_backups: 10
  sources:
    - "./hash_store"
    - "./screenshots"
    - "./data"
    - "./configs"
```

**修改文件**: `internal/config/config.go`

### 2.4 测试覆盖

| 文件 | 用例数 | 覆盖范围 |
|------|-------|---------|
| `internal/backup/backup_test.go` (新) | 11 | tar.gz 创建、无源错误、默认目录、列出空/有备份、前缀过滤、清理策略、文件收集 |
| `web/backup_handlers_test.go` (新) | 6 | 创建备份、列出备份、创建+列出串联、大小格式化、目录存在性检查 |

---

## 三、负载测试脚本（Phase 2.4）

**新建 `scripts/load_test.sh`**:
- 纯 bash/curl 实现，无外部依赖
- 支持自定义目标 URL、并发数、持续时间
- 自动测试 6 个关键端点：健康检查、查询、截图、篡改检测、导入 URL、就绪检查
- 统计 RPS、成功率、p50/p99/max 延迟、429 限流次数
- 结果输出到 `results/load_test_YYYYMMDD_HHMMSS/`
- 可选生成 JSON 报告（需 jq）

**用法**:
```bash
./scripts/load_test.sh                                    # 默认配置
./scripts/load_test.sh http://10.0.0.1:8448               # 指定目标
./scripts/load_test.sh http://localhost:8448 20 60        # 20并发, 60秒
```

---

## 四、CI/CD 完善（Phase 3.4）

### 4.1 Docker 构建与推送

**修改 `.github/workflows/ci.yml`**:
- 新增 `docker` job，依赖 test/lint/security 全部通过后执行
- 仅在 `main`/`master` 分支 push 时触发
- 使用 GitHub Container Registry (ghcr.io) 推送镜像
- 支持 SHA、分支名、semver、latest 四种标签
- 启用 GitHub Actions cache 加速构建

**Docker 镜像标签**:
- `ghcr.io/<owner>/<repo>:<sha>` — 每次提交
- `ghcr.io/<owner>/<repo>:main` — 分支名
- `ghcr.io/<owner>/<repo>:latest` — 最新稳定
- `ghcr.io/<owner>/<repo>:<version>` — semver 版本

---

## 五、本轮涉及的其他已完成项（之前轮次）

| 阶段 | 项目 | 状态 | 文件 |
|------|------|------|------|
| 2.2 | 审计日志中间件 | 已完成 | `web/middleware_audit.go`, `web/middleware_audit_test.go` |
| 3.1 | 健康检查增强 | 已完成 | `web/health_handlers.go`, `web/health_handlers_test.go` |
| 3.2 | 优雅降级（熔断器） | 已完成 | `internal/adapter/orchestrator.go` |
| 3.3 | 日志分级告警 | 已完成 | `internal/logger/alert_hook.go`, `internal/logger/alert_hook_test.go` |

---

## 六、变更文件清单

| 文件 | 类型 | 说明 |
|------|------|------|
| `web/middleware_ratelimit.go` | 修改 | 滑动窗口 + X-RateLimit-* 响应头 |
| `web/middleware_ratelimit_test.go` | 新增 | 9 个限流测试 |
| `web/backup_handlers.go` | 新增 | 备份 API handlers |
| `web/backup_handlers_test.go` | 新增 | 6 个备份 handler 测试 |
| `internal/backup/backup.go` | 之前已创建 | 备份核心模块 |
| `internal/backup/backup_test.go` | 新增 | 11 个备份模块测试 |
| `web/router.go` | 修改 | 注册备份路由 |
| `internal/config/config.go` | 修改 | 新增 Backup 配置段 |
| `scripts/load_test.sh` | 新增 | 负载测试脚本 |
| `.github/workflows/ci.yml` | 修改 | Docker build & push stage |

---

## 七、测试结果

| 检查项 | 结果 |
|--------|------|
| `go build ./...` | 0 错误 |
| `go vet ./...` | 0 警告 |
| `go test -race ./...` | 新增 38 个测试全部通过 |
| 历史遗留失败 | 6 个（未恶化，非本轮引入） |

**遗留失败用例**（已知，非本轮引入）:
- 3 个 node handler 鉴权测试 — 测试未配置 distributed token
- 1 个 scheduler 测试 — `TestLoadRebuildsIDCounter`
- 1 个 workerpool 竞态测试
- 1 个 resourcepool 并发测试

---

**更新日期**: 2026-04-14
**版本号**: v2.1.7
**更新者**: Phase 2/3 Production Readiness Sprint
