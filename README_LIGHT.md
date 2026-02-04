# UniMap Light - 轻量级网络空间资产查询工具

基于原 UniMap + ICP-Hunter 项目核心功能，剥离所有非核心依赖，打造的**极简且聚焦**的网络空间资产查询工具。

## 核心特性

- 🎯 **极简纯净**: 无需数据库、缓存、Docker等外部依赖，单可执行文件即可运行
- 🖥️ **GUI交互**: 基于 Fyne 的跨平台图形界面，直观易用
- 🔍 **多引擎查询**: 支持 FOFA、Hunter、ZoomEye、Quake 等主流搜索引擎
- 📝 **统一查询语言**: 支持 UQL (UniMap Query Language) 统一查询语法
- 🔄 **智能聚合**: 多引擎结果自动去重和标准化
- 📊 **结果导出**: 支持导出为 JSON 或 Excel 格式
- 🔒 **安全存储**: API Key 仅存储在内存中，不落地数据库

## 技术栈

- **语言**: Go 1.21+
- **GUI框架**: Fyne v2.4
- **HTTP客户端**: Resty v2
- **Excel导出**: Excelize v2

## 项目结构

```
unimap-light/
├── cmd/
│   └── unimap-gui/          # GUI主程序入口
├── internal/
│   ├── core/unimap/         # UQL解析、结果聚合泛化
│   ├── adapter/             # 搜索引擎适配器
│   ├── exporter/            # JSON/Excel 导出
│   └── model/               # 核心数据模型
├── res/                     # GUI资源文件（可选）
├── go.mod
├── go.sum
└── README.md
```

## 快速开始

### 1. 安装依赖（仅开发环境需要）

#### Linux
```bash
# Ubuntu/Debian
sudo apt-get install -y gcc libgl1-mesa-dev xorg-dev

# Fedora/RHEL
sudo dnf install -y gcc mesa-libGL-devel libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel
```

#### macOS
```bash
# 无需额外依赖
xcode-select --install  # 如果没有安装 Xcode Command Line Tools
```

#### Windows
```bash
# 无需额外依赖，但需要安装 MinGW-w64 GCC
# 推荐使用 MSYS2: https://www.msys2.org/
```

### 2. 编译程序

```bash
# 克隆仓库
git clone https://github.com/ElaineRosa6/unimap-icp-hunter.git
cd unimap-icp-hunter

# 编译
go build -o unimap-gui ./cmd/unimap-gui

# 或者交叉编译（例如：在 Linux 上编译 Windows 版本）
GOOS=windows GOARCH=amd64 go build -o unimap-gui.exe ./cmd/unimap-gui
```

### 3. 运行程序

```bash
# Linux/macOS
./unimap-gui

# Windows
unimap-gui.exe
```

### 4. 配置引擎 API Key

1. 点击界面顶部的 **"配置引擎 API Key"** 按钮
2. 填入各引擎的 API Key：
   - **FOFA**: 需要 API Key 和 Email
   - **Hunter**: 需要 API Key
   - **ZoomEye**: 需要 API Key
   - **Quake**: 需要 API Key
3. 点击 **"保存"** 按钮

> 注意：API Key 仅存储在内存中，程序关闭后会清除，下次启动需要重新配置。

### 5. 执行查询

1. 在 **"查询输入"** 区域输入查询语句（支持 UQL 语法）
2. 勾选要使用的引擎（可多选）
3. 点击 **"开始查询"** 按钮
4. 查询结果会自动显示在下方的表格中

### 6. 导出结果

- 点击 **"导出 JSON"** 按钮导出为 JSON 格式（包含完整字段）
- 点击 **"导出 Excel"** 按钮导出为 Excel 表格（结构化展示）

## UQL 查询语法

### 基本语法

```
# 等于
country="CN"
port="80"

# 逻辑与
country="CN" && port="80"

# 逻辑或
port="80" || port="443"

# 范围查询
port IN ["80", "443", "8080"]

# 组合查询
country="CN" && (port="80" || port="443") && protocol="http"
```

### 支持的字段

| 字段 | 说明 | 示例 |
|------|------|------|
| ip | IP地址 | `ip="192.168.1.1"` |
| port | 端口 | `port="80"` |
| protocol | 协议 | `protocol="http"` |
| country | 国家代码 | `country="CN"` |
| region | 地区 | `region="beijing"` |
| city | 城市 | `city="shanghai"` |
| title | 网页标题 | `title="管理后台"` |
| body | 网页内容 | `body="login"` |
| server | Server头 | `server="nginx"` |
| host | 域名 | `host="example.com"` |
| asn | ASN编号 | `asn="AS4134"` |
| org | 组织 | `org="Tencent"` |
| isp | 运营商 | `isp="China Telecom"` |

### 查询示例

