# 全量代码深度审查报告

- 审查对象：当前项目全部 Go 代码、Web 路由、中间件、调度器、分布式任务、截图/篡改检测、备份、缓存、CLI/GUI 相关路径
- 技术栈判断：Go 单体项目，核心模块位于 `web/`、`internal/service`、`internal/scheduler`、`internal/distributed`、`internal/screenshot`、`internal/tamper`、`internal/backup`、`internal/utils`
- 审查维度：功能性 Bug、安全性问题、逻辑与代码质量、业务逻辑问题
- 说明：受项目规则限制，本报告不使用 emoji；严重程度按“严重 > 高 > 中 > 低”排序。

## 严重问题

*(会导致系统崩溃、数据丢失、资金损失或严重安全泄露的问题)*

- **[认证授权] WebSocket 绕过全局 Admin Token 鉴权**
  - **位置**：`web/server.go` 第 563-569 行；`web/websocket_handlers.go` 第 133-152 行
  - **风险**：`/api/ws` 在 `rootHandler` 中直接调用 `s.handleWebSocket`，没有经过 `adminAuthMiddleware`。只要 `UNIMAP_WS_TOKEN` 未配置，任意客户端都能建立 WebSocket 并触发查询，绕过 Web 管理端鉴权。
  - **修复建议**：不要在 `rootHandler` 中绕过已包装的 `handler`；WebSocket 也必须经过统一鉴权。若保留独立 WebSocket Token，也应与 Admin Token 二选一校验。
  - **修复代码**：

```go
rootHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    handler.ServeHTTP(w, r)
})

func (s *Server) validateWebSocketRequest(r *http.Request) bool {
    if s.adminToken() != "" {
        token := r.Header.Get("X-Admin-Token")
        if token == "" {
            token = extractBearerToken(r.Header.Get("Authorization"))
        }
        if subtle.ConstantTimeCompare([]byte(token), []byte(s.adminToken())) == 1 {
            return true
        }
    }

    configToken := os.Getenv("UNIMAP_WS_TOKEN")
    token := r.Header.Get("X-WebSocket-Token")
    if configToken == "" || token == "" {
        return false
    }
    return subtle.ConstantTimeCompare([]byte(token), []byte(configToken)) == 1
}
```

- **[认证授权/资源管理] Bridge API 绕过认证、限流、请求体限制和审计**
  - **位置**：`web/server.go` 第 572-575 行；`web/screenshot_bridge_handlers.go` 第 166 行、第 524 行
  - **风险**：`/api/screenshot/bridge/` 被直接交给原始 `mux`，绕过 `adminAuthMiddleware`、`requestSizeLimitMiddleware`、`auditMiddleware`、`metricsMiddleware`。其中 `handleScreenshotBridgeMockResult` 直接 `io.ReadAll`，随后 base64 解码整块图片数据，攻击者可提交超大 body 造成内存/磁盘 DoS。
  - **修复建议**：Bridge API 不应绕过全局 middleware；对回调接口单独加 `http.MaxBytesReader`，并强制 token/HMAC 校验。
  - **修复代码**：

```go
rootHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    handler.ServeHTTP(w, r)
})

func (s *Server) handleScreenshotBridgeMockResult(w http.ResponseWriter, r *http.Request) {
    if !requireMethod(w, r, http.MethodPost) {
        return
    }

    maxBodyBytes := int64(10 * 1024 * 1024)
    if s.config != nil && s.config.Web.RequestLimits.MaxBodyBytes > 0 {
        maxBodyBytes = s.config.Web.RequestLimits.MaxBodyBytes
    }
    r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

    token, ok := s.validateBridgeAuthIfRequired(w, r)
    if !ok || strings.TrimSpace(token) == "" {
        writeAPIError(w, http.StatusUnauthorized, "unauthorized_bridge", "bridge token required", nil)
        return
    }

    rawBody, err := io.ReadAll(r.Body)
    if err != nil {
        writeAPIError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body too large", nil)
        return
    }
}
```

