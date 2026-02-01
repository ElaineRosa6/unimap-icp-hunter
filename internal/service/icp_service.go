package service

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/core/unimap"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/repository"
	"github.com/unimap-icp-hunter/project/internal/service/notification"
	"github.com/unimap-icp-hunter/project/internal/service/scheduler"
	"github.com/unimap-icp-hunter/project/internal/service/worker"
	"go.uber.org/zap"
)

// ICPService ICP-Hunter核心服务
type ICPService struct {
	repo         repository.Repository
	redis        *redis.Client
	orchestrator *adapter.EngineOrchestrator
	merger       *unimap.ResultMerger
	scheduler    *scheduler.ScanScheduler
	worker       *worker.ProbeExecutor
	notifier     *notification.Notifier
	logger       *zap.Logger
}

// NewICPService 创建ICP服务
func NewICPService(
	repo repository.Repository,
	redis *redis.Client,
	orchestrator *adapter.EngineOrchestrator,
	merger *unimap.ResultMerger,
	scheduler *scheduler.ScanScheduler,
	worker *worker.ProbeExecutor,
	notifier *notification.Notifier,
	logger *zap.Logger,
) *ICPService {
	return &ICPService{
		repo:         repo,
		redis:        redis,
		orchestrator: orchestrator,
		merger:       merger,
		scheduler:    scheduler,
		worker:       worker,
		notifier:     notifier,
		logger:       logger,
	}
}

// RunDailyScan 执行每日扫描
func (s *ICPService) RunDailyScan(ctx context.Context) error {
	s.logger.Info("Starting daily scan workflow")

	// 1. 触发调度器执行策略
	// 调度器会自动处理策略生成、引擎查询、结果归并和队列推送

	// 2. 启动Worker处理队列任务
	if err := s.worker.Start(ctx); err != nil {
		return fmt.Errorf("failed to start worker: %v", err)
	}

	// 3. 等待扫描完成（简化实现，实际应使用更复杂的协调机制）
	time.Sleep(30 * time.Second)

	// 4. 生成并发送日报
	return s.generateAndSendDailyReport(ctx)
}

// ProcessProbeResult 处理探测结果
func (s *ICPService) ProcessProbeResult(ctx context.Context, check *model.ICPCheck, task *model.ProbeTask) error {
	// 1. 查找或创建资产
	asset, err := s.repo.GetAssetByIPPort(ctx, task.IP, task.Port)
	if err != nil {
		// 资产不存在，创建新资产
		asset = &model.Asset{
			IP:       task.IP,
			Port:     task.Port,
			Protocol: task.Protocol,
			URL:      task.URL,
		}
		if err := s.repo.CreateAsset(ctx, asset); err != nil {
			return err
		}
	}

	// 2. 检查之前的状态
	previousCheck, err := s.repo.GetLatestCheckByAssetID(ctx, asset.ID)
	isNewAsset := err != nil // 如果获取不到，说明是新资产

	// 3. 设置AssetID和标签
	check.AssetID = asset.ID
	if isNewAsset {
		check.Tags = append(check.Tags, "new")
	}

	// 4. 保存检测记录
	if err := s.repo.CreateICPCheck(ctx, check); err != nil {
		return err
	}

	// 5. 更新资产信息
	asset.Title = check.Title
	asset.StatusCode = check.HTTPStatusCode
	asset.LastSeenAt = time.Now()
	asset.LatestCheckID = check.ID
	if err := s.repo.UpdateAsset(ctx, asset); err != nil {
		return err
	}

	// 6. 触发通知
	if check.IsRegistered == 0 {
		// 疑似未备案
		if isNewAsset || (previousCheck != nil && previousCheck.IsRegistered != 0) {
			// 新发现或从已备案变为未备案
			if err := s.notifier.NotifyUnregistered(check, asset); err != nil {
				s.logger.Error("Failed to send unregistered notification", zap.Error(err))
			}
		}
	} else if check.IsRegistered == 1 {
		// 已备案
		if previousCheck != nil && previousCheck.IsRegistered == 0 {
			// 从未备案恢复为已备案
			if err := s.notifier.NotifyRecovery(check, asset); err != nil {
				s.logger.Error("Failed to send recovery notification", zap.Error(err))
			}
		}
	}

	return nil
}

// QueryUnified 统一查询接口
func (s *ICPService) QueryUnified(ctx context.Context, uql string, engineNames []string, pageSize, maxPages int) ([]model.UnifiedAsset, error) {
	// 1. 解析UQL
	parser := unimap.NewUQLParser()
	ast, err := parser.Parse(uql)
	if err != nil {
		return nil, fmt.Errorf("failed to parse UQL: %v", err)
	}

	// 2. 执行统一查询
	assets, err := s.orchestrator.ExecuteUnifiedQuery(ast, engineNames, pageSize, maxPages)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %v", err)
	}

	// 3. 归并结果
	// 这里orchestrator已经做了标准化，merger用于多引擎结果合并
	// 简化处理，直接返回
	return assets, nil
}

