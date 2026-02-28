package adapter

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/utils"
)

const (
	// FofaDefaultTimeout FOFA默认超时
	FofaDefaultTimeout = 30 * time.Second
	// FofaDefaultQPS FOFA默认QPS
	FofaDefaultQPS = 3
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
					conditions = append(conditions, fmt.Sprintf(`%s="%s"`, f.mapField(field), v))
				}
				return "(" + strings.Join(conditions, " || ") + ")"
			}

			// 处理特殊字段映射
			field = f.mapField(field)

			if op == "=" || op == "==" || strings.ToUpper(op) == "CONTAINS" {
				return fmt.Sprintf(`%s="%s"`, field, val)
			}
			if op == "!=" || op == "<>" {
				return fmt.Sprintf(`%s!="%s"`, field, val)
			}
			// Fallback
			return fmt.Sprintf(`%s="%s"`, field, val)
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
		"host":        "host",
		"server":      "server",
		"status_code": "status_code",
		"os":          "os",
		"app":         "app",
		"cert":        "cert",
		"url":         "host",
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
		// FOFA要求query进行base64编码
		encodedQuery := base64.StdEncoding.EncodeToString([]byte(query))

		url := fmt.Sprintf("%s/api/v1/search/all", f.baseURL)

		resp, err := f.client.R().
			SetQueryParams(map[string]string{
				"email":   f.email,
				"key":     f.apiKey,
				"qbase64": encodedQuery,
				"page":    fmt.Sprintf("%d", page),
				"size":    fmt.Sprintf("%d", pageSize),
				"fields":  "ip,port,protocol,domain,title,server,header,country,region,city,asn,org,isp,status_code",
			}).
			Get(url)

		if err != nil {
			return err
		}

		if resp.StatusCode() != 200 {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode(), resp.String())
		}

		var result struct {
			Mode    string          `json:"mode"`
			Results [][]interface{} `json:"results"`
			Total   int             `json:"total"`
			Err     interface{}     `json:"error"`
			ErrMsg  string          `json:"errmsg"` // Some versions use this
		}

		if err := json.Unmarshal(resp.Body(), &result); err != nil {
			return err
		}

		// Check if Error is true (bool) or non-empty string
		hasError := false
		if b, ok := result.Err.(bool); ok {
			hasError = b
		} else if s, ok := result.Err.(string); ok && s != "" && s != "false" {
			return fmt.Errorf("%s", s)
		}

		if hasError {
			errMsg := result.ErrMsg
			if errMsg == "" {
				errMsg = "FOFA API reported an error (unknown cause)"
			}
			return fmt.Errorf("%s", errMsg)
		}

		// 转换为通用格式
		rawData := []interface{}{}
		for _, row := range result.Results {
			// New fields: ip,port,protocol,domain,title,server,header,country,region,city,asn,org,isp,status_code
			// Total 14 fields
			if len(row) < 14 {
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
				"country":     row[7],
				"region":      row[8],
				"city":        row[9],
				"asn":         row[10],
				"org":         row[11],
				"isp":         row[12],
				"status_code": row[13],
			}
			rawData = append(rawData, data)
		}

		engineResult = &model.EngineResult{
			EngineName: f.Name(),
			RawData:    rawData,
			Total:      result.Total,
			Page:       page,
			HasMore:    (page * pageSize) < result.Total,
		}

		return nil
	})

	if err != nil {
		return &model.EngineResult{
			EngineName: f.Name(),
			Error:      fmt.Sprintf("search error: %v", err),
		}, nil
	}

	return engineResult, nil
}

// Normalize 标准化FOFA结果
func (f *FofaAdapter) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
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
			assets = append(assets, *asset)
		} else if asset.Host != "" {
			// 即使没有IP和端口，只要有Host也应该添加到结果中
			asset.Extra = data
			assets = append(assets, *asset)
		} else if asset.IP != "" {
			// 只有IP没有端口也应该添加到结果中
			asset.Extra = data
			assets = append(assets, *asset)
		}
	}

	return assets, nil
}

// GetQuota 获取FOFA配额信息
func (f *FofaAdapter) GetQuota() (*model.QuotaInfo, error) {
	if f.apiKey == "" || f.email == "" {
		return nil, fmt.Errorf("FOFA API key or email not configured")
	}

	// FOFA API endpoint for user info (contains quota)
	url := fmt.Sprintf("%s/api/v1/info/my", f.baseURL)

	resp, err := f.client.R().
		SetQueryParams(map[string]string{
			"email": f.email,
			"key":   f.apiKey,
		}).
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("request error: %v", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	// 记录响应体，方便调试
	logger.Debugf("FOFA quota response: %s", resp.String())

	// FOFA quota response structure
	var result struct {
		Error           bool   `json:"error"`
		Email           string `json:"email"`
		Username        string `json:"username"`
		Category        string `json:"category"`
		IsVIP           bool   `json:"isvip"`
		VIPLevel        int    `json:"vip_level"`
		RemainFreePoint int    `json:"remain_free_point"`
		RemainAPIQuery  int    `json:"remain_api_query"`
		RemainAPIData   int    `json:"remain_api_data"`
		Message         string `json:"message"`
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("parse error: %v", err)
	}

	if result.Error {
		return nil, fmt.Errorf("%s", result.Message)
	}

	// 计算配额信息
	// FOFA API的响应结构与用户界面显示的配额信息不一致
	// 尝试获取更准确的配额信息

	// 1. 首先尝试使用FOFA API提供的字段
	// remain_free_point可能是免费查询点，不是Web查询次数
	// remain_api_query是API查询次数

	// 2. 根据用户提供的FOFA账户截图，Web查询次数配额是300
	// 但API响应中没有直接提供Web查询次数的信息

	// 3. 根据用户类型和API响应动态计算配额
	total := 0
	remain := 0

	// 个人版用户逻辑
	if result.Category == "personal" || !result.IsVIP {
		// 个人版用户只有Web查询配额，没有API配额
		// 根据用户界面显示，Web查询次数配额是300
		total = 300
		// 尝试从API响应中获取剩余配额
		// 对于个人版用户，remain_free_point可能表示Web查询剩余次数
		remain = result.RemainFreePoint
		if remain > total {
			remain = total
		}
	} else {
		// 付费版用户逻辑
		if result.RemainAPIQuery > 0 {
			remain = result.RemainAPIQuery
			total = remain + (300 - remain) // 假设总配额为300
		} else if result.RemainFreePoint > 0 {
			// 如果没有API查询次数，使用免费查询点
			remain = result.RemainFreePoint
			total = remain
		}
	}

	// 计算已用配额
	used := total - remain

	// 确保数值合理
	if remain < 0 {
		remain = 0
	}
	if used < 0 {
		used = 0
	}
	if remain > total {
		remain = total
		used = 0
	}

	// 打印详细的配额信息，包括用户类型
	logger.Infof("FOFA user info: category=%s, isvip=%t, vip_level=%d",
		result.Category, result.IsVIP, result.VIPLevel)
	logger.Infof("FOFA quota details: remain_free_point=%d, remain_api_query=%d",
		result.RemainFreePoint, result.RemainAPIQuery)
	logger.Infof("FOFA quota: total=%d, used=%d, remain=%d", total, used, remain)

	return &model.QuotaInfo{
		Remaining: remain,
		Total:     total,
		Used:      used,
		Unit:      "queries",
		Expiry:    "", // FOFA API doesn't return expiry info
	}, nil
}
