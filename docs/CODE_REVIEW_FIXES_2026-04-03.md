# 项目缺陷检查与修复报告

**日期**: 2026-04-03
**检查范围**: 全项目代码审查

## 一、已修复的问题

### 1. 浏览器扩展截图权限错误 (高优先级)

**问题描述**:
使用浏览器扩展进行批量截图时报错：
```
Error: plugin_capture_failed: Error: Either the '' or 'activeTab' permission is required.
```

**原因分析**:
- `captureVisibleTab` API 需要 `<all_urls>` 权限或用户交互触发的 `activeTab` 权限
- 后台轮询截图模式没有用户交互，无法激活 `activeTab`

**修复方案**:
- 文件: `tools/extension-screenshot/manifest.json`
- 将 `host_permissions` 从具体URL改为 `<all_urls>`

```json
"host_permissions": [
  "<all_urls>"
]
```

---

### 2. 批量截图内存消耗过大 (高优先级)

**问题描述**:
- 批量截图时每个URL创建新Tab但不关闭
- 大量URL导致浏览器内存耗尽、系统卡顿

**修复方案**:
- 文件: `tools/extension-screenshot/src/capture.js`
- 实现Tab池管理机制：
  - 最大保留3个Tab用于复用
  - 截图完成后释放Tab（复用或关闭）
  - 导航到 `about:blank` 释放页面内存
  - 30秒后自动清理闲置Tab池

```javascript
// Tab pool for reuse - limits memory usage
let tabPool = [];
const MAX_TAB_POOL_SIZE = 3;
const TAB_REUSE_TIMEOUT_MS = 30000;
```

---

### 3. Rate Limiter Goroutine泄漏 (中优先级)

**问题描述**:
- `RateLimiter.cleanup()` goroutine无限循环运行
- Ticker从未停止，导致goroutine和ticker泄漏

**修复方案**:
- 文件: `web/middleware_ratelimit.go`
- 添加 `Stop()` 方法和停止通道

```go
type RateLimiter struct {
    // ...
    stopChan   chan struct{} // 停止信号
    stopped    bool
}

func (r *RateLimiter) Stop() {
    r.mu.Lock()
    if !r.stopped {
        r.stopped = true
        close(r.stopChan)
    }
    r.mu.Unlock()
}
```

---

### 4. Query Status清理Goroutine泄漏 (中优先级)

**问题描述**:
- 查询状态清理goroutine使用 `time.Sleep` 无法被取消
- 服务关闭时goroutine继续运行5分钟

**修复方案**:
- 文件: `web/websocket_handlers.go`, `web/server.go`
- 添加 `shutdownCtx` 支持服务关闭时取消后台任务

```go
// Server结构体新增字段
shutdownCtx    context.Context
shutdownCancel context.CancelFunc

// 清理goroutine改为select模式
go func() {
    select {
    case <-time.After(5 * time.Minute):
        // 清理...
    case <-s.shutdownCtx.Done():
        // 服务关闭，立即清理
    }
}()
```

---

### 5. 分布式模式默认行为不安全 (中优先级)

**问题描述**:
- 当 `config == nil` 时，分布式模式默认返回 `true`
- 可能意外暴露节点管理端点

**修复方案**:
- 文件: `web/node_handlers.go`
- 默认禁用分布式模式

```go
func (s *Server) isDistributedEnabled() bool {
    if s.config == nil {
        return false  // 安全默认值
    }
    return s.config.Distributed.Enabled
}
```

---

### 6. 类型断言Panic风险 (中优先级)

**问题描述**:
- `node_handlers.go` 中排序函数直接进行类型断言
- 若 `egress_ip` 为nil或非字符串会导致panic

**修复方案**:
- 文件: `web/node_handlers.go`
- 添加类型断言检查

```go
sort.Slice(egressSummary, func(i, j int) bool {
    a, aOk := egressSummary[i]["egress_ip"].(string)
    b, bOk := egressSummary[j]["egress_ip"].(string)
    if !aOk {
        a = ""
    }
    if !bOk {
        b = ""
    }
    return strings.TrimSpace(a) < strings.TrimSpace(b)
})
```

---

### 7. 扩展API URL硬编码 (低优先级)

