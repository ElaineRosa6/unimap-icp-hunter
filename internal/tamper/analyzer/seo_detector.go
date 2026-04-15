package analyzer

import (
	"fmt"
	"regexp"
	"strings"
)

// SEODetector SEO检测工具
type SEODetector struct {
	patterns *SEOPatterns
}

// SEOPatterns SEO检测模式
type SEOPatterns struct {
	// CSS隐藏模式
	CSSHidden []*regexp.Regexp
	// iframe隐藏模式
	IframeHidden []*regexp.Regexp
	// 链接劫持模式
	LinkHijacking []*regexp.Regexp
	// 重定向模式
	RedirectPatterns []*regexp.Regexp
	// 关键词堆砌模式
	KeywordStuffing []*regexp.Regexp
}

// SEOAnalysisResult SEO分析结果
type SEOAnalysisResult struct {
	CSSHiddenElements []ElementInfo
	HiddenIframes     []ElementInfo
	LinkHijacking     []ElementInfo
	Redirects         []RedirectInfo
	KeywordStuffing   []KeywordInfo
	SuspiciousSignals []string
}

// ElementInfo 元素信息
type ElementInfo struct {
	Type    string
	Content string
	Line    int
	Pattern string
}

// RedirectInfo 重定向信息
type RedirectInfo struct {
	Type   string
	URL    string
	Method string
	Line   int
}

// KeywordInfo 关键词信息
type KeywordInfo struct {
	Keyword string
	Count   int
	Density float64
}

// NewSEODetector 创建SEO检测器
func NewSEODetector() *SEODetector {
	return &SEODetector{
		patterns: &SEOPatterns{
			CSSHidden: []*regexp.Regexp{
				regexp.MustCompile(`display\s*:\s*none`),
				regexp.MustCompile(`visibility\s*:\s*hidden`),
				regexp.MustCompile(`opacity\s*:\s*0`),
				regexp.MustCompile(`position\s*:\s*absolute.*?left\s*:\s*-?\d+px`),
				regexp.MustCompile(`font-size\s*:\s*0`),
				regexp.MustCompile(`color\s*:\s*transparent`),
				regexp.MustCompile(`height\s*:\s*0`),
				regexp.MustCompile(`width\s*:\s*0`),
			},
			IframeHidden: []*regexp.Regexp{
				regexp.MustCompile(`<iframe[^>]*style[^>]*display\s*:\s*none[^>]*>`),
				regexp.MustCompile(`<iframe[^>]*style[^>]*visibility\s*:\s*hidden[^>]*>`),
				regexp.MustCompile(`<iframe[^>]*width\s*=\s*['"]0['"][^>]*>`),
				regexp.MustCompile(`<iframe[^>]*height\s*=\s*['"]0['"][^>]*>`),
				regexp.MustCompile(`<iframe[^>]*style[^>]*opacity\s*:\s*0[^>]*>`),
			},
			LinkHijacking: []*regexp.Regexp{
				regexp.MustCompile(`<a[^>]*href[^>]*javascript:`),
				regexp.MustCompile(`<a[^>]*onclick\s*=\s*['"][^'"]*location\.href[^'"]*['"]`),
				regexp.MustCompile(`<a[^>]*onmouseover\s*=\s*['"][^'"]*location\.href[^'"]*['"]`),
				regexp.MustCompile(`<a[^>]*onmousedown\s*=\s*['"][^'"]*location\.href[^'"]*['"]`),
			},
			RedirectPatterns: []*regexp.Regexp{
				regexp.MustCompile(`<meta[^>]*http-equiv\s*=\s*['"]refresh['"][^>]*content\s*=\s*['"][^'"]*url=([^'"]*)['"]`),
				regexp.MustCompile(`location\.href\s*=\s*['"]([^'"]*)['"]`),
				regexp.MustCompile(`window\.location\s*=\s*['"]([^'"]*)['"]`),
				regexp.MustCompile(`document\.location\s*=\s*['"]([^'"]*)['"]`),
				regexp.MustCompile(`window\.open\s*\(\s*['"]([^'"]*)['"]`),
			},
			KeywordStuffing: []*regexp.Regexp{
				regexp.MustCompile(`\b\w+\b\s+\b\w+\b\s+\b\w+\b`),
				regexp.MustCompile(`\b\w+\b[\s,.!?:;-]{1,3}\b\w+\b[\s,.!?:;-]{1,3}\b\w+\b`),
			},
		},
	}
}

