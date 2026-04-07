package adapter

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/utils"
)

// ShodanAdapter Shodan引擎适配器
type ShodanAdapter struct {
	client  *resty.Client
	baseURL string
	apiKey  string
	qps     int
	timeout time.Duration
}

// NewShodanAdapter 创建Shodan适配器
func NewShodanAdapter(baseURL, apiKey string, qps int, timeout time.Duration) *ShodanAdapter {
	client := resty.New().
		SetTimeout(timeout).
		SetHeader("User-Agent", "UniMap-ICP-Hunter/1.0")

	return &ShodanAdapter{
		client:  client,
		baseURL: baseURL,
		apiKey:  apiKey,
		qps:     qps,
		timeout: timeout,
	}
}

// Name 返回引擎名称
func (s *ShodanAdapter) Name() string {
	return "shodan"
}

// Translate 将UQL AST转换为Shodan查询语法
func (s *ShodanAdapter) Translate(ast *model.UQLAST) (string, error) {
	if ast == nil || ast.Root == nil {
		return "", fmt.Errorf("invalid AST")
	}

	// Shodan使用类似ES的查询语法
	// 简单实现：遍历AST构建查询字符串
	query := s.translateNode(ast.Root)
	return query, nil
}

func (s *ShodanAdapter) translateNode(node *model.UQLNode) string {
	if node == nil {
		return ""
	}

	switch node.Type {
	case "condition":
		// field= value 或 field IN [values]
		field := node.Value
		if len(node.Children) >= 2 {
			op := node.Children[0].Value
			val := node.Children[1].Value

			if op == "IN" {
				// Shodan不支持IN语法，需要转换为多个OR
				values := strings.Split(val, ",")
				conditions := []string{}
				for _, v := range values {
					conditions = append(conditions, fmt.Sprintf("%s:%s", s.mapField(field), v))
				}
				return "(" + strings.Join(conditions, " OR ") + ")"
			}

			// 处理特殊字段映射
			field = s.mapField(field)

			if op == "=" || op == "==" || strings.ToUpper(op) == "CONTAINS" {
				return fmt.Sprintf("%s:%s", field, val)
			}
			if op == "!=" || op == "<>" {
				return fmt.Sprintf("-%s:%s", field, val)
			}
			// Fallback
			return fmt.Sprintf("%s:%s", field, val)
		}

	case "logical":
		if len(node.Children) >= 2 {
			left := s.translateNode(node.Children[0])
			right := s.translateNode(node.Children[1])
			if node.Value == "OR" {
				return fmt.Sprintf("(%s OR %s)", left, right)
			}
			return fmt.Sprintf("(%s AND %s)", left, right)
		}
	}

	return ""
}

// mapField 映射统一字段到Shodan字段
func (s *ShodanAdapter) mapField(field string) string {
	mapping := map[string]string{
		"body":        "html",
		"title":       "title",
		"header":      "http",
		"port":        "port",
		"protocol":    "transport",
		"ip":          "ip",
		"country":     "country",
		"region":      "region",
		"city":        "city",
		"asn":         "asn",
		"org":         "org",
		"isp":         "isp",
		"domain":      "domain",
		"host":        "hostnames",
		"server":      "server",
		"status_code": "status",
		"os":          "os",
		"app":         "product",
		"cert":        "ssl",
		"url":         "hostnames",
	}

	if mapped, ok := mapping[field]; ok {
		return mapped
	}
	return field
}

// Search 执行Shodan搜索
func (s *ShodanAdapter) Search(query string, page, pageSize int) (*model.EngineResult, error) {
	if s.apiKey == "" {
		return &model.EngineResult{
			EngineName: s.Name(),
			Error:      "Shodan API key not configured",
		}, nil
	}

	var engineResult *model.EngineResult

	retryConfig := utils.RetryConfig{
		MaxRetries:  3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    2 * time.Second,
		Exponential: true,
		Jitter:      true,
		RetryableFunc: func(err error) bool {
			// 网络错误可重试
			return true
		},
	}

	err := utils.Retry(retryConfig, func() error {
		// Shodan API endpoint for search
		url := fmt.Sprintf("%s/shodan/host/search", s.baseURL)

		resp, err := s.client.R().
			SetQueryParams(map[string]string{
				"key":   s.apiKey,
				"query": query,
				"page":  fmt.Sprintf("%d", page),
				"limit": fmt.Sprintf("%d", pageSize),
			}).
			Get(url)

		if err != nil {
			return err
		}

		if resp.StatusCode() != 200 {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode(), resp.String())
		}

		var result struct {
			Matches []struct {
				IP        string            `json:"ip_str"`
				Port      int               `json:"port"`
				Transport string            `json:"transport"`
				Hostnames []string          `json:"hostnames"`
				Domain    string            `json:"domain"`
				Title     string            `json:"title"`
				Server    string            `json:"server"`
				HTTP      map[string]string `json:"http"`
				Status    int               `json:"status"`
				Country   string            `json:"country_code"`
				Region    string            `json:"region_code"`
				City      string            `json:"city"`
				ASN       string            `json:"asn"`
				Org       string            `json:"org"`
				ISP       string            `json:"isp"`
				OS        string            `json:"os"`
				Product   string            `json:"product"`
				Version   string            `json:"version"`
				Data      string            `json:"data"`
			} `json:"matches"`
			Total int    `json:"total"`
			Error string `json:"error,omitempty"`
		}

		if err := json.Unmarshal(resp.Body(), &result); err != nil {
			return err
		}

		if result.Error != "" {
			return fmt.Errorf("%s", result.Error)
		}

		// 转换为通用格式
		rawData := []interface{}{}
		for _, match := range result.Matches {
			data := map[string]interface{}{
				"ip":          match.IP,
				"port":        match.Port,
				"protocol":    match.Transport,
				"domain":      match.Domain,
				"hostnames":   match.Hostnames,
				"title":       match.Title,
				"server":      match.Server,
				"http":        match.HTTP,
				"status_code": match.Status,
				"country":     match.Country,
				"region":      match.Region,
				"city":        match.City,
				"asn":         match.ASN,
				"org":         match.Org,
				"isp":         match.ISP,
				"os":          match.OS,
				"product":     match.Product,
				"version":     match.Version,
				"data":        match.Data,
			}
			rawData = append(rawData, data)
		}

		engineResult = &model.EngineResult{
			EngineName: s.Name(),
			RawData:    rawData,
			Total:      result.Total,
			Page:       page,
			HasMore:    (page * pageSize) < result.Total,
		}

		return nil
	})

	if err != nil {
		return &model.EngineResult{
			EngineName: s.Name(),
			Error:      fmt.Sprintf("search error: %v", err),
		}, nil
	}

	return engineResult, nil
}

