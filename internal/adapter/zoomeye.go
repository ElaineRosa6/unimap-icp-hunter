package adapter

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/utils"
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
				return "(" + strings.Join(conditions, " ") + ")"
			}

			return z.buildCondition(field, op, val)
		}

	case "logical":
		if len(node.Children) >= 2 {
			left := z.translateNode(node.Children[0])
			right := z.translateNode(node.Children[1])
			if node.Value == "OR" {
				return fmt.Sprintf("%s %s", left, right)
			}
			return fmt.Sprintf("+%s +%s", left, right)
		}
	}

	return ""
}

func (z *ZoomEyeAdapter) buildCondition(field, op, value string) string {
	// 字段映射
	mapping := map[string]string{
		"body":        "site",
		"title":       "title",
		"header":      "headers",
		"port":        "port",
		"protocol":    "service",
		"ip":          "ip",
		"country":     "country",
		"region":      "subdivisions",
		"city":        "city",
		"asn":         "asn",
		"org":         "org",
		"isp":         "isp",
		"domain":      "hostname",
		"app":         "app",
		"os":          "os",
		"device":      "device",
		"banner":      "banner",
		"server":      "app",
		"host":        "hostname",
		"url":         "site",
		"status_code": "site",
	}

	if mapped, ok := mapping[field]; ok {
		field = mapped
	}

	sender := ""
	if op == "!=" || op == "<>" {
		sender = "-"
	} else {
		sender = "+"
	}

	// ZoomEye search syntax: app:"nginx" or +app:"nginx" -app:"apache"
	return fmt.Sprintf(`%s%s:"%s"`, sender, field, value)
}

// Search 执行搜索
func (z *ZoomEyeAdapter) Search(query string, page, pageSize int) (*model.EngineResult, error) {
	var engineResult *model.EngineResult

	retryConfig := utils.RetryConfig{
		MaxRetries:  3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    2 * time.Second,
		Exponential: true,
		Jitter:      true,
		RetryableFunc: func(err error) bool {
			// 网络错误可重试，但402错误（需要付费）不可重试
			errMsg := err.Error()
			return !strings.Contains(errMsg, "402") && !strings.Contains(errMsg, "Payment Required")
		},
	}

	err := utils.Retry(retryConfig, func() error {
		// 实现搜索逻辑
		// ZoomEye API endpoint: /v2/search
		url := fmt.Sprintf("%s/v2/search", z.baseURL)

		// 将查询语句转换为Base64编码
		encodedQuery := base64.StdEncoding.EncodeToString([]byte(query))
		// 替换Base64编码中的不安全字符，确保URL安全
		encodedQuery = strings.ReplaceAll(encodedQuery, "+", "-")
		encodedQuery = strings.ReplaceAll(encodedQuery, "/", "_")
		encodedQuery = strings.TrimRight(encodedQuery, "=")

		// 构建请求体
		requestBody := map[string]interface{}{
			"qbase64":  encodedQuery,
			"page":     page,
			"pagesize": pageSize,
		}

		// 记录请求信息，方便调试
		logger.Debugf("ZoomEye search request: URL=%s, Query=%s, EncodedQuery=%s, Page=%d, PageSize=%d", url, query, encodedQuery, page, pageSize)

		resp, err := z.client.R().
			SetHeader("Content-Type", "application/json").
			SetBody(requestBody).
			Post(url)

		if err != nil {
			return err
		}

		if resp.StatusCode() != 200 {
			errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode(), resp.String())
			if resp.StatusCode() == 402 {
				errMsg = fmt.Sprintf("ZoomEye API Payment Required (402): %s. Please check if your account is mobile-verified or if you have sufficient quota/credits.", resp.String())
			}
			return fmt.Errorf("%s", errMsg)
		}

		// 解析ZoomEye响应
		var result struct {
			Code    int           `json:"code"`
			Error   string        `json:"error"`
			Message string        `json:"message"`
			Total   int           `json:"total"`
			Query   string        `json:"query"`
			Data    []interface{} `json:"data"`
		}

		if err := json.Unmarshal(resp.Body(), &result); err != nil {
			return err
		}

		// 检查响应代码
		if result.Code != 60000 {
			// 构建详细的错误信息
			errorMsg := fmt.Sprintf("ZoomEye API error (code=%d, error=%s): %s", result.Code, result.Error, result.Message)
			// 特别处理额度不足的情况
			if result.Code == 50000 && result.Error == "credits_insufficient" {
				errorMsg = fmt.Sprintf("ZoomEye API credits insufficient: %s. Please check your account balance or upgrade your plan.", result.Message)
			}
			return fmt.Errorf("%s", errorMsg)
		}

		engineResult = &model.EngineResult{
			EngineName: z.Name(),
			RawData:    result.Data,
			Total:      result.Total,
			Page:       page,
			HasMore:    (page * pageSize) < result.Total,
		}

		return nil
	})

	if err != nil {
		return &model.EngineResult{
			EngineName: z.Name(),
			Error:      err.Error(),
		}, nil
	}

	return engineResult, nil
}