- **[SSRF] 内网地址拦截只检查字面 IP，不解析域名**
  - **位置**：`web/http_helpers.go` 第 201-205 行；`web/monitor_handlers.go` 第 105-115 行、第 156-166 行；`web/screenshot_handlers.go` 第 156-159 行
  - **风险**：`isPrivateOrInternalIP("evil.example.com")` 会返回 `false`，即使该域名解析到 `127.0.0.1`、`10.0.0.0/8` 或云元数据地址。截图、URL 可达性、端口扫描都可被 DNS Rebinding 或恶意域名绕过，形成 SSRF/内网扫描。
  - **修复建议**：对 hostname 做 DNS 解析，所有解析结果都必须是公网 IP；HTTP client 还要限制 redirect 后的目标地址。
  - **修复代码**：

```go
func isPrivateOrInternalHost(ctx context.Context, host string) bool {
    host = strings.Trim(strings.TrimSpace(host), "[]")
    if host == "" {
        return false
    }

    if ip := net.ParseIP(host); ip != nil {
        return isBlockedIP(ip)
    }

    ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
    if err != nil {
        return true
    }
    for _, addr := range ips {
        if isBlockedIP(addr.IP) {
            return true
        }
    }
    return false
}

func isBlockedIP(ip net.IP) bool {
    return ip.IsLoopback() ||
        ip.IsPrivate() ||
        ip.IsLinkLocalUnicast() ||
        ip.IsLinkLocalMulticast() ||
        ip.IsUnspecified()
}
```

- **[并发/死锁] 删除定时任务会自死锁**
  - **位置**：`internal/scheduler/scheduler.go` 第 496-508 行；`internal/scheduler/scheduler.go` 第 332-338 行
  - **风险**：`DeleteTask` 已持有 `s.mu.Lock()`，第 507 行又调用 `s.Save()`，`Save()` 再尝试 `s.mu.RLock()`。Go 的 `RWMutex` 不支持锁升级/重入，该请求会永久卡死，相关 HTTP handler 线程挂起，定时任务管理可能整体不可用。
  - **修复建议**：持锁场景下调用 `saveLocked()`，不要再进入 `Save()`。
  - **修复代码**：

```go
func (s *Scheduler) DeleteTask(id string) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    if entryID, ok := s.cronIDs[id]; ok {
        s.cron.Remove(entryID)
        delete(s.cronIDs, id)
    }
    if _, ok := s.tasks[id]; !ok {
        return fmt.Errorf("task %s not found", id)
    }
    delete(s.tasks, id)
    return s.saveLocked()
}
```

- **[功能性 Bug] 自动生成的 Admin Token 不输出、不持久化，默认启动后管理端不可用**
  - **位置**：`internal/config/config.go` 第 694-700 行；`web/server.go` 第 549-552 行
  - **风险**：未配置 `web.auth.admin_token` 时，代码自动生成随机 token 并强制启用认证，但启动日志只打印 “admin token authentication active”，没有输出 token，也没有持久化到配置。结果是默认启动后用户无法知道 token，所有非公开页面/API 都会 401，管理端被自己锁死。服务重启后 token 还会变化，自动化脚本也会失效。
  - **修复建议**：不要静默生成不可见 token。生产推荐启动失败并要求显式配置；开发模式可生成 token 但必须只在本地绑定时打印一次，或写入受权限保护的本地文件。
  - **修复代码**：

```go
if strings.TrimSpace(config.Web.Auth.AdminToken) == "" {
    if config.Web.BindAddress != "127.0.0.1" && config.Web.BindAddress != "localhost" {
        return fmt.Errorf("web.auth.admin_token is required when binding non-loopback address")
    }

    token := generateSecureToken(32)
    config.Web.Auth.AdminToken = token
    config.Web.Auth.Enabled = true
    logger.Warnf("Generated development admin token: %s", token)
}
```

