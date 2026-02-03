package adapter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/unimap-icp-hunter/project/internal/model"
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
			connector := "AND"
			if node.Value == "OR" {
				connector = "OR"
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
		"header":      "web.header",
		"port":        "port",
		"protocol":    "service",
		"ip":          "ip",
		"country":     "location.country_cn",
		"region":      "location.province_cn",
		"city":        "location.city_cn",
		"asn":         "asn",
		"org":         "org",
		"isp":         "isp",
		"domain":      "domain",
		"status_code": "web.status_code",
	}

	mappedField, ok := mapping[field]
	if !ok {
		mappedField = field
	}

	if op == "=" || op == "==" {
		return fmt.Sprintf(`%s:"%s"`, mappedField, value)
	}

	return ""
}

// Search 执行Hunter搜索
func (h *HunterAdapter) Search(query string, page, pageSize int) (*model.EngineResult, error) {
	if h.apiKey == "" {
		return &model.EngineResult{
			EngineName: h.Name(),
			Error:      "Hunter API key not configured",
		}, nil
	}

	url := fmt.Sprintf("%s/api/v1/search", h.baseURL)

	// Hunter的API参数可能不同，这里根据常见格式实现
	// 实际需要根据Hunter API文档调整
	resp, err := h.client.R().
		SetQueryParams(map[string]string{
			"api-key":   h.apiKey,
			"query":     query,
			"page":      fmt.Sprintf("%d", page),
			"page_size": fmt.Sprintf("%d", pageSize),
		}).
		Get(url)

	if err != nil {
		return &model.EngineResult{
			EngineName: h.Name(),
			Error:      fmt.Sprintf("request error: %v", err),
		}, nil
	}

	if resp.StatusCode() != 200 {
		return &model.EngineResult{
			EngineName: h.Name(),
			Error:      fmt.Sprintf("HTTP %d: %s", resp.StatusCode(), resp.String()),
		}, nil
	}

	// Hunter返回格式解析
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Total int                      `json:"total"`
			Items []map[string]interface{} `json:"items"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return &model.EngineResult{
			EngineName: h.Name(),
			Error:      fmt.Sprintf("parse error: %v", err),
		}, nil
	}

	if result.Code != 200 {
		return &model.EngineResult{
			EngineName: h.Name(),
			Error:      result.Message,
		}, nil
	}

	// 转换为通用格式
	rawData := []interface{}{}
	for _, item := range result.Data.Items {
		rawData = append(rawData, item)
	}

	return &model.EngineResult{
		EngineName: h.Name(),
		RawData:    rawData,
		Total:      result.Data.Total,
		Page:       page,
		HasMore:    (page * pageSize) < result.Data.Total,
	}, nil
}

// Normalize 标准化Hunter结果
func (h *HunterAdapter) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
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
			Source: h.Name(),
		}

		// Hunter的字段结构可能包含web子对象
		if web, ok := data["web"].(map[string]interface{}); ok {
			if ip, ok := web["ip"].(string); ok {
				asset.IP = ip
			}
			if port, ok := web["port"].(float64); ok {
				asset.Port = int(port)
			} else if port, ok := web["port"].(int); ok {
				asset.Port = port
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
			} else if status, ok := web["status_code"].(int); ok {
				asset.StatusCode = status
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

		// 如果web对象中没有IP，尝试直接从根对象获取
		if asset.IP == "" {
			if ip, ok := data["ip"].(string); ok {
				asset.IP = ip
			}
		}
		if asset.Port == 0 {
			if port, ok := data["port"].(float64); ok {
				asset.Port = int(port)
			} else if port, ok := data["port"].(int); ok {
				asset.Port = port
			}
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
