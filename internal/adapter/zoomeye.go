package adapter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/unimap-icp-hunter/project/internal/model"
)

// ZoomEyeAdapter ZoomEye引擎适配器
type ZoomEyeAdapter struct {
	client  *resty.Client
	baseURL string
	apiKey  string
	qps     int
	timeout time.Duration
}

// NewZoomEyeAdapter 创建ZoomEye适配器
func NewZoomEyeAdapter(baseURL, apiKey string, qps int, timeout time.Duration) *ZoomEyeAdapter {
	client := resty.New().
		SetTimeout(timeout).
		SetHeader("User-Agent", "UniMap-ICP-Hunter/1.0").
		SetHeader("API-KEY", apiKey)

	return &ZoomEyeAdapter{
		client:  client,
		baseURL: baseURL,
		apiKey:  apiKey,
		qps:     qps,
		timeout: timeout,
	}
}

// Name 返回引擎名称
func (z *ZoomEyeAdapter) Name() string {
	return "zoomeye"
}

// Translate 将UQL AST转换为ZoomEye查询语法
func (z *ZoomEyeAdapter) Translate(ast *model.UQLAST) (string, error) {
	if ast == nil || ast.Root == nil {
		return "", fmt.Errorf("invalid AST")
	}

	query := z.translateNode(ast.Root)
	return query, nil
}

func (z *ZoomEyeAdapter) translateNode(node *model.UQLNode) string {
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
					conditions = append(conditions, z.buildCondition(field, "=", v))
				}
				// ZoomEye 使用 + 表示 AND
				return "(" + strings.Join(conditions, " ") + ")"
			}

			return z.buildCondition(field, op, val)
		}

	case "logical":
		if len(node.Children) >= 2 {
			left := z.translateNode(node.Children[0])
			right := z.translateNode(node.Children[1])
			if node.Value == "OR" {
				// ZoomEye普通API不支持OR语法，这里降级为AND
				return fmt.Sprintf("%s %s", left, right)
			}
			return fmt.Sprintf("%s +%s", left, right) // + for AND
		}
	}

	return ""
}

func (z *ZoomEyeAdapter) buildCondition(field, op, value string) string {
	// 字段映射
	mapping := map[string]string{
		"body":     "site", // partial match for site content? ZoomEye keys are specific.
		"title":    "title",
		"header":   "headers",
		"port":     "port",
		"protocol": "service",
		"ip":       "ip",
		"country":  "country",
		"region":   "subdivisions",
		"city":     "city",
		"asn":      "asn",
		"org":      "org",
		"isp":      "isp",
		"domain":   "hostname",
		"app":      "app",
		"os":       "os",
		"device":   "device",
	}

	if mapped, ok := mapping[field]; ok {
		field = mapped
	}

	// ZoomEye search syntax: app:"nginx"
	return fmt.Sprintf(`%s:"%s"`, field, value)
}

// Search 执行搜索
func (z *ZoomEyeAdapter) Search(query string, page, pageSize int) (*model.EngineResult, error) {
	// 实现搜索逻辑
	// ZoomEye API endpoint: /host/search
	url := fmt.Sprintf("%s/host/search", z.baseURL)

	resp, err := z.client.R().
		SetQueryParam("query", query).
		SetQueryParam("page", fmt.Sprintf("%d", page)).
		SetQueryParam("pageSize", fmt.Sprintf("%d", pageSize)).
		Get(url)

	if err != nil {
		return &model.EngineResult{
			EngineName: z.Name(),
			Error:      fmt.Sprintf("request error: %v", err),
		}, nil
	}

	if resp.StatusCode() != 200 {
		return &model.EngineResult{
			EngineName: z.Name(),
			Error:      fmt.Sprintf("HTTP %d: %s", resp.StatusCode(), resp.String()),
		}, nil
	}

	// 解析ZoomEye响应
	var result struct {
		Total   int           `json:"total"`
		Matches []interface{} `json:"matches"`
		// Other fields...
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return &model.EngineResult{
			EngineName: z.Name(),
			Error:      fmt.Sprintf("parse error: %v", err),
		}, nil
	}

	return &model.EngineResult{
		EngineName: z.Name(),
		RawData:    result.Matches,
		Total:      result.Total,
		Page:       page,
		HasMore:    (page * pageSize) < result.Total,
	}, nil
}

// Normalize 标准化结果
func (z *ZoomEyeAdapter) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
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
			Source: z.Name(),
		}

		// 解析ZoomEye数据结构
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
			if title, ok := portinfo["title"].(string); ok { // Sometimes title is here
				asset.Title = title
			}
			if banner, ok := portinfo["banner"].(string); ok {
				asset.BodySnippet = banner // Use banner as snippet
			}
		}

		if geoinfo, ok := data["geoinfo"].(map[string]interface{}); ok {
			if country, ok := geoinfo["country"].(map[string]interface{}); ok {
				if code, ok := country["code"].(string); ok {
					asset.CountryCode = code
				}
			}
			// More geo info extracting...
		}

		assets = append(assets, asset)
	}

	return assets, nil
}
