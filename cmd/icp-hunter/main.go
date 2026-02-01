package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/core/unimap"
	"github.com/unimap-icp-hunter/project/internal/repository"
	"github.com/unimap-icp-hunter/project/internal/service"
	"github.com/unimap-icp-hunter/project/internal/service/notification"
	"github.com/unimap-icp-hunter/project/internal/service/scheduler"
	"github.com/unimap-icp-hunter/project/internal/service/worker"
	"github.com/unimap-icp-hunter/project/pkg/utils"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
	cfgFile string
	logger  *zap.Logger
)

// rootCmd ICP-Hunter根命令
var rootCmd = &cobra.Command{
	Use:   "icp-hunter",
	Short: "ICP-Hunter - 自动化未备案网站测绘系统",
	Long: `ICP-Hunter 是一个自动化未备案网站测绘系统。

基于多引擎测绘结果，自动探测网站并检测ICP备案信息，
支持每日定时扫描、实时告警和报表生成。`,

	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("ICP-Hunter v1.0 - 使用 --help 查看可用命令")
	},
}

// runCmd 运行完整系统
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "运行完整系统（调度器+Worker）",
	Long: `运行完整的ICP-Hunter系统，包括：
- 定时任务调度器
- HTTP探测Worker
- 自动告警和报表

系统会按照配置文件中的cron表达式执行每日扫描任务。`,
	Run: func(cmd *cobra.Command, args []string) {
		initLogger()
		defer logger.Sync()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// 初始化服务
		svc, err := initICPService()
		if err != nil {
			logger.Fatal("Failed to initialize service", zap.Error(err))
		}

		// 启动调度器
		if err := svc.StartScheduler(ctx); err != nil {
			logger.Fatal("Failed to start scheduler", zap.Error(err))
		}

		// 启动Worker
		if err := svc.StartWorker(ctx); err != nil {
			logger.Fatal("Failed to start worker", zap.Error(err))
		}

		logger.Info("ICP-Hunter system started successfully")

		// 等待中断信号
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh

		logger.Info("Shutting down...")
		svc.StopScheduler()
		svc.StopWorker()
	},
}

// scanNowCmd 立即执行扫描
var scanNowCmd = &cobra.Command{
	Use:   "scan [policy_name]",
	Short: "立即执行扫描任务",
	Long: `立即执行指定策略的扫描任务。

如果不指定策略名称，将执行所有已启用的策略。

示例:
  icp-hunter scan
  icp-hunter scan http_80
  icp-hunter scan https_443`,
	Run: func(cmd *cobra.Command, args []string) {
		initLogger()
		defer logger.Sync()

		ctx := context.Background()
		svc, err := initICPService()
		if err != nil {
			logger.Fatal("Failed to initialize service", zap.Error(err))
		}

		if len(args) > 0 {
			// 执行单个策略
			policyName := args[0]
			fmt.Printf("执行策略: %s\n", policyName)
			err = svc.scheduler.ExecuteImmediate(ctx, policyName)
		} else {
			// 执行所有策略
			fmt.Println("执行所有已启用策略...")
			policies := svc.scheduler.ListPolicies()
			for _, p := range policies {
				fmt.Printf("  - %s\n", p.Name)
				if err := svc.scheduler.ExecuteImmediate(ctx, p.Name); err != nil {
					logger.Error("Failed to execute policy", zap.String("policy", p.Name), zap.Error(err))
				}
			}
		}

		if err != nil {
			logger.Fatal("Scan failed", zap.Error(err))
		}

		fmt.Println("扫描完成")
	},
}

// workerCmd 单独运行Worker
var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "单独运行探测Worker",
	Long: `单独运行探测Worker，从Redis队列消费任务并执行HTTP探测。

适用于分布式部署场景，可以运行多个Worker实例。`,
	Run: func(cmd *cobra.Command, args []string) {
		initLogger()
		defer logger.Sync()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		svc, err := initICPService()
		if err != nil {
			logger.Fatal("Failed to initialize service", zap.Error(err))
		}

		if err := svc.StartWorker(ctx); err != nil {
			logger.Fatal("Failed to start worker", zap.Error(err))
		}

		logger.Info("Worker started")

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh

		svc.StopWorker()
		logger.Info("Worker stopped")
	},
}

