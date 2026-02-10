package adapter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/utils"
)

// QuakeAdapter Quake引擎适配器
type QuakeAdapter struct {
	client  *resty.Client
	baseURL string
	apiKey  string
	qps     int
	timeout time.Duration
}

func quakeIsSuccessCode(code interface{}) bool {
	switch v := code.(type) {
	case nil:
		return true // some responses omit code on success
	case int:
		return v == 0 || v == 200
	case int64:
		return v == 0 || v == 200
	case float64:
		return int(v) == 0 || int(v) == 200
	case string:
		vv := strings.TrimSpace(v)
		return vv == "0" || vv == "200" || strings.EqualFold(vv, "success")
	default:
		return false
	}
}

// NewQuakeAdapter 创建Quake适配器
func NewQuakeAdapter(baseURL, apiKey string, qps int, timeout time.Duration) *QuakeAdapter {
	client := resty.New().
		SetTimeout(timeout).
		SetHeader("User-Agent", "UniMap-ICP-Hunter/1.0").
		SetHeader("X-QuakeToken", apiKey)

	return &QuakeAdapter{
		client:  client,
		baseURL: baseURL,
		apiKey:  apiKey,
		qps:     qps,
		timeout: timeout,
	}
}

// Name 返回引擎名称
func (q *QuakeAdapter) Name() string {
	return "quake"
}

// Translate 将UQL AST转换为Quake查询语法
func (q *QuakeAdapter) Translate(ast *model.UQLAST) (string, error) {
	if ast == nil || ast.Root == nil {
		return "", fmt.Errorf("invalid AST")
	}

	query := q.translateNode(ast.Root)
	return query, nil
}

func (q *QuakeAdapter) translateNode(node *model.UQLNode) string {
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
					conditions = append(conditions, q.buildCondition(field, "=", v))
				}
				return "(" + strings.Join(conditions, " OR ") + ")"
			}

			return q.buildCondition(field, op, val)
		}

	case "logical":
		if len(node.Children) >= 2 {
			left := q.translateNode(node.Children[0])
			right := q.translateNode(node.Children[1])
			if node.Value == "OR" {
				return fmt.Sprintf("(%s OR %s)", left, right)
			}
			return fmt.Sprintf("(%s AND %s)", left, right)
		}
	}

	return ""
}

func (q *QuakeAdapter) buildCondition(field, op, value string) string {
	// 字段映射
	mapping := map[string]string{
		"body":        "response",
		"title":       "title",
		"header":      "headers",
		"port":        "port",
		"protocol":    "service",
		"ip":          "ip",
		"country":     "country",
		"region":      "province",
		"city":        "city",
		"asn":         "asn",
		"org":         "org",
		"isp":         "isp",
		"domain":      "domain",
		"app":         "app",
		"os":          "os",
		"server":      "app",
		"host":        "domain",
		"url":         "url",
		"status_code": "status_code",
	}

	if mapped, ok := mapping[field]; ok {
		field = mapped
	}

	if op == "!=" || op == "<>" {
		return fmt.Sprintf(`NOT %s:"%s"`, field, value)
	}

	// Quake syntax: field:"value"
	return fmt.Sprintf(`%s:"%s"`, field, value)
}

// Search 执行搜索
func (q *QuakeAdapter) Search(query string, page, pageSize int) (*model.EngineResult, error) {
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
		// Quake API endpoint: /v3/search/quake_service
		url := fmt.Sprintf("%s/v3/search/quake_service", q.baseURL)

		reqBody := map[string]interface{}{
			"query": query,
			"start": (page - 1) * pageSize,
			"size":  pageSize,
		}

		resp, err := q.client.R().
			SetBody(reqBody).
			Post(url)

		if err != nil {
			return err
		}

		if resp.StatusCode() != 200 {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode(), resp.String())
		}

		// 解析Quake响应
		var result struct {
			Code    interface{}   `json:"code"` // Can be int or string depending on version/error
			Message string        `json:"message"`
			Data    []interface{} `json:"data"`
			Meta    struct {
				Pagination struct {
					Total int `json:"total"`
					Count int `json:"count"`
				} `json:"pagination"`
			} `json:"meta"`
		}

		if err := json.Unmarshal(resp.Body(), &result); err != nil {
			return err
		}

		if !quakeIsSuccessCode(result.Code) {
			return fmt.Errorf("quake API error (code=%v): %s", result.Code, result.Message)
		}

		engineResult = &model.EngineResult{
			EngineName: q.Name(),
			RawData:    result.Data,
			Total:      result.Meta.Pagination.Total,
			Page:       page,
			HasMore:    (result.Meta.Pagination.Total > page*pageSize),
		}

		return nil
	})

	if err != nil {
		return &model.EngineResult{
			EngineName: q.Name(),
			Error:      fmt.Sprintf("search error: %v", err),
		}, nil
	}

	return engineResult, nil
}

