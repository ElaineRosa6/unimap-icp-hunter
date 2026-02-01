package worker

import (
	"crypto/md5"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/model"
	"go.uber.org/zap"
)

// ContentAnalyzer 内容分析器
type ContentAnalyzer struct {
	icpPatterns       []*regexp.Regexp
	uncertainPatterns []*regexp.Regexp
	whitelistIPs      []string
	whitelistDomains  []string
	whitelistASNs     []string
	whitelistOrgs     []string
	logger            *zap.Logger
}

// NewContentAnalyzer 创建内容分析器
func NewContentAnalyzer(logger *zap.Logger) *ContentAnalyzer {
	analyzer := &ContentAnalyzer{
		logger: logger,
	}

	// 初始化ICP备案号正则模式
	analyzer.initICPPatterns()

	// 初始化不确定页面特征
	analyzer.initUncertainPatterns()

	// 初始化白名单（实际应从配置读取）
	analyzer.initWhitelist()

	return analyzer
}

// initICPPatterns 初始化ICP备案号正则
func (c *ContentAnalyzer) initICPPatterns() {
	// 常见ICP备案格式
	patterns := []string{
		`京ICP备\d{6,}-\d{1,3}号`,
		`沪ICP备\d{6,}号`,
		`粤ICP备\d{6,}号`,
		`浙ICP备\d{6,}号`,
		`苏ICP备\d{6,}号`,
		`鲁ICP备\d{6,}号`,
		`闽ICP备\d{6,}号`,
		`冀ICP备\d{6,}号`,
		`豫ICP备\d{6,}号`,
		`鄂ICP备\d{6,}号`,
		`湘ICP备\d{6,}号`,
		`皖ICP备\d{6,}号`,
		`陕ICP备\d{6,}号`,
		`川ICP备\d{6,}号`,
		`渝ICP备\d{6,}号`,
		`辽ICP备\d{6,}号`,
		`黑ICP备\d{6,}号`,
		`吉ICP备\d{6,}号`,
		`晋ICP备\d{6,}号`,
		`蒙ICP备\d{6,}号`,
		`赣ICP备\d{6,}号`,
		`桂ICP备\d{6,}号`,
		`琼ICP备\d{6,}号`,
		`贵ICP备\d{6,}号`,
		`云ICP备\d{6,}号`,
		`藏ICP备\d{6,}号`,
		`甘ICP备\d{6,}号`,
		`青ICP备\d{6,}号`,
		`宁ICP备\d{6,}号`,
		`新ICP备\d{6,}号`,
		`津ICP备\d{6,}号`,
		`京公网安备\d{14}号`,
		`\.ICP备\d{8,}-\d{1,5}号`,
		`\d{12,}-\d{1,5}`, // 长数字格式
	}

	for _, pattern := range patterns {
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			c.logger.Warn("Failed to compile ICP pattern", zap.String("pattern", pattern), zap.Error(err))
			continue
		}
		c.icpPatterns = append(c.icpPatterns, re)
	}
}

// initUncertainPatterns 初始化不确定页面特征
func (c *ContentAnalyzer) initUncertainPatterns() {
	patterns := []string{
		`访问错误`,
		`404 Not Found`,
		`502 Bad Gateway`,
		`503 Service Unavailable`,
		`Maintenance Mode`,
		`CDN Error`,
		`Access Denied`,
		`Site Not Found`,
		`Domain Not Configured`,
	}

	for _, pattern := range patterns {
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			c.logger.Warn("Failed to compile uncertain pattern", zap.String("pattern", pattern), zap.Error(err))
			continue
		}
		c.uncertainPatterns = append(c.uncertainPatterns, re)
	}
}

// initWhitelist 初始化白名单
func (c *ContentAnalyzer) initWhitelist() {
	// 默认白名单
	c.whitelistIPs = []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	c.whitelistDomains = []string{
		"gov.cn",
		"edu.cn",
		"mil.cn",
		"org.cn",
	}

	c.whitelistASNs = []string{}
	c.whitelistOrgs = []string{}
}

