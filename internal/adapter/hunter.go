package adapter

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/utils"
)

// HunterAdapter Hunter引擎适配器
type HunterAdapter struct {
	client  *resty.Client
	baseURL string
	apiKey  string
	qps     int
	timeout time.Duration
}

// NewHunterAdapter 创建Hunter适配器
func NewHunterAdapter(baseURL, apiKey string, qps int, timeout time.Duration) *HunterAdapter {
	client := resty.New().
		SetTimeout(timeout).
		SetHeader("User-Agent", "UniMap-ICP-Hunter/1.0")

	return &HunterAdapter{
		client:  client,
		baseURL: baseURL,
		apiKey:  apiKey,
		qps:     qps,
		timeout: timeout,
	}
}

// Name 返回引擎名称
func (h *HunterAdapter) Name() string {
	return "hunter"
}

// Translate 将UQL AST转换为Hunter查询语法
func (h *HunterAdapter) Translate(ast *model.UQLAST) (string, error) {
	if ast == nil || ast.Root == nil {
		return "", fmt.Errorf("invalid AST")
	}

	query := h.translateNode(ast.Root)
	return query, nil
}

func (h *HunterAdapter) translateNode(node *model.UQLNode) string {
	if node == nil {
		return ""
	}

	switch node.Type {
	case "condition":
		field := node.Value
		if len(node.Children) >= 2 {
			op := node.Children[0].Value
			val := node.Children[1].Value

			if op == "IN" {
				values := strings.Split(val, ",")
				conditions := []string{}
				for _, v := range values {
					conditions = append(conditions, h.buildCondition(field, "=", v))
				}
				return "(" + strings.Join(conditions, " OR ") + ")"
			}

			return h.buildCondition(field, op, val)
		}

	case "logical":
		if len(node.Children) >= 2 {
			left := h.translateNode(node.Children[0])
			right := h.translateNode(node.Children[1])

			// Hunter supports AND, OR, NOT (via operator usually, but logical nodes are AND/OR)
			// Ensure parentheses for precedence
			connector := "AND"
			if node.Value == "OR" {
				connector = "OR"
			} else if node.Value == "NOT" {
				// Unary NOT? usually binary logical in this tree structure but let's see UQL
				// If UQL "NOT" is a unary operator on a condition, it might be different structure
				// Assuming binary tree here
			}
			return fmt.Sprintf("(%s %s %s)", left, connector, right)
		}
	}

	return ""
}

func (h *HunterAdapter) buildCondition(field, op, value string) string {
	// Hunter使用类似ES的查询语法
	// 字段映射
	mapping := map[string]string{
		"body":        "web.body",
		"title":       "web.title",
		"header":      "header",
		"port":        "port",
		"protocol":    "protocol",
		"ip":          "ip",
		"country":     "ip.country",
		"region":      "ip.province",
		"city":        "ip.city",
		"asn":         "ip.asn",
		"org":         "ip.org",
		"isp":         "ip.isp",
		"domain":      "domain",
		"status_code": "web.status_code",
		"os":          "os",
		"app":         "app.name",
		"server":      "header_server",
		"host":        "domain",
		"url":         "url",
	}

	mappedField, ok := mapping[field]
	if !ok {
		mappedField = field
	}

	// Handle Operators
	switch op {
	case "=", "==":
		return fmt.Sprintf(`%s="%s"`, mappedField, value)
	case "!=", "<>":
		return fmt.Sprintf(`%s!="%s"`, mappedField, value)
	case "CONTAINS":
		// Hunter默认支持包含匹配
		return fmt.Sprintf(`%s="%s"`, mappedField, value)
	default:
		return fmt.Sprintf(`%s="%s"`, mappedField, value)
	}
}

