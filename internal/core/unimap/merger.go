package unimap

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/utils"
)

// ResultMerger 结果归并与去重
type ResultMerger struct {
	mu               sync.RWMutex
	assetPool        *utils.AssetPool
	slicePool        *utils.SlicePool
	mapPool          *utils.MapPool
	interfaceMapPool *utils.InterfaceMapPool
}

// 引擎优先级（数字越小优先级越高）
var enginePriorities = map[string]int{
	"fofa":    1,
	"hunter":  2,
	"zoomeye": 3,
	"quake":   4,
}

// NewResultMerger 创建归并器
func NewResultMerger() *ResultMerger {
	return &ResultMerger{
		assetPool:        utils.NewAssetPool(),
		slicePool:        utils.NewSlicePool(),
		mapPool:          utils.NewMapPool(),
		interfaceMapPool: utils.NewInterfaceMapPool(),
	}
}

// Merge 归并多个引擎的结果
func (m *ResultMerger) Merge(assets []model.UnifiedAsset) *model.MergeResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	assetMap := make(map[string]*model.UnifiedAsset)
	duplicates := 0

	// 使用对象池获取sourceStats映射
	sourceStats := m.interfaceMapPool.Get()
	defer m.interfaceMapPool.Put(sourceStats)

	for _, asset := range assets {
		// 增加来源统计
		if count, exists := sourceStats[asset.Source].(int); exists {
			sourceStats[asset.Source] = count + 1
		} else {
			sourceStats[asset.Source] = 1
		}

		// 生成去重键
		key := m.generateKey(asset)

		if existing, exists := assetMap[key]; exists {
			// 合并已有记录
			m.mergeAssets(existing, asset)
			duplicates++
		} else {
			// 从对象池获取资产对象
			newAsset := m.assetPool.Get()
			// 复制资产数据
			m.copyAsset(newAsset, asset)
			assetMap[key] = newAsset
		}
	}

	// 对结果进行排序
	mergedResult := &model.MergeResult{
		Assets:     assetMap,
		Total:      len(assetMap),
		Duplicates: duplicates,
	}

	// 添加来源统计信息到Extra字段
	for _, asset := range assetMap {
		if asset.Extra == nil {
			asset.Extra = m.interfaceMapPool.Get()
		}
		asset.Extra["source_stats"] = sourceStats
	}

	return mergedResult
}

// copyAsset 复制资产数据
func (m *ResultMerger) copyAsset(dest *model.UnifiedAsset, src model.UnifiedAsset) {
	dest.IP = src.IP
	dest.Port = src.Port
	dest.Protocol = src.Protocol
	dest.Host = src.Host
	dest.URL = src.URL
	dest.Title = src.Title
	dest.BodySnippet = src.BodySnippet
	dest.Server = src.Server
	dest.StatusCode = src.StatusCode
	dest.CountryCode = src.CountryCode
	dest.Region = src.Region
	dest.City = src.City
	dest.ASN = src.ASN
	dest.Org = src.Org
	dest.ISP = src.ISP
	dest.Source = src.Source

	// 复制Headers
	if src.Headers != nil {
		for k, v := range src.Headers {
			dest.Headers[k] = v
		}
	}

	// 复制Extra
	if src.Extra != nil {
		for k, v := range src.Extra {
			dest.Extra[k] = v
		}
	}
}

// MergeEngineResults 归并多个引擎的原始结果（兼容旧接口）
func (m *ResultMerger) MergeEngineResults(results []*model.EngineResult, adapters map[string]adapter.EngineAdapter) *model.MergeResult {
	// 使用对象池获取资产切片
	allAssetsSlice := m.slicePool.Get()
	defer m.slicePool.Put(allAssetsSlice)

	for _, result := range results {
		if result == nil || result.Error != "" {
			continue
		}

		// 使用适配器进行标准化
		if engineAdapter, exists := adapters[result.EngineName]; exists {
			assets, err := engineAdapter.Normalize(result)
			if err == nil && len(assets) > 0 {
				*allAssetsSlice = append(*allAssetsSlice, assets...)
			}
		}
	}

	return m.Merge(*allAssetsSlice)
}

// generateKey 生成资产去重键
func (m *ResultMerger) generateKey(asset model.UnifiedAsset) string {
	// 主要基于IP和端口去重
	if asset.IP != "" && asset.Port > 0 {
		return fmt.Sprintf("%s:%d", asset.IP, asset.Port)
	}

	// 如果没有IP和端口，基于URL去重
	if asset.URL != "" {
		return fmt.Sprintf("url:%s", asset.URL)
	}

	// 如果没有IP、端口和URL，基于Host去重
	if asset.Host != "" {
		return fmt.Sprintf("host:%s", asset.Host)
	}

	// 兜底方案，使用所有可用字段
	return fmt.Sprintf("%s:%d:%s:%s", asset.IP, asset.Port, asset.Host, asset.URL)
}