- **[资源管理/进程状态] Chrome 子进程退出后 `s.chromeCmd` 永远非 nil，CDP 无法自愈**
  - **位置**：`web/cdp_handlers.go` 第 240-245 行、第 302-315 行
  - **风险**：`startCDPChrome` 只判断 `s.chromeCmd != nil` 就认为 Chrome 已启动。实际 Chrome 进程退出后，后台 goroutine 只 `cmd.Wait()`，没有把 `s.chromeCmd` 清空。后续调用会直接返回 nil，不会重新拉起 Chrome，导致 CDP 截图/登录状态检测永久不可用，直到服务重启。
  - **修复建议**：存储 `*exec.Cmd` 而不是裸 `*os.Process`；`Wait()` 结束后在锁内清空；启动前检查进程是否仍存活。
  - **修复代码**：

```go
cmd := exec.Command(chromePath, args...)
if err := cmd.Start(); err != nil {
    return err
}

s.chromeCmd = cmd.Process

go func() {
    err := cmd.Wait()
    s.chromeCmdMu.Lock()
    if s.chromeCmd == cmd.Process {
        s.chromeCmd = nil
    }
    s.chromeCmdMu.Unlock()

    if err != nil {
        logger.Debugf("Chrome process exited: %v", err)
    }
}()
```

## 高优先级问题

*(大概率触发 Bug、存在明显安全漏洞或严重性能问题)*

- **[SSRF] Webhook 校验可被 DNS/重定向绕过**
  - **位置**：`internal/scheduler/scheduler.go` 第 635-661 行、第 1023-1024 行
  - **风险**：Webhook URL 创建时只校验字面 hostname；发送时使用默认 `http.Client`，会跟随重定向。攻击者可使用公网域名解析到内网，或先返回 302 跳转到内网元数据地址，绕过当前 SSRF 防护。
  - **修复建议**：发送前和每次 redirect 都重新校验目标；自定义 `Transport.DialContext`，在实际连接 IP 层阻断私网地址。
  - **修复代码**：

```go
client := &http.Client{
    Timeout: 30 * time.Second,
    CheckRedirect: func(req *http.Request, via []*http.Request) error {
        if err := ValidateWebhookURLPublic(req.URL.String()); err != nil {
            return err
        }
        if isPrivateOrInternalHost(req.Context(), req.URL.Hostname()) {
            return fmt.Errorf("webhook redirect target is private/internal")
        }
        return nil
    },
}
```

- **[并发/认证] APIKeyManager 在读锁下写状态，且持久化后密钥不可再验证**
  - **位置**：`internal/auth/api_key.go` 第 87-104 行、第 196-235 行
  - **风险**：`ValidateAPIKey` 使用 `RLock`，但第 103 行修改 `apiKey.Status`，并发下会数据竞争。更严重的是 `saveToStorage` 故意把 `Key` 清空，`loadFromStorage` 又用 `key.ID` 作为 map key，服务重启后原始 API Key 永远无法通过验证。
  - **修复建议**：不要存明文 Key，存 Key Hash；内存 map 以 hash 为 key；修改状态时使用写锁。
  - **修复代码**：

```go
func (m *APIKeyManager) ValidateAPIKey(key string) (*APIKey, error) {
    keyHash := hashAPIKey(key)

    m.mutex.Lock()
    defer m.mutex.Unlock()

    apiKey, exists := m.keys[keyHash]
    if !exists {
        return nil, unierror.APIUnauthorized("Invalid API key")
    }
    if apiKey.Status != "active" {
        return nil, unierror.APIUnauthorized("API key is not active")
    }
    if !apiKey.ExpiresAt.IsZero() && time.Now().After(apiKey.ExpiresAt) {
        apiKey.Status = "expired"
        m.saveToStorage()
        return nil, unierror.APIUnauthorized("API key has expired")
    }
    return apiKey, nil
}

func hashAPIKey(key string) string {
    sum := sha256.Sum256([]byte(key))
    return hex.EncodeToString(sum[:])
}
```

- **[资源管理] 篡改检测并发数未设置上限**
  - **位置**：`web/tamper_handlers.go` 第 54-69 行、第 96-109 行；`internal/service/tamper_app_service.go` 第 60-74 行、第 135-145 行
  - **风险**：用户可传入极大的 `concurrency`，直接进入 `BatchCheckTampering`/`BatchSetBaseline`，可能同时拉起大量浏览器页面或网络请求，导致 CPU、内存、文件句柄耗尽。
  - **修复建议**：限制 URL 数量和并发上限，例如并发最大 10，URL 最大 100。
  - **修复代码**：