// Search 执行Hunter搜索
func (h *HunterAdapter) Search(query string, page, pageSize int) (*model.EngineResult, error) {
	if h.apiKey == "" {
		return &model.EngineResult{
			EngineName: h.Name(),
			Error:      "Hunter API key not configured",
		}, nil
	}

	var engineResult *model.EngineResult

	retryConfig := utils.RetryConfig{
		MaxRetries:  3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    2 * time.Second,
		Exponential: true,
		Jitter:      true,
		ErrorHandler: func(err error, attempt int) {
			// 可以在这里添加错误日志记录
			// log.Printf("Hunter search error (attempt %d): %v", attempt, err)
		},
	}

	err := utils.Retry(retryConfig, func() error {
		// Hunter API endpoint: /openApi/search
		// 修正API URL格式
		baseURL := strings.TrimRight(h.baseURL, "/")
		url := fmt.Sprintf("%s/openApi/search", baseURL)

		// Base64 encode query
		encodedQuery := base64.URLEncoding.EncodeToString([]byte(query))

		// Hunter的API参数
		// 修正is_web参数，0表示全部，1表示web资产，2表示非web资产
		resp, err := h.client.R().
			SetQueryParams(map[string]string{
				"api-key":   h.apiKey,
				"search":    encodedQuery,
				"page":      fmt.Sprintf("%d", page),
				"page_size": fmt.Sprintf("%d", pageSize),
				"is_web":    "0", // 0表示全部资产
			}).
			Get(url)

		if err != nil {
			return fmt.Errorf("hunter request error: %v", err)
		}

		if resp.StatusCode() != 200 {
			return fmt.Errorf("hunter HTTP error %d: %s", resp.StatusCode(), resp.String())
		}

		// Hunter返回格式解析
		var result struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				Total int                      `json:"total"`
				Items []map[string]interface{} `json:"arr"`
			} `json:"data"`
		}

		if err := json.Unmarshal(resp.Body(), &result); err != nil {
			return fmt.Errorf("hunter response parse error: %v", err)
		}

		if result.Code != 200 {
			// 特殊处理常见错误
			switch result.Code {
			case 401:
				return fmt.Errorf("hunter authentication error: %s", result.Message)
			case 429:
				return fmt.Errorf("hunter rate limit exceeded: %s", result.Message)
			case 402:
				return fmt.Errorf("hunter payment required: %s", result.Message)
			default:
				return fmt.Errorf("hunter API error: %s", result.Message)
			}
		}

		// 转换为通用格式
		rawData := []interface{}{}
		for _, item := range result.Data.Items {
			rawData = append(rawData, item)
		}

		engineResult = &model.EngineResult{
			EngineName: h.Name(),
			RawData:    rawData,
			Total:      result.Data.Total,
			Page:       page,
			HasMore:    (page * pageSize) < result.Data.Total,
		}

		return nil
	})

	if err != nil {
		return &model.EngineResult{
			EngineName: h.Name(),
			Error:      fmt.Sprintf("search error: %v", err),
		}, nil
	}

	return engineResult, nil
}