```
# 查询中国境内的 80 端口
country="CN" && port="80"

# 查询标题包含"管理后台"的网站
title="管理后台"

# 查询常见 Web 端口
port IN ["80", "443", "8080", "8443"]

# 组合查询
country="CN" && port="443" && protocol="https"
```

## 获取 API Key

### FOFA
1. 访问 https://fofa.info/
2. 注册并登录账号
3. 进入 **个人中心** -> **API密钥管理** 获取 API Key 和 Email

### Hunter（鹰图）
1. 访问 https://hunter.qianxin.com/
2. 注册并登录账号
3. 进入 **个人中心** -> **API管理** 获取 API Key

### ZoomEye
1. 访问 https://www.zoomeye.org/
2. 注册并登录账号
3. 进入 **个人中心** -> **API密钥** 获取 API Key

### Quake（360）
1. 访问 https://quake.360.cn/
2. 注册并登录账号
3. 进入 **个人中心** -> **密钥管理** 获取 API Key

## 跨平台编译

### 编译 Windows 版本
```bash
GOOS=windows GOARCH=amd64 go build -o unimap-gui-windows-amd64.exe ./cmd/unimap-gui
```

### 编译 macOS 版本
```bash
GOOS=darwin GOARCH=amd64 go build -o unimap-gui-macos-amd64 ./cmd/unimap-gui
GOOS=darwin GOARCH=arm64 go build -o unimap-gui-macos-arm64 ./cmd/unimap-gui
```

### 编译 Linux 版本
```bash
GOOS=linux GOARCH=amd64 go build -o unimap-gui-linux-amd64 ./cmd/unimap-gui
GOOS=linux GOARCH=arm64 go build -o unimap-gui-linux-arm64 ./cmd/unimap-gui
```

## 核心模块说明

### 1. UQL 解析器 (`internal/core/unimap/parser.go`)

负责将用户输入的 UQL 查询语句解析为抽象语法树（AST）。

### 2. 引擎适配器 (`internal/adapter/`)

将 UQL AST 转换为各引擎的原生查询语法，并将结果标准化为统一格式：

- `fofa.go` - FOFA 适配器
- `hunter.go` - Hunter 适配器
- `zoomeye.go` - ZoomEye 适配器
- `quake.go` - Quake 适配器
- `orchestrator.go` - 多引擎编排器

### 3. 结果合并器 (`internal/core/unimap/merger.go`)

负责多引擎结果的去重和标准化：
- 基于 IP:Port 的智能去重
- 字段补全和优先级合并
- 来源标记

### 4. 导出器 (`internal/exporter/`)

支持将查询结果导出为不同格式：
- JSON 导出（完整字段）
- Excel 导出（结构化表格）

## 与原版的区别

| 特性 | 原版 UniMap + ICP-Hunter | UniMap Light |
|------|------------------------|--------------|
| 部署方式 | Docker Compose 多容器 | 单可执行文件 |
| 外部依赖 | MySQL, Redis, MinIO | 无 |
| 交互方式 | CLI + Web API | GUI |
| 功能范围 | 资产查询 + ICP检测 + 监控 | 仅资产查询 |
| 配置管理 | YAML文件 + 环境变量 | GUI内存配置 |
| 数据持久化 | 数据库 | 不持久化 |
| 调度任务 | Cron定时任务 | 无 |
| 适用场景 | 企业级持续监控 | 临时快速查询 |

## 常见问题

### Q: 为什么不持久化 API Key？
A: 出于安全考虑，API Key 仅存储在内存中，避免泄露风险。如果需要持久化，可以自行修改代码实现（建议使用加密存储）。

### Q: 如何查看某个引擎的原生查询语句？
A: 目前 GUI 暂不支持显示转换后的原生查询，后续版本会考虑添加此功能。

### Q: 编译时报错 "X11/Xlib.h: No such file or directory"
A: 这是 Fyne GUI 框架需要的系统依赖，请参考 "安装依赖" 章节安装相应的开发库。

### Q: 能否支持更多搜索引擎？
A: 可以！只需实现 `EngineAdapter` 接口，添加新的适配器即可。欢迎提交 PR。

### Q: 查询结果为空是什么原因？
A: 可能原因：
1. API Key 配置错误或未配置
2. 查询语法错误
3. 引擎返回空结果
4. API 配额已用完

请检查引擎配置和查询语法。

## 贡献指南

欢迎提交 Issue 和 Pull Request！

### 开发环境设置

```bash
# 克隆仓库
git clone https://github.com/ElaineRosa6/unimap-icp-hunter.git
cd unimap-icp-hunter

# 安装依赖
go mod download

# 运行测试
go test ./...

# 编译
go build -o unimap-gui ./cmd/unimap-gui
```

## 许可证

本项目仅供学习和研究使用，请遵守相关法律法规和服务条款。

## 致谢

基于原 [UniMap + ICP-Hunter](https://github.com/ElaineRosa6/unimap-icp-hunter) 项目开发。

---

**UniMap Light** © 2026 | 轻量化改造 by Claude Code