// Normalize 标准化结果
func (q *QuakeAdapter) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
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
			Source: q.Name(),
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
				if statusCode, ok := http["status_code"].(float64); ok {
					asset.StatusCode = int(statusCode)
				}
			}
		}

		if location, ok := data["location"].(map[string]interface{}); ok {
			if country, ok := location["country_code"].(string); ok {
				asset.CountryCode = country
			}
			if city, ok := location["city_cn"].(string); ok {
				asset.City = city
			}
			if province, ok := location["province_cn"].(string); ok {
				asset.Region = province
			}
		}

		if asset.IP != "" {
			assets = append(assets, *asset)
		}
	}

	return assets, nil
}

// GetQuota 获取Quake配额信息
func (q *QuakeAdapter) GetQuota() (*model.QuotaInfo, error) {
	if q.apiKey == "" {
		return nil, fmt.Errorf("Quake API key not configured")
	}

	// Quake API endpoint for quota info
	url := fmt.Sprintf("%s/v3/user/info", q.baseURL)

	resp, err := q.client.R().
		SetQueryParam("key", q.apiKey).
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("request error: %v", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	// Quake quota response structure can vary; parse defensively to avoid silently
	// returning 0/0/0 when the schema changes.
	var raw map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &raw); err != nil {
		return nil, fmt.Errorf("parse error: %v", err)
	}

	code := raw["code"]
	message, _ := raw["message"].(string)
	if !quakeIsSuccessCode(code) {
		return nil, fmt.Errorf("quake API error (code=%v): %s", code, message)
	}

	asMap := func(v interface{}) (map[string]interface{}, bool) {
		m, ok := v.(map[string]interface{})
		return m, ok
	}
	asInt := func(v interface{}) (int, bool) {
		switch vv := v.(type) {
		case int:
			return vv, true
		case int64:
			return int(vv), true
		case float64:
			return int(vv), true
		case string:
			vv = strings.TrimSpace(vv)
			if vv == "" {
				return 0, false
			}
			// best-effort parse
			var n int
			_, err := fmt.Sscanf(vv, "%d", &n)
			if err != nil {
				return 0, false
			}
			return n, true
		default:
			return 0, false
		}
	}

	// 尝试不同的响应结构
	var total, used, remain int
	var found bool

	// 结构1: data.resource.query_limit 或 data.resource.queryLimit
	data, dataOk := asMap(raw["data"])
	if dataOk {
		resource, resourceOk := asMap(data["resource"])
		if resourceOk {
			// Prefer snake_case query_limit but accept camelCase queryLimit as well.
			queryLimit, ok := asMap(resource["query_limit"])
			if !ok {
				queryLimit, ok = asMap(resource["queryLimit"])
			}
			if ok {
				total, _ = asInt(queryLimit["total"])
				used, _ = asInt(queryLimit["used"])
				remain, _ = asInt(queryLimit["remain"])
				found = true
			}
		}
	}

	// 结构2: 直接在data中
	if !found && dataOk {
		queryLimit, ok := asMap(data["query_limit"])
		if !ok {
			queryLimit, ok = asMap(data["queryLimit"])
		}
		if ok {
			total, _ = asInt(queryLimit["total"])
			used, _ = asInt(queryLimit["used"])
			remain, _ = asInt(queryLimit["remain"])
			found = true
		}
	}

	// 结构3: Quake实际响应结构 - data.credit 和 data.month_remaining_credit
	if !found && dataOk {
		t, totalOk := asInt(data["credit"])
		r, remainOk := asInt(data["month_remaining_credit"])
		if totalOk && remainOk {
			total = t
			remain = r
			used = total - remain
			found = true
			logger.Infof("Quake quota: total=%d, used=%d, remain=%d", total, used, remain)
		}
	}

	// 结构4: 直接在raw中
	if !found {
		queryLimit, ok := asMap(raw["query_limit"])
		if !ok {
			queryLimit, ok = asMap(raw["queryLimit"])
		}
		if ok {
			total, _ = asInt(queryLimit["total"])
			used, _ = asInt(queryLimit["used"])
			remain, _ = asInt(queryLimit["remain"])
			found = true
		}
	}

	// 如果仍然没找到，返回默认值而不是错误
	if !found {
		logger.Warnf("Quake quota response structure different than expected, using default values: %v", raw)
		// 返回默认配额信息，避免查询失败
		return &model.QuotaInfo{
			Remaining: 0,
			Total:     0,
			Used:      0,
			Unit:      "queries",
			Expiry:    "",
		}, nil
	}

	return &model.QuotaInfo{
		Remaining: remain,
		Total:     total,
		Used:      used,
		Unit:      "queries",
		Expiry:    "", // Quake API doesn't return expiry info
	}, nil
}
