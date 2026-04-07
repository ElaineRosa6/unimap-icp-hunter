package adapter

import (
	"fmt"
	"github.com/unimap-icp-hunter/project/internal/model"
)

// WebOnlyAdapterBase Web-only 适配器基类
type WebOnlyAdapterBase struct {
	adapter EngineAdapter
	name    string
}

// NewWebOnlyAdapterBase 创建 Web-only 适配器基类
func NewWebOnlyAdapterBase(adapter EngineAdapter, name string) *WebOnlyAdapterBase {
	return &WebOnlyAdapterBase{
		adapter: adapter,
		name:    name,
	}
}

// Name 获取引擎名称
func (w *WebOnlyAdapterBase) Name() string {
	return w.name
}

// Translate 翻译查询（Web-only 模式下返回空查询）
func (w *WebOnlyAdapterBase) Translate(ast *model.UQLAST) (string, error) {
	return "", fmt.Errorf("web-only mode: translation not supported")
}

// Search 搜索（Web-only 模式下返回错误）
func (w *WebOnlyAdapterBase) Search(query string, page, pageSize int) (*model.EngineResult, error) {
	return nil, fmt.Errorf("web-only mode: search not supported")
}

// Normalize 标准化结果（Web-only 模式下返回空结果）
func (w *WebOnlyAdapterBase) Normalize(result *model.EngineResult) ([]model.UnifiedAsset, error) {
	return []model.UnifiedAsset{}, nil
}

// GetQuota 获取配额（Web-only 模式下返回空配额）
func (w *WebOnlyAdapterBase) GetQuota() (*model.QuotaInfo, error) {
	return nil, fmt.Errorf("web-only mode: quota not supported")
}

// IsWebOnly 检查是否为 Web-only 模式
func (w *WebOnlyAdapterBase) IsWebOnly() bool {
	return true
}
