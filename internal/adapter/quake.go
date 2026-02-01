package adapter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/unimap-icp-hunter/project/internal/model"
)

// QuakeAdapter Quake引擎适配器
type QuakeAdapter struct {
	client  *resty.Client
	baseURL string
	apiKey  string
	qps     int
	timeout time.Duration
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
		"body":     "response",
		"title":    "title",
		"header":   "headers",
		"port":     "port",
		"protocol": "service",
		"ip":       "ip",
		"country":  "country",
		"region":   "province",
		"city":     "city",
		"asn":      "asn",
		"org":      "org",
		"isp":      "isp",
		"domain":   "domain",
	}

	if mapped, ok := mapping[field]; ok {
		field = mapped
	}

	// Quake syntax: field:"value"
	return fmt.Sprintf(`%s:"%s"`, field, value)
}

// Search 执行搜索
func (q *QuakeAdapter) Search(query string, page, pageSize int) (*model.EngineResult, error) {
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
		return &model.EngineResult{
			EngineName: q.Name(),
			Error:      fmt.Sprintf("request error: %v", err),
		}, nil
	}

	if resp.StatusCode() != 200 {
		return &model.EngineResult{
			EngineName: q.Name(),
			Error:      fmt.Sprintf("HTTP %d: %s", resp.StatusCode(), resp.String()),
		}, nil
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
		return &model.EngineResult{
			EngineName: q.Name(),
			Error:      fmt.Sprintf("parse error: %v", err),
		}, nil
	}

	// 检查Code, Quake 0 is success
	// (Handling type check simplisticly)

	return &model.EngineResult{
		EngineName: q.Name(),
		RawData:    result.Data,
		Total:      result.Meta.Pagination.Total,
		Page:       page,
		HasMore:    (result.Meta.Pagination.Total > page*pageSize),
	}, nil
}

// Normalize 标准化结果
func (q *QuakeAdapter) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
	assets := []model.UnifiedAsset{}

	if raw == nil || len(raw.RawData) == 0 {
		return assets, nil
	}

	for _, item := range raw.RawData {
		data, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		asset := model.UnifiedAsset{
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

		assets = append(assets, asset)
	}

	return assets, nil
}
