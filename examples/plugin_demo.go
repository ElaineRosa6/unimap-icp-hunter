package main

import (
	"context"
	"fmt"
	"log"

	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/plugin"
	"github.com/unimap-icp-hunter/project/internal/plugin/processors"
	"github.com/unimap-icp-hunter/project/internal/service"
)

func main() {
	fmt.Println("=== UniMap 插件系统示例 ===\n")

	// 1. 创建统一服务
	svc := service.NewUnifiedService()

	// 2. 注册数据处理器
	fmt.Println("注册数据处理器...")
	
	// 数据清洗处理器
	cleaner := processors.NewDataCleaningProcessor()
	if err := svc.RegisterProcessor(cleaner, map[string]interface{}{
		"removeEmpty":    true,
		"normalizeURLs":  true,
		"trimWhitespace": true,
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ 数据清洗处理器已注册")

	// 数据验证处理器
	validator := processors.NewValidationProcessor(false)
	if err := svc.RegisterProcessor(validator, map[string]interface{}{
		"strictMode":     false,
		"validateIP":     true,
		"validatePort":   true,
		"allowPrivateIP": true,
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ 数据验证处理器已注册")

	// 数据富化处理器
	enricher := processors.NewEnrichmentProcessor()
	if err := svc.RegisterProcessor(enricher, map[string]interface{}{
		"addTimestamp":     true,
		"addFingerprint":   true,
		"normalizeCountry": true,
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ 数据富化处理器已注册")

	// 数据去重处理器
	dedup := processors.NewDeduplicationProcessor(processors.StrategyAdvanced)
	if err := svc.RegisterProcessor(dedup, map[string]interface{}{
		"strategy": "advanced",
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ 数据去重处理器已注册")

	// 3. 注册钩子
	fmt.Println("\n注册钩子...")
	hooks := svc.GetPluginManager().GetHooks()

	// 查询前钩子
	hooks.RegisterHook(plugin.HookBeforeQuery, func(pluginName string, data map[string]interface{}) error {
		fmt.Printf("→ 查询前: query=%v, engines=%v\n", data["query"], data["engines"])
		return nil
	})

	// 查询后钩子
	hooks.RegisterHook(plugin.HookAfterQuery, func(pluginName string, data map[string]interface{}) error {
		fmt.Printf("→ 查询后: 结果数量=%v\n", data["result_count"])
		return nil
	})

	// 数据处理前钩子
	hooks.RegisterHook(plugin.HookBeforeProcess, func(pluginName string, data map[string]interface{}) error {
		fmt.Println("→ 开始数据处理...")
		return nil
	})

	// 数据处理后钩子
	hooks.RegisterHook(plugin.HookAfterProcess, func(pluginName string, data map[string]interface{}) error {
		fmt.Printf("→ 数据处理完成: 原始=%v, 处理后=%v\n", 
			data["original_count"], data["processed_count"])
		return nil
	})
	fmt.Println("✓ 钩子已注册")

	// 4. 列出已注册的处理器
	fmt.Println("\n已注册的处理器:")
	processors := svc.ListProcessors()
	for _, proc := range processors {
		fmt.Printf("  - %s (v%s) [优先级: %d]\n", 
			proc["name"], proc["version"], proc["priority"])
		fmt.Printf("    %s\n", proc["description"])
	}

	// 5. 模拟查询和数据处理
	fmt.Println("\n执行模拟查询...")
	
	// 创建模拟数据
	mockAssets := []model.UnifiedAsset{
		{
			IP:          "192.168.1.1",
			Port:        80,
			Protocol:    "HTTP",
			Host:        "example.com ",  // 注意空格
			Title:       "  Example Site  ",
			CountryCode: "cn",
			Source:      "mock",
		},
		{
			IP:          "192.168.1.1",  // 重复
			Port:        80,
			Protocol:    "http",  // 小写
			Host:        "example.com",
			Title:       "Example Site",
			CountryCode: "CN",
			Source:      "mock",
		},
		{
			IP:          "10.0.0.1",
			Port:        443,
			Protocol:    "https",
			Host:        "test.com",
			Title:       "Test Site",
			CountryCode: "US",
			Source:      "mock",
		},
		{
			IP:          "",  // 空 IP
			Port:        0,
			Host:        "",
			Title:       "",
			Source:      "mock",
		},
	}

	fmt.Printf("原始数据: %d 条\n", len(mockAssets))
	
	// 手动调用处理管道
	ctx := context.Background()
	processedAssets, err := processAssets(svc, ctx, mockAssets)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\n处理后数据: %d 条\n", len(processedAssets))
	
	// 6. 显示处理结果
	fmt.Println("\n处理结果:")
	for i, asset := range processedAssets {
		fmt.Printf("\n资产 #%d:\n", i+1)
		fmt.Printf("  IP:          %s\n", asset.IP)
		fmt.Printf("  Port:        %d\n", asset.Port)
		fmt.Printf("  Protocol:    %s\n", asset.Protocol)
		fmt.Printf("  Host:        %s\n", asset.Host)
		fmt.Printf("  Title:       %s\n", asset.Title)
		fmt.Printf("  CountryCode: %s\n", asset.CountryCode)
		fmt.Printf("  Source:      %s\n", asset.Source)
		if len(asset.Extra) > 0 {
			fmt.Printf("  Extra:\n")
			for k, v := range asset.Extra {
				fmt.Printf("    %s: %v\n", k, v)
			}
		}
	}

	// 7. 健康检查
	fmt.Println("\n执行健康检查...")
	health := svc.HealthCheck()
	for name, status := range health {
		if status.Healthy {
			fmt.Printf("✓ %s: %s\n", name, status.Message)
		} else {
			fmt.Printf("✗ %s: %s\n", name, status.Message)
		}
	}

	// 8. 关闭服务
	fmt.Println("\n关闭服务...")
	if err := svc.Shutdown(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ 服务已关闭")

	fmt.Println("\n=== 示例完成 ===")
}

// processAssets 手动处理资产数据
func processAssets(svc *service.UnifiedService, ctx context.Context, assets []model.UnifiedAsset) ([]model.UnifiedAsset, error) {
	processors := svc.GetPluginManager().GetRegistry().GetProcessorPlugins()
	if len(processors) == 0 {
		return assets, nil
	}

	pipeline := plugin.NewProcessorPipeline(processors)
	return pipeline.Process(ctx, assets)
}
