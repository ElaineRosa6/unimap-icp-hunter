package unimap

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/unimap-icp-hunter/project/internal/model"
)

// ResultMerger 结果归并与去重
type ResultMerger struct {
	mu sync.RWMutex
}

// NewResultMerger 创建归并器
func NewResultMerger() *ResultMerger {
	return &ResultMerger{}
}

// Merge 归并多个引擎的结果
func (m *ResultMerger) Merge(results []*model.EngineResult) *model.MergeResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	assetMap := make(map[string]*model.UnifiedAsset)
	duplicates := 0

	for _, result := range results {
		if result == nil || result.Error != "" {
			continue
		}

		assets, err := m.normalizeEngineResult(result)
		if err != nil {
			continue
		}

		for _, asset := range assets {
			key := fmt.Sprintf("%s:%d", asset.IP, asset.Port)

			if existing, exists := assetMap[key]; exists {
				// 合并已有记录
				m.mergeAssets(existing, asset)
				duplicates++
			} else {
				assetMap[key] = &asset
			}
		}
	}

	return &model.MergeResult{
		Assets:     assetMap,
		Total:      len(assetMap),
		Duplicates: duplicates,
	}
}

// normalizeEngineResult 标准化引擎结果
func (m *ResultMerger) normalizeEngineResult(result *model.EngineResult) ([]model.UnifiedAsset, error) {
	assets := []model.UnifiedAsset{}

	// 根据引擎名称调用对应的标准化逻辑
	switch result.EngineName {
	case "fofa":
		return m.normalizeFofa(result)
	case "hunter":
		return m.normalizeHunter(result)
	case "zoomeye":
		return m.normalizeZoomEye(result)
	case "quake":
		return m.normalizeQuake(result)
	default:
		return assets, fmt.Errorf("unknown engine: %s", result.EngineName)
	}
}

// normalizeFofa 标准化FOFA结果
func (m *ResultMerger) normalizeFofa(result *model.EngineResult) ([]model.UnifiedAsset, error) {
	assets := []model.UnifiedAsset{}

	for _, item := range result.RawData {
		data, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		asset := model.UnifiedAsset{
			Source: "fofa",
		}

		// 提取IP
		if ip, ok := data["ip"].(string); ok {
			asset.IP = ip
		}

		// 提取Port
		if port, ok := data["port"].(float64); ok {
			asset.Port = int(port)
		} else if port, ok := data["port"].(int); ok {
			asset.Port = port
		}

		// 提取协议
		if proto, ok := data["protocol"].(string); ok {
			asset.Protocol = proto
		}

		// 提取域名
		if domain, ok := data["domain"].(string); ok {
			asset.Host = domain
		}

		// 构建URL
		if asset.Protocol == "https" {
			asset.URL = fmt.Sprintf("https://%s:%d", asset.IP, asset.Port)
			if asset.Host != "" {
				asset.URL = fmt.Sprintf("https://%s:%d", asset.Host, asset.Port)
			}
		} else {
			asset.URL = fmt.Sprintf("http://%s:%d", asset.IP, asset.Port)
			if asset.Host != "" {
				asset.URL = fmt.Sprintf("http://%s:%d", asset.Host, asset.Port)
			}
		}

		// Web信息
		if title, ok := data["title"].(string); ok {
			asset.Title = title
		}
		if server, ok := data["server"].(string); ok {
			asset.Server = server
		}
		if body, ok := data["body"].(string); ok {
			if len(body) > 200 {
				asset.BodySnippet = body[:200]
			} else {
				asset.BodySnippet = body
			}
		}
		if status, ok := data["status_code"].(float64); ok {
			asset.StatusCode = int(status)
		}

		// 地理信息
		if country, ok := data["country"].(string); ok {
			asset.CountryCode = country
		}
		if region, ok := data["region"].(string); ok {
			asset.Region = region
		}
		if city, ok := data["city"].(string); ok {
			asset.City = city
		}
		if asn, ok := data["asn"].(string); ok {
			asset.ASN = asn
		}
		if org, ok := data["org"].(string); ok {
			asset.Org = org
		}
		if isp, ok := data["isp"].(string); ok {
			asset.ISP = isp
		}

		// Extra
		asset.Extra = data

		if asset.IP != "" && asset.Port > 0 {
			assets = append(assets, asset)
		}
	}

	return assets, nil
}

