# UniMap + ICP-Hunter 代码审查报告

## 执行时间
2025-01-01

## 审查范围
- 代码功能完成度检查
- 业务逻辑分析
- 代码逻辑分析
- Bug、漏洞和功能无效问题识别

---

## 一、配置文件问题

### 1.1 config.yaml YAML语法错误
**文件位置**: `configs/config.yaml`

**问题描述**:
- 第132行: `chrome_remote_debug_url` 缩进不正确，应该与前面的配置项对齐
- 第9行: Quake cookie的name字段包含未转义的双引号: `"test_cookie_enable`
- 第53行: Hunter cookie的name字段包含未转义的双引号: `"__8qcehdE7ZaRq2q6M__`
- 第87行: FOFA cookie的name字段包含未转义的双引号: `"theme`
- 第72行: Hunter cookie的token value包含未闭合的双引号

**影响**: 配置文件无法正确解析，可能导致程序启动失败

**修复建议**:
```yaml
# 第132行缩进修正
chrome_remote_debug_url: ""

# Cookie字段修正示例
- name: test_cookie_enable  # 移除多余的引号
```

---

## 二、业务逻辑问题

### 2.1 FOFA配额计算逻辑不合理
**文件位置**: `internal/adapter/fofa.go:446-486`

**问题描述**:
- FOFA配额计算逻辑过于复杂且硬编码了假设值
- 第465行: `total = remain + (300 - remain)` 逻辑无意义，结果总是300
- 个人版和付费版的区分逻辑可能不准确
- 配额信息可能与实际API响应不匹配

**影响**: 配额信息显示不准确，可能误导用户

**修复建议**:
- 简化配额计算逻辑
- 直接使用API返回的字段而不做过多假设
- 增加更详细的日志记录以便调试

### 2.2 Quake适配器URL构建不一致
**文件位置**: `internal/adapter/quake.go:165` 和 `internal/screenshot/manager.go:545`

**问题描述**:
- API调用使用: `https://quake.360.net/api`
- 截图URL使用: `https://quake.360.cn`
- 域名不一致可能导致某些功能异常

**影响**: 可能导致截图功能无法正常访问Quake搜索结果页

---

## 三、代码逻辑问题

### 3.1 资源泄漏风险 - Chrome进程管理
**文件位置**: `web/server.go:672-676`

**问题描述**:
```go
cmd := exec.Command(chromePath, args...)
if err := cmd.Start(); err != nil {
    return err
}
_ = cmd.Process.Release()  // 进程被释放后无法管理
```

**影响**: 
- Chrome进程可能成为僵尸进程
- 无法正确关闭或监控Chrome进程
- 长期运行可能导致系统资源耗尽

**修复建议**:
- 保存cmd引用以便后续管理
- 实现进程监控和清理机制
- 添加优雅关闭逻辑

### 3.2 WebSocket连接验证不足
**文件位置**: `web/server.go:1755-1784`

**问题描述**:
- 当没有配置`UNIMAP_WS_TOKEN`时，允许任意连接
- 虽然记录警告但仍允许连接
- 生产环境存在安全风险

**影响**: 潜在的未授权访问风险

**修复建议**:
- 生产环境强制要求token验证
- 添加配置选项控制是否允许无token连接
- 增加连接频率限制

### 3.3 错误处理不一致
**文件位置**: `internal/adapter/orchestrator.go:143-149` 和 `internal/adapter/orchestrator.go:306-315`

**问题描述**:
- `SearchTask` 使用errorChan发送错误
- `PaginatedSearchTask` 直接在result中返回错误
- 两种任务的错误处理模式不一致

**影响**: 代码维护困难，错误处理逻辑分散

### 3.4 缓存键生成潜在问题
**文件位置**: `internal/service/unified_service.go:91`

**问题描述**:
```go
cacheKey := fmt.Sprintf("%s:%s:%d:%t", strings.Join(sortedEngines, ","), req.Query, req.PageSize, req.ProcessData)
```
- 没有对查询字符串进行特殊字符转义
- 可能导致缓存键冲突

**影响**: 不同查询可能生成相同的缓存键

---

## 四、潜在Bug

### 4.1 配置文件加载失败后的行为
**文件位置**: `internal/config/config.go:120-160`

**问题描述**:
- 配置文件加载失败时，会应用默认配置并继续运行
- 但仍然返回错误给调用者
- 可能导致上层逻辑混乱

**影响**: 程序可能以默认配置运行，但上层收到错误信息