**问题描述**:
- 扩展API基础URL硬编码为 `http://127.0.0.1:8448`
- 无法适配不同服务器配置

**修复方案**:
- 文件: `tools/extension-screenshot/src/api.js`, `storage.js`
- API URL改为可配置，存储在chrome.storage

```javascript
// storage.js
export async function saveAPIBaseURL(url) {
  await chrome.storage.local.set({ apiBaseURL: url });
}

export async function loadAPIBaseURL() {
  const data = await chrome.storage.local.get(["apiBaseURL"]);
  return data.apiBaseURL || "http://127.0.0.1:8448";
}
```

---

### 8. 前端批量截图并发过高 (中优先级)

**问题描述**:
- 前端批量截图每批5个并发请求
- 对大量URL仍会造成服务器压力

**修复方案**:
- 文件: `web/static/js/main.js`
- 使用批量URL截图API，并发降至3

```javascript
fetch('/api/screenshot/batch-urls', {
    method: 'POST',
    body: JSON.stringify({
        urls: urls,
        batch_id: batchID,
        concurrency: 3  // 降低并发
    })
})
```

---

## 二、剩余建议性问题

### 1. 模板渲染错误未处理 (低优先级)

**位置**: 多个handler文件
**问题**: `ExecuteTemplate` 调用忽略错误返回值
**建议**: 检查错误并记录日志

```go
if err := s.templates.ExecuteTemplate(w, "index.html", data); err != nil {
    logger.Errorf("Template rendering failed: %v", err)
}
```

---

### 2. 分布式任务队列无持久化 (中优先级)

**位置**: `internal/distributed/task_queue.go`
**问题**: 任务仅存储在内存，服务重启后丢失
**建议**: 添加持久化存储（如SQLite或文件）

---

### 3. Node Registry无清理机制 (低优先级)

**位置**: `internal/distributed/registry.go`
**问题**: 离线节点永不移除，map无限增长
**建议**: 定期清理超时的离线节点

---

### 4. SSRF风险 (中优先级)

**位置**: `web/screenshot_handlers.go`
**问题**: 截图端点接受任意URL，无白名单验证
**建议**: 添加URL白名单或禁止内网地址

```go
func isURLAllowed(targetURL string) bool {
    // 禁止内网地址
    parsed, _ := url.Parse(targetURL)
    host := parsed.Hostname()
    if net.ParseIP(host).IsPrivate() {
        return false
    }
    return true
}
```

---

## 三、修改文件清单

| 文件 | 修改内容 |
|------|----------|
| `tools/extension-screenshot/manifest.json` | 权限修复 |
| `tools/extension-screenshot/src/api.js` | API URL可配置 |
| `tools/extension-screenshot/src/storage.js` | 新增URL存储函数 |
| `tools/extension-screenshot/src/capture.js` | Tab池管理 |
| `tools/extension-screenshot/src/background.js` | Tab释放逻辑 |
| `web/middleware_ratelimit.go` | Goroutine泄漏修复 |
| `web/websocket_handlers.go` | 上下文取消支持 |
| `web/server.go` | shutdownCtx字段 |
| `web/node_handlers.go` | 默认值修复、类型安全 |
| `web/node_auth.go` | 注释完善 |
| `web/static/js/main.js` | 批量截图优化 |

---

## 四、使用说明

### 重新加载扩展
修改manifest后需在Chrome扩展管理页面重新加载扩展

### 配置扩展API地址（可选）
```javascript
// 在扩展控制台执行
chrome.storage.local.set({ apiBaseURL: "http://your-server:port" })
```

### 内存优化参数调整
- `MAX_TAB_POOL_SIZE`: Tab池大小（默认3）
- `TAB_REUSE_TIMEOUT_MS`: Tab复用超时（默认30秒）
- `concurrency`: 批量截图并发数（默认3）

---

## 五、测试建议

1. **扩展截图测试**
   - 测试单个URL截图
   - 测试批量截图（10+ URL）
   - 验证Tab是否正确释放

2. **服务关闭测试**
   - 启动服务后执行查询
   - 关闭服务，确认无goroutine泄漏

3. **内存监控**
   - 监控浏览器内存使用
   - 验证Tab数量是否正常