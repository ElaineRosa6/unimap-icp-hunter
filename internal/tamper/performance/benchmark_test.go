package performance

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// BenchmarkCachePerformance 测试缓存性能
func BenchmarkCachePerformance(b *testing.B) {
	cm := NewCacheManager()

	// 预热缓存
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("test_key_%d", i)
		value := fmt.Sprintf("test_value_%d", i)
		cm.Set(key, value, time.Minute)
	}

	b.ResetTimer()

	// 测试缓存读取性能
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("test_key_%d", i%100)
		cm.Get(key)
	}
}

// BenchmarkOptimizerPerformance 测试优化器性能
func BenchmarkOptimizerPerformance(b *testing.B) {
	po := NewPerformanceOptimizer()

	testContent := `<!DOCTYPE html><html><head><title>Test Page</title></head><body><h1>Test Content</h1></body></html>`

	b.ResetTimer()

	// 测试优化器性能
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("test_site_%d", i%100)
		po.Optimize(key, testContent)
	}
}

// BenchmarkConcurrencyControl 测试并发控制
func BenchmarkConcurrencyControl(b *testing.B) {
	cm := NewConcurrencyManager()
	cm.SetMaxConcurrent(5)

	var wg sync.WaitGroup
	b.ResetTimer()

	// 测试并发控制
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cm.Acquire()
			time.Sleep(time.Microsecond * 10)
			cm.Release()
		}()
	}

	wg.Wait()
}

// BenchmarkCacheEviction 测试缓存淘汰性能
func BenchmarkCacheEviction(b *testing.B) {
	cm := NewCacheManager()

	// 设置较小的缓存大小以触发淘汰
	cm.maxSize = 100

	b.ResetTimer()

	// 测试缓存淘汰
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("test_key_%d", i)
		value := fmt.Sprintf("test_value_%d", i)
		cm.Set(key, value, time.Minute)
	}
}

// BenchmarkMetricsRecording 测试指标记录性能
func BenchmarkMetricsRecording(b *testing.B) {
	pm := NewPerformanceMetrics()

	b.ResetTimer()

	// 测试指标记录
	for i := 0; i < b.N; i++ {
		siteURL := fmt.Sprintf("test_site_%d", i%100)
		duration := time.Duration(i%1000) * time.Microsecond
		pm.RecordOptimization(siteURL, duration)
	}
}

// BenchmarkCacheHitRate 测试缓存命中率
func BenchmarkCacheHitRate(b *testing.B) {
	po := NewPerformanceOptimizer()

	testContent := `<!DOCTYPE html><html><head><title>Test Page</title></head><body><h1>Test Content</h1></body></html>`

	// 预热缓存
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("test_site_%d", i)
		po.Optimize(key, testContent)
	}

	b.ResetTimer()

	// 测试缓存命中率
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("test_site_%d", i%100)
		po.Optimize(key, testContent)
	}

	// 打印缓存命中率
	stats := po.GetPerformanceMetrics().GetStats()
	b.Logf("Cache Hit Rate: %.2f%%", stats["cache_hit_rate"].(float64)*100)
}

// BenchmarkMultipleSites 测试多站点性能
func BenchmarkMultipleSites(b *testing.B) {
	po := NewPerformanceOptimizer()

	testContent := `<!DOCTYPE html><html><head><title>Test Page</title></head><body><h1>Test Content</h1></body></html>`

	// 准备多个站点
	sites := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		sites[i] = fmt.Sprintf("test_site_%d", i)
	}

	b.ResetTimer()

	// 测试多站点性能
	for i := 0; i < b.N; i++ {
		site := sites[i%1000]
		po.Optimize(site, testContent)
	}
}

// BenchmarkOptimizationOverhead 测试优化开销
func BenchmarkOptimizationOverhead(b *testing.B) {
	po := NewPerformanceOptimizer()

	testContent := `<!DOCTYPE html><html><head><title>Test Page</title></head><body><h1>Test Content</h1></body></html>`

	// 禁用缓存以测试优化开销
	po.GetCacheManager().Clear()

	b.ResetTimer()

	// 测试优化开销
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("test_site_%d", i)
		po.Optimize(key, testContent)
	}
}

// BenchmarkMemoryUsage 测试内存使用
func BenchmarkMemoryUsage(b *testing.B) {
	cm := NewCacheManager()

	// 设置较大的缓存大小
	cm.maxSize = 10000

	b.ResetTimer()

	// 测试内存使用
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("test_key_%d", i)
		value := fmt.Sprintf("test_value_%d", i)
		cm.Set(key, value, time.Minute)
	}

	// 获取缓存统计
	stats := cm.GetStats()
	b.Logf("Cache Size: %d items", stats["total_items"])
}

// BenchmarkConcurrencyScaling 测试并发扩展性
func BenchmarkConcurrencyScaling(b *testing.B) {
	// 测试不同并发级别
	concurrencyLevels := []int{1, 5, 10, 20, 50}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(b *testing.B) {
			cm := NewConcurrencyManager()
			cm.SetMaxConcurrent(concurrency)

			var wg sync.WaitGroup
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					cm.Acquire()
					time.Sleep(time.Microsecond * 5)
					cm.Release()
				}()
			}

			wg.Wait()
		})
	}
}
