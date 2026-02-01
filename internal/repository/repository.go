package repository

import (
	"context"
	"time"

	"github.com/unimap-icp-hunter/project/internal/model"
	"gorm.io/gorm"
)

// Repository 数���层接口
type Repository interface {
	// Asset 资产相关
	CreateAsset(ctx context.Context, asset *model.Asset) error
	UpdateAsset(ctx context.Context, asset *model.Asset) error
	GetAssetByIPPort(ctx context.Context, ip string, port int) (*model.Asset, error)
	ListAssets(ctx context.Context, page, pageSize int) ([]model.Asset, int64, error)
	UpdateAssetLastSeen(ctx context.Context, id uint) error

	// ICPCheck 检测�录相关
	CreateICPCheck(ctx context.Context, check *model.ICPCheck) error
	GetLatestCheckByAssetID(ctx context.Context, assetID uint) (*model.ICPCheck, error)
	ListChecksByAssetID(ctx context.Context, assetID uint, page, pageSize int) ([]model.ICPCheck, int64, error)
	GetUnregisteredCount(ctx context.Context, since time.Time) (int64, error)
	GetNewUnregistered(ctx context.Context, date time.Time) ([]model.ICPCheck, error)

	// ScanPolicy 策略相关
	CreateScanPolicy(ctx context.Context, policy *model.ScanPolicy) error
	UpdateScanPolicy(ctx context.Context, policy *model.ScanPolicy) error
	GetScanPolicy(ctx context.Context, id uint) (*model.ScanPolicy, error)
	ListScanPolicies(ctx context.Context, enabled bool) ([]model.ScanPolicy, error)

	// ScanTask 任务相关
	CreateScanTask(ctx context.Context, task *model.ScanTask) error
	UpdateScanTask(ctx context.Context, task *model.ScanTask) error
	GetScanTask(ctx context.Context, id uint) (*model.ScanTask, error)
	ListScanTasks(ctx context.Context, page, pageSize int) ([]model.ScanTask, int64, error)

	// Whitelist 白名单相关
	CreateWhitelist(ctx context.Context, wl *model.Whitelist) error
	DeleteWhitelist(ctx context.Context, id uint) error
	ListWhitelists(ctx context.Context, page, pageSize int) ([]model.Whitelist, int64, error)
	IsWhitelisted(ctx context.Context, ip, domain, asn, org string) (bool, error)

	// Stats 统�相关
	GetProvinceStats(ctx context.Context, date time.Time) ([]map[string]interface{}, error)
	GetPortStats(ctx context.Context, date time.Time) ([]map[string]interface{}, error)
	GetTopServers(ctx context.Context, date time.Time, limit int) ([]map[string]interface{}, error)

	// GetDB 获取底层DB�例
	GetDB() *gorm.DB
}

// GORMRepository GORM�现的Repository
type GORMRepository struct {
	db *gorm.DB
}

// NewGORMRepository 创建GORM仓库
func NewGORMRepository(db *gorm.DB) Repository {
	return &GORMRepository{db: db}
}

// CreateAsset 创建资产
func (r *GORMRepository) CreateAsset(ctx context.Context, asset *model.Asset) error {
	return r.db.WithContext(ctx).Create(asset).Error
}

// UpdateAsset 更新资产
func (r *GORMRepository) UpdateAsset(ctx context.Context, asset *model.Asset) error {
	return r.db.WithContext(ctx).Save(asset).Error
}

// GetAssetByIPPort 根�IP和端口获取资产
func (r *GORMRepository) GetAssetByIPPort(ctx context.Context, ip string, port int) (*model.Asset, error) {
	var asset model.Asset
	err := r.db.WithContext(ctx).Where("ip = ? AND port = ?", ip, port).First(&asset).Error
	if err != nil {
		return nil, err
	}
	return &asset, nil
}

// ListAssets 列出资产
func (r *GORMRepository) ListAssets(ctx context.Context, page, pageSize int) ([]model.Asset, int64, error) {
	var assets []model.Asset
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Asset{})
	query.Count(&total)

	offset := (page - 1) * pageSize
	err := query.Offset(offset).Limit(pageSize).Order("last_seen_at DESC").Find(&assets).Error

	return assets, total, err
}

// UpdateAssetLastSeen 更新资产最后出现时间
func (r *GORMRepository) UpdateAssetLastSeen(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&model.Asset{}).
		Where("id = ?", id).
		Update("last_seen_at", time.Now()).Error
}

// CreateICPCheck 创建检测�录
func (r *GORMRepository) CreateICPCheck(ctx context.Context, check *model.ICPCheck) error {
	return r.db.WithContext(ctx).Create(check).Error
}

