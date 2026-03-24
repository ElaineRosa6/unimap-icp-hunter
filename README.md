# UniMap

多引擎网络空间资产查询与网页监控工具，提供 Web、CLI、GUI 三种入口，支持查询、截图、篡改检测与结果导出。

## 当前版本定位

本仓库当前主线以 UniMap 查询与监控能力为主：

- 多引擎统一查询：FOFA、Hunter、ZoomEye、Quake、Shodan
- UQL 查询语言：统一语法，多引擎翻译
- Web 控制台：查询、Cookie 管理、截图、篡改检测、历史记录
- CLI 工具：快速查询与导出
- GUI 工具：桌面交互查询
- 网页篡改检测：基线管理、历史记录、宽松/严格检测模式

说明：仓库中仍保留部分历史文档（ICP-Hunter/Docker 全链路描述），请以本 README 与 QUICKSTART 为准。

## 目录结构

```text
cmd/
  unimap-cli/      CLI 入口
  unimap-gui/      GUI 入口
  unimap-web/      Web 入口
internal/
  adapter/         引擎适配与编排
  core/unimap/     UQL 解析与结果归并
  service/         统一服务层
  plugin/          插件与处理管道
  screenshot/      截图能力
  tamper/          网页篡改检测
  config/          配置管理
web/
  server.go        Web 服务与路由
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

### 3. 使用 CLI

```bash
go run ./cmd/unimap-cli -q 'country="CN" && port="80"' -e fofa,hunter -l 100 -o result.csv
```

### 4. 使用 GUI

```bash
go run -tags gui ./cmd/unimap-gui
```

GUI 构建依赖请参考 GUI_BUILD.md。

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

## 技术栈

- Go 1.24
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

## 许可证

MIT
