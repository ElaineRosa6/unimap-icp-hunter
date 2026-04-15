package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	unierror "github.com/unimap-icp-hunter/project/internal/error"
)

// APIKey API密钥结构
type APIKey struct {
	ID          string    `json:"id"`
	Key         string    `json:"key,omitempty"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	LastUsed    time.Time `json:"last_used,omitempty"`
	Permissions []string  `json:"permissions"`
	Status      string    `json:"status"` // active, revoked, expired
}

// APIKeyManager API密钥管理器
type APIKeyManager struct {
	keys        map[string]*APIKey
	mutex       sync.RWMutex
	storagePath string
}

// NewAPIKeyManager 创建API密钥管理器
func NewAPIKeyManager(storagePath string) *APIKeyManager {
	manager := &APIKeyManager{
		keys:        make(map[string]*APIKey),
		storagePath: storagePath,
	}

	// 加载已保存的API密钥
	manager.loadFromStorage()

	return manager
}

// GenerateAPIKey 生成新的API密钥
func (m *APIKeyManager) GenerateAPIKey(description string, permissions []string, expiresIn time.Duration) (*APIKey, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 生成密钥ID（使用随机字节避免并发冲突）
	idBytes := make([]byte, 8)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, unierror.New(unierror.ErrorTypeRuntime, unierror.ErrAPIInternalServer, "Failed to generate API key ID")
	}
	id := fmt.Sprintf("key_%s", base64.RawURLEncoding.EncodeToString(idBytes))

	// 生成随机密钥
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, unierror.New(unierror.ErrorTypeRuntime, unierror.ErrAPIInternalServer, "Failed to generate API key")
	}

	key := base64.URLEncoding.EncodeToString(keyBytes)

	apiKey := &APIKey{
		ID:          id,
		Key:         key,
		Description: description,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(expiresIn),
		Permissions: permissions,
		Status:      "active",
	}

	m.keys[key] = apiKey
	m.saveToStorage()

	return apiKey, nil
}

// ValidateAPIKey 验证API密钥
func (m *APIKeyManager) ValidateAPIKey(key string) (*APIKey, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	apiKey, exists := m.keys[key]
	if !exists {
		return nil, unierror.APIUnauthorized("Invalid API key")
	}

	// 检查密钥状态
	if apiKey.Status != "active" {
		return nil, unierror.APIUnauthorized("API key is not active")
	}

	// 检查是否过期
	if !apiKey.ExpiresAt.IsZero() && time.Now().After(apiKey.ExpiresAt) {
		// 更新状态为过期
		apiKey.Status = "expired"
		return nil, unierror.APIUnauthorized("API key has expired")
	}

	return apiKey, nil
}

// CheckPermission 检查密钥是否有权限
func (m *APIKeyManager) CheckPermission(key string, permission string) bool {
	apiKey, err := m.ValidateAPIKey(key)
	if err != nil {
		return false
	}

	for _, p := range apiKey.Permissions {
		if p == permission || p == "admin" {
			return true
		}
	}

	return false
}

// GetAPIKey 获取API密钥信息
func (m *APIKeyManager) GetAPIKey(id string) *APIKey {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, apiKey := range m.keys {
		if apiKey.ID == id {
			return apiKey
		}
	}

	return nil
}

// ListAPIKeys 列出所有API密钥
func (m *APIKeyManager) ListAPIKeys() []*APIKey {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var keys []*APIKey
	for _, apiKey := range m.keys {
		// 不返回实际密钥，只返回元数据
		keyCopy := *apiKey
		keyCopy.Key = ""
		keys = append(keys, &keyCopy)
	}

	return keys
}

// RevokeAPIKey 撤销API密钥
func (m *APIKeyManager) RevokeAPIKey(id string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, apiKey := range m.keys {
		if apiKey.ID == id {
			apiKey.Status = "revoked"
			m.saveToStorage()
			return nil
		}
	}

	return errors.New("API key not found")
}

// UpdateLastUsed 更新最后使用时间
func (m *APIKeyManager) UpdateLastUsed(key string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	apiKey, exists := m.keys[key]
	if exists {
		apiKey.LastUsed = time.Now()
		m.saveToStorage()
	}
}

// saveToStorage 保存API密钥到文件
func (m *APIKeyManager) saveToStorage() {
	if m.storagePath == "" {
		return
	}

	// 确保存储目录存在
	dir := filepath.Dir(m.storagePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	// 准备保存数据（不包含实际密钥）
	var keysToSave []*APIKey
	for _, apiKey := range m.keys {
		keyCopy := *apiKey
		keyCopy.Key = "" // 不保存实际密钥
		keysToSave = append(keysToSave, &keyCopy)
	}

	data, err := json.MarshalIndent(keysToSave, "", "  ")
	if err != nil {
		return
	}

	// 保存到文件
	if err := os.WriteFile(m.storagePath, data, 0600); err != nil {
		return
	}
}

// loadFromStorage 从文件加载API密钥
func (m *APIKeyManager) loadFromStorage() {
	if m.storagePath == "" || !fileExists(m.storagePath) {
		return
	}

	data, err := os.ReadFile(m.storagePath)
	if err != nil {
		return
	}

	var keys []*APIKey
	if err := json.Unmarshal(data, &keys); err != nil {
		return
	}

	// 注意：加载时没有实际密钥，只加载元数据
	// 实际密钥需要通过其他方式管理
	for _, key := range keys {
		m.keys[key.ID] = key
	}
}

// fileExists 检查文件是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// CleanupExpiredKeys 清理过期的API密钥
func (m *APIKeyManager) CleanupExpiredKeys() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	for _, apiKey := range m.keys {
		if !apiKey.ExpiresAt.IsZero() && now.After(apiKey.ExpiresAt) && apiKey.Status == "active" {
			apiKey.Status = "expired"
		}
	}

	m.saveToStorage()
}

// GetKeyStats 获取密钥统计信息
func (m *APIKeyManager) GetKeyStats() map[string]int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := map[string]int{
		"total":   0,
		"active":  0,
		"revoked": 0,
		"expired": 0,
	}

	for _, apiKey := range m.keys {
		stats["total"]++
		switch apiKey.Status {
		case "active":
			stats["active"]++
		case "revoked":
			stats["revoked"]++
		case "expired":
			stats["expired"]++
		}
	}

	return stats
}