// GetLatestCheckByAssetID 获取资产最新检测�录
func (r *GORMRepository) GetLatestCheckByAssetID(ctx context.Context, assetID uint) (*model.ICPCheck, error) {
	var check model.ICPCheck
	err := r.db.WithContext(ctx).Where("asset_id = ?", assetID).
		Order("check_time DESC").First(&check).Error
	if err != nil {
		return nil, err
	}
	return &check, nil
}

// ListChecksByAssetID 列出资产检测�录
func (r *GORMRepository) ListChecksByAssetID(ctx context.Context, assetID uint, page, pageSize int) ([]model.ICPCheck, int64, error) {
	var checks []model.ICPCheck
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ICPCheck{}).Where("asset_id = ?", assetID)
	query.Count(&total)

	offset := (page - 1) * pageSize
	err := query.Offset(offset).Limit(pageSize).Order("check_time DESC").Find(&checks).Error

	return checks, total, err
}

// GetUnregisteredCount 获取未备案数量
func (r *GORMRepository) GetUnregisteredCount(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.ICPCheck{}).
		Where("is_registered = 0 AND check_time >= ?", since).
		Count(&count).Error
	return count, err
}

// GetNewUnregistered 获取新增未备案
func (r *GORMRepository) GetNewUnregistered(ctx context.Context, date time.Time) ([]model.ICPCheck, error) {
	var checks []model.ICPCheck
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.AddDate(0, 0, 1)

	err := r.db.WithContext(ctx).
		Where("is_registered = 0 AND check_time >= ? AND check_time < ?", start, end).
		Find(&checks).Error

	return checks, err
}

// CreateScanPolicy 创建策略
func (r *GORMRepository) CreateScanPolicy(ctx context.Context, policy *model.ScanPolicy) error {
	return r.db.WithContext(ctx).Create(policy).Error
}

// UpdateScanPolicy 更新策略
func (r *GORMRepository) UpdateScanPolicy(ctx context.Context, policy *model.ScanPolicy) error {
	return r.db.WithContext(ctx).Save(policy).Error
}

// GetScanPolicy 获取策略
func (r *GORMRepository) GetScanPolicy(ctx context.Context, id uint) (*model.ScanPolicy, error) {
	var policy model.ScanPolicy
	err := r.db.WithContext(ctx).First(&policy, id).Error
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

// ListScanPolicies 列出策略
func (r *GORMRepository) ListScanPolicies(ctx context.Context, enabled bool) ([]model.ScanPolicy, error) {
	var policies []model.ScanPolicy
	query := r.db.WithContext(ctx)
	if enabled {
		query = query.Where("enabled = ?", true)
	}
	err := query.Order("created_at DESC").Find(&policies).Error
	return policies, err
}

// CreateScanTask 创建任务
func (r *GORMRepository) CreateScanTask(ctx context.Context, task *model.ScanTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// UpdateScanTask 更新任务
func (r *GORMRepository) UpdateScanTask(ctx context.Context, task *model.ScanTask) error {
	return r.db.WithContext(ctx).Save(task).Error
}

// GetScanTask 获取任务
func (r *GORMRepository) GetScanTask(ctx context.Context, id uint) (*model.ScanTask, error) {
	var task model.ScanTask
	err := r.db.WithContext(ctx).First(&task, id).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// ListScanTasks 列出任务
func (r *GORMRepository) ListScanTasks(ctx context.Context, page, pageSize int) ([]model.ScanTask, int64, error) {
	var tasks []model.ScanTask
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ScanTask{})
	query.Count(&total)

	offset := (page - 1) * pageSize
	err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&tasks).Error

	return tasks, total, err
}

// CreateWhitelist 创建白名单
func (r *GORMRepository) CreateWhitelist(ctx context.Context, wl *model.Whitelist) error {
	return r.db.WithContext(ctx).Create(wl).Error
}

// DeleteWhitelist 删除白名单
func (r *GORMRepository) DeleteWhitelist(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.Whitelist{}, id).Error
}

// ListWhitelists 列出白名单
func (r *GORMRepository) ListWhitelists(ctx context.Context, page, pageSize int) ([]model.Whitelist, int64, error) {
	var whitelists []model.Whitelist
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Whitelist{})
	query.Count(&total)

	offset := (page - 1) * pageSize
	err := query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&whitelists).Error

	return whitelists, total, err
}

