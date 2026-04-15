package threshold

import (
	"fmt"
	"sync"
	"time"
)

// ThresholdManager 动态阈值管理器
type ThresholdManager struct {
	thresholds       map[string]*ThresholdConfig
	siteSensitivity  map[string]float64
	ruleWeights      map[string]float64
	historicalData   map[string]*HistoricalData
	mu               sync.RWMutex
}

// ThresholdConfig 阈值配置
type ThresholdConfig struct {
	BaseThreshold     float64
	Sensitivity       float64
	AdjustmentFactor  float64
	LastAdjusted      time.Time
}

// HistoricalData 历史数据
type HistoricalData struct {
	TotalScans        int
	FalsePositives    int
	TruePositives     int
	LastScanTime      time.Time
}

// NewThresholdManager 创建动态阈值管理器
func NewThresholdManager() *ThresholdManager {
	return &ThresholdManager{
		thresholds:      make(map[string]*ThresholdConfig),
		siteSensitivity: make(map[string]float64),
		ruleWeights:     make(map[string]float64),
		historicalData:  make(map[string]*HistoricalData),
	}
}

// GetThreshold 获取网站的动态阈值
func (tm *ThresholdManager) GetThreshold(siteURL string) float64 {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	// 如果网站有自定义阈值，使用自定义阈值
	if config, exists := tm.thresholds[siteURL]; exists {
		return config.BaseThreshold * (1 - config.AdjustmentFactor)
	}
	
	// 默认阈值
	return 0.5
}

// SetSiteSensitivity 设置网站灵敏度
func (tm *ThresholdManager) SetSiteSensitivity(siteURL string, sensitivity float64) error {
	if sensitivity< 0.1 || sensitivity >10.0 {
		return fmt.Errorf("sensitivity must be between 0.1 and 10.0")
	}
	
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	tm.siteSensitivity[siteURL] = sensitivity
	
	return nil
}

// GetSiteSensitivity 获取网站灵敏度
func (tm *ThresholdManager) GetSiteSensitivity(siteURL string) float64 {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	if sensitivity, exists := tm.siteSensitivity[siteURL]; exists {
		return sensitivity
	}
	
	// 默认灵敏度
	return 1.0
}

// SetRuleWeight 设置规则权重
func (tm *ThresholdManager) SetRuleWeight(ruleID string, weight float64) error {
	if weight < 0.0 || weight > 10.0 {
		return fmt.Errorf("weight must be between 0.0 and 10.0")
	}
	
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	tm.ruleWeights[ruleID] = weight
	
	return nil
}

// GetRuleWeight 获取规则权重
func (tm *ThresholdManager) GetRuleWeight(ruleID string) float64 {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	if weight, exists := tm.ruleWeights[ruleID]; exists {
		return weight
	}
	
	// 默认权重
	return 1.0
}

// RecordScanResult 记录扫描结果
func (tm *ThresholdManager) RecordScanResult(siteURL string, isMalicious bool, isFalsePositive bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	data, exists := tm.historicalData[siteURL]
	if !exists {
		data = &HistoricalData{}
		tm.historicalData[siteURL] = data
	}
	
	data.TotalScans++
	data.LastScanTime = time.Now()
	
	if isMalicious && !isFalsePositive {
		data.TruePositives++
	} else if !isMalicious && isFalsePositive {
		data.FalsePositives++
	}
	
	// 调整阈值
	tm.adjustThreshold(siteURL)
}

// adjustThreshold 调整阈值
func (tm *ThresholdManager) adjustThreshold(siteURL string) {
	data := tm.historicalData[siteURL]
	
	// 如果扫描次数太少，不进行调整
	if data.TotalScans< 10 {
		return
	}
	
	// 计算误报率
	falsePositiveRate := float64(data.FalsePositives) / float64(data.TotalScans)
	
	// 获取当前阈值配置
	config, exists := tm.thresholds[siteURL]
	if !exists {
		config = &ThresholdConfig{
			BaseThreshold:    0.5,
			Sensitivity:      1.0,
			AdjustmentFactor: 0.0,
			LastAdjusted:     time.Now(),
		}
		tm.thresholds[siteURL] = config
	}
	
	// 根据误报率调整阈值
	if falsePositiveRate > 0.2 {
		// 误报率太高，提高阈值
		config.AdjustmentFactor = min(config.AdjustmentFactor+0.05, 0.5)
	} else if falsePositiveRate< 0.05 && data.TruePositives >0 {
		// 误报率太低，降低阈值以提高检测率
		config.AdjustmentFactor = max(config.AdjustmentFactor-0.02, 0.0)
	}
	
	config.LastAdjusted = time.Now()
}