// AddWhitelist 添加白名单
func (s *ICPService) AddWhitelist(ctx context.Context, typ, value, reason, creator string) error {
	wl := &model.Whitelist{
		Type:      typ,
		Value:     value,
		Reason:    reason,
		CreatedBy: creator,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	return s.repo.CreateWhitelist(ctx, wl)
}

// RemoveWhitelist 删除白名单
func (s *ICPService) RemoveWhitelist(ctx context.Context, id uint) error {
	return s.repo.DeleteWhitelist(ctx, id)
}

// ListWhitelists 列出白名单
func (s *ICPService) ListWhitelists(ctx context.Context, page, pageSize int) ([]model.Whitelist, int64, error) {
	return s.repo.ListWhitelists(ctx, page, pageSize)
}

// GetDailyReport 获取日报数据
func (s *ICPService) GetDailyReport(ctx context.Context, date time.Time) (*notification.DailyStats, error) {
	// 获取统计数据
	stats, err := s.repo.GetDailyStats(ctx, date)
	if err != nil {
		return nil, err
	}

	// 获取省份统计
	provinceStats, err := s.repo.GetProvinceStats(ctx, date)
	if err != nil {
		return nil, err
	}

	// 获取端口统计
	portStats, err := s.repo.GetPortStats(ctx, date)
	if err != nil {
		return nil, err
	}

	// 获取Top服务器
	topServers, err := s.repo.GetTopServers(ctx, date, 10)
	if err != nil {
		return nil, err
	}

	// 转换为通知服务的格式
	dailyStats := notification.DailyStats{
		Date:              date.Format("2006-01-02"),
		NewUnregistered:   stats["new_unregistered"].(int),
		TotalUnregistered: stats["unregistered"].(int),
		TotalRegistered:   stats["registered"].(int),
		TotalScanned:      stats["total_scanned"].(int),
		GenerateTime:      time.Now().Format("2006-01-02 15:04:05"),
	}

	// 转换省份统计
	for _, p := range provinceStats {
		dailyStats.ProvinceStats = append(dailyStats.ProvinceStats, notification.ProvinceStat{
			Name:         fmt.Sprint(p["name"]),
			Unregistered: int(p["unregistered"].(int64)),
			Total:        int(p["total"].(int64)),
		})
	}

	// 转换端口统计
	for _, p := range portStats {
		dailyStats.PortStats = append(dailyStats.PortStats, notification.PortStat{
			Port:         int(p["port"].(int64)),
			Unregistered: int(p["unregistered"].(int64)),
			Total:        int(p["total"].(int64)),
		})
	}

	// 转换服务器统计
	for _, s := range topServers {
		dailyStats.TopServers = append(dailyStats.TopServers, notification.ServerStat{
			Name:  fmt.Sprint(s["name"]),
			Count: int(s["count"].(int64)),
		})
	}

	return &dailyStats, nil
}

// generateAndSendDailyReport 生成并发送日报
func (s *ICPService) generateAndSendDailyReport(ctx context.Context) error {
	yesterday := time.Now().AddDate(0, 0, -1)
	stats, err := s.GetDailyReport(ctx, yesterday)
	if err != nil {
		return err
	}

	return s.notifier.SendDailyReport(stats)
}

// StartScheduler 启动调度器
func (s *ICPService) StartScheduler(ctx context.Context) error {
	return s.scheduler.Start(ctx)
}

// StopScheduler 停止调度器
func (s *ICPService) StopScheduler() {
	s.scheduler.Stop()
}

// StartWorker 启动Worker
func (s *ICPService) StartWorker(ctx context.Context) error {
	return s.worker.Start(ctx)
}

// StopWorker 停止Worker
func (s *ICPService) StopWorker() {
	s.worker.Stop()
}

// GetScanStats 获取扫描统计
func (s *ICPService) GetScanStats(ctx context.Context) (map[string]interface{}, error) {
	// 获取今日扫描统计
	today := time.Now()
	stats, err := s.repo.GetDailyStats(ctx, today)
	if err != nil {
		return nil, err
	}

	// 获取队列长度
	queueLen, err := s.redis.XLen(ctx, model.StreamKey).Result()
	if err != nil {
		queueLen = 0
	}

	// 获取策略数量
	policies, err := s.repo.ListScanPolicies(ctx, true)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"today_stats":     stats,
		"queue_length":    queueLen,
		"active_policies": len(policies),
		"timestamp":       time.Now().Format("2006-01-02 15:04:05"),
	}

	return result, nil
}

// GetUnregisteredSites 获取未备案站点列表
func (s *ICPService) GetUnregisteredSites(ctx context.Context, page, pageSize int) ([]map[string]interface{}, error) {
	// 查询未备案的最新检测记录
	var results []map[string]interface{}

	// 使用子查询获取每个资产的最新未备案记录
	query := `
		SELECT a.id, a.ip, a.port, a.url, a.title, a.region, a.country_code,
		       c.check_time, c.icp_code, c.screenshot_path
		FROM assets a
		JOIN icp_checks c ON a.id = c.asset_id
		WHERE c.is_registered = 0
		AND c.check_time = (
			SELECT MAX(check_time)
			FROM icp_checks
			WHERE asset_id = a.id
			AND is_registered = 0
		)
		ORDER BY c.check_time DESC
		LIMIT ? OFFSET ?
	`

	offset := (page - 1) * pageSize
	err := s.repo.GetDB().Raw(query, pageSize, offset).Scan(&results).Error

	return results, err
}

// GetScheduler returns the scheduler instance
func (s *ICPService) GetScheduler() *scheduler.ScanScheduler {
	return s.scheduler
}

// GetNotifier returns the notifier instance
func (s *ICPService) GetNotifier() *notification.Notifier {
	return s.notifier
}