### 4.2 FOFA Normalize中的条件判断
**文件位置**: `internal/adapter/fofa.go:350-380`

**问题描述**:
```go
if asset.IP != "" && asset.Port > 0 {
    // ...
} else if asset.Host != "" {
    // ...
} else if asset.IP != "" {
    // ...
}
```
- 第三个条件永远不会被触发，因为第一个条件已经检查了IP
- 只有IP没有端口的资产可能被遗漏

**影响**: 某些资产可能无法正确标准化

### 4.3 Hunter配额计算可能为负数
**文件位置**: `internal/adapter/hunter.go:435`

**问题描述**:
```go
used := total - remain
```
- 如果`remain > total`，used会变成负数
- 没有边界检查

**影响**: 配额显示可能出现负数

---

## 五、安全问题

### 5.1 API密钥泄露风险
**文件位置**: `configs/config.yaml`

**问题描述**:
- 配置文件中包含明文API密钥
- 已提交到版本控制系统
- 多个引擎的真实API密钥暴露

**影响**: 
- API密钥可能被滥用
- 账户配额可能被盗用
- 安全违规风险

**修复建议**:
- 使用环境变量或密钥管理系统
- 将config.yaml添加到.gitignore
- 提供config.yaml.example作为模板
- 立即轮换已暴露的API密钥

### 5.2 WebSocket CheckOrigin过于宽松
**文件位置**: `web/server.go:106-110`

**问题描述**:
```go
CheckOrigin: func(r *http.Request) bool {
    return true // 允许所有来源的WebSocket连接
}
```

**影响**: 容易受到CSRF攻击

**修复建议**:
- 实现严格的Origin检查
- 允许配置可信来源列表

### 5.3 路径遍历风险
**文件位置**: `web/server.go` 和 `internal/screenshot/manager.go`

**问题描述**:
- 用户输入的URL可能用于文件操作
- 虽然有一定的验证，但可能存在绕过

**影响**: 潜在的路径遍历攻击风险

---

## 六、功能无效/不完善

### 6.1 Shodan适配器缺失
**问题描述**:
- 配置文件中有Shodan配置项
- 但代码中没有找到完整的Shodan适配器实现
- Shodan引擎无法使用

**影响**: Shodan功能不可用

### 6.2 缺少输入验证
**问题描述**:
- 多处用户输入缺少充分验证
- 查询长度限制为1000字符，但其他输入可能过长
- 文件名生成可能包含特殊字符

### 6.3 错误消息国际化不完善
**问题描述**:
- 错误消息混合中英文
- 没有统一的错误处理机制
- 用户体验不一致

---

## 七、代码质量问题

### 7.1 魔法数字
**文件位置**: 多处

**问题描述**:
- 硬编码的数字如300、1000、50等
- 缺乏常量定义

### 7.2 注释不足
**问题描述**:
- 复杂逻辑缺少注释说明
- 业务决策背景不清晰

### 7.3 测试覆盖有限
**问题描述**:
- 虽然有一些测试，但覆盖不够全面
- 关键路径缺少测试

---

## 八、修复优先级建议

### 高优先级（立即修复）
1. **配置文件YAML语法错误** - 阻止程序正常启动
2. **API密钥泄露** - 安全风险极高
3. **WebSocket CheckOrigin** - 安全风险
4. **FOFA Normalize条件判断** - 功能错误

### 中优先级（尽快修复）
1. **Chrome进程管理** - 资源泄漏
2. **配额计算逻辑** - 信息不准确
3. **错误处理一致性** - 代码可维护性
4. **缓存键生成** - 潜在bug

### 低优先级（逐步优化）
1. **代码注释** - 可维护性
2. **魔法数字** - 代码质量
3. **测试覆盖** - 代码质量
4. **错误消息国际化** - 用户体验

---

## 九、总结

### 主要发现
- **1个阻塞性问题**: 配置文件YAML语法错误
- **3个安全问题**: API密钥泄露、WebSocket安全、路径遍历风险
- **5个功能bug**: 配额计算、条件判断、资源泄漏等
- **多个代码质量问题**

### 整体评估
代码整体结构良好，模块化设计合理，核心功能基本完整。但存在一些需要立即修复的安全问题和功能性bug。建议优先处理高优先级问题，然后逐步优化中低优先级项目。

### 建议
1. 建立代码审查流程
2. 增加自动化测试覆盖
3. 实施安全编码规范
4. 完善错误处理和日志记录
5. 建立配置管理最佳实践