// Analyze 分析HTML内容
func (d *SEODetector) Analyze(html string) (*SEOAnalysisResult, error) {
	if html == "" {
		return &SEOAnalysisResult{}, nil
	}

	result := &SEOAnalysisResult{}

	// 检测CSS隐藏元素
	d.detectCSSHidden(html, result)

	// 检测隐藏iframe
	d.detectHiddenIframes(html, result)

	// 检测链接劫持
	d.detectLinkHijacking(html, result)

	// 检测重定向
	d.detectRedirects(html, result)

	// 检测关键词堆砌
	d.detectKeywordStuffing(html, result)

	// 检测可疑信号
	d.detectSuspiciousSignals(result)

	return result, nil
}

// detectCSSHidden 检测CSS隐藏元素
func (d *SEODetector) detectCSSHidden(html string, result *SEOAnalysisResult) {
	lines := strings.Split(html, "\n")

	for i, line := range lines {
		for _, pattern := range d.patterns.CSSHidden {
			if pattern.MatchString(line) {
				match := pattern.FindString(line)
				element := ElementInfo{
					Type:    "css_hidden",
					Content: match,
					Line:    i + 1,
					Pattern: pattern.String(),
				}
				result.CSSHiddenElements = append(result.CSSHiddenElements, element)
			}
		}
	}
}

// detectHiddenIframes 检测隐藏iframe
func (d *SEODetector) detectHiddenIframes(html string, result *SEOAnalysisResult) {
	lines := strings.Split(html, "\n")

	for i, line := range lines {
		for _, pattern := range d.patterns.IframeHidden {
			if pattern.MatchString(line) {
				match := pattern.FindString(line)
				element := ElementInfo{
					Type:    "hidden_iframe",
					Content: match,
					Line:    i + 1,
					Pattern: pattern.String(),
				}
				result.HiddenIframes = append(result.HiddenIframes, element)
			}
		}
	}
}

// detectLinkHijacking 检测链接劫持
func (d *SEODetector) detectLinkHijacking(html string, result *SEOAnalysisResult) {
	lines := strings.Split(html, "\n")

	for i, line := range lines {
		for _, pattern := range d.patterns.LinkHijacking {
			if pattern.MatchString(line) {
				match := pattern.FindString(line)
				element := ElementInfo{
					Type:    "link_hijacking",
					Content: match,
					Line:    i + 1,
					Pattern: pattern.String(),
				}
				result.LinkHijacking = append(result.LinkHijacking, element)
			}
		}
	}
}

// detectRedirects 检测重定向
func (d *SEODetector) detectRedirects(html string, result *SEOAnalysisResult) {
	lines := strings.Split(html, "\n")

	for i, line := range lines {
		// 检测meta refresh重定向
		matches := d.patterns.RedirectPatterns[0].FindStringSubmatch(line)
		if matches != nil && len(matches) >= 2 {
			redirect := RedirectInfo{
				Type:   "meta_refresh",
				URL:    matches[1],
				Method: "meta",
				Line:   i + 1,
			}
			result.Redirects = append(result.Redirects, redirect)
			continue
		}

		// 检测JavaScript重定向
		for j := 1; j < len(d.patterns.RedirectPatterns); j++ {
			matches := d.patterns.RedirectPatterns[j].FindStringSubmatch(line)
			if matches != nil && len(matches) >= 2 {
				var redirectType string
				switch j {
				case 1:
					redirectType = "location_href"
				case 2:
					redirectType = "window_location"
				case 3:
					redirectType = "document_location"
				case 4:
					redirectType = "window_open"
				default:
					redirectType = "javascript_redirect"
				}

				redirect := RedirectInfo{
					Type:   redirectType,
					URL:    matches[1],
					Method: "javascript",
					Line:   i + 1,
				}
				result.Redirects = append(result.Redirects, redirect)
				break
			}
		}
	}
}