```go
func normalizeTamperConcurrency(v int) int {
    if v <= 0 {
        return 5
    }
    if v > 10 {
        return 10
    }
    return v
}

if len(req.URLs) > 100 {
    return nil, fmt.Errorf("too many URLs: max 100")
}
req.Concurrency = normalizeTamperConcurrency(req.Concurrency)
```

- **[功能性 Bug] 截图文件服务路径校验在默认相对路径下会误判失败**
  - **位置**：`web/screenshot_handlers.go` 第 100-109 行
  - **风险**：`absFullPath` 是绝对路径，但 `baseDir` 默认可能是相对路径 `./screenshots`。`filepath.Rel(baseDir, absFullPath)` 会因 base/target 一个相对一个绝对而返回错误，导致已生成截图无法访问。
  - **修复建议**：先把 `baseDir` 转成绝对路径，再做 `Rel`。
  - **修复代码**：

```go
baseDir := s.resolveScreenshotBaseDir()
absBaseDir, err := filepath.Abs(baseDir)
if err != nil {
    writeAPIError(w, http.StatusInternalServerError, "invalid_base_dir", "invalid screenshot base dir", nil)
    return
}

fullPath := filepath.Join(absBaseDir, cleanRelPath)
absFullPath, err := filepath.Abs(fullPath)
if err != nil {
    writeAPIError(w, http.StatusBadRequest, "invalid_path", "invalid path", nil)
    return
}

relToBase, err := filepath.Rel(absBaseDir, absFullPath)
if err != nil || relToBase == "." || strings.HasPrefix(relToBase, ".."+string(os.PathSeparator)) || relToBase == ".." {
    writeAPIError(w, http.StatusBadRequest, "invalid_path", "invalid path", nil)
    return
}
```

- **[安全性问题] 限流信任客户端可伪造的 `X-Forwarded-For`，可被直接绕过**
  - **位置**：`web/middleware_ratelimit.go` 第 214-229 行
  - **风险**：限流 key 直接取 `X-Forwarded-For` 第一个 IP 或 `X-Real-IP`，没有判断请求是否来自可信代理。攻击者每次请求伪造不同 `X-Forwarded-For` 即可绕过所有 API 限流，批量触发查询、截图、篡改检测等高成本操作。
  - **修复建议**：只有 `RemoteAddr` 属于可信代理网段时才读取代理头；否则使用真实 `RemoteAddr`。
  - **修复代码**：

```go
func getClientIP(r *http.Request) string {
    remoteHost, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
    if err != nil {
        remoteHost = strings.TrimSpace(r.RemoteAddr)
    }

    if isTrustedProxy(net.ParseIP(remoteHost)) {
        if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
            parts := strings.Split(xff, ",")
            if ip := net.ParseIP(strings.TrimSpace(parts[0])); ip != nil {
                return ip.String()
            }
        }
        if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
            if ip := net.ParseIP(xri); ip != nil {
                return ip.String()
            }
        }
    }

    return remoteHost
}

func isTrustedProxy(ip net.IP) bool {
    return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}
```

- **[数据泄露/归档安全] 备份 tar 使用绝对/原始路径作为 entry name**
  - **位置**：`internal/backup/backup.go` 第 178-192 行
  - **风险**：`header.Name = path` 会把源文件的原始路径写入 tar。如果配置了绝对路径，备份包会泄露服务器目录结构；解压时还可能生成绝对路径或包含 `..` 的危险 entry，给后续恢复工具埋下 Tar Slip 风险。
  - **修复建议**：以每个 source 为 root 计算相对路径，强制使用 `/` 分隔，拒绝绝对路径和 `..` entry。
  - **修复代码**：

