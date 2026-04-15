package main

import (
	"fmt"
	"time"

	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/utils"
)

func main() {
	// 初始化日志
	fmt.Println("Performance benchmark starting...")

	// 加载配置
	cfgManager := config.NewManager("config.yaml")
	if err := cfgManager.Load(); err != nil {
		fmt.Printf("Warning: failed to load config: %v\n", err)
	}
	cfg := cfgManager.GetConfig()

	// 创建内存缓存
	memCache := utils.NewMemoryCache(10000, 30*time.Second)

	// 创建Redis缓存（如果配置了）
	var redisCache utils.QueryCache
	if cfg.Cache.Backend == "redis" {
		redisCfg := utils.RedisConfig{
			Addr:            cfg.Cache.Redis.Addr,
			Password:        cfg.Cache.Redis.Password,
			DB:              cfg.Cache.Redis.DB,
			Prefix:          cfg.Cache.Redis.Prefix,
			PoolSize:        cfg.Cache.Redis.PoolSize,
			MinIdleConns:    cfg.Cache.Redis.MinIdleConns,
			MaxIdleConns:    cfg.Cache.Redis.MaxIdleConns,
			MaxRetries:      cfg.Cache.Redis.MaxRetries,
			DialTimeout:     time.Duration(cfg.Cache.Redis.DialTimeout) * time.Millisecond,
			ReadTimeout:     time.Duration(cfg.Cache.Redis.ReadTimeout) * time.Millisecond,
			WriteTimeout:    time.Duration(cfg.Cache.Redis.WriteTimeout) * time.Millisecond,
			PoolTimeout:     time.Duration(cfg.Cache.Redis.PoolTimeout) * time.Millisecond,
			ConnMaxLifetime: time.Duration(cfg.Cache.Redis.ConnMaxLifetime) * time.Millisecond,
			ConnMaxIdleTime: time.Duration(cfg.Cache.Redis.ConnMaxIdleTime) * time.Millisecond,
		}
		redisCache = utils.NewRedisCache(redisCfg)
	}

	fmt.Println("=== Performance Test Results ===")

	// 测试内存缓存
	if memCache != nil {
		fmt.Println("\n1. Memory Cache Performance:")
		testCachePerformance(memCache, "memory", 10000)
	}

	// 测试Redis缓存
	if redisCache != nil {
		fmt.Println("\n2. Redis Cache Performance:")
		testCachePerformance(redisCache, "redis", 10000)
	}

	// 测试批量操作
	if memCache != nil {
		fmt.Println("\n3. Memory Cache Batch Operations:")
		testBatchOperations(memCache, "memory", 100, 100)
	}

	if redisCache != nil {
		fmt.Println("\n4. Redis Cache Batch Operations:")
		testBatchOperations(redisCache, "redis", 100, 100)
	}

	fmt.Println("\n=== Performance Test Completed ===")
}

func testCachePerformance(c utils.QueryCache, name string, operations int) {
	startTime := time.Now()

	for i := 0; i < operations; i++ {
		key := fmt.Sprintf("perf:%s:%d", name, i)
		data := []model.UnifiedAsset{{
			IP:     key,
			Port:   80,
			Host:   "test.example.com",
			Title:  "Performance Test",
			Source: "test",
		}}

		// Write operation
		c.Set(key, data, 5*time.Minute)

		// Read operation
		_, _ = c.Get(key)
	}

	totalTime := time.Since(startTime)
	throughput := float64(operations) / totalTime.Seconds()

	fmt.Printf("   Operations: %d\n", operations)
	fmt.Printf("   Total Time: %v\n", totalTime)
	fmt.Printf("   Throughput: %.2f ops/sec\n", throughput)
	fmt.Printf("   Average Latency: %v\n", totalTime/time.Duration(operations))
}

func testBatchOperations(c utils.QueryCache, name string, batchSize, batches int) {
	startTime := time.Now()

	for batch := 0; batch < batches; batch++ {
		// Prepare batch data
		keyMap := make(map[string][]model.UnifiedAsset)
		keys := make([]string, 0, batchSize)

		for i := 0; i < batchSize; i++ {
			key := fmt.Sprintf("batch:%s:%d:%d", name, batch, i)
			keyMap[key] = []model.UnifiedAsset{{
				IP:     key,
				Port:   80,
				Host:   "test.example.com",
				Title:  "Batch Test",
				Source: "test",
			}}
			keys = append(keys, key)
		}

		// Batch write
		c.SetMulti(keyMap, 5*time.Minute)

		// Batch read
		_ = c.GetMulti(keys)
	}

	totalTime := time.Since(startTime)
	totalOperations := batches * batchSize
	throughput := float64(totalOperations) / totalTime.Seconds()

	fmt.Printf("   Batch Size: %d, Batches: %d\n", batchSize, batches)
	fmt.Printf("   Total Operations: %d\n", totalOperations)
	fmt.Printf("   Total Time: %v\n", totalTime)
	fmt.Printf("   Throughput: %.2f ops/sec\n", throughput)
	fmt.Printf("   Average Latency: %v\n", totalTime/time.Duration(totalOperations))
}
