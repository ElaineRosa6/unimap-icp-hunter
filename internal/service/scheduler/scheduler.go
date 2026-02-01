package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/core/unimap"
	"github.com/unimap-icp-hunter/project/internal/model"
	"go.uber.org/zap"
)

// ScanScheduler 扫描任务调度器
type ScanScheduler struct {
	cron           *cron.Cron
	orchestrator   *adapter.EngineOrchestrator
	merger         *unimap.ResultMerger
	redisClient    *redis.Client
	config         *SchedulerConfig
	logger         *zap.Logger
	mu             sync.Mutex
	running        bool
}

// SchedulerConfig 调度器配置
type SchedulerConfig struct {
	Policies []PolicyConfig `yaml:"policies"`
	DailyScan struct {
		Enabled bool   `yaml:"enabled"`
		Cron    string `yaml:"cron"`
		Concurrency struct {
			EngineCalls      int           `yaml:"engine_calls"`
			QueuePushBatch   int           `yaml:"queue_push_batch"`
			QueuePushInterval time.Duration `yaml:"queue_push_interval"`
		} `yaml:"concurrency"`
	} `yaml:"daily_scan"`
}

// PolicyConfig 策略配置
type PolicyConfig struct {
	ID         uint     `yaml:"id"`
	Name       string   `yaml:"name"`
	UQL        string   `yaml:"uql"`
	Engines    []string `yaml:"engines"`
	PageSize   int      `yaml:"page_size"`
	MaxRecords int      `yaml:"max_records"`
	Ports      []int    `yaml:"ports"`
	Enabled    bool     `yaml:"enabled"`
}

// NewScanScheduler 创建调度器
func NewScanScheduler(
	orchestrator *adapter.EngineOrchestrator,
	merger *unimap.ResultMerger,
	redisClient *redis.Client,
	config *SchedulerConfig,
	logger *zap.Logger,
) *ScanScheduler {
	return &ScanScheduler{
		cron:         cron.New(),
		orchestrator: orchestrator,
		merger:       merger,
		redisClient:  redisClient,
		config:       config,
		logger:       logger,
	}
}

// Start 启动调度器
func (s *ScanScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("scheduler already running")
	}

	// 注册定时任务
	if s.config.DailyScan.Enabled {
		_, err := s.cron.AddFunc(s.config.DailyScan.Cron, func() {
			s.runDailyScan(ctx)
		})
		if err != nil {
			return fmt.Errorf("failed to add daily scan cron: %v", err)
		}
		s.logger.Info("Daily scan scheduled", zap.String("cron", s.config.DailyScan.Cron))
	}

	s.cron.Start()
	s.running = true
	s.logger.Info("Scheduler started")

	return nil
}

// Stop 停止调度器
func (s *ScanScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.cron.Stop()
	s.running = false
	s.logger.Info("Scheduler stopped")
}

// runDailyScan 执行每日扫描
func (s *ScanScheduler) runDailyScan(ctx context.Context) {
	s.logger.Info("Starting daily scan")

	// 遍历所有策略
	for _, policyConfig := range s.config.Policies {
		if err := s.executePolicy(ctx, policyConfig); err != nil {
			s.logger.Error("Failed to execute policy",
				zap.String("policy", policyConfig.Name),
				zap.Error(err),
			)
		}
	}

	s.logger.Info("Daily scan completed")
}

// executePolicy 执行单个策略
func (s *ScanScheduler) executePolicy(ctx context.Context, policyConfig PolicyConfig) error {
	s.logger.Info("Executing policy", zap.String("policy", policyConfig.Name))

	// 1. 解析UQL
	parser := unimap.NewUQLParser()
	ast, err := parser.Parse(policyConfig.UQL)
	if err != nil {
		return fmt.Errorf("failed to parse UQL: %v", err)
	}

	// 2. 翻译为各引擎查询
	queries, err := s.orchestrator.TranslateQuery(ast, policyConfig.Engines)
	if err != nil {
		return fmt.Errorf("failed to translate query: %v", err)
	}

	// 3. 设置分页参数
	for i := range queries {
		queries[i].PageSize = policyConfig.PageSize
	}

	// 4. 并行搜索各引擎
	engineResults, err := s.orchestrator.SearchEnginesWithPagination(
		queries,
		policyConfig.PageSize,
		policyConfig.MaxRecords/policyConfig.PageSize+1,
	)
	if err != nil {
		return fmt.Errorf("failed to search engines: %v", err)
	}

	// 5. 归并结果
	mergeResult := s.merger.Merge(engineResults)

	s.logger.Info("Merge result",
		zap.String("policy", policyConfig.Name),
		zap.Int("total", mergeResult.Total),
		zap.Int("duplicates", mergeResult.Duplicates),
	)

	// 6. 推送到Redis队列
	if err := s.pushToQueue(ctx, mergeResult.Assets, policyConfig); err != nil {
		return fmt.Errorf("failed to push to queue: %v", err)
	}

	return nil
}