// mergeAssets 合并两个资产信息
func (m *ResultMerger) mergeAssets(existing *model.UnifiedAsset, new model.UnifiedAsset) {
	// 合并来源
	if !contains(existing.Source, new.Source) {
		existing.Source += "," + new.Source
	}

	// 基于引擎优先级合并字段
	existingPriority := m.getMinPriority(existing.Source)
	newPriority := m.getMinPriority(new.Source)

	// 优先保留高优先级引擎的字段值
	if newPriority < existingPriority {
		// 新引擎优先级更高，优先使用其字段
		m.updateAssetFields(existing, new, true)
	} else {
		// 新引擎优先级相同或更低，只补全缺失字段
		m.updateAssetFields(existing, new, false)
	}
}

// getMinPriority 获取多个来源中的最小优先级（最高优先级）
func (m *ResultMerger) getMinPriority(sources string) int {
	minPriority := 999
	sourceList := strings.Split(sources, ",")

	for _, source := range sourceList {
		source = strings.TrimSpace(source)
		if priority, exists := enginePriorities[source]; exists && priority < minPriority {
			minPriority = priority
		}
	}

	return minPriority
}

// updateAssetFields 更新资产字段
func (m *ResultMerger) updateAssetFields(existing *model.UnifiedAsset, new model.UnifiedAsset, override bool) {
	// 补全或覆盖字段
	if override || existing.Title == "" {
		existing.Title = new.Title
	}
	if override || existing.Server == "" {
		existing.Server = new.Server
	}
	if override || existing.BodySnippet == "" {
		existing.BodySnippet = new.BodySnippet
	}
	if override || existing.StatusCode == 0 {
		existing.StatusCode = new.StatusCode
	}
	if override || existing.Host == "" {
		existing.Host = new.Host
	}
	if override || existing.URL == "" {
		existing.URL = new.URL
	}
	if override || existing.Protocol == "" {
		existing.Protocol = new.Protocol
	}

	// 地理信息
	if override || existing.CountryCode == "" {
		existing.CountryCode = new.CountryCode
	}
	if override || existing.Region == "" {
		existing.Region = new.Region
	}
	if override || existing.City == "" {
		existing.City = new.City
	}
	if override || existing.ASN == "" {
		existing.ASN = new.ASN
	}
	if override || existing.Org == "" {
		existing.Org = new.Org
	}
	if override || existing.ISP == "" {
		existing.ISP = new.ISP
	}

	// 合并Headers
	if existing.Headers == nil {
		if new.Headers != nil {
			existing.Headers = m.mapPool.Get()
			// 复制Headers
			for k, v := range new.Headers {
				existing.Headers[k] = v
			}
		}
	} else if new.Headers != nil {
		for k, v := range new.Headers {
			if _, exists := existing.Headers[k]; !exists {
				existing.Headers[k] = v
			}
		}
	}

	// 合并Extra
	if existing.Extra == nil {
		if new.Extra != nil {
			existing.Extra = m.interfaceMapPool.Get()
			// 复制Extra
			for k, v := range new.Extra {
				existing.Extra[k] = v
			}
		}
	} else if new.Extra != nil {
		for k, v := range new.Extra {
			if _, exists := existing.Extra[k]; !exists {
				existing.Extra[k] = v
			}
		}
	}
}

// contains 检查字符串是否在逗号分隔的字符串中
func contains(list, item string) bool {
	parts := strings.Split(list, ",")
	for _, p := range parts {
		if strings.TrimSpace(p) == item {
			return true
		}
	}
	return false
}

// SortAssets 对资产进行排序（按IP和端口）
func (m *ResultMerger) SortAssets(assets []*model.UnifiedAsset) {
	sort.Slice(assets, func(i, j int) bool {
		if assets[i].IP != assets[j].IP {
			return assets[i].IP < assets[j].IP
		}
		return assets[i].Port < assets[j].Port
	})
}

// GetSortedAssets 获取排序后的资产列表
func (m *ResultMerger) GetSortedAssets(mergeResult *model.MergeResult) []*model.UnifiedAsset {
	assets := make([]*model.UnifiedAsset, 0, len(mergeResult.Assets))
	for _, asset := range mergeResult.Assets {
		assets = append(assets, asset)
	}

	m.SortAssets(assets)
	return assets
}

// GetSourceStats 获取各引擎的结果统计
func (m *ResultMerger) GetSourceStats(assets []model.UnifiedAsset) map[string]int {
	stats := make(map[string]int)
	for _, asset := range assets {
		stats[asset.Source]++
	}
	return stats
}