// Normalize 标准化结果
func (z *ZoomEyeAdapter) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
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
			Source: z.Name(),
		}

		// 解析ZoomEye数据结构
		// 1. 解析IP地址
		if ip, ok := data["ip"].(string); ok {
			asset.IP = ip
		}

		// 2. 解析端口和服务信息（支持新旧两种格式）
		// 新格式：直接在顶层
		if port, ok := data["port"].(float64); ok {
			asset.Port = int(port)
		} else if port, ok := data["port"].(int); ok {
			asset.Port = port
		}

		if service, ok := data["service"].(string); ok {
			asset.Protocol = service
		}

		// 旧格式：在portinfo中
		if portinfo, ok := data["portinfo"].(map[string]interface{}); ok {
			if asset.Port == 0 {
				if port, ok := portinfo["port"].(float64); ok {
					asset.Port = int(port)
				}
			}
			if asset.Protocol == "" {
				if service, ok := portinfo["service"].(string); ok {
					asset.Protocol = service
				}
			}
			if asset.Title == "" {
				if title, ok := portinfo["title"].(string); ok { // Sometimes title is here
					asset.Title = title
				}
			}
			if asset.BodySnippet == "" {
				if banner, ok := portinfo["banner"].(string); ok {
					asset.BodySnippet = banner // Use banner as snippet
				}
			}
		}

		// 3. 解析标题（支持数组和字符串格式）
		if title, ok := data["title"].([]interface{}); ok && len(title) > 0 {
			if titleStr, ok := title[0].(string); ok {
				asset.Title = titleStr
			}
		} else if title, ok := data["title"].(string); ok {
			asset.Title = title
		}

		// 4. 解析其他字段
		if banner, ok := data["banner"].(string); ok {
			asset.BodySnippet = banner
		}

		if server, ok := data["header.server.name"].(string); ok {
			asset.Server = server
		}

		if url, ok := data["url"].(string); ok {
			asset.URL = url
		}

		if domain, ok := data["domain"].(string); ok {
			asset.Host = domain
		} else if hostname, ok := data["hostname"].(string); ok {
			asset.Host = hostname
		}

		// 5. 解析地理位置信息（支持新旧两种格式）
		// 新格式：使用点号分隔的字段名
		if countryName, ok := data["country.name"].(string); ok {
			// 存储到Extra字段
			if asset.Extra == nil {
				asset.Extra = make(map[string]interface{})
			}
			asset.Extra["country"] = countryName
		}

		if provinceName, ok := data["province.name"].(string); ok {
			asset.Region = provinceName
		}

		if cityName, ok := data["city.name"].(string); ok {
			asset.City = cityName
		}

		// 旧格式：在geoinfo中
		if geoinfo, ok := data["geoinfo"].(map[string]interface{}); ok {
			if asset.CountryCode == "" {
				if country, ok := geoinfo["country"].(map[string]interface{}); ok {
					if code, ok := country["code"].(string); ok {
						asset.CountryCode = code
					}
				}
			}
			if asset.City == "" {
				if city, ok := geoinfo["city"].(string); ok {
					asset.City = city
				}
			}
			if asset.Region == "" {
				if subdivisions, ok := geoinfo["subdivisions"].(string); ok {
					asset.Region = subdivisions
				}
			}
		}

		// 6. 解析ASN、组织和ISP信息
		if asn, ok := data["asn"].(float64); ok {
			asset.ASN = strconv.Itoa(int(asn))
		} else if asn, ok := data["asn"].(int); ok {
			asset.ASN = strconv.Itoa(asn)
		} else if asn, ok := data["asn"].(string); ok {
			asset.ASN = asn
		}

		if org, ok := data["organization.name"].(string); ok {
			asset.Org = org
		} else if org, ok := data["org"].(string); ok {
			asset.Org = org
		}

		if isp, ok := data["isp.name"].(string); ok {
			asset.ISP = isp
		} else if isp, ok := data["isp"].(string); ok {
			asset.ISP = isp
		}

		// 7. 存储其他有用的信息到Extra字段
		if os, ok := data["os"].(string); ok {
			if asset.Extra == nil {
				asset.Extra = make(map[string]interface{})
			}
			asset.Extra["os"] = os
		}

		if product, ok := data["product"].(string); ok {
			if asset.Extra == nil {
				asset.Extra = make(map[string]interface{})
			}
			asset.Extra["product"] = product
		} else if app, ok := data["app"].(string); ok {
			if asset.Extra == nil {
				asset.Extra = make(map[string]interface{})
			}
			asset.Extra["app"] = app
		}

		if version, ok := data["version"].(string); ok {
			if asset.Extra == nil {
				asset.Extra = make(map[string]interface{})
			}
			asset.Extra["version"] = version
		}

		if device, ok := data["device"].(string); ok {
			if asset.Extra == nil {
				asset.Extra = make(map[string]interface{})
			}
			asset.Extra["device"] = device
		}

		if body, ok := data["body"].(string); ok {
			if asset.Extra == nil {
				asset.Extra = make(map[string]interface{})
			}
			asset.Extra["body"] = body
		}

		if header, ok := data["header"].(string); ok {
			if asset.Extra == nil {
				asset.Extra = make(map[string]interface{})
			}
			asset.Extra["header"] = header
		}

		// 7. 添加到结果集
		// 只要资产有IP地址、URL、域名或主机名，就应该被添加到结果集中
		if asset.IP != "" || asset.URL != "" || asset.Host != "" {
			assets = append(assets, *asset)
		}
	}

	return assets, nil
}

