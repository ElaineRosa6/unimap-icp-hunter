package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

// AuthMiddleware 认证中间件
type AuthMiddleware struct {
	keyManager *APIKeyManager
}

// NewAuthMiddleware 创建认证中间件
func NewAuthMiddleware(keyManager *APIKeyManager) *AuthMiddleware {
	return &AuthMiddleware{
		keyManager: keyManager,
	}
}

// RequireAPIKey 需要API密钥的中间件
func (m *AuthMiddleware) RequireAPIKey(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 从请求中提取API密钥
			apiKey := m.extractAPIKey(r)
			if apiKey == "" {
				m.writeAuthError(w, "API key is required")
				return
			}

			// 验证API密钥
			keyInfo, err := m.keyManager.ValidateAPIKey(apiKey)
			if err != nil {
				m.writeAuthError(w, err.Error())
				return
			}

			// 检查权限
			if permission != "" && !m.keyManager.CheckPermission(apiKey, permission) {
				m.writeAuthError(w, "Insufficient permissions")
				return
			}

			// 更新最后使用时间
			m.keyManager.UpdateLastUsed(apiKey)

			// 将密钥信息存储到上下文中
			r.Header.Set("X-API-Key-ID", keyInfo.ID)
			r.Header.Set("X-API-Key-Permissions", strings.Join(keyInfo.Permissions, ","))

			next.ServeHTTP(w, r)
		})
	}
}

// OptionalAPIKey 可选API密钥的中间件（不强制要求）
func (m *AuthMiddleware) OptionalAPIKey() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 从请求中提取API密钥
			apiKey := m.extractAPIKey(r)
			if apiKey != "" {
				// 验证API密钥
				keyInfo, err := m.keyManager.ValidateAPIKey(apiKey)
				if err == nil {
					// 更新最后使用时间
					m.keyManager.UpdateLastUsed(apiKey)

					// 将密钥信息存储到上下文中
					r.Header.Set("X-API-Key-ID", keyInfo.ID)
					r.Header.Set("X-API-Key-Permissions", strings.Join(keyInfo.Permissions, ","))
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractAPIKey 从请求中提取API密钥
func (m *AuthMiddleware) extractAPIKey(r *http.Request) string {
	// 从Authorization头提取
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		if strings.HasPrefix(strings.ToLower(authHeader), "apikey ") {
			return strings.TrimSpace(authHeader[7:])
		}
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			return strings.TrimSpace(authHeader[7:])
		}
	}

	// 从X-API-Key头提取
	apiKeyHeader := r.Header.Get("X-API-Key")
	if apiKeyHeader != "" {
		return strings.TrimSpace(apiKeyHeader)
	}

	// 从查询参数提取
	apiKeyParam := r.URL.Query().Get("api_key")
	if apiKeyParam != "" {
		return strings.TrimSpace(apiKeyParam)
	}

	return ""
}

// writeAuthError 写入认证错误响应
func (m *AuthMiddleware) writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)

	errorResponse := map[string]interface{}{
		"code":    "unauthorized",
		"message": message,
		"status":  "error",
	}

	// 使用JSON响应
	json.NewEncoder(w).Encode(errorResponse)
}

// GetAPIKeyID 从请求中获取API密钥ID
func GetAPIKeyID(r *http.Request) string {
	return r.Header.Get("X-API-Key-ID")
}

// GetAPIKeyPermissions 从请求中获取API密钥权限
func GetAPIKeyPermissions(r *http.Request) []string {
	permissionsStr := r.Header.Get("X-API-Key-Permissions")
	if permissionsStr == "" {
		return []string{}
	}
	return strings.Split(permissionsStr, ",")
}

// HasPermission 检查请求是否具有特定权限
func HasPermission(r *http.Request, permission string) bool {
	permissions := GetAPIKeyPermissions(r)
	for _, p := range permissions {
		if p == permission || p == "admin" {
			return true
		}
	}
	return false
}