// GetHistoricalData 获取历史数据
func (tm *ThresholdManager) GetHistoricalData(siteURL string) (*HistoricalData, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	data, exists := tm.historicalData[siteURL]
	return data, exists
}

// ResetSiteData 重置网站数据
func (tm *ThresholdManager) ResetSiteData(siteURL string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	delete(tm.thresholds, siteURL)
	delete(tm.historicalData, siteURL)
}

// GetThresholdStats 获取阈值统计信息
func (tm *ThresholdManager) GetThresholdStats() map[string]interface{} {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	stats := make(map[string]interface{})
	
	totalSites := len(tm.historicalData)
	totalScans := 0
	totalFalsePositives := 0
	totalTruePositives := 0
	
	for _, data := range tm.historicalData {
		totalScans += data.TotalScans
		totalFalsePositives += data.FalsePositives
		totalTruePositives += data.TruePositives
	}
	
	stats["total_sites"] = totalSites
	stats["total_scans"] = totalScans
	stats["total_false_positives"] = totalFalsePositives
	stats["total_true_positives"] = totalTruePositives
	
	if totalScans > 0 {
		stats["overall_false_positive_rate"] = float64(totalFalsePositives) / float64(totalScans)
	}
	
	return stats
}

// CalculateDynamicScore 计算动态评分
func (tm *ThresholdManager) CalculateDynamicScore(siteURL string, baseScore float64, ruleMatches []RuleMatch) float64 {
	sensitivity := tm.GetSiteSensitivity(siteURL)
	
	// 应用规则权重
	weightedScore := 0.0
	for _, match := range ruleMatches {
		ruleWeight := tm.GetRuleWeight(match.RuleID)
		weightedScore += match.Score * ruleWeight
	}
	
	// 如果没有规则匹配，使用基础评分
	if len(ruleMatches) == 0 {
		weightedScore = baseScore
	}
	
	// 应用灵敏度调整
	finalScore := weightedScore * sensitivity
	
	return finalScore
}

// RuleMatch 规则匹配信息
type RuleMatch struct {
	RuleID string
	Score  float64
}

// GetAdjustedThreshold 获取调整后的阈值
func (tm *ThresholdManager) GetAdjustedThreshold(siteURL string) float64 {
	baseThreshold := tm.GetThreshold(siteURL)
	sensitivity := tm.GetSiteSensitivity(siteURL)
	
	// 灵敏度影响阈值
	adjustedThreshold := baseThreshold / sensitivity
	
	// 限制阈值范围
	adjustedThreshold = max(0.1, min(adjustedThreshold, 0.9))
	
	return adjustedThreshold
}

// IsAboveThreshold 判断是否超过阈值
func (tm *ThresholdManager) IsAboveThreshold(siteURL string, score float64) bool {
	threshold := tm.GetAdjustedThreshold(siteURL)
	return score >= threshold
}

// UpdateRuleWeights 批量更新规则权重
func (tm *ThresholdManager) UpdateRuleWeights(weights map[string]float64) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	for ruleID, weight := range weights {
		if weight < 0.0 || weight > 10.0 {
			return fmt.Errorf("weight for rule %s must be between 0.0 and 10.0", ruleID)
		}
		tm.ruleWeights[ruleID] = weight
	}
	
	return nil
}

// GetAllRuleWeights 获取所有规则权重
func (tm *ThresholdManager) GetAllRuleWeights() map[string]float64 {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	// 返回副本以避免并发修改
	weights := make(map[string]float64)
	for ruleID, weight := range tm.ruleWeights {
		weights[ruleID] = weight
	}
	
	return weights
}

// CleanupOldData 清理旧数据
func (tm *ThresholdManager) CleanupOldData(maxAge time.Duration) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	now := time.Now()
	for siteURL, data := range tm.historicalData {
		if now.Sub(data.LastScanTime) > maxAge {
			delete(tm.thresholds, siteURL)
			delete(tm.historicalData, siteURL)
			delete(tm.siteSensitivity, siteURL)
		}
	}
}

// Helper functions
func min(a, b float64) float64 {
	if a< b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a >b {
		return a
	}
	return b
}