// normalizeHunter 标准化Hunter结果
func (m *ResultMerger) normalizeHunter(result *model.EngineResult) ([]model.UnifiedAsset, error) {
	assets := []model.UnifiedAsset{}

	for _, item := range result.RawData {
		data, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		asset := model.UnifiedAsset{
			Source: "hunter",
		}

		// Hunter的字段结构可能不同，需要根据实际API调整
		// 这里假设web字段包含Web信息
		if web, ok := data["web"].(map[string]interface{}); ok {
			if ip, ok := web["ip"].(string); ok {
				asset.IP = ip
			}
			if port, ok := web["port"].(float64); ok {
				asset.Port = int(port)
			}
			if proto, ok := web["protocol"].(string); ok {
				asset.Protocol = proto
			}
			if domain, ok := web["domain"].(string); ok {
				asset.Host = domain
			}
			if title, ok := web["title"].(string); ok {
				asset.Title = title
			}
			if server, ok := web["server"].(string); ok {
				asset.Server = server
			}
			if status, ok := web["status_code"].(float64); ok {
				asset.StatusCode = int(status)
			}
		}

		// 位置信息
		if location, ok := data["location"].(map[string]interface{}); ok {
			if country, ok := location["country_cn"].(string); ok {
				asset.CountryCode = country
			}
			if province, ok := location["province_cn"].(string); ok {
				asset.Region = province
			}
			if city, ok := location["city_cn"].(string); ok {
				asset.City = city
			}
		}

		// ASN等
		if asn, ok := data["asn"].(string); ok {
			asset.ASN = asn
		}
		if org, ok := data["org"].(string); ok {
			asset.Org = org
		}
		if isp, ok := data["isp"].(string); ok {
			asset.ISP = isp
		}

		// 构建URL
		if asset.IP != "" && asset.Port > 0 {
			if asset.Protocol == "" {
				if asset.Port == 443 {
					asset.Protocol = "https"
				} else {
					asset.Protocol = "http"
				}
			}

			if asset.Protocol == "https" {
				asset.URL = fmt.Sprintf("https://%s:%d", asset.IP, asset.Port)
				if asset.Host != "" {
					asset.URL = fmt.Sprintf("https://%s:%d", asset.Host, asset.Port)
				}
			} else {
				asset.URL = fmt.Sprintf("http://%s:%d", asset.IP, asset.Port)
				if asset.Host != "" {
					asset.URL = fmt.Sprintf("http://%s:%d", asset.Host, asset.Port)
				}
			}

			asset.Extra = data
			assets = append(assets, asset)
		}
	}

	return assets, nil
}

// normalizeZoomEye 标准化ZoomEye结果
func (m *ResultMerger) normalizeZoomEye(result *model.EngineResult) ([]model.UnifiedAsset, error) {
	assets := []model.UnifiedAsset{}

	for _, item := range result.RawData {
		data, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		asset := model.UnifiedAsset{
			Source: "zoomeye",
		}

		// ZoomEye的字段结构
		if ip, ok := data["ip"].(string); ok {
			asset.IP = ip
		}

		if portinfo, ok := data["portinfo"].(map[string]interface{}); ok {
			if port, ok := portinfo["port"].(float64); ok {
				asset.Port = int(port)
			}
			if service, ok := portinfo["service"].(string); ok {
				asset.Protocol = service
			}
			if hostname, ok := portinfo["hostname"].(string); ok {
				asset.Host = hostname
			}
		}

		if app, ok := data["app"].(string); ok {
			asset.Server = app
		}

		if asset.IP != "" && asset.Port > 0 {
			if asset.Protocol == "" {
				if asset.Port == 443 {
					asset.Protocol = "https"
				} else {
					asset.Protocol = "http"
				}
			}

			if asset.Protocol == "https" {
				asset.URL = fmt.Sprintf("https://%s:%d", asset.IP, asset.Port)
				if asset.Host != "" {
					asset.URL = fmt.Sprintf("https://%s:%d", asset.Host, asset.Port)
				}
			} else {
				asset.URL = fmt.Sprintf("http://%s:%d", asset.IP, asset.Port)
				if asset.Host != "" {
					asset.URL = fmt.Sprintf("http://%s:%d", asset.Host, asset.Port)
				}
			}

			asset.Extra = data
			assets = append(assets, asset)
		}
	}

	return assets, nil
}

