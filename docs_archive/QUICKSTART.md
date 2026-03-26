# UniMap 快速开始

本文档只覆盖当前仓库可直接运行的路径：Web、CLI、GUI。

## 0. 前置条件

- Go 1.24+
- 可访问各引擎 API
- Windows 建议使用 PowerShell；Linux/macOS 使用 Bash

## 1. 准备配置

1. 复制配置模板：

```bash
cp configs/config.yaml.example configs/config.yaml
```

2. 编辑 configs/config.yaml，至少配置：

- engines.fofa.api_key + email（如启用）
- engines.hunter.api_key（如启用）
- engines.zoomeye.api_key（如启用）
- engines.quake.api_key（如启用）
- engines.shodan.api_key（如启用）

3. 如需截图/CDP，额外配置 screenshot 段（可选）。

## 2. 启动 Web（推荐）

```bash
go run ./cmd/unimap-web
```

启动后访问：http://localhost:8448

可在页面完成：

- 多引擎查询
- Cookie 配置与验证
- 截图（单个/批量）
- 篡改检测（基线、检查、历史）

## 3. 使用 CLI

### 3.1 查看帮助

```bash
go run ./cmd/unimap-cli --help
```

### 3.2 发起查询

```bash
go run ./cmd/unimap-cli -q 'country="CN" && port="80"' -e fofa,hunter -l 100
```

### 3.3 导出结果

```bash
go run ./cmd/unimap-cli -q 'title="login"' -e fofa -o result.csv
go run ./cmd/unimap-cli -q 'title="login"' -e fofa -o result.json
go run ./cmd/unimap-cli -q 'title="login"' -e fofa -o result.xlsx
```

## 4. 启动 GUI（可选）

```bash
go run -tags gui ./cmd/unimap-gui
```

说明：GUI 依赖系统图形库，具体见 GUI_BUILD.md。

## 5. 本地检查命令

```bash
go vet ./...
go test ./...
```

## 6. 常见问题

### Q1: 启动后查询无结果

- 检查对应引擎是否 enabled
- 检查 API Key 是否有效
- 检查查询语法是否被目标引擎支持

### Q2: Web 能打开但截图失败

- 检查本机 Chrome 可用性
- 检查 screenshot.chrome_path
- 若用 CDP，检查 screenshot.chrome_remote_debug_url

### Q3: CLI 提示没有可用引擎

- 检查 configs/config.yaml 中引擎 enabled 与 key 配置
- 或在命令里显式指定 -e

## 7. 建议阅读顺序

1. README.md
2. QUICKSTART.md（本文）
3. USAGE.md
4. PROJECT_FULL_REVIEW_2026-03-20.md