```go
func addFileToTar(tw *tar.Writer, root string, path string) error {
    info, err := os.Stat(path)
    if err != nil {
        return err
    }

    rel, err := filepath.Rel(root, path)
    if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
        return fmt.Errorf("unsafe backup path: %s", path)
    }

    header, err := tar.FileInfoHeader(info, "")
    if err != nil {
        return err
    }
    header.Name = filepath.ToSlash(rel)

    if err := tw.WriteHeader(header); err != nil {
        return err
    }
    if info.IsDir() {
        return nil
    }

    f, err := os.Open(path)
    if err != nil {
        return err
    }
    defer f.Close()

    _, err = io.Copy(tw, f)
    return err
}
```

- **[数据泄露] 默认备份包含 `configs`，会把 API Key、Cookie、Token 一起打包**
  - **位置**：`web/backup_handlers.go` 第 118-121 行；`internal/backup/backup.go` 第 69-87 行
  - **风险**：默认备份源包含 `./configs`，而配置中存在 FOFA/Hunter/Quake/ZoomEye/Shodan API Key、Cookie、Redis 密码、Webhook Token 等敏感信息。备份包一旦被下载、同步或误传，等同于凭据泄露。
  - **修复建议**：默认不备份敏感配置；或备份前做字段脱敏；备份结果不应返回完整服务器路径。
  - **修复代码**：

```go
func (s *Server) buildBackupSources() []string {
    sources := []string{}

    if dirExists("./hash_store") {
        sources = append(sources, "./hash_store")
    }
    if dirExists("./screenshots") {
        sources = append(sources, "./screenshots")
    }
    if dirExists("./data") {
        sources = append(sources, "./data")
    }

    return sources
}
```

- **[安全配置] 配置默认绑定 `0.0.0.0`，与 `Server.bindAddr()` 的安全默认值冲突**
  - **位置**：`internal/config/config.go` 第 664-666 行；`web/server.go` 第 689-695 行
  - **风险**：`Server.bindAddr()` 声称默认 `127.0.0.1`，但配置默认值提前把 `config.Web.BindAddress` 设置为 `0.0.0.0`。实际只要走配置加载，服务默认监听所有网卡。结合 WebSocket/Bridge 绕过、限流可绕过等问题，暴露面被放大。
  - **修复建议**：配置层默认也改为 `127.0.0.1`，公网监听必须显式配置。
  - **修复代码**：

```go
if config.Web.BindAddress == "" {
    config.Web.BindAddress = "127.0.0.1"
}
```

## 中优先级问题

*(代码逻辑不严谨、不符合业务常理、存在一致性隐患)*

- **[数据泄露] Admin Token 支持 URL Query 传递**
  - **位置**：`web/middleware_auth.go` 第 20-24 行
  - **风险**：`?admin_token=xxx` 会进入浏览器历史、反向代理日志、Referer、监控链路和审计日志，管理密钥泄露概率高。
  - **修复建议**：禁用 Query Token，只允许 `Authorization: Bearer` 或 `X-Admin-Token` Header。
  - **修复代码**：

```go
token := r.Header.Get("X-Admin-Token")
if token == "" {
    token = extractBearerToken(r.Header.Get("Authorization"))
}
if token == "" {
    writeJSON(w, http.StatusUnauthorized, map[string]string{
        "error": "unauthorized: admin token header required",
    })
    return
}
```

- **[信息泄露] 多处接口把内部错误原样返回给前端**
  - **位置**：`web/screenshot_handlers.go` 第 193-200 行；`web/monitor_handlers.go` 第 123-125 行、第 173-176 行
  - **风险**：错误中可能包含本机路径、Chrome 启动参数、代理地址、目标网络错误、内部服务地址，给攻击者提供环境指纹。
  - **修复建议**：客户端返回稳定错误码和泛化信息，详细错误只写服务端日志。
  - **修复代码**：

```go
if err != nil {
    logger.Errorf("screenshot failed: %v", err)
    writeAPIError(w, http.StatusInternalServerError, "screenshot_failed", "screenshot failed", nil)
    return
}
```

- **[业务一致性] 分布式任务队列是纯内存实现，服务重启会丢失任务状态**
  - **位置**：`web/server.go` 第 224-227 行；`internal/distributed/task_queue.go` 第 80-95 行
  - **风险**：节点任务 enqueue/claim/result 全部落在内存 map 中。服务重启后 pending/claimed 任务、执行结果、重试状态全部丢失，正在执行的任务无法恢复，业务上会出现“任务已下发但平台无记录”的状态不一致。
  - **修复建议**：将任务队列状态持久化到文件/数据库/Redis；至少在状态变更时写 WAL，启动时恢复 pending/claimed 并按 lease 做补偿。
  - **修复代码**：

