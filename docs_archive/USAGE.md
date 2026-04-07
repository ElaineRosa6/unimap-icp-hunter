# UniMap Light 使用指南

本文档提供 UniMap Light 的详细使用说明和示例。

## 目录

1. [安装与启动](#安装与启动)
2. [界面概览](#界面概览)
3. [配置引擎](#配置引擎)
4. [执行查询](#执行查询)
5. [查看结果](#查看结果)
6. [导出数据](#导出数据)
7. [Web 端与 CDP 连接](#web-端与-cdp-连接)
8. [常见问题](#常见问题)
9. [查询示例](#查询示例)

## 安装与启动

### 方式一: 使用预编译二进制

下载对应平台的可执行文件后，直接运行：

**Windows:**
```bash
unimap-gui-windows-amd64.exe
```

**macOS:**
```bash
chmod +x unimap-gui-macos-amd64
./unimap-gui-macos-amd64
```

**Linux:**
```bash
chmod +x unimap-gui-linux-amd64
./unimap-gui-linux-amd64
```

### 方式二: 从源码编译

```bash
# 克隆仓库
git clone https://github.com/ElaineRosa6/unimap-icp-hunter.git
cd unimap-icp-hunter

# 编译
go build -o unimap-gui ./cmd/unimap-gui

# 运行
./unimap-gui
```

## 界面概览

UniMap Light 主界面包含以下几个区域：

```
┌─────────────────────────────────────────────────┐
│ UniMap 查询工具              [配置引擎 API Key] │
├─────────────────────────────────────────────────┤
│ 查询输入:                                        │
│ ┌─────────────────────────────────────────────┐ │
│ │ country="CN" && port="80"                   │ │
│ └─────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────┤
│ 选择引擎:                                        │
│ □ FOFA   □ Hunter   □ ZoomEye   □ Quake        │
├─────────────────────────────────────────────────┤
│ [开始查询]  [导出 JSON]  [导出 Excel]            │
│ 就绪            [进度条]                         │
├─────────────────────────────────────────────────┤
│ 查询结果:                                        │
│ ┌─────────────────────────────────────────────┐ │
│ │ IP      | Port | Protocol | Host | URL | ... │ │
│ │ 1.2.3.4 | 80   | http     | ...  | ... | ... │ │
│ └─────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘
```

### 主要组件说明

1. **顶部栏**: 显示工具标题和配置按钮
2. **查询输入区**: 输入 UQL 查询语句
3. **引擎选择区**: 通过复选框选择要使用的引擎
4. **操作按钮区**: 执行查询和导出操作
5. **状态栏**: 显示当前操作状态和进度
6. **结果显示区**: 以表格形式展示查询结果

## 配置引擎

### 步骤 1: 打开配置对话框

点击界面顶部的 **"配置引擎 API Key"** 按钮。

### 步骤 2: 填写 API Key

在弹出的对话框中，根据您拥有的引擎账号填写对应的 API Key：

#### FOFA 配置
```
FOFA: [输入 FOFA API Key]
FOFA Email: [输入注册邮箱]
```

#### Hunter 配置
```
Hunter: [输入 Hunter API Key]
```

#### ZoomEye 配置
```
ZoomEye: [输入 ZoomEye API Key]
```

#### Quake 配置
```
Quake: [输入 Quake API Key]
```

### 步骤 3: 保存配置

点击 **"保存"** 按钮，配置将保存到内存中。

> **注意**: API Key 仅存储在内存中，程序关闭后会自动清除。下次启动需要重新配置。

### 获取 API Key

如果您还没有 API Key，请参考 [README_LIGHT.md](README_LIGHT.md) 中的"获取 API Key"章节。

## 执行查询

### 步骤 1: 编写查询语句

在 **"查询输入"** 区域输入 UQL 查询语句。例如：

```
country="CN" && port="80"
```

### 步骤 2: 选择引擎

勾选要使用的引擎复选框。可以选择一个或多个引擎：

- ☑️ FOFA
- ☑️ Hunter
- ☐ ZoomEye
- ☐ Quake

> **提示**: 
> - 多选引擎可以获得更全面的结果
> - 结果会自动去重和合并
> - 只有已配置 API Key 的引擎才能被使用

### 步骤 3: 开始查询

点击 **"开始查询"** 按钮。

查询过程中：
- 状态栏显示 "正在查询..."
- 进度条显示查询进度
- 按钮变为灰色不可点击

查询完成后：
- 状态栏显示结果数量
- 结果自动显示在下方表格中
- 按钮恢复可点击状态

## 查看结果

查询结果以表格形式展示，包含以下列：

| 列名 | 说明 | 示例 |
|------|------|------|
| IP | IP 地址 | 1.2.3.4 |
| Port | 端口号 | 80 |
| Protocol | 协议 | http |
| Host | 域名/主机名 | example.com |
| URL | 完整 URL | http://example.com |
| Title | 网页标题 | 示例网站 |
| Server | 服务器类型 | nginx/1.20.1 |
| Country | 国家代码 | CN |
| Source | 数据来源引擎 | fofa |

### 表格操作

- **滚动查看**: 使用鼠标滚轮或拖动滚动条
- **排序**: 点击列标题进行排序（如果支持）
- **选择**: 点击行可查看详细信息（如果支持）

## 导出数据

### 导出 JSON 格式

1. 点击 **"导出 JSON"** 按钮
2. 选择保存位置和文件名
3. 点击 **"保存"** 确认

**JSON 格式特点:**
- 包含所有字段的完整数据
- 便于程序处理和二次分析

## Web 端与 CDP 连接

项目内置 Web 端（unimap-web），适合做多引擎查询、Cookie 填写、以及截图校验。

### 启动 Web 端

```bash
go run ./cmd/unimap-web
```

默认端口为 **8448**，浏览器访问：

```
http://localhost:8448
```

### 优雅关闭

Web 端支持优雅关闭，可通过以下方式触发：

1. **发送终止信号**: 在终端按 `Ctrl+C` 发送 SIGINT 信号
2. **超时控制**: 默认 30 秒超时，确保所有请求处理完成
3. **资源清理**: 自动关闭 WebSocket 连接和 HTTP 服务器

```bash
# 启动 Web 端
./unimap-web

# 按 Ctrl+C 触发优雅关闭
# 看到 "Application stopped gracefully" 表示关闭成功
```

### 连接 CDP（复用浏览器登录态）

1. 在 Web 页面中点击 **"连接 CDP"** 按钮。
2. 系统会自动检测 `127.0.0.1:9222`，若未在线会尝试启动 Chrome 并开启调试端口。
3. 连接成功后，可复用浏览器登录态进行截图验证。

可在 `configs/config.yaml` 中配置：

- `screenshot.chrome_path`
- `screenshot.proxy_server`
- `screenshot.chrome_user_data_dir`
- `screenshot.chrome_profile_dir`
- `screenshot.chrome_remote_debug_url`
- 支持嵌套结构和自定义字段

服务性能相关配置（`system`）：

- `system.max_concurrent`：引擎编排并发数
- `system.cache_ttl`：查询缓存过期时间（秒）
- `system.cache_max_size`：内存缓存最大条目数
- `system.cache_cleanup_interval`：缓存清理周期（秒）

**示例 JSON 结构:**
```json
[
  {
    "ip": "1.2.3.4",
    "port": 80,
    "protocol": "http",
    "host": "example.com",
    "url": "http://example.com",
    "title": "Example Site",
    "body_snippet": "Welcome to...",
    "server": "nginx",
    "headers": {
      "Content-Type": "text/html"
    },
    "status_code": 200,
    "country_code": "CN",
    "region": "Beijing",
    "city": "Beijing",
    "asn": "AS4134",
    "org": "Example Org",
    "isp": "China Telecom",
    "source": "fofa",
    "extra": {}
  }
]
```

### 导出 Excel 格式

1. 点击 **"导出 Excel"** 按钮
2. 选择保存位置和文件名
3. 点击 **"保存"** 确认

**Excel 格式特点:**
- 结构化表格，易于查看
- 可用 Excel/WPS 等软件打开
- 支持筛选、排序、图表等操作

**Excel 表格结构:**
```
| IP      | Port | Protocol | Host        | URL                  | Title       | ... |
|---------|------|----------|-------------|----------------------|-------------|-----|
| 1.2.3.4 | 80   | http     | example.com | http://example.com   | Example Site| ... |
```

## 常见问题

### Q1: 查询返回 "未配置引擎" 错误

**原因**: 未配置或未选择引擎。

**解决方法**:
1. 点击 "配置引擎 API Key" 按钮
2. 填写至少一个引擎的 API Key
3. 勾选对应的引擎复选框
4. 重新执行查询

### Q2: 查询返回 "语法错误"

**原因**: UQL 语句格式不正确。

**解决方法**:
- 检查引号是否配对: `country="CN"`
- 检查逻辑运算符: `&&` 或 `||`
- 检查字段名是否正确
- 参考查询示例

### Q3: 查询结果为空

**可能原因**:
1. 查询条件过于严格，没有匹配的结果
2. API Key 配额已用完
3. 引擎服务暂时不可用
4. 网络连接问题

**解决方法**:
- 放宽查询条件
- 检查 API 配额
- 稍后重试
- 更换引擎

### Q4: 导出失败

**可能原因**:
1. 没有可导出的结果
2. 文件路径不存在或无写入权限
3. 磁盘空间不足

**解决方法**:
- 确保已执行查询且有结果
- 选择有权限的目录
- 检查磁盘空间

### Q5: 程序无法启动

**Linux 系统**:
可能缺少 GUI 依赖库。安装方法：

```bash
# Ubuntu/Debian
sudo apt-get install -y libgl1-mesa-dev xorg-dev

# Fedora/RHEL
sudo dnf install -y mesa-libGL-devel libXcursor-devel
```

**macOS/Windows**:
通常无需额外依赖。如果有问题，请检查：
- 文件是否有执行权限
- 是否被安全软件拦截

## 查询示例

### 示例 1: 查询中国境内的 Web 服务器

```
country="CN" && port="80"
```

**说明**: 查询位于中国且开放 80 端口的服务器。

### 示例 2: 查询 HTTPS 服务

```
protocol="https" && port="443"
```

**说明**: 查询使用 HTTPS 协议的服务器。

### 示例 3: 查询包含特定关键词的网站

```
title="管理后台" && country="CN"
```

**说明**: 查询标题包含"管理后台"且位于中国的网站。

### 示例 4: 查询多个端口

```
port IN ["80", "443", "8080", "8443"]
```

**说明**: 查询开放常见 Web 端口的服务器。

### 示例 5: 组合查询

```
country="CN" && (port="80" || port="443") && protocol="http"
```

**说明**: 查询中国境内，开放 80 或 443 端口，使用 HTTP 协议的服务器。

### 示例 6: 按服务器类型查询

```
server="nginx" && country="CN"
```

**说明**: 查询使用 Nginx 服务器且位于中国的网站。

### 示例 7: 查询特定组织

```
org="Tencent" && port="443"
```

**说明**: 查询属于腾讯组织且开放 443 端口的服务器。

### 示例 8: 查询特定城市

```
city="shanghai" && protocol="https"
```

**说明**: 查询位于上海且使用 HTTPS 的服务器。

## 高级技巧

### 1. 善用逻辑运算符

使用括号控制优先级：
```
(country="CN" || country="US") && port="80"
```

### 2. 结合多个条件

```
country="CN" && port="443" && title="login" && server="nginx"
```

### 3. 使用 IN 运算符批量查询

```
port IN ["21", "22", "23", "3389", "5900"]
```

查询常见远程管理端口。

### 4. 多引擎策略

- **快速查询**: 只选择 FOFA 或 Hunter
- **全面覆盖**: 同时选择所有引擎
- **对比验证**: 先单独查询，再组合查询

### 5. 结果去重

程序会自动基于 IP:Port 去重，相同资产只保留一条记录。

### 6. 导出策略

- **程序处理**: 导出 JSON 格式
- **人工分析**: 导出 Excel 格式
- **备份存档**: 两种格式都导出

## 技术支持

如果遇到问题或需要帮助：

1. 查看 [README_LIGHT.md](README_LIGHT.md) 获取更多信息
2. 提交 Issue: https://github.com/ElaineRosa6/unimap-icp-hunter/issues
3. 查看项目文档和示例

---

**UniMap Light 使用指南** © 2026 | 持续更新中