// detectKeywordStuffing 检测关键词堆砌
func (d *SEODetector) detectKeywordStuffing(html string, result *SEOAnalysisResult) {
	// 简化处理：检测重复关键词
	words := regexp.MustCompile(`\b\w+\b`).FindAllString(strings.ToLower(html), -1)
	wordCount := make(map[string]int)

	for _, word := range words {
		if len(word) > 3 { // 只统计长度大于3的单词
			wordCount[word]++
		}
	}

	totalWords := len(words)
	if totalWords > 0 {
		for word, count := range wordCount {
			density := float64(count) / float64(totalWords) * 100
			if count > 5 && density > 3.0 { // 出现次数超过5次且密度超过3%
				keyword := KeywordInfo{
					Keyword: word,
					Count:   count,
					Density: density,
				}
				result.KeywordStuffing = append(result.KeywordStuffing, keyword)
			}
		}
	}

	// 检测连续重复的关键词（简化处理）
	lines := strings.Split(html, "\n")
	for _, line := range lines {
		words := regexp.MustCompile(`\b\w+\b`).FindAllString(line, -1)
		// 检查连续重复的单词
		for i := 0; i < len(words)-2; i++ {
			if words[i] == words[i+1] && words[i] == words[i+2] {
				result.KeywordStuffing = append(result.KeywordStuffing, KeywordInfo{
					Keyword: words[i],
					Count:   3,
					Density: 0,
				})
				break
			}
		}
	}
}

// detectSuspiciousSignals 检测可疑信号
func (d *SEODetector) detectSuspiciousSignals(result *SEOAnalysisResult) {
	// 检测CSS隐藏元素
	if len(result.CSSHiddenElements) > 0 {
		result.SuspiciousSignals = append(result.SuspiciousSignals, fmt.Sprintf("css_hidden_elements:%d", len(result.CSSHiddenElements)))
	}

	// 检测隐藏iframe
	if len(result.HiddenIframes) > 0 {
		result.SuspiciousSignals = append(result.SuspiciousSignals, fmt.Sprintf("hidden_iframes:%d", len(result.HiddenIframes)))
	}

	// 检测链接劫持
	if len(result.LinkHijacking) > 0 {
		result.SuspiciousSignals = append(result.SuspiciousSignals, fmt.Sprintf("link_hijacking:%d", len(result.LinkHijacking)))
	}

	// 检测重定向
	if len(result.Redirects) > 0 {
		result.SuspiciousSignals = append(result.SuspiciousSignals, fmt.Sprintf("redirects:%d", len(result.Redirects)))
	}

	// 检测关键词堆砌
	if len(result.KeywordStuffing) > 0 {
		result.SuspiciousSignals = append(result.SuspiciousSignals, fmt.Sprintf("keyword_stuffing:%d", len(result.KeywordStuffing)))
	}

	// 综合评估
	totalSignals := len(result.SuspiciousSignals)
	if totalSignals >= 3 {
		result.SuspiciousSignals = append(result.SuspiciousSignals, "high_seo_risk")
	} else if totalSignals >= 1 {
		result.SuspiciousSignals = append(result.SuspiciousSignals, "medium_seo_risk")
	}
}

// GetRiskLevel 获取风险级别
func (d *SEODetector) GetRiskLevel(result *SEOAnalysisResult) int {
	level := 0

	// 根据可疑信号计算级别
	for _, signal := range result.SuspiciousSignals {
		switch {
		case strings.HasPrefix(signal, "hidden_iframes"):
			level += 5
		case strings.HasPrefix(signal, "link_hijacking"):
			level += 4
		case strings.HasPrefix(signal, "redirects"):
			level += 3
		case strings.HasPrefix(signal, "css_hidden_elements"):
			level += 2
		case strings.HasPrefix(signal, "keyword_stuffing"):
			level += 1
		case signal == "high_seo_risk":
			level += 10
		case signal == "medium_seo_risk":
			level += 5
		}
	}

	return level
}

// IsSuspicious 判断是否可疑
func (d *SEODetector) IsSuspicious(result *SEOAnalysisResult) bool {
	return d.GetRiskLevel(result) >= 5
}