// schedulerCmd 单独运行调度器
var schedulerCmd = &cobra.Command{
	Use:   "scheduler",
	Short: "单独运行调度器",
	Long: `单独运行调度器，负责生成扫描任务并推送到队列。

适用于分布式部署场景，调度器和Worker可以分开部署。`,
	Run: func(cmd *cobra.Command, args []string) {
		initLogger()
		defer logger.Sync()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		svc, err := initICPService()
		if err != nil {
			logger.Fatal("Failed to initialize service", zap.Error(err))
		}

		if err := svc.StartScheduler(ctx); err != nil {
			logger.Fatal("Failed to start scheduler", zap.Error(err))
		}

		logger.Info("Scheduler started")

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh

		svc.StopScheduler()
		logger.Info("Scheduler stopped")
	},
}

// reportCmd 生成报告
var reportCmd = &cobra.Command{
	Use:   "report [date]",
	Short: "生成日报",
	Long: `生成指定日期的扫描报告。

日期格式: YYYY-MM-DD
如果不指定日期，默认生成昨天的报告。

示例:
  icp-hunter report
  icp-hunter report 2024-01-14`,
	Run: func(cmd *cobra.Command, args []string) {
		initLogger()
		defer logger.Sync()

		ctx := context.Background()
		svc, err := initICPService()
		if err != nil {
			logger.Fatal("Failed to initialize service", zap.Error(err))
		}

		var date time.Time
		if len(args) > 0 {
			var err error
			date, err = time.Parse("2006-01-02", args[0])
			if err != nil {
				logger.Fatal("Invalid date format", zap.Error(err))
			}
		} else {
			date = time.Now().AddDate(0, 0, -1)
		}

		stats, err := svc.GetDailyReport(ctx, date)
		if err != nil {
			logger.Fatal("Failed to generate report", zap.Error(err))
		}

		// 输出报告
		fmt.Printf("=== ICP-Hunter 日报 %s ===\n", stats.Date)
		fmt.Printf("新增疑似未备案: %d\n", stats.NewUnregistered)
		fmt.Printf("总疑似未备案: %d\n", stats.TotalUnregistered)
		fmt.Printf("已备案: %d\n", stats.TotalRegistered)
		fmt.Printf("扫描总数: %d\n", stats.TotalScanned)

		if len(stats.ProvinceStats) > 0 {
			fmt.Println("\n按省份分布:")
			for _, p := range stats.ProvinceStats {
				fmt.Printf("  %s: 未备案 %d / 总数 %d\n", p.Name, p.Unregistered, p.Total)
			}
		}

		// 发送报告
		if cmd.Flag("send").Value.String() == "true" {
			if err := svc.notifier.SendDailyReport(stats); err != nil {
				logger.Error("Failed to send report", zap.Error(err))
			} else {
				fmt.Println("\n报告已发送")
			}
		}
	},
}

// whitelistCmd 白名单管理
var whitelistCmd = &cobra.Command{
	Use:   "whitelist",
	Short: "白名单管理",
}

var whitelistAddCmd = &cobra.Command{
	Use:   "add [type] [value]",
	Short: "添加白名单",
	Long: `添加白名单条目。

类型: ip, domain, asn, org

示例:
  icp-hunter whitelist add ip 192.168.1.0/24
  icp-hunter whitelist add domain example.com
  icp-hunter whitelist add org "Example Corp"`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		initLogger()
		defer logger.Sync()

		typ := args[0]
		value := args[1]
		reason, _ := cmd.Flags().GetString("reason")

		ctx := context.Background()
		svc, err := initICPService()
		if err != nil {
			logger.Fatal("Failed to initialize service", zap.Error(err))
		}

		if err := svc.AddWhitelist(ctx, typ, value, reason, "cli"); err != nil {
			logger.Fatal("Failed to add whitelist", zap.Error(err))
		}

		fmt.Printf("✅ 已添加白名单: %s = %s\n", typ, value)
	},
}

var whitelistListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出白名单",
	Run: func(cmd *cobra.Command, args []string) {
		initLogger()
		defer logger.Sync()

		ctx := context.Background()
		svc, err := initICPService()
		if err != nil {
			logger.Fatal("Failed to initialize service", zap.Error(err))
		}

		whitelists, total, err := svc.ListWhitelists(ctx, 1, 100)
		if err != nil {
			logger.Fatal("Failed to list whitelist", zap.Error(err))
		}

		fmt.Printf("白名单列表 (共 %d 条):\n", total)
		fmt.Println("ID\t类型\t值\t原因")
		for _, wl := range whitelists {
			fmt.Printf("%d\t%s\t%s\t%s\n", wl.ID, wl.Type, wl.Value, wl.Reason)
		}
	},
}