```go
type TaskStore interface {
    Save(tasks map[string]*TaskRecord, pending []string) error
    Load() (map[string]*TaskRecord, []string, error)
}

func (q *TaskQueue) persistLocked() error {
    if q.store == nil {
        return nil
    }
    return q.store.Save(q.tasks, q.pending)
}
```

- **[并发安全] `DefaultCacheStrategy` 统计字段无锁，查询并发下会数据竞争**
  - **位置**：`internal/utils/cache_strategy.go` 第 473-508 行
  - **风险**：`RecordQuery` 修改 `s.stats`，`GetStats` 同时读取，没有 mutex/atomic。项目查询编排是并发模型，开启 `go test -race` 或高并发运行时会出现 data race，统计数据也可能错乱。
  - **修复建议**：给策略加 `sync.RWMutex`，所有读写统计都加锁。
  - **修复代码**：

```go
type DefaultCacheStrategy struct {
    baseDuration time.Duration
    stats        CacheStrategyStats
    mu           sync.RWMutex
}

func (s *DefaultCacheStrategy) RecordQuery(engineName, query string, page, pageSize int, duration time.Duration, success bool) {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.stats.TotalQueries++
    s.stats.TotalDuration += duration
    s.stats.AverageDuration = s.stats.TotalDuration / time.Duration(s.stats.TotalQueries)
    s.stats.LastUpdate = time.Now()
}

func (s *DefaultCacheStrategy) GetStats() CacheStrategyStats {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.stats
}
```

- **[资源管理] 内存缓存 `maxSize <= 0` 时仍可无限写入**
  - **位置**：`internal/utils/cache.go` 第 126-147 行、第 217-239 行
  - **风险**：`Set` 中 `len(c.cache) >= c.maxSize` 在 `maxSize == 0` 时恒成立，但空 cache 无法驱逐，随后仍写入新 key。配置或测试传入 0/负数时，缓存实际无上限增长。
  - **修复建议**：构造函数对 `maxSize` 做安全默认值；或 `maxSize <= 0` 时直接禁用写入。
  - **修复代码**：

```go
func NewMemoryCache(maxSize int, cleanupInterval time.Duration) *MemoryCache {
    if maxSize <= 0 {
        maxSize = 1000
    }
    return &MemoryCache{
        cache:           make(map[string]cacheItem),
        maxSize:         maxSize,
        cleanupInterval: cleanupInterval,
        lastCleanup:     time.Now(),
        stopChan:        make(chan struct{}),
    }
}
```

- **[文件安全] CLI 输出文件使用 `os.Create`/`WriteFile` 直接覆盖**
  - **位置**：`cmd/unimap-cli/main.go` 第 235-254 行；`cmd/unimap-cli/api_subcommands.go` 第 238-243 行
  - **风险**：用户传错路径会静默覆盖已有文件；如果 CLI 以较高权限运行，可能覆盖重要文件。导出工具通常应避免无提示破坏性覆盖，除非显式 `--force`。
  - **修复建议**：默认使用 `O_EXCL` 防覆盖；需要覆盖时增加显式参数。
  - **修复代码**：

```go
func writeJSONFile(path string, v interface{}) error {
    data, err := json.MarshalIndent(v, "", "  ")
    if err != nil {
        return err
    }

    f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
    if err != nil {
        return err
    }
    defer f.Close()

    _, err = f.Write(data)
    return err
}
```

## 低优先级问题

*(代码规范、命名、可读性优化建议)*

- **[CORS 配置] 默认允许请求头缺少 X-Admin-Token**
  - **位置**：`web/server.go` 第 523 行；`internal/config/config.go` 第 673-674 行
  - **风险**：跨 Origin 的合法管理前端如果使用 `X-Admin-Token` 会预检失败，导致使用方退回到 URL Query Token，进一步放大 Token 泄露风险。
  - **修复建议**：默认 `AllowedHeaders` 加入 `X-Admin-Token`。
  - **修复代码**：

