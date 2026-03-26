package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/appversion"
	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/service"
	"github.com/unimap-icp-hunter/project/internal/utils"
	"github.com/unimap-icp-hunter/project/web"
)

const configPath = "configs/config.yaml"

func main() {
	// 初始化日志系统
	logger.Init(logger.Config{
		Level:    logger.LevelInfo,
		Encoding: "console",
		File:     "",
	})

	// 加载配置
	cfgManager := config.NewManager(configPath)
	if err := cfgManager.Load(); err != nil {
		logger.Warnf("Failed to load config from %s: %v", configPath, err)
	}
	cfg := cfgManager.GetConfig()

	// 创建统一服务
	svc := service.NewUnifiedServiceWithConfig(cfg)

	// 注册引擎适配器
	if cfg != nil {
		registerEngines(svc, cfg)
	}

	// 从服务中获取编排器
	orchestrator := svc.GetOrchestrator()

	// 创建Web服务器
	server, err := web.NewServer(8448, svc, orchestrator, cfg, cfgManager)
	if err != nil {
		logger.Errorf("Failed to initialize Web server: %v", err)
		os.Exit(1)
	}

	// 创建优雅关闭管理器
	shutdownManager := utils.NewShutdownManager(30 * time.Second)

	// 注册关闭处理函数
	shutdownManager.RegisterHandler(func(ctx context.Context) error {
		logger.Info("Shutting down Web server...")
		return server.Shutdown(ctx)
	})

	shutdownManager.RegisterHandler(func(ctx context.Context) error {
		logger.Info("Shutting down service...")
		return svc.Shutdown()
	})

	// 启动优雅关闭监听
	shutdownManager.Start()

	// 启动Web服务器（在goroutine中运行，不阻塞）
	go func() {
		fmt.Printf("Starting Web server %s on :8448...\n", appversion.Short())
		if err := server.Start(); err != nil {
			logger.Errorf("Web server error: %v", err)
			shutdownManager.Shutdown()
		}
	}()

	// 等待关闭信号
	shutdownManager.Wait()
	logger.Info("Application stopped gracefully")
}

// registerEngines 注册引擎适配器
func registerEngines(svc *service.UnifiedService, cfg *config.Config) {
	// 注册FOFA
	if cfg.Engines.Fofa.Enabled {
		if cfg.Engines.Fofa.APIKey != "" {
			svc.RegisterAdapter(adapter.NewFofaAdapter(
				cfg.Engines.Fofa.BaseURL,
				cfg.Engines.Fofa.APIKey,
				cfg.Engines.Fofa.Email,
				cfg.Engines.Fofa.QPS,
				time.Duration(cfg.Engines.Fofa.Timeout)*time.Second,
			))
			logger.Info("FOFA engine registered (API mode)")
		} else {
			// Web-only模式：注册适配器但不支持API查询
			svc.RegisterAdapter(adapter.NewFofaAdapterWebOnly())
			logger.Info("FOFA engine registered (Web-only mode)")
		}
	}

	// 注册Hunter
	if cfg.Engines.Hunter.Enabled {
		if cfg.Engines.Hunter.APIKey != "" {
			svc.RegisterAdapter(adapter.NewHunterAdapter(
				cfg.Engines.Hunter.BaseURL,
				cfg.Engines.Hunter.APIKey,
				cfg.Engines.Hunter.QPS,
				time.Duration(cfg.Engines.Hunter.Timeout)*time.Second,
			))
			logger.Info("Hunter engine registered (API mode)")
		} else {
			svc.RegisterAdapter(adapter.NewHunterAdapterWebOnly())
			logger.Info("Hunter engine registered (Web-only mode)")
		}
	}

	// 注册ZoomEye
	if cfg.Engines.Zoomeye.Enabled {
		if cfg.Engines.Zoomeye.APIKey != "" {
			svc.RegisterAdapter(adapter.NewZoomEyeAdapter(
				cfg.Engines.Zoomeye.BaseURL,
				cfg.Engines.Zoomeye.APIKey,
				cfg.Engines.Zoomeye.QPS,
				time.Duration(cfg.Engines.Zoomeye.Timeout)*time.Second,
			))
			logger.Info("ZoomEye engine registered (API mode)")
		} else {
			svc.RegisterAdapter(adapter.NewZoomEyeAdapterWebOnly())
			logger.Info("ZoomEye engine registered (Web-only mode)")
		}
	}

	// 注册Quake
	if cfg.Engines.Quake.Enabled {
		if cfg.Engines.Quake.APIKey != "" {
			svc.RegisterAdapter(adapter.NewQuakeAdapter(
				cfg.Engines.Quake.BaseURL,
				cfg.Engines.Quake.APIKey,
				cfg.Engines.Quake.QPS,
				time.Duration(cfg.Engines.Quake.Timeout)*time.Second,
			))
			logger.Info("Quake engine registered (API mode)")
		} else {
			svc.RegisterAdapter(adapter.NewQuakeAdapterWebOnly())
			logger.Info("Quake engine registered (Web-only mode)")
		}
	}

	// 注册Shodan
	if cfg.Engines.Shodan.Enabled {
		if cfg.Engines.Shodan.APIKey != "" {
			svc.RegisterAdapter(adapter.NewShodanAdapter(
				cfg.Engines.Shodan.BaseURL,
				cfg.Engines.Shodan.APIKey,
				cfg.Engines.Shodan.QPS,
				time.Duration(cfg.Engines.Shodan.Timeout)*time.Second,
			))
			logger.Info("Shodan engine registered (API mode)")
		} else {
			svc.RegisterAdapter(adapter.NewShodanAdapterWebOnly())
			logger.Info("Shodan engine registered (Web-only mode)")
		}
	}
}