// pushToQueue 将资产推送到Redis Stream队列
func (s *ScanScheduler) pushToQueue(ctx context.Context, assets map[string]*model.UnifiedAsset, policyConfig PolicyConfig) error {
	if len(assets) == 0 {
		s.logger.Info("No assets to push", zap.String("policy", policyConfig.Name))
		return nil
	}

	batchSize := s.config.DailyScan.Concurrency.QueuePushBatch
	if batchSize == 0 {
		batchSize = 50
	}

	pushInterval := s.config.DailyScan.Concurrency.QueuePushInterval
	if pushInterval == 0 {
		pushInterval = 5 * time.Second
	}

	// 分批推送
	var batch []map[string]interface{}
	count := 0

	for _, asset := range assets {
		// 构建任务数据
		taskData := map[string]interface{}{
			model.StreamFieldTaskID: fmt.Sprintf("%s:%d", asset.IP, asset.Port),
			model.StreamFieldURL:    asset.URL,
			model.StreamFieldIP:     asset.IP,
			model.StreamFieldPort:   fmt.Sprintf("%d", asset.Port),
			model.StreamFieldProto:  asset.Protocol,
			model.StreamFieldPolicy: fmt.Sprintf("%d", policyConfig.ID),
			model.StreamFieldRetry:  "0",
		}

		batch = append(batch, taskData)
		count++

		// 达到批次大小，推送
		if len(batch) >= batchSize {
			if err := s.pushBatch(ctx, batch); err != nil {
				return err
			}
			batch = batch[:0] // 清空批次

			// 间隔控制
			time.Sleep(pushInterval)
		}
	}

	// 推送剩余任务
	if len(batch) > 0 {
		if err := s.pushBatch(ctx, batch); err != nil {
			return err
		}
	}

	s.logger.Info("Pushed tasks to queue",
		zap.String("policy", policyConfig.Name),
		zap.Int("total", count),
	)

	return nil
}

// pushBatch 批量推送任务到Redis Stream
func (s *ScanScheduler) pushBatch(ctx context.Context, batch []map[string]interface{}) error {
	if len(batch) == 0 {
		return nil
	}

	// 使用管道批量推送
	pipe := s.redisClient.Pipeline()

	for _, taskData := range batch {
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: model.StreamKey,
			Values: taskData,
		})
	}

	_, err := pipe.Exec(ctx)
	return err
}

// ExecuteImmediate 立即执行扫描（手动触发）
func (s *ScanScheduler) ExecuteImmediate(ctx context.Context, policyName string) error {
	var policyConfig *PolicyConfig
	for _, p := range s.config.Policies {
		if p.Name == policyName {
			policyConfig = &p
			break
		}
	}

	if policyConfig == nil {
		return fmt.Errorf("policy not found: %s", policyName)
	}

	return s.executePolicy(ctx, *policyConfig)
}

// ListPolicies 列出所有策略
func (s *ScanScheduler) ListPolicies() []PolicyConfig {
	return s.config.Policies
}

// GetPolicy 获取指定策略
func (s *ScanScheduler) GetPolicy(name string) (*PolicyConfig, error) {
	for _, p := range s.config.Policies {
		if p.Name == name {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("policy not found: %s", name)
}

// UpdatePolicy 更新策略（动态配置）
func (s *ScanScheduler) UpdatePolicy(config PolicyConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, p := range s.config.Policies {
		if p.Name == config.Name {
			s.config.Policies[i] = config
			return nil
		}
	}

	// 如果不存在，添加新策略
	s.config.Policies = append(s.config.Policies, config)
	return nil
}