// Analyze 分析HTTP响应内容
func (c *ContentAnalyzer) Analyze(task *model.ProbeTask, response *HTTPResponse) *model.ICPCheck {
	check := &model.ICPCheck{
		URL:            task.URL,
		HTTPStatusCode: response.StatusCode,
		Title:          c.extractTitle(response.Body),
		HTMLHash:       c.calculateHash(response.Body),
		IsRegistered:   2, // 默认不确定
		MatchMethod:    "none",
	}

	// 1. 检查白名单
	if c.isWhitelisted(task.IP, task.Host) {
		check.IsRegistered = 1
		check.MatchMethod = "whitelist"
		check.Tags = model.StringArray{"whitelisted"}
		return check
	}

	// 2. 检查不确定页面特征
	if c.isUncertainPage(response.Body) {
		check.IsRegistered = 2
		check.MatchMethod = "uncertain"
		check.Tags = model.StringArray{"uncertain_page"}
		return check
	}

	// 3. 检查ICP备案号
	icpCode := c.findICPCode(response.Body)
	if icpCode != "" {
		check.IsRegistered = 1
		check.ICPCode = icpCode
		check.MatchMethod = "regex"
	} else {
		// 未找到备案号
		check.IsRegistered = 0
		check.MatchMethod = "regex"
	}

	return check
}

// extractTitle 提取页面标题
func (c *ContentAnalyzer) extractTitle(html string) string {
	titleRegex := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
	matches := titleRegex.FindStringSubmatch(html)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// calculateHash 计算HTML内容哈希
func (c *ContentAnalyzer) calculateHash(html string) string {
	if html == "" {
		return ""
	}
	hash := md5.Sum([]byte(html))
	return hex.EncodeToString(hash[:])
}

// findICPCode 查找ICP备案号
func (c *ContentAnalyzer) findICPCode(html string) string {
	if html == "" {
		return ""
	}

	// 在HTML中搜索匹配的备案号
	for _, pattern := range c.icpPatterns {
		matches := pattern.FindString(html)
		if matches != "" {
			return matches
		}
	}

	return ""
}

// isUncertainPage 检查是否为不确定页面
func (c *ContentAnalyzer) isUncertainPage(html string) bool {
	if html == "" {
		return true
	}

	for _, pattern := range c.uncertainPatterns {
		if pattern.MatchString(html) {
			return true
		}
	}

	return false
}

// isWhitelisted 检查是否在白名单中
func (c *ContentAnalyzer) isWhitelisted(ip, domain string) bool {
	// 检查IP白名单
	if ip != "" {
		for _, cidr := range c.whitelistIPs {
			if c.matchCIDR(ip, cidr) {
				return true
			}
		}
	}

	// 检查域名白名单
	if domain != "" {
		for _, wlDomain := range c.whitelistDomains {
			if strings.HasSuffix(domain, wlDomain) {
				return true
			}
		}
	}

	return false
}

// matchCIDR 简单的CIDR匹配（实际应使用net包）
func (c *ContentAnalyzer) matchCIDR(ip, cidr string) bool {
	// 简化实现：直接匹配
	if strings.Contains(cidr, "/") {
		// 处理CIDR格式
		parts := strings.Split(cidr, "/")
		if len(parts) == 2 {
			baseIP := parts[0]
			if ip == baseIP {
				return true
			}
		}
	}
	return ip == cidr
}

// SetWhitelist 设置白名单
func (c *ContentAnalyzer) SetWhitelist(ips, domains, asns, orgs []string) {
	c.whitelistIPs = append(c.whitelistIPs, ips...)
	c.whitelistDomains = append(c.whitelistDomains, domains...)
	c.whitelistASNs = append(c.whitelistASNs, asns...)
	c.whitelistOrgs = append(c.whitelistOrgs, orgs...)
}

// AddICPPattern 添加ICP正则模式
func (c *ContentAnalyzer) AddICPPattern(pattern string) error {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return err
	}
	c.icpPatterns = append(c.icpPatterns, re)
	return nil
}