// Normalize 标准化Shodan结果
func (s *ShodanAdapter) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
	assets := make([]model.UnifiedAsset, 0, len(raw.RawData))

	if raw == nil || len(raw.RawData) == 0 {
		return assets, nil
	}

	for _, item := range raw.RawData {
		data, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// 创建新的资产对象
		asset := &model.UnifiedAsset{
			Source: s.Name(),
		}

		// 提取字段
		if ip, ok := data["ip"].(string); ok {
			asset.IP = ip
		}
		if port, ok := data["port"].(float64); ok {
			asset.Port = int(port)
		} else if port, ok := data["port"].(int); ok {
			asset.Port = port
		}
		if proto, ok := data["protocol"].(string); ok {
			asset.Protocol = proto
		}
		if domain, ok := data["domain"].(string); ok {
			asset.Host = domain
		} else if hostnames, ok := data["hostnames"].([]interface{}); ok && len(hostnames) > 0 {
			if hostname, ok := hostnames[0].(string); ok {
				asset.Host = hostname
			}
		}
		if title, ok := data["title"].(string); ok {
			asset.Title = title
		}
		if server, ok := data["server"].(string); ok {
			asset.Server = server
		}
		if body, ok := data["data"].(string); ok {
			if len(body) > 200 {
				asset.BodySnippet = body[:200]
			} else {
				asset.BodySnippet = body
			}
		}
		if status, ok := data["status_code"].(float64); ok {
			asset.StatusCode = int(status)
		} else if status, ok := data["status_code"].(int); ok {
			asset.StatusCode = status
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

		// 构建URL
		if asset.IP != "" && asset.Port > 0 {
			if asset.Protocol == "" {
				if asset.Port == 443 {
					asset.Protocol = "https"
				} else {
					asset.Protocol = "http"
				}
			}

			// 使用 url.URL 结构体安全构建 URL
			u := &url.URL{
				Scheme: asset.Protocol,
			}
			if asset.Host != "" {
				u.Host = fmt.Sprintf("%s:%d", asset.Host, asset.Port)
			} else {
				u.Host = fmt.Sprintf("%s:%d", asset.IP, asset.Port)
			}
			asset.URL = u.String()

			asset.Extra = data
			assets = append(assets, *asset)
		}
	}

	return assets, nil
}

// GetQuota 获取Shodan配额信息
func (s *ShodanAdapter) GetQuota() (*model.QuotaInfo, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("Shodan API key not configured")
	}

	// Shodan API endpoint for API info (contains quota)
	url := fmt.Sprintf("%s/api-info", s.baseURL)

	resp, err := s.client.R().
		SetQueryParams(map[string]string{
			"key": s.apiKey,
		}).
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("request error: %v", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	// Shodan API info response structure
	var result struct {
		Plan         string `json:"plan"`
		Credits      int    `json:"query_credits"`
		ScanCredits  int    `json:"scan_credits"`
		MonitoredIPs int    `json:"monitored_ips"`
		Unlocked     bool   `json:"unlocked"`
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("parse error: %v", err)
	}

	return &model.QuotaInfo{
		Remaining: result.Credits,
		Total:     result.Credits, // Shodan API doesn't return total credits
		Used:      0,              // Shodan API doesn't return used credits
		Unit:      "credits",
		Expiry:    "", // Shodan API doesn't return expiry info
	}, nil
}

// IsWebOnly 检查是否为 Web-only 模式
func (s *ShodanAdapter) IsWebOnly() bool {
	return false
}

// ShodanAdapterWebOnly Shodan Web-only模式适配器
type ShodanAdapterWebOnly struct {
	*WebOnlyAdapterBase
}

// NewShodanAdapterWebOnly 创建Shodan Web-only适配器
func NewShodanAdapterWebOnly() *ShodanAdapterWebOnly {
	baseAdapter := NewShodanAdapter("", "", 3, 30*time.Second)
	return &ShodanAdapterWebOnly{
		WebOnlyAdapterBase: NewWebOnlyAdapterBase(baseAdapter, "shodan"),
	}
}
