# 更新日志 (Changelog)

## [2.0.1] - 2026-03-09

### 代码缺陷修复

#### 1. 错误处理优化
- **FOFA 适配器**: 优化 `result.Err` 错误处理逻辑，统一错误信息格式
  - 修复布尔类型和字符串类型错误的处理分支
  - 添加更详细的错误信息前缀
  - 文件: `internal/adapter/fofa.go`

#### 2. 空指针检查增强
- **编排器**: 添加 `engineNames` 空数组检查，防止无效查询
  - 在 `TranslateQuery` 方法中添加长度验证
  - 返回明确的错误信息
  - 文件: `internal/adapter/orchestrator.go`

#### 3. URL 构建安全修复
- **FOFA 适配器**: 使用 `url.URL` 结构体安全构建 URL，避免特殊字符问题
- **Hunter 适配器**: 使用 `url.URL` 结构体安全构建 URL
- **Shodan 适配器**: 使用 `url.URL` 结构体安全构建 URL
- 添加 `net/url` 导入，确保 URL 编码正确
- 文件: `internal/adapter/fofa.go`, `hunter.go`, `shodan.go`

#### 4. 统一日志记录
- **CLI 工具**: 将 `log.Printf`/`log.Fatalf` 替换为 `logger.Warnf`/`logger.Errorf`
- **Web 服务**: 将 `fmt.Println` 替换为 `logger.Info`
- 统一使用内部 logger 模块，支持日志级别控制
- 文件: `cmd/unimap-cli/main.go`, `cmd/unimap-web/main.go`

### 新增功能

#### 1. 优雅关闭机制
- **新增文件**: `internal/utils/shutdown.go`
  - `ShutdownManager` 结构体，管理应用生命周期
  - 支持信号监听（SIGINT, SIGTERM, SIGHUP）
  - 支持并发执行关闭处理函数
  - 支持超时控制（默认 30 秒）
  - 简化的 `GracefulShutdown` 辅助函数

- **Web 服务器**: 添加 `Shutdown` 方法
  - 支持优雅关闭 HTTP 服务器
  - 关闭所有 WebSocket 连接
  - 清理连接管理器资源
  - 文件: `web/server.go`

- **CLI 和 Web 入口**: 集成优雅关闭
  - CLI: 服务关闭时调用 `svc.Shutdown()`
  - Web: 信号触发时按顺序关闭服务器和服务
  - 文件: `cmd/unimap-cli/main.go`, `cmd/unimap-web/main.go`

### 架构改进

#### 1. HTTP 服务器重构
- 使用 `http.NewServeMux()` 替代默认多路复用器
- 添加 `httpServer` 字段到 `Server` 结构体
- 支持通过 `Shutdown` 方法优雅关闭

#### 2. 代码质量
- 所有代码通过 `go vet` 检查
- 所有现有测试通过 (`go test ./...`)
- 移除未使用的导入

### 兼容性
- 向后兼容：所有改进均为内部实现优化
- 无配置文件变更
- 无 API 变更

---

## [2.0.0] - 2026-03-03

### 新增功能

#### 1. Shodan 搜索引擎支持
- 新增 Shodan 引擎适配器 (`internal/adapter/shodan.go`)
- 支持 Shodan API 查询和结果标准化
- 在 CLI、Web 和 GUI 中注册 Shodan 引擎
- 配置文件添加 Shodan 配置项

#### 2. 批量 URL 截图功能
- 新增批量 URL 截图页面 (`/batch-screenshot`)
- 支持上传 URL 列表进行批量截图
- 支持并发截图（可配置 1-10 个并发）
- 支持文件导入：.txt、.csv、.xlsx 格式
- 自动 URL 标准化和去重
- 截图结果实时显示和下载

#### 3. 文件导入功能
- 支持从文件导入 URL 列表
- 支持格式：TXT（每行一个）、CSV（第一列）、Excel（第一列）
- 自动识别表头并跳过
- 自动去重和验证 URL 格式

### 优化改进

#### 1. Chrome 截图优化
- **智能 Chrome 路径检测**：自动检测 Windows/Linux/macOS 常见 Chrome 安装路径
- **CDP 模式自动回退**：当远程调试端口不可用时，自动切换到本地启动模式
- **Cookie 设置优化**：CDP 模式下跳过 Cookie 设置（浏览器已保持登录状态）

#### 2. 代码质量改进
- **UTF-8 解析修复**：修复 UQL 解析器 UTF-8 字符串遍历问题
- **缓存错误处理**：优化 orchestrator 缓存类型断言和错误处理
- **锁粒度优化**：merger 中将 generateKey 移到锁外，提高并发性能
- **对象池清理**：确保从对象池获取的对象已清理

#### 3. 配置管理优化
- 启动时自动检测远程调试端口可用性
- 不可用时自动清空配置，使用本地 Chrome
- 添加详细的日志提示

### 修复问题

#### 1. 截图功能修复
- 修复 "No connection could be made" 错误
- 修复 Chrome 路径检测失败问题
- 修复 CDP 模式下重复设置 Cookie 问题

#### 2. 解析器修复
- 修复 UQL 解析器多字节字符处理
- 修复 tokenize 函数 UTF-8 遍历错误

#### 3. 缓存修复
- 修复缓存类型断言可能 panic 的问题
- 修复 Normalize 错误被静默忽略的问题

### 文档更新

#### 1. README.md
- 更新功能特性列表
- 添加批量截图功能说明
- 更新 CDP 模式使用说明
- 添加 Chrome 路径配置说明

#### 2. README_LIGHT.md
- 更新轻量版功能说明
- 添加文件导入功能说明
- 更新截图功能说明

#### 3. USAGE.md
- 添加批量截图使用指南
- 更新 CDP 模式最佳实践
- 添加文件导入格式说明

### API 变更

#### 新增接口
```
POST /api/screenshot/batch-urls    # 批量 URL 截图
POST /api/import/urls              # 导入 URL 文件
GET  /batch-screenshot             # 批量截图页面
```

#### 配置变更
```yaml
engines:
  shodan:                          # 新增 Shodan 配置
    enabled: false
    api_key: ""
    base_url: "https://api.shodan.io"
    qps: 1
    timeout: 30
```

### 依赖更新
- 添加 `github.com/xuri/excelize/v2` 用于 Excel 文件解析

### 性能改进
- 批量截图支持并发处理
- 文件导入支持大文件（最大 10MB）
- 优化内存使用（对象池复用）

### 兼容性
- 向后兼容：现有配置文件无需修改即可运行
- 新增功能默认关闭，需手动启用

---

## [1.0.0] - 2026-01-15

### 初始版本
- UniMap + ICP-Hunter 完整功能
- 支持 FOFA、Hunter、ZoomEye、Quake 引擎
- 支持 UQL 统一查询语言
- 支持 Web 服务和 GUI 界面
- 支持截图和 CDP 模式
- 支持 ICP 备案检测

---

**更新日期**: 2026-03-03  
**版本号**: v2.0.0  
**更新者**: Claude Code