var whitelistRemoveCmd = &cobra.Command{
	Use:   "remove [id]",
	Short: "删除白名单",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		initLogger()
		defer logger.Sync()

		var id uint
		fmt.Sscanf(args[0], "%d", &id)

		ctx := context.Background()
		svc, err := initICPService()
		if err != nil {
			logger.Fatal("Failed to initialize service", zap.Error(err))
		}

		if err := svc.RemoveWhitelist(ctx, id); err != nil {
			logger.Fatal("Failed to remove whitelist", zap.Error(err))
		}

		fmt.Printf("✅ 已删除白名单 ID: %d\n", id)
	},
}

// statsCmd 统计信息
var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "查看统计信息",
	Run: func(cmd *cobra.Command, args []string) {
		initLogger()
		defer logger.Sync()

		ctx := context.Background()
		svc, err := initICPService()
		if err != nil {
			logger.Fatal("Failed to initialize service", zap.Error(err))
		}

		stats, err := svc.GetScanStats(ctx)
		if err != nil {
			logger.Fatal("Failed to get stats", zap.Error(err))
		}

		fmt.Println("=== 系统统计 ===")
		fmt.Printf("生成时间: %s\n", stats["timestamp"])

		if todayStats, ok := stats["today_stats"].(map[string]interface{}); ok {
			fmt.Println("\n今日扫描:")
			fmt.Printf("  扫描总数: %v\n", todayStats["total_scanned"])
			fmt.Printf("  未备案: %v\n", todayStats["unregistered"])
			fmt.Printf("  已备案: %v\n", todayStats["registered"])
			fmt.Printf("  新增未备案: %v\n", todayStats["new_unregistered"])
		}

		if queueLen, ok := stats["queue_length"].(int64); ok {
			fmt.Printf("\n队列长度: %d\n", queueLen)
		}

		if activePolicies, ok := stats["active_policies"].(int); ok {
			fmt.Printf("活跃策略数: %d\n", activePolicies)
		}
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径")

	// run命令
	rootCmd.AddCommand(runCmd)

	// scan命令
	rootCmd.AddCommand(scanNowCmd)

	// worker命令
	rootCmd.AddCommand(workerCmd)

	// scheduler命令
	rootCmd.AddCommand(schedulerCmd)

	// report命令
	reportCmd.Flags().Bool("send", false, "发送报告")
	rootCmd.AddCommand(reportCmd)

	// whitelist命令
	whitelistAddCmd.Flags().String("reason", "", "添加原因")
	whitelistCmd.AddCommand(whitelistAddCmd)
	whitelistCmd.AddCommand(whitelistListCmd)
	whitelistCmd.AddCommand(whitelistRemoveCmd)
	rootCmd.AddCommand(whitelistCmd)

	// stats命令
	rootCmd.AddCommand(statsCmd)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("./configs")
		viper.AddConfigPath(".")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("ICP")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Printf("警告: 无法读取配置文件: %v\n", err)
		}
	}
}