// GetQuota 获取ZoomEye配额信息
func (z *ZoomEyeAdapter) GetQuota() (*model.QuotaInfo, error) {
	if z.apiKey == "" {
		return nil, fmt.Errorf("ZoomEye API key not configured")
	}

	// ZoomEye API endpoint for quota info
	url := fmt.Sprintf("%s/resources-info", z.baseURL)

	resp, err := z.client.R().
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("request error: %v", err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode(), resp.String())
	}

	// 打印响应体，方便调试
	logger.Debugf("ZoomEye quota response: %s", resp.String())

	// ZoomEye quota response structure
	var result struct {
		Code      int    `json:"code"`
		Plan      string `json:"plan"`
		Resources struct {
			Search   int    `json:"search"`
			Interval string `json:"interval"`
		} `json:"resources"`
		UserInfo struct {
			Name      string `json:"name"`
			Role      string `json:"role"`
			ExpiredAt string `json:"expired_at"`
		} `json:"user_info"`
		QuotaInfo struct {
			RemainFreeQuota  int `json:"remain_free_quota"`
			RemainPayQuota   int `json:"remain_pay_quota"`
			RemainTotalQuota int `json:"remain_total_quota"`
		} `json:"quota_info"`
	}

	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return nil, fmt.Errorf("parse error: %v", err)
	}

	if result.Code != 60000 {
		return nil, fmt.Errorf("ZoomEye API error code: %d", result.Code)
	}

	// 计算配额信息
	// ZoomEye的响应中，quota_info.remain_total_quota是剩余的总配额，resources.search是总配额
	total := result.Resources.Search
	remain := result.QuotaInfo.RemainTotalQuota
	used := total - remain

	// 打印解析后的配额信息
	logger.Infof("ZoomEye quota: total=%d, used=%d, remain=%d", total, used, remain)

	return &model.QuotaInfo{
		Remaining: remain,
		Total:     total,
		Used:      used,
		Unit:      "queries",
		Expiry:    result.UserInfo.ExpiredAt,
	}, nil
}

// IsWebOnly 检查是否为 Web-only 模式
func (z *ZoomEyeAdapter) IsWebOnly() bool {
	return false
}

// ZoomEyeAdapterWebOnly ZoomEye Web-only模式适配器
type ZoomEyeAdapterWebOnly struct {
	*WebOnlyAdapterBase
}

// NewZoomEyeAdapterWebOnly 创建ZoomEye Web-only适配器
func NewZoomEyeAdapterWebOnly() *ZoomEyeAdapterWebOnly {
	baseAdapter := NewZoomEyeAdapter("", "", 3, 30*time.Second)
	return &ZoomEyeAdapterWebOnly{
		WebOnlyAdapterBase: NewWebOnlyAdapterBase(baseAdapter, "zoomeye"),
	}
}