// Normalize 标准化Hunter结果
func (h *HunterAdapter) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
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
			Source: h.Name(),
			Extra:  data,
		}

		// Helper to safely get string from map
		getString := func(k string) string {
			if v, ok := data[k].(string); ok {
				return v
			}
			return ""
		}

		// Helper to safely get int from map
		getInt := func(k string) int {
			if v, ok := data[k].(float64); ok {
				return int(v)
			}
			if v, ok := data[k].(int); ok {
				return v
			}
			return 0
		}

		// Attempt to read from flat structure (New API)
		asset.IP = getString("ip")
		asset.Port = getInt("port")
		asset.Protocol = getString("protocol")
		asset.Host = getString("domain")
		asset.Title = getString("web_title")
		asset.Server = getString("header_server")
		asset.StatusCode = getInt("status_code")
		asset.CountryCode = getString("country")
		asset.Region = getString("province")
		asset.City = getString("city")
		asset.ISP = getString("isp")
		asset.Org = getString("as_org")
		asset.URL = getString("url")

		// If essential fields are missing, try legacy nested structure
		if asset.IP == "" {
			if web, ok := data["web"].(map[string]interface{}); ok {
				if v, ok := web["ip"].(string); ok {
					asset.IP = v
				}
				if v, ok := web["port"].(float64); ok {
					asset.Port = int(v)
				}
				if v, ok := web["protocol"].(string); ok {
					asset.Protocol = v
				}
				if v, ok := web["domain"].(string); ok {
					asset.Host = v
				}
				if v, ok := web["title"].(string); ok {
					asset.Title = v
				}
				if v, ok := web["server"].(string); ok {
					asset.Server = v
				}
				if v, ok := web["status_code"].(float64); ok {
					asset.StatusCode = int(v)
				}
			}
			if loc, ok := data["location"].(map[string]interface{}); ok {
				if v, ok := loc["country_cn"].(string); ok {
					asset.CountryCode = v
				}
				if v, ok := loc["province_cn"].(string); ok {
					asset.Region = v
				}
				if v, ok := loc["city_cn"].(string); ok {
					asset.City = v
				}
			}
			// Fallback: direct IP/Port in root if not found in web
			if asset.IP == "" {
				asset.IP = getString("ip")
			}
			if asset.Port == 0 {
				asset.Port = getInt("port")
			}
		}

		// Ensure URL
		if asset.URL == "" && asset.IP != "" && asset.Port > 0 {
			proto := asset.Protocol
			if proto == "" {
				if asset.Port == 443 {
					proto = "https"
				} else {
					proto = "http"
				}
			}
			host := asset.IP
			if asset.Host != "" {
				host = asset.Host
			}
			// 使用 url.URL 结构体安全构建 URL
			urlScheme := "http"
			if strings.HasPrefix(proto, "https") || asset.Port == 443 {
				urlScheme = "https"
			}
			u := &url.URL{
				Scheme: urlScheme,
				Host:   fmt.Sprintf("%s:%d", host, asset.Port),
			}
			asset.URL = u.String()
		}

		if asset.IP != "" {
			assets = append(assets, *asset)
		}
	}

	return assets, nil
}

// GetQuota 获取Hunter配额信息
func (h *HunterAdapter) GetQuota() (*model.QuotaInfo, error) {
	if h.apiKey == "" {
		return nil, fmt.Errorf("Hunter API key not configured")
	}

	// Hunter API endpoint for quota info
	baseURL := strings.TrimRight(h.baseURL, "/")
	// NOTE: Hunter uses camelCase path: /openApi/userInfo
	url := fmt.Sprintf("%s/openApi/userInfo", baseURL)

	resp, err := h.client.R().
		SetQueryParam("api-key", h.apiKey).
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("request error: %v", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	// 打印响应体，方便调试
	logger.Debugf("Hunter quota response: %s", resp.String())

	// Hunter quota response structure
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			RestFreePoint   int `json:"rest_free_point"`
			DayFreePoint    int `json:"day_free_point"`
			RestEquityPoint int `json:"rest_equity_point"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("parse error: %v", err)
	}

	if result.Code != 200 {
		return nil, fmt.Errorf("%s", result.Message)
	}

	// 计算配额信息
	// Hunter的响应中，RestFreePoint是剩余的免费点数，DayFreePoint是每日免费点数
	total := result.Data.DayFreePoint
	remain := result.Data.RestFreePoint

	// 边界检查：确保数值合理
	if remain < 0 {
		remain = 0
	}
	if total < 0 {
		total = 0
	}

	// 计算已用配额，确保不会出现负数
	used := total - remain
	if used < 0 {
		used = 0
	}

	// 如果剩余大于总数，调整总数
	if remain > total {
		total = remain
		used = 0
	}

	// 打印解析后的配额信息
	logger.Infof("Hunter quota: total=%d, used=%d, remain=%d", total, used, remain)

	return &model.QuotaInfo{
		Remaining: remain,
		Total:     total,
		Used:      used,
		Unit:      "queries",
		Expiry:    "", // Hunter API doesn't return expiry info
	}, nil
}