func initLogger() {
	if logger != nil {
		return
	}

	logLevel := viper.GetString("app.log_level")
	if logLevel == "" {
		logLevel = "info"
	}

	var level zapcore.Level
	switch logLevel {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	config := zap.Config{
		Level:            zap.NewAtomicLevelAt(level),
		Development:      false,
		Encoding:         "console",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, _ = config.Build()
}

func initICPService() (*service.ICPService, error) {
	// 初始化数据库
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		viper.GetString("database.username"),
		viper.GetString("database.password"),
		viper.GetString("database.host"),
		viper.GetInt("database.port"),
		viper.GetString("database.name"),
		viper.GetString("database.charset"),
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %v", err)
	}

	// 自动迁移表结构
	if err := repository.InitDatabase(db); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %v", err)
	}

	// 初始化Redis
	redisClient := utils.NewRedisClient(viper.Sub("redis"))
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect redis: %v", err)
	}

	// 创建Repository
	repo := repository.NewGORMRepository(db)

	// 创建引擎适配器
	orchestrator := adapter.NewEngineOrchestrator()

	// 注册FOFA
	if viper.GetBool("unimap.engines.fofa.enabled") {
		fofaAdapter := adapter.NewFofaAdapter(
			viper.GetString("unimap.engines.fofa.base_url"),
			viper.GetString("unimap.engines.fofa.api_key"),
			viper.GetString("unimap.engines.fofa.email"),
			viper.GetInt("unimap.engines.fofa.qps"),
			viper.GetDuration("unimap.engines.fofa.timeout"),
		)
		orchestrator.RegisterAdapter(fofaAdapter)
	}

	// 注册Hunter
	if viper.GetBool("unimap.engines.hunter.enabled") {
		hunterAdapter := adapter.NewHunterAdapter(
			viper.GetString("unimap.engines.hunter.base_url"),
			viper.GetString("unimap.engines.hunter.api_key"),
			viper.GetInt("unimap.engines.hunter.qps"),
			viper.GetDuration("unimap.engines.hunter.timeout"),
		)
		orchestrator.RegisterAdapter(hunterAdapter)
	}

	// 注册ZoomEye
	if viper.GetBool("unimap.engines.zoomeye.enabled") {
		zoomeyeAdapter := adapter.NewZoomEyeAdapter(
			viper.GetString("unimap.engines.zoomeye.base_url"),
			viper.GetString("unimap.engines.zoomeye.api_key"),
			viper.GetInt("unimap.engines.zoomeye.qps"),
			viper.GetDuration("unimap.engines.zoomeye.timeout"),
		)
		orchestrator.RegisterAdapter(zoomeyeAdapter)
	}

	// 注册Quake
	if viper.GetBool("unimap.engines.quake.enabled") {
		quakeAdapter := adapter.NewQuakeAdapter(
			viper.GetString("unimap.engines.quake.base_url"),
			viper.GetString("unimap.engines.quake.api_key"),
			viper.GetInt("unimap.engines.quake.qps"),
			viper.GetDuration("unimap.engines.quake.timeout"),
		)
		orchestrator.RegisterAdapter(quakeAdapter)
	}

	// 创建归并器
	merger := unimap.NewResultMerger()

	// 创建调度器
	schedulerConfig := &scheduler.SchedulerConfig{}
	if err := viper.UnmarshalKey("icp_hunter", schedulerConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scheduler config: %v", err)
	}

	scanScheduler := scheduler.NewScanScheduler(orchestrator, merger, redisClient, schedulerConfig, logger)

	// 创建Worker
	probeConfig := worker.ProbeConfig{
		QueueName:      viper.GetString("icp_hunter.executor.queue_name"),
		ConsumerGroup:  viper.GetString("icp_hunter.executor.consumer_group"),
		ConsumerName:   viper.GetString("icp_hunter.executor.consumer_name"),
		WorkerPoolSize: viper.GetInt("icp_hunter.executor.worker_pool_size"),
		HTTP: worker.HTTPConfig{
			Timeout:          viper.GetDuration("icp_hunter.executor.http.timeout"),
			MaxRedirects:     viper.GetInt("icp_hunter.executor.http.max_redirects"),
			UserAgent:        viper.GetString("icp_hunter.executor.http.user_agent"),
			FollowRedirects:  viper.GetBool("icp_hunter.executor.http.follow_redirects"),
			DisableTLSVerify: viper.GetBool("icp_hunter.executor.http.disable_tls_verify"),
		},
		Retry: worker.RetryConfig{
			MaxAttempts:   viper.GetInt("icp_hunter.executor.retry.max_attempts"),
			BackoffFactor: viper.GetInt("icp_hunter.executor.retry.backoff_factor"),
			InitialDelay:  viper.GetDuration("icp_hunter.executor.retry.initial_delay"),
		},
	}

	probeExecutor := worker.NewProbeExecutor(redisClient, probeConfig, logger)

	// 创建通知服务
	notifyConfig := notification.NotifyConfig{}
	if err := viper.UnmarshalKey("icp_hunter.notification", notifyConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notification config: %v", err)
	}

	notifier := notification.NewNotifier(notifyConfig, logger)

	// 创建ICP服务
	icpService := service.NewICPService(
		repo,
		redisClient,
		orchestrator,
		merger,
		scanScheduler,
		probeExecutor,
		notifier,
		logger,
	)

	return icpService, nil
}
