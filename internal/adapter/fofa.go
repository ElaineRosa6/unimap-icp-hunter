package adapter

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/unimap-icp-hunter/project/internal/model"
)

// FofaAdapter FOFA引擎适配器
type FofaAdapter struct {
	client  *resty.Client
	baseURL string
	apiKey  string
	email   string
	qps     int
	timeout time.Duration
}

// NewFofaAdapter 创建FOFA适配器
func NewFofaAdapter(baseURL, apiKey, email string, qps int, timeout time.Duration) *FofaAdapter {
	client := resty.New().
		SetTimeout(timeout).
		SetHeader("User-Agent", "UniMap-ICP-Hunter/1.0")

	return &FofaAdapter{
		client:  client,
		baseURL: baseURL,
		apiKey:  apiKey,
		email:   email,
		qps:     qps,
		timeout: timeout,
	}
}

// Name 返回引擎名称
func (f *FofaAdapter) Name() string {
	return "fofa"
}

// Translate 将UQL AST转换为FOFA查询语法
func (f *FofaAdapter) Translate(ast *model.UQLAST) (string, error) {
	if ast == nil || ast.Root == nil {
		return "", fmt.Errorf("invalid AST")
	}

	// FOFA使用类似ES的查询语法
	// 简单实现：遍历AST构建查询字符串
	query := f.translateNode(ast.Root)
	return query, nil
}

func (f *FofaAdapter) translateNode(node *model.UQLNode) string {
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
				// FOFA不支持IN语法，需要转换为多个OR
				values := strings.Split(val, ",")
				conditions := []string{}
				for _, v := range values {
					conditions = append(conditions, fmt.Sprintf(`%s="%s"`, field, v))
				}
				return "(" + strings.Join(conditions, " || ") + ")"
			}

			// 处理特殊字段映射
			field = f.mapField(field)

			if op == "=" || op == "==" {
				return fmt.Sprintf(`%s="%s"`, field, val)
			}
		}

	case "logical":
		if len(node.Children) >= 2 {
			left := f.translateNode(node.Children[0])
			right := f.translateNode(node.Children[1])
			if node.Value == "OR" {
				return fmt.Sprintf("(%s || %s)", left, right)
			}
			return fmt.Sprintf("(%s && %s)", left, right)
		}
	}

	return ""
}

// mapField 映射统一字段到FOFA字段
func (f *FofaAdapter) mapField(field string) string {
	mapping := map[string]string{
		"body":        "body",
		"title":       "title",
		"header":      "header",
		"port":        "port",
		"protocol":    "protocol",
		"ip":          "ip",
		"country":     "country",
		"region":      "region",
		"city":        "city",
		"asn":         "asn",
		"org":         "org",
		"isp":         "isp",
		"domain":      "domain",
		"status_code": "status_code",
	}

	if mapped, ok := mapping[field]; ok {
		return mapped
	}
	return field
}

// Search 执行FOFA搜索
func (f *FofaAdapter) Search(query string, page, pageSize int) (*model.EngineResult, error) {
	if f.apiKey == "" || f.email == "" {
		return &model.EngineResult{
			EngineName: f.Name(),
			Error:      "FOFA API key or email not configured",
		}, nil
	}

	// FOFA要求query进行base64编码
	encodedQuery := base64.StdEncoding.EncodeToString([]byte(query))

	url := fmt.Sprintf("%s/api/v1/search/all", f.baseURL)

	resp, err := f.client.R().
		SetQueryParams(map[string]string{
			"email":      f.email,
			"key":        f.apiKey,
			"qbase64":    encodedQuery,
			"page":       fmt.Sprintf("%d", page),
			"size":       fmt.Sprintf("%d", pageSize),
			"fields":     "ip,port,protocol,domain,title,server,header,body,country,region,city,asn,org,isp,status_code",
		}).
		Get(url)

	if err != nil {
		return &model.EngineResult{
			EngineName: f.Name(),
			Error:      fmt.Sprintf("request error: %v", err),
		}, nil
	}

	if resp.StatusCode() != 200 {
		return &model.EngineResult{
			EngineName: f.Name(),
			Error:      fmt.Sprintf("HTTP %d: %s", resp.StatusCode(), resp.String()),
		}, nil
	}

	var result struct {
		Mode   string        `json:"mode"`
		Results [][]interface{} `json:"results"`
		Total  int           `json:"total"`
		Err    string        `json:"error"`
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return &model.EngineResult{
			EngineName: f.Name(),
			Error:      fmt.Sprintf("parse error: %v", err),
		}, nil
	}

	if result.Err != "" {
		return &model.EngineResult{
			EngineName: f.Name(),
			Error:      result.Err,
		}, nil
	}

	// 转换为通用格式
	rawData := []interface{}{}
	for _, row := range result.Results {
		if len(row) < 11 {
			continue
		}
		data := map[string]interface{}{
			"ip":          row[0],
			"port":        row[1],
			"protocol":    row[2],
			"domain":      row[3],
			"title":       row[4],
			"server":      row[5],
			"header":      row[6],
			"body":        row[7],
			"country":     row[8],
			"region":      row[9],
			"city":        row[10],
			"asn":         row[11],
			"org":         row[12],
			"isp":         row[13],
			"status_code": row[14],
		}
		rawData = append(rawData, data)
	}

	return &model.EngineResult{
		EngineName: f.Name(),
		RawData:    rawData,
		Total:      result.Total,
		Page:       page,
		HasMore:    (page * pageSize) < result.Total,
	}, nil
}

// Normalize 标准化FOFA结果
func (f *FofaAdapter) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
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
			Source: f.Name(),
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
		}
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