// IsWhitelisted 检查是否在白名单中
func (r *GORMRepository) IsWhitelisted(ctx context.Context, ip, domain, asn, org string) (bool, error) {
	var count int64

	query := r.db.WithContext(ctx).Model(&model.Whitelist{})

	// 构建OR查询
	conditions := r.db.Where("1=0")
	if ip != "" {
		conditions = conditions.Or("type = 'ip' AND value = ?", ip)
	}
	if domain != "" {
		conditions = conditions.Or("type = 'domain' AND value = ?", domain)
	}
	if asn != "" {
		conditions = conditions.Or("type = 'asn' AND value = ?", asn)
	}
	if org != "" {
		conditions = conditions.Or("type = 'org' AND value = ?", org)
	}

	query = query.Where(conditions)
	query.Count(&count)

	return count > 0, nil
}

// GetProvinceStats 获取省份统�
func (r *GORMRepository) GetProvinceStats(ctx context.Context, date time.Time) ([]map[string]interface{}, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.AddDate(0, 0, 1)

	var results []map[string]interface{}
	err := r.db.WithContext(ctx).
		Model(&model.Asset{}).
		Select("region as name, SUM(CASE WHEN icp_checks.is_registered = 0 THEN 1 ELSE 0 END) as unregistered, COUNT(*) as total").
		Joins("LEFT JOIN icp_checks ON assets.id = icp_checks.asset_id").
		Where("icp_checks.check_time >= ? AND icp_checks.check_time < ?", start, end).
		Group("region").
		Find(&results).Error

	return results, err
}

// GetPortStats 获取端口统�
func (r *GORMRepository) GetPortStats(ctx context.Context, date time.Time) ([]map[string]interface{}, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.AddDate(0, 0, 1)

	var results []map[string]interface{}
	err := r.db.WithContext(ctx).
		Model(&model.Asset{}).
		Select("port, SUM(CASE WHEN icp_checks.is_registered = 0 THEN 1 ELSE 0 END) as unregistered, COUNT(*) as total").
		Joins("LEFT JOIN icp_checks ON assets.id = icp_checks.asset_id").
		Where("icp_checks.check_time >= ? AND icp_checks.check_time < ?", start, end).
		Group("port").
		Find(&results).Error

	return results, err
}

// GetTopServers 获取Top服务器
func (r *GORMRepository) GetTopServers(ctx context.Context, date time.Time, limit int) ([]map[string]interface{}, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.AddDate(0, 0, 1)

	var results []map[string]interface{}
	err := r.db.WithContext(ctx).
		Model(&model.Asset{}).
		Select("server as name, COUNT(*) as count").
		Where("server != '' AND last_seen_at >= ? AND last_seen_at < ?", start, end).
		Group("server").
		Order("count DESC").
		Limit(limit).
		Find(&results).Error

	return results, err
}

// InitDatabase 初始化数�库表
func InitDatabase(db *gorm.DB) error {
	// 自动迁移表结构
	return db.AutoMigrate(
		&model.Asset{},
		&model.ICPCheck{},
		&model.ScanPolicy{},
		&model.ScanTask{},
		&model.Whitelist{},
	)
}

// GetDailyStats 获取日报统�数�
func (r *GORMRepository) GetDailyStats(ctx context.Context, date time.Time) (map[string]interface{}, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	end := start.AddDate(0, 0, 1)

	stats := make(map[string]interface{})

	// 总扫描数
	var totalScanned int64
	r.db.WithContext(ctx).Model(&model.ICPCheck{}).
		Where("check_time >= ? AND check_time < ?", start, end).
		Count(&totalScanned)
	stats["total_scanned"] = totalScanned

	// 未备案数
	var unregistered int64
	r.db.WithContext(ctx).Model(&model.ICPCheck{}).
		Where("is_registered = 0 AND check_time >= ? AND check_time < ?", start, end).
		Count(&unregistered)
	stats["unregistered"] = unregistered

	// 已备案数
	var registered int64
	r.db.WithContext(ctx).Model(&model.ICPCheck{}).
		Where("is_registered = 1 AND check_time >= ? AND check_time < ?", start, end).
		Count(&istered)
	stats["registered"] = registered

	// 新增未备案（首次发现）
	var newUnregistered int64
	r.db.WithContext(ctx).Model(&model.ICPCheck{}).
		Where("is_registered = 0 AND check_time >= ? AND check_time < ? AND tags LIKE ?", start, end, "%new%").
		Count(&newUnregistered)
	stats["new_unregistered"] = newUnregistered

	return stats, nil
}

// GetDB 获取底层DB�例
func (r *GORMRepository) GetDB() *gorm.DB {
	return r.db
}