// normalizeQuake 标准化Quake结果
func (m *ResultMerger) normalizeQuake(result *model.EngineResult) ([]model.UnifiedAsset, error) {
	assets := []model.UnifiedAsset{}

	for _, item := range result.RawData {
		data, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		asset := model.UnifiedAsset{
			Source: "quake",
		}

		if ip, ok := data["ip"].(string); ok {
			asset.IP = ip
		}
		if port, ok := data["port"].(float64); ok {
			asset.Port = int(port)
		}
		if service, ok := data["service"].(map[string]interface{}); ok {
			if name, ok := service["name"].(string); ok {
				asset.Protocol = name
			}
			if http, ok := service["http"].(map[string]interface{}); ok {
				if title, ok := http["title"].(string); ok {
					asset.Title = title
				}
				if server, ok := http["server"].(string); ok {
					asset.Server = server
				}
			}
		}

		if asset.IP != "" && asset.Port > 0 {
			if asset.Protocol == "" {
				if asset.Port == 443 {
					asset.Protocol = "https"
				} else {
					asset.Protocol = "http"
				}
			}

			if asset.Protocol == "https" {
				asset.URL = fmt.Sprintf("https://%s:%d", asset.IP, asset.Port)
			} else {
				asset.URL = fmt.Sprintf("http://%s:%d", asset.IP, asset.Port)
			}

			asset.Extra = data
			assets = append(assets, asset)
		}
	}

	return assets, nil
}

// mergeAssets 合并两个资产信息
func (m *ResultMerger) mergeAssets(existing *model.UnifiedAsset, new model.UnifiedAsset) {
	// 字段优先级策略：保留非空值，如果都有则保留更新时间较新的（这里简化为保留已有）

	// 合并来源
	if !contains(existing.Source, new.Source) {
		existing.Source += "," + new.Source
	}

	// 补全缺失字段
	if existing.Title == "" && new.Title != "" {
		existing.Title = new.Title
	}
	if existing.Server == "" && new.Server != "" {
		existing.Server = new.Server
	}
	if existing.BodySnippet == "" && new.BodySnippet != "" {
		existing.BodySnippet = new.BodySnippet
	}
	if existing.StatusCode == 0 && new.StatusCode != 0 {
		existing.StatusCode = new.StatusCode
	}
	if existing.Host == "" && new.Host != "" {
		existing.Host = new.Host
	}
	if existing.URL == "" && new.URL != "" {
		existing.URL = new.URL
	}
	if existing.Protocol == "" && new.Protocol != "" {
		existing.Protocol = new.Protocol
	}

	// 地理信息
	if existing.CountryCode == "" && new.CountryCode != "" {
		existing.CountryCode = new.CountryCode
	}
	if existing.Region == "" && new.Region != "" {
		existing.Region = new.Region
	}
	if existing.City == "" && new.City != "" {
		existing.City = new.City
	}
	if existing.ASN == "" && new.ASN != "" {
		existing.ASN = new.ASN
	}
	if existing.Org == "" && new.Org != "" {
		existing.Org = new.Org
	}
	if existing.ISP == "" && new.ISP != "" {
		existing.ISP = new.ISP
	}

	// 合并Headers
	if existing.Headers == nil {
		existing.Headers = new.Headers
	} else if new.Headers != nil {
		for k, v := range new.Headers {
			if _, exists := existing.Headers[k]; !exists {
				existing.Headers[k] = v
			}
		}
	}

	// 合并Extra
	if existing.Extra == nil {
		existing.Extra = new.Extra
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