```go
allowedHeaders := []string{
    "Content-Type",
    "Authorization",
    "X-Admin-Token",
    "X-Requested-With",
    "X-WebSocket-Token",
    requestid.HeaderName,
}
```

- **[设计不足] `internal/auth` 看起来是未接入主 Web 鉴权链路的平行认证体系**
  - **位置**：`internal/auth/api_key.go` 第 29 行；`internal/auth/middleware.go` 第 21 行
  - **风险**：项目同时存在 Web Admin Token、Distributed Admin Token、Node Token、APIKeyManager，但 APIKeyManager 未接入 `web/router.go` 的主路由。认证模型分裂会让后续开发误用未生效的权限系统。
  - **修复建议**：明确废弃或接入；如果接入，统一权限模型和审计字段。
  - **修复代码**：

```go
// 方向一：删除未接入的 APIKeyManager。
// 方向二：在 router 注册前统一包装需要 API Key 的路由。
handler = authMiddleware.RequireAPIKey("admin")(handler)
```

- **[可维护性] 备份配置注释说支持“相对于 baseDir”，实际没有 baseDir 实现**
  - **位置**：`internal/backup/backup.go` 第 17-23 行、第 69-76 行
  - **风险**：注释与实现不一致，调用方可能误以为 source 会被限制在某个 baseDir 内，实际可以传绝对路径。这类文档偏差容易演变为越权备份或敏感文件误打包。
  - **修复建议**：要么实现 `BaseDir` 并限制 source，不然删除“相对于 baseDir”的注释。
  - **修复代码**：

```go
type BackupConfig struct {
    BaseDir    string
    Sources    []string
    OutputDir  string
    MaxBackups int
    Prefix     string
}

func resolveBackupSource(baseDir, src string) (string, error) {
    if filepath.IsAbs(src) {
        return "", fmt.Errorf("absolute backup source is not allowed: %s", src)
    }

    baseAbs, err := filepath.Abs(baseDir)
    if err != nil {
        return "", err
    }

    targetAbs, err := filepath.Abs(filepath.Join(baseAbs, src))
    if err != nil {
        return "", err
    }

    rel, err := filepath.Rel(baseAbs, targetAbs)
    if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
        return "", fmt.Errorf("backup source escapes base dir: %s", src)
    }

    return targetAbs, nil
}
```

- **[观测性] 多个持久化/备份错误被降级为 warn 或直接吞掉，调用方无法知道备份不完整**
  - **位置**：`internal/backup/backup.go` 第 70-87 行
  - **风险**：某个 source 收集失败或某个文件写入 tar 失败时，仅记录 warn，最终仍可能返回“备份成功”。业务上会产生虚假的成功备份，恢复时才发现关键数据缺失。
  - **修复建议**：返回部分失败列表；关键 source 失败时直接返回错误。
  - **修复代码**：

```go
var failed []string
for _, file := range files {
    if err := addFileToTar(tw, root, file); err != nil {
        failed = append(failed, fmt.Sprintf("%s: %v", file, err))
    }
}
if len(failed) > 0 {
    return nil, fmt.Errorf("backup incomplete: %s", strings.Join(failed, "; "))
}
```

## 审查总结

项目整体质量中等偏下，最大问题不是单点实现粗糙，而是安全边界和状态边界不统一：WebSocket 与 Bridge API 可绕过全局中间件，SSRF 防护停留在字符串层，默认配置偏公网暴露，限流信任可伪造代理头，备份默认打包敏感配置，调度器和缓存存在真实并发/死锁问题。核心修改方向应按优先级推进：先统一所有 HTTP/WebSocket/Bridge 入口的认证、限流、请求体限制和审计；再把所有外连 URL 校验升级到 DNS/IP/Redirect/实际连接层；随后修复 Scheduler 死锁、Chrome 进程状态、API Key 持久化、任务队列持久化；最后收敛认证体系和备份边界，避免多套 token 与敏感数据默认暴露。
