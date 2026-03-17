package adapter

import (
	"github.com/unimap-icp-hunter/project/internal/model"
)

// EngineAdapter 引擎适配器接口
type EngineAdapter interface {
	// Name 返回引擎标识
	Name() string

	// Translate 将UQL AST转换为引擎专用查询串
	Translate(ast *model.UQLAST) (string, error)

	// Search 执行搜索并返回原生结果
	Search(query string, page, pageSize int) (*model.EngineResult, error)

	// Normalize 将原生结果映射到统一资产模型
	Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error)

	// GetQuota 获取引擎配额信息
	GetQuota() (*model.QuotaInfo, error)

	// IsWebOnly 检查是否为 Web-only 模式
	IsWebOnly() bool
}
