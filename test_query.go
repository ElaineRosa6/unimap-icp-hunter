package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/core/unimap"
	"github.com/unimap-icp-hunter/project/internal/model"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Engines struct {
		Quake struct {
			Enabled bool   `yaml:"enabled"`
			APIKey  string `yaml:"api_key"`
			BaseURL string `yaml:"base_url"`
		} `yaml:"quake"`
		Zoomeye struct {
			Enabled bool   `yaml:"enabled"`
			APIKey  string `yaml:"api_key"`
			BaseURL string `yaml:"base_url"`
		} `yaml:"zoomeye"`
		Hunter struct {
			Enabled bool   `yaml:"enabled"`
			APIKey  string `yaml:"api_key"`
			BaseURL string `yaml:"base_url"`
		} `yaml:"hunter"`
		Fofa struct {
			Enabled bool   `yaml:"enabled"`
			APIKey  string `yaml:"api_key"`
			Email   string `yaml:"email"`
			BaseURL string `yaml:"base_url"`
		} `yaml:"fofa"`
	} `yaml:"engines"`
}

func main() {
	// 读取配置文件
	configFile, err := os.ReadFile("configs/config.yaml")
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(configFile, &config); err != nil {
		log.Fatalf("Failed to unmarshal config: %v", err)
	}

	// 创建引擎编排器
	orchestrator := adapter.NewEngineOrchestrator()

	// 注册引擎适配器
	if config.Engines.Fofa.Enabled {
		fofaAdapter := adapter.NewFofaAdapter(config.Engines.Fofa.BaseURL, config.Engines.Fofa.APIKey, config.Engines.Fofa.Email, 5, 30*time.Second)
		orchestrator.RegisterAdapter(fofaAdapter)
	}

	if config.Engines.Hunter.Enabled {
		hunterAdapter := adapter.NewHunterAdapter(config.Engines.Hunter.BaseURL, config.Engines.Hunter.APIKey, 5, 30*time.Second)
		orchestrator.RegisterAdapter(hunterAdapter)
	}

	if config.Engines.Zoomeye.Enabled {
		zoomeyeAdapter := adapter.NewZoomEyeAdapter(config.Engines.Zoomeye.BaseURL, config.Engines.Zoomeye.APIKey, 5, 30*time.Second)
		orchestrator.RegisterAdapter(zoomeyeAdapter)
	}

	if config.Engines.Quake.Enabled {
		quakeAdapter := adapter.NewQuakeAdapter(config.Engines.Quake.BaseURL, config.Engines.Quake.APIKey, 5, 30*time.Second)
		orchestrator.RegisterAdapter(quakeAdapter)
	}

	// 测试UQL解析
	parser := unimap.NewUQLParser()
	query := `country="CN" && port="80" && protocol="http"`
	ast, err := parser.Parse(query)
	if err != nil {
		log.Fatalf("Failed to parse UQL query: %v", err)
	}

	fmt.Printf("Parsed UQL query: %s\n", query)

	// 测试引擎列表
	engines := orchestrator.ListAdapters()
	fmt.Printf("Available engines: %v\n", engines)

	// 测试查询翻译
	engineNames := []string{"fofa", "hunter", "zoomeye", "quake"}
	queries, err := orchestrator.TranslateQuery(ast, engineNames)
	if err != nil {
		log.Fatalf("Failed to translate query: %v", err)
	}

	fmt.Println("Translated queries:")
	for _, q := range queries {
		fmt.Printf("  %s: %s\n", q.EngineName, q.Query)
	}

	// 检查引擎配额
	fmt.Println("\nChecking engine quotas...")
	for _, engineName := range engineNames {
		adapter, exists := orchestrator.GetAdapter(engineName)
		if exists {
			quota, err := adapter.GetQuota()
			if err != nil {
				fmt.Printf("  %s: Failed to check quota - %v\n", engineName, err)
			} else {
				fmt.Printf("  %s: Quota - Remaining: %d, Used: %d, Total: %d %s\n",
					engineName, quota.Remaining, quota.Used, quota.Total, quota.Unit)
			}
		}
	}

	// 测试执行搜索
	fmt.Println("\nExecuting search...")

	// 先翻译查询，查看每个引擎的查询语句
	queries, err = orchestrator.TranslateQuery(ast, engineNames)
	if err != nil {
		log.Fatalf("Failed to translate query: %v", err)
	}

	// 手动执行搜索，查看每个引擎的详细错误
	fmt.Println("\nDetailed engine results:")

	// 优化策略：
	// 1. 根据引擎特性设置不同的页面大小
	// 2. 处理API限制，避免请求过多
	// 3. 尽可能获取更多结果

	// 为每个引擎设置不同的参数
	engineParams := map[string]struct {
		pageSize  int
		maxPages  int
		rateLimit time.Duration
	}{
		"hunter": {
			pageSize:  50, // Hunter每页最多50条
			maxPages:  1,  // HunterAPI限制严格，只获取1页
			rateLimit: 500 * time.Millisecond,
		},
		"quake": {
			pageSize:  50, // Quake每页最多50条
			maxPages:  3,  // QuakeAPI限制宽松，获取3页
			rateLimit: 200 * time.Millisecond,
		},
		"fofa": {
			pageSize:  10, // Fofa每页10条
			maxPages:  1,  // 只获取1页
			rateLimit: 300 * time.Millisecond,
		},
		"zoomeye": {
			pageSize:  10, // ZoomEye每页10条
			maxPages:  1,  // 只获取1页
			rateLimit: 300 * time.Millisecond,
		},
	}

	// 执行搜索，使用优化参数
	var wg sync.WaitGroup
	resultsChan := make(chan *model.EngineResult, len(queries)*3)

	for _, q := range queries {
		wg.Add(1)
		go func(query model.EngineQuery) {
			defer wg.Done()

			adapter, exists := orchestrator.GetAdapter(query.EngineName)
			if !exists {
				resultsChan <- &model.EngineResult{
					EngineName: query.EngineName,
					Error:      fmt.Sprintf("failed to find adapter: %s", query.EngineName),
				}
				return
			}

			// 获取引擎参数
			params := engineParams[query.EngineName]
			if params.pageSize == 0 {
				params.pageSize = 10
				params.maxPages = 1
				params.rateLimit = 300 * time.Millisecond
			}

			// 分页获取
			for page := 1; page <= params.maxPages; page++ {
				result, err := adapter.Search(query.Query, page, params.pageSize)
				if err != nil {
					resultsChan <- &model.EngineResult{
						EngineName: query.EngineName,
						Error:      fmt.Sprintf("search failed on page %d: %v", page, err),
					}
					break
				}

				resultsChan <- result

				if !result.HasMore || page >= params.maxPages {
					break
				}

				// 速率控制
				time.Sleep(params.rateLimit)
			}
		}(q)
	}

	wg.Wait()
	close(resultsChan)

	// 收集结果
	engineResults := []*model.EngineResult{}
	for result := range resultsChan {
		engineResults = append(engineResults, result)
	}

	// 输出每个引擎的结果和错误
	for _, result := range engineResults {
		if result.Error != "" {
			fmt.Printf("  %s: ERROR - %s\n", result.EngineName, result.Error)
		} else {
			fmt.Printf("  %s: Found %d assets (Total: %d)\n", result.EngineName, len(result.RawData), result.Total)
		}
	}

	// 标准化结果
	assets, err := orchestrator.NormalizeResults(engineResults)
	if err != nil {
		log.Fatalf("Failed to normalize results: %v", err)
	}

	fmt.Printf("\nSearch completed. Found %d assets\n", len(assets))

	// 输出前10个结果
	limit := 10
	if len(assets) < limit {
		limit = len(assets)
	}

	fmt.Println("\nTop results:")
	for i, asset := range assets[:limit] {
		fmt.Printf("%d. IP: %s, Port: %d, Protocol: %s, Source: %s\n",
			i+1, asset.IP, asset.Port, asset.Protocol, asset.Source)
		if asset.Host != "" {
			fmt.Printf("   Host: %s\n", asset.Host)
		}
		if asset.Title != "" {
			fmt.Printf("   Title: %s\n", asset.Title)
		}
		fmt.Println()
	}

	// 统计每个引擎的结果数量
	fmt.Println("\nResults by engine:")
	engineCount := make(map[string]int)
	for _, asset := range assets {
		engineCount[asset.Source]++
	}
	for engine, count := range engineCount {
		fmt.Printf("  %s: %d assets\n", engine, count)
	}

	// 测试结果合并
	if len(assets) > 0 {
		merger := unimap.NewResultMerger()

		// 使用标准化后的资产列表调用Merge方法
		mergedResult := merger.Merge(assets)
		fmt.Printf("Merged result: %d total assets\n", mergedResult.Total)
		fmt.Printf("Unique assets after merging: %d\n", len(mergedResult.Assets))
		fmt.Printf("Duplicate assets: %d\n", mergedResult.Duplicates)
	}
}
