# 安全问题修复总结

## 修复日期
2026-03-09

## 修复的问题列表

### 1. 安全响应头 ✅

**问题描述**:
- HTTP响应缺少安全响应头
- 容易受到点击劫持、XSS等攻击

**修复方案**:
- 添加`securityMiddleware`中间件
- 设置以下安全响应头:
  - `X-Frame-Options: DENY` - 防止点击劫持
  - `X-Content-Type-Options: nosniff` - 防止MIME类型嗅探
  - `X-XSS-Protection: 1; mode=block` - XSS保护
  - `Content-Security-Policy` - 内容安全策略
  - `Referrer-Policy` - 引用策略
  - `Permissions-Policy` - 权限策略

**修改文件**: `web/server.go:255-283`

**代码变更**:
```go
// securityMiddleware 添加安全响应头的中间件
func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 防止点击劫持
		w.Header().Set("X-Frame-Options", "DENY")
		// 防止MIME类型嗅探
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// XSS保护
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		// 内容安全策略
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:;")
		// 引用策略
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// 权限策略
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		
		next.ServeHTTP(w, r)
	})
}
```

---

### 2. 路径遍历防护 ✅

**问题描述**:
- 用户输入的URL可能用于文件操作
- 文件名可能包含特殊字符导致路径遍历
- 缺少路径验证机制

**修复方案**:
- 添加`sanitizeFilename`函数清理文件名中的危险字符
- 添加`validatePath`函数验证路径是否在允许的基础目录内
- 添加`safeJoinPath`函数安全地连接路径

**修改文件**: `internal/screenshot/manager.go:729-801`

**代码变更**:
```go
// sanitizeFilename 清理文件名中的危险字符
func sanitizeFilename(name string) string {
	// 替换所有可能的路径遍历字符
	replacer := strings.NewReplacer(
		"../", "",
		"..\\", "",
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		"\x00", "",
	)
	clean := replacer.Replace(name)
	
	// 移除开头的点（防止隐藏文件）
	clean = strings.TrimLeft(clean, ".")
	
	// 限制长度
	if len(clean) > 200 {
		clean = clean[:200]
	}
	
	// 确保文件名不为空
	if clean == "" {
		clean = "unnamed"
	}
	
	return clean
}

// validatePath 验证路径是否在允许的基础目录内
func validatePath(baseDir, targetPath string) error {
	// 获取绝对路径
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute base path: %w", err)
	}
	
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute target path: %w", err)
	}
	
	// 检查目标路径是否在基础目录内
	if !strings.HasPrefix(absTarget, absBase) {
		return fmt.Errorf("path traversal detected: target path is outside base directory")
	}
	
	return nil
}

// safeJoinPath 安全地连接路径，防止路径遍历攻击
func safeJoinPath(baseDir string, elems []string) (string, error) {
	// 清理每个路径元素
	cleanElems := make([]string, len(elems))
	for i, e := range elems {
		cleanElems[i] = sanitizeFilename(e)
	}
	
	// 连接路径
	allElems := append([]string{baseDir}, cleanElems...)
	result := filepath.Join(allElems...)
	
	// 验证结果路径
	if err := validatePath(baseDir, result); err != nil {
		return "", err
	}
	
	return result, nil
}
```

---

## 验证结果

✅ 编译测试通过：`go build ./...` - 无错误
✅ 单元测试通过：`go test -v ./...` - 所有测试通过

---

## 安全修复汇总

| 问题类型 | 修复状态 | 风险等级 |
|----------|----------|----------|
| API密钥泄露 | ✅ 已修复 | 高 |
| WebSocket CheckOrigin | ✅ 已修复 | 高 |
| 安全响应头缺失 | ✅ 已修复 | 中 |
| 路径遍历风险 | ✅ 已修复 | 中 |

---

## 后续安全建议

1. **定期安全审计**: 建议定期进行代码安全审计
2. **依赖更新**: 定期更新依赖包以修复已知漏洞
3. **输入验证**: 对所有用户输入进行严格验证
4. **日志审计**: 记录安全相关事件以便审计
5. **HTTPS**: 生产环境强制使用HTTPS
6. **认证授权**: 实现完善的认证授权机制
