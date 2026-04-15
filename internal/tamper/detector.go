package tamper

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/unimap-icp-hunter/project/internal/alerting"
	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/utils"
	"github.com/unimap-icp-hunter/project/internal/utils/workerpool"
)

const (
	SegmentHead        = "head"
	SegmentBody        = "body"
	SegmentHeader      = "header"
	SegmentNav         = "nav"
	SegmentMain        = "main"
	SegmentArticle     = "article"
	SegmentSection     = "section"
	SegmentAside       = "aside"
	SegmentFooter      = "footer"
	SegmentScripts     = "scripts"
	SegmentStyles      = "styles"
	SegmentMeta        = "meta"
	SegmentLinks       = "links"
	SegmentImages      = "images"
	SegmentJSFiles     = "js_files"
	SegmentFavicon     = "favicon"
	SegmentButtons     = "buttons"
	SegmentForms       = "forms"
	SegmentFullContent = "full_content"

	DetectionModeRelaxed  = "relaxed"
	DetectionModeStrict   = "strict"
	DetectionModeSecurity = "security"
	DetectionModeBalanced = "balanced"
	DetectionModePrecise  = "precise"

	PerformanceModeFast          = "fast"
	PerformanceModeBalanced      = "balanced"
	PerformanceModeComprehensive = "comprehensive"
)

var relaxedVolatileSegments = map[string]struct{}{
	SegmentHead:        {},
	SegmentBody:        {},
	SegmentHeader:      {},
	SegmentNav:         {},
	SegmentFooter:      {},
	SegmentLinks:       {},
	SegmentScripts:     {},
	SegmentStyles:      {},
	SegmentMeta:        {},
	SegmentFullContent: {},
}

var compatibilityOptionalSegments = map[string]struct{}{
	SegmentJSFiles: {},
	SegmentFavicon: {},
	SegmentButtons: {},
}

var (
	reMultipleSpaces = regexp.MustCompile(`(?i)\s+`)
	reComments       = regexp.MustCompile(`(?i)<!--.*?-->`)
	reDataImages     = regexp.MustCompile(`(?i)data:image/[^"']*`)
	reNonce          = regexp.MustCompile(`(?i)nonce="[^"]*"`)
	reCSRFToken      = regexp.MustCompile(`(?i)csrf[^"]*_token["']?\s*[:=]\s*["'][^"']*["']`)
)

var (
	maliciousScriptKeywords = []string{
		"eval(",
		"document.write(",
		"Function(",
		"atob(",
		"btoa(",
		"unescape(",
		"decodeURIComponent(",
		"String.fromCharCode",
		"crypto",
		"miner",
		"coin-hive",
		"coinhive",
		"cryptonight",
	}

	suspiciousDomainKeywords = []string{
		"xxx",
		"porn",
		"sex",
		"adult",
		"casino",
		"gambling",
		"bet",
		"lottery",
		"crypto",
		"bitcoin",
		"mining",
		"coin-hive",
		"coinhive",
		"cryptonight",
	}

	suspiciousPathKeywords = []string{
		"shell",
		"backdoor",
		"webshell",
		"hacked",
		"deface",
		"phishing",
		"fake",
		"login",
		"admin",
	}

	hiddenIframePattern = regexp.MustCompile(`(?i)<iframe[^>]*style\s*=\s*["'][^"']*(display\s*:\s*none|visibility\s*:\s*hidden|width\s*:\s*0|height\s*:\s*0)[^"']*["']`)

	// 危险的事件处理器模式
	dangerousEventPattern = regexp.MustCompile(`(?i)on(?:error|load|mouseover|click|keyup|keydown|submit)\s*=\s*["'][^"']*(?:eval\(|document\.write\(|Function\(|atob\(|btoa\(|unescape\(|decodeURIComponent\(|String\.fromCharCode\(|crypto|miner|coin-hive|coinhive|cryptonight)[^"']*["']`)
)

func detectMaliciousContent(html string) []string {
	var flags []string
	htmlLower := strings.ToLower(html)

	// 检测恶意脚本关键词
	for _, keyword := range maliciousScriptKeywords {
		if strings.Contains(htmlLower, strings.ToLower(keyword)) {
			flags = append(flags, fmt.Sprintf("suspicious_script: %s", keyword))
		}
	}

	// 检测可疑域名关键词（需要多个关键词同时出现才标记）
	domainMatches := 0
	for _, keyword := range suspiciousDomainKeywords {
		if strings.Contains(htmlLower, strings.ToLower(keyword)) {
			domainMatches++
			// 只有当多个域名关键词同时出现时才标记
			if domainMatches >= 2 {
				flags = append(flags, fmt.Sprintf("suspicious_domain_keywords: multiple matches"))
				break
			}
		}
	}

	// 检测可疑路径关键词（需要多个关键词同时出现才标记）
	pathMatches := 0
	for _, keyword := range suspiciousPathKeywords {
		if strings.Contains(htmlLower, strings.ToLower(keyword)) {
			pathMatches++
			// 只有当多个路径关键词同时出现时才标记
			if pathMatches >= 2 {
				flags = append(flags, fmt.Sprintf("suspicious_path_keywords: multiple matches"))
				break
			}
		}
	}

	// 检测隐藏的iframe（这是一个强信号）
	if hiddenIframePattern.MatchString(html) {
		flags = append(flags, "hidden_iframe_detected")
	}

	// 检测危险的事件处理器（包含恶意代码的事件）
	if dangerousEventPattern.MatchString(html) {
		flags = append(flags, "dangerous_event_handler")
	}

	return flags
}

type SegmentHash struct {
	Name     string `json:"name"`
	Hash     string `json:"hash"`
	Content  string `json:"content,omitempty"`
	Length   int    `json:"length"`
	Elements int    `json:"elements"`
}

type PageHashResult struct {
	URL           string        `json:"url"`
	Title         string        `json:"title"`
	FullHash      string        `json:"full_hash"`
	SimpleMD5Hash string        `json:"simple_md5_hash"`
	SegmentHashes []SegmentHash `json:"segment_hashes"`
	Timestamp     int64         `json:"timestamp"`
	HTMLLength    int           `json:"html_length"`
	Status        string        `json:"status"`
	RawHTML       string        `json:"-"` // 不序列化到JSON，仅用于内存检测
}

type TamperCheckResult struct {
	URL              string          `json:"url"`
	CurrentHash      *PageHashResult `json:"current_hash"`
	BaselineHash     *PageHashResult `json:"baseline_hash,omitempty"`
	Tampered         bool            `json:"tampered"`
	Status           string          `json:"status"` // no_baseline | unreachable | tampered | normal | suspicious
	ErrorType        string          `json:"error_type,omitempty"`
	ErrorMessage     string          `json:"error_message,omitempty"`
	TamperedSegments []string        `json:"tampered_segments,omitempty"`
	Changes          []SegmentChange `json:"changes,omitempty"`
	SuspiciousFlags  []string        `json:"suspicious_flags,omitempty"` // 可疑内容标记
	Timestamp        int64           `json:"timestamp"`
}

type SegmentChange struct {
	Segment     string `json:"segment"`
	OldHash     string `json:"old_hash"`
	NewHash     string `json:"new_hash"`
	ChangeType  string `json:"change_type"`
	Description string `json:"description"`
}

type HashStorage struct {
	baseDir string
	mu      sync.RWMutex
}

type cacheEntry struct {
	result    *PageHashResult
	timestamp time.Time
}

type Detector struct {
	storage         *HashStorage
	allocCtx        context.Context
	allocCancel     context.CancelFunc
	detectionMode   string
	performanceMode string
	alertManager    *alerting.Manager
	cache           map[string]*cacheEntry
	cacheMu         sync.RWMutex
	mu              sync.Mutex
}

type DetectorConfig struct {
	BaseDir         string
	DetectionMode   string
	PerformanceMode string
	AlertManager    *alerting.Manager
}

func NewHashStorage(baseDir string) *HashStorage {
	if baseDir == "" {
		baseDir = "./hash_store"
	}
	return &HashStorage{baseDir: baseDir}
}

func (s *HashStorage) SaveBaseline(url string, result *PageHashResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.baseDir, 0755); err != nil {
		return fmt.Errorf("failed to create hash store directory: %w", err)
	}

	safeFilename := sanitizeFilenameForStorage(url)
	filePath := filepath.Join(s.baseDir, safeFilename+".json")

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal hash result: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to save baseline: %w", err)
	}

	logger.Infof("Saved baseline hash for %s to %s", url, filePath)
	return nil
}

func (s *HashStorage) LoadBaseline(url string) (*PageHashResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	safeFilename := sanitizeFilenameForStorage(url)
	filePath := filepath.Join(s.baseDir, safeFilename+".json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("baseline not found for %s: %w", url, err)
	}

	var result PageHashResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal baseline: %w", err)
	}

	return &result, nil
}

func (s *HashStorage) HasBaseline(url string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	safeFilename := sanitizeFilenameForStorage(url)
	filePath := filepath.Join(s.baseDir, safeFilename+".json")

	_, err := os.Stat(filePath)
	return err == nil
}

func (s *HashStorage) ListBaselines() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	files, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, err
	}

	var urls []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".json") {
			filePath := filepath.Join(s.baseDir, file.Name())
			data, readErr := os.ReadFile(filePath)
			if readErr != nil {
				continue
			}

			var result PageHashResult
			if unmarshalErr := json.Unmarshal(data, &result); unmarshalErr != nil {
				continue
			}

			if strings.TrimSpace(result.URL) != "" {
				urls = append(urls, result.URL)
			}
		}
	}
	sort.Strings(urls)
	return urls, nil
}

func (s *HashStorage) DeleteBaseline(url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	safeFilename := sanitizeFilenameForStorage(url)
	filePath := filepath.Join(s.baseDir, safeFilename+".json")

	return os.Remove(filePath)
}

func NewDetector(cfg DetectorConfig) *Detector {
	storage := NewHashStorage(cfg.BaseDir)
	return &Detector{
		storage:         storage,
		detectionMode:   normalizeDetectionMode(cfg.DetectionMode),
		performanceMode: normalizePerformanceMode(cfg.PerformanceMode),
		alertManager:    cfg.AlertManager,
		cache:           make(map[string]*cacheEntry),
	}
}

func normalizeDetectionMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case DetectionModeStrict:
		return DetectionModeStrict
	case DetectionModeSecurity:
		return DetectionModeSecurity
	case DetectionModeBalanced:
		return DetectionModeBalanced
	case DetectionModePrecise:
		return DetectionModePrecise
	default:
		return DetectionModeRelaxed
	}
}

func normalizePerformanceMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case PerformanceModeFast:
		return PerformanceModeFast
	case PerformanceModeBalanced:
		return PerformanceModeBalanced
	case PerformanceModeComprehensive:
		return PerformanceModeComprehensive
	default:
		return PerformanceModeBalanced
	}
}

func (d *Detector) SetAllocator(ctx context.Context, allocCtx context.Context, allocCancel context.CancelFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.allocCtx = allocCtx
	d.allocCancel = allocCancel
}

func (d *Detector) ComputePageHash(ctx context.Context, targetURL string) (*PageHashResult, error) {
	cacheKey := fmt.Sprintf("%s:%s", targetURL, d.performanceMode)

	d.cacheMu.RLock()
	if entry, exists := d.cache[cacheKey]; exists {
		if time.Since(entry.timestamp) < 5*time.Minute {
			d.cacheMu.RUnlock()
			logger.CtxDebugf(ctx, "Using cached hash result for %s", targetURL)
			return entry.result, nil
		}
	}
	d.cacheMu.RUnlock()

	var html string
	var title string

	if d.performanceMode == PerformanceModeFast {
		result, err := d.computeHashWithHTTP(ctx, targetURL)
		if err == nil {
			d.cacheMu.Lock()
			d.cache[cacheKey] = &cacheEntry{
				result:    result,
				timestamp: time.Now(),
			}
			d.cacheMu.Unlock()
			return result, nil
		}
		logger.CtxWarnf(ctx, "Fast mode failed, falling back to chromedp: %v", err)
	}

	runCtx := ctx
	runCancel := func() {}
	if chromedp.FromContext(runCtx) == nil {
		d.mu.Lock()
		allocCtx := d.allocCtx
		d.mu.Unlock()
		if allocCtx == nil {
			allocCtx = context.Background()
		}
		runCtx, runCancel = chromedp.NewContext(allocCtx)
	}
	defer runCancel()

	timeoutCtx, timeoutCancel := context.WithTimeout(runCtx, 45*time.Second)
	defer timeoutCancel()

	if err := chromedp.Run(timeoutCtx,
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Title(&title),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	); err != nil {
		return nil, fmt.Errorf("failed to load page: %w", err)
	}

	result, err := d.ComputeHashFromHTML(targetURL, title, html)
	if err == nil {
		d.cacheMu.Lock()
		d.cache[cacheKey] = &cacheEntry{
			result:    result,
			timestamp: time.Now(),
		}
		d.cacheMu.Unlock()
	}

	return result, err
}

func (d *Detector) computeHashWithHTTP(ctx context.Context, targetURL string) (*PageHashResult, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client := utils.DefaultHTTPClient()

	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	html := string(bodyBytes)

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	title := doc.Find("title").Text()
	if title == "" {
		title = targetURL
	}

	return d.ComputeHashFromHTML(targetURL, title, html)
}

func (d *Detector) ComputeHashFromHTML(url, title, html string) (*PageHashResult, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	result := &PageHashResult{
		URL:        url,
		Title:      title,
		Timestamp:  time.Now().Unix(),
		HTMLLength: len(html),
		Status:     "success",
		RawHTML:    html,
	}

	segmentHashes := d.computeSegmentHashes(doc, html)
	result.SegmentHashes = segmentHashes

	result.FullHash = d.computeFullHash(segmentHashes)
	result.SimpleMD5Hash = computeSimpleMD5Hash(html)

	return result, nil
}

func computeSimpleMD5Hash(html string) string {
	headerEnd := strings.Index(strings.ToLower(html), "</head>")
	if headerEnd == -1 {
		headerEnd = strings.Index(html, "<body")
		if headerEnd == -1 {
			headerEnd = len(html) / 2
		}
	}

	headerPart := html[:headerEnd]
	bodyPart := html[headerEnd:]

	combined := headerPart + "\r\n\r\n" + bodyPart
	hash := md5.Sum([]byte(combined))
	return hex.EncodeToString(hash[:])
}

type segmentTask struct {
	name     string
	hashFunc func() SegmentHash
}

func (d *Detector) computeSegmentHashes(doc *goquery.Document, html string) []SegmentHash {
	var tasks []segmentTask

	switch d.performanceMode {
	case PerformanceModeFast:
		tasks = []segmentTask{
			{name: SegmentScripts, hashFunc: func() SegmentHash { return d.computeScriptHash(doc) }},
			{name: SegmentJSFiles, hashFunc: func() SegmentHash { return d.computeJSFileHash(doc) }},
			{name: SegmentForms, hashFunc: func() SegmentHash { return d.computeFormHash(doc) }},
			{name: SegmentMain, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "main", SegmentMain) }},
			{name: SegmentArticle, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "article", SegmentArticle) }},
		}

	case PerformanceModeBalanced:
		tasks = []segmentTask{
			{name: SegmentScripts, hashFunc: func() SegmentHash { return d.computeScriptHash(doc) }},
			{name: SegmentJSFiles, hashFunc: func() SegmentHash { return d.computeJSFileHash(doc) }},
			{name: SegmentForms, hashFunc: func() SegmentHash { return d.computeFormHash(doc) }},
			{name: SegmentLinks, hashFunc: func() SegmentHash { return d.computeLinkHash(doc) }},
			{name: SegmentMain, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "main", SegmentMain) }},
			{name: SegmentArticle, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "article", SegmentArticle) }},
			{name: SegmentBody, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "body", SegmentBody) }},
		}

	default:
		tasks = []segmentTask{
			{name: SegmentHead, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "head", SegmentHead) }},
			{name: SegmentBody, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "body", SegmentBody) }},
			{name: SegmentHeader, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "header", SegmentHeader) }},
			{name: SegmentNav, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "nav", SegmentNav) }},
			{name: SegmentMain, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "main", SegmentMain) }},
			{name: SegmentArticle, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "article", SegmentArticle) }},
			{name: SegmentSection, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "section", SegmentSection) }},
			{name: SegmentAside, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "aside", SegmentAside) }},
			{name: SegmentFooter, hashFunc: func() SegmentHash { return d.computeElementHash(doc, "footer", SegmentFooter) }},
			{name: SegmentScripts, hashFunc: func() SegmentHash { return d.computeScriptHash(doc) }},
			{name: SegmentJSFiles, hashFunc: func() SegmentHash { return d.computeJSFileHash(doc) }},
			{name: SegmentStyles, hashFunc: func() SegmentHash { return d.computeStyleHash(doc) }},
			{name: SegmentMeta, hashFunc: func() SegmentHash { return d.computeMetaHash(doc) }},
			{name: SegmentFavicon, hashFunc: func() SegmentHash { return d.computeFaviconHash(doc) }},
			{name: SegmentLinks, hashFunc: func() SegmentHash { return d.computeLinkHash(doc) }},
			{name: SegmentImages, hashFunc: func() SegmentHash { return d.computeImageHash(doc) }},
			{name: SegmentButtons, hashFunc: func() SegmentHash { return d.computeButtonHash(doc) }},
			{name: SegmentForms, hashFunc: func() SegmentHash { return d.computeFormHash(doc) }},
		}
	}

	resultChan := make(chan SegmentHash, len(tasks))
	var wg sync.WaitGroup

	for _, task := range tasks {
		wg.Add(1)
		go func(t segmentTask) {
			defer wg.Done()
			resultChan <- t.hashFunc()
		}(task)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var segments []SegmentHash
	for segment := range resultChan {
		segments = append(segments, segment)
	}

	if d.performanceMode == PerformanceModeComprehensive {
		cleanHTML := d.cleanHTML(html)
		fullContentHash := SegmentHash{
			Name:     SegmentFullContent,
			Hash:     computeSHA256(cleanHTML),
			Length:   len(cleanHTML),
			Elements: 1,
		}
		segments = append(segments, fullContentHash)
	}

	return segments
}

func (d *Detector) computeElementHash(doc *goquery.Document, selector, segmentName string) SegmentHash {
	selection := doc.Find(selector)
	content, _ := selection.Html()

	cleanContent := d.cleanHTML(content)
	hash := computeSHA256(cleanContent)

	elementCount := selection.Length()

	return SegmentHash{
		Name:     segmentName,
		Hash:     hash,
		Length:   len(cleanContent),
		Elements: elementCount,
	}
}

func (d *Detector) computeScriptHash(doc *goquery.Document) SegmentHash {
	var scripts []string
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		src, _ := s.Attr("src")
		integrity, _ := s.Attr("integrity")
		async, _ := s.Attr("async")
		deferAttr, _ := s.Attr("defer")
		content := s.Text()
		scripts = append(scripts, strings.Join([]string{src, integrity, async, deferAttr, content}, ":"))
	})

	sort.Strings(scripts)
	combined := strings.Join(scripts, "|")

	return SegmentHash{
		Name:     SegmentScripts,
		Hash:     computeSHA256(combined),
		Length:   len(combined),
		Elements: len(scripts),
	}
}

func (d *Detector) computeJSFileHash(doc *goquery.Document) SegmentHash {
	var jsFiles []string
	doc.Find("script[src]").Each(func(i int, s *goquery.Selection) {
		src, _ := s.Attr("src")
		integrity, _ := s.Attr("integrity")
		crossorigin, _ := s.Attr("crossorigin")
		referrerpolicy, _ := s.Attr("referrerpolicy")
		jsFiles = append(jsFiles, strings.Join([]string{src, integrity, crossorigin, referrerpolicy}, ":"))
	})

	sort.Strings(jsFiles)
	combined := strings.Join(jsFiles, "|")

	return SegmentHash{
		Name:     SegmentJSFiles,
		Hash:     computeSHA256(combined),
		Length:   len(combined),
		Elements: len(jsFiles),
	}
}

func (d *Detector) computeStyleHash(doc *goquery.Document) SegmentHash {
	var styles []string
	doc.Find("style").Each(func(i int, s *goquery.Selection) {
		styles = append(styles, s.Text())
	})
	doc.Find("link[rel='stylesheet']").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		styles = append(styles, href)
	})

	sort.Strings(styles)
	combined := strings.Join(styles, "|")

	return SegmentHash{
		Name:     SegmentStyles,
		Hash:     computeSHA256(combined),
		Length:   len(combined),
		Elements: len(styles),
	}
}

func (d *Detector) computeMetaHash(doc *goquery.Document) SegmentHash {
	var metas []string
	doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		name, _ := s.Attr("name")
		content, _ := s.Attr("content")
		property, _ := s.Attr("property")
		metas = append(metas, fmt.Sprintf("%s:%s:%s", name, property, content))
	})

	sort.Strings(metas)
	combined := strings.Join(metas, "|")

	return SegmentHash{
		Name:     SegmentMeta,
		Hash:     computeSHA256(combined),
		Length:   len(combined),
		Elements: len(metas),
	}
}

func (d *Detector) computeFaviconHash(doc *goquery.Document) SegmentHash {
	var icons []string
	doc.Find("link").Each(func(i int, s *goquery.Selection) {
		rel, _ := s.Attr("rel")
		relLower := strings.ToLower(rel)
		if !strings.Contains(relLower, "icon") {
			return
		}
		href, _ := s.Attr("href")
		typ, _ := s.Attr("type")
		sizes, _ := s.Attr("sizes")
		icons = append(icons, strings.Join([]string{relLower, href, typ, sizes}, ":"))
	})

	sort.Strings(icons)
	combined := strings.Join(icons, "|")

	return SegmentHash{
		Name:     SegmentFavicon,
		Hash:     computeSHA256(combined),
		Length:   len(combined),
		Elements: len(icons),
	}
}

func (d *Detector) computeLinkHash(doc *goquery.Document) SegmentHash {
	var links []string
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		text := s.Text()
		links = append(links, fmt.Sprintf("%s:%s", href, text))
	})

	sort.Strings(links)
	combined := strings.Join(links, "|")

	return SegmentHash{
		Name:     SegmentLinks,
		Hash:     computeSHA256(combined),
		Length:   len(combined),
		Elements: len(links),
	}
}

func (d *Detector) computeImageHash(doc *goquery.Document) SegmentHash {
	var images []string
	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		src, _ := s.Attr("src")
		srcset, _ := s.Attr("srcset")
		alt, _ := s.Attr("alt")
		width, _ := s.Attr("width")
		height, _ := s.Attr("height")
		loading, _ := s.Attr("loading")
		decoding, _ := s.Attr("decoding")
		images = append(images, strings.Join([]string{src, srcset, alt, width, height, loading, decoding}, ":"))
	})

	sort.Strings(images)
	combined := strings.Join(images, "|")

	return SegmentHash{
		Name:     SegmentImages,
		Hash:     computeSHA256(combined),
		Length:   len(combined),
		Elements: len(images),
	}
}

func (d *Detector) computeButtonHash(doc *goquery.Document) SegmentHash {
	var buttons []string

	doc.Find("button").Each(func(i int, s *goquery.Selection) {
		typ, _ := s.Attr("type")
		id, _ := s.Attr("id")
		class, _ := s.Attr("class")
		name, _ := s.Attr("name")
		ariaLabel, _ := s.Attr("aria-label")
		text := strings.TrimSpace(s.Text())
		buttons = append(buttons, strings.Join([]string{"button", typ, id, class, name, ariaLabel, text}, ":"))
	})

	doc.Find("input[type='button'], input[type='submit'], input[type='reset']").Each(func(i int, s *goquery.Selection) {
		typ, _ := s.Attr("type")
		id, _ := s.Attr("id")
		class, _ := s.Attr("class")
		name, _ := s.Attr("name")
		value, _ := s.Attr("value")
		buttons = append(buttons, strings.Join([]string{"input", typ, id, class, name, value}, ":"))
	})

	doc.Find("a[role='button']").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		id, _ := s.Attr("id")
		class, _ := s.Attr("class")
		ariaLabel, _ := s.Attr("aria-label")
		text := strings.TrimSpace(s.Text())
		buttons = append(buttons, strings.Join([]string{"anchor", href, id, class, ariaLabel, text}, ":"))
	})

	sort.Strings(buttons)
	combined := strings.Join(buttons, "|")

	return SegmentHash{
		Name:     SegmentButtons,
		Hash:     computeSHA256(combined),
		Length:   len(combined),
		Elements: len(buttons),
	}
}

func (d *Detector) computeFormHash(doc *goquery.Document) SegmentHash {
	var forms []string
	doc.Find("form").Each(func(i int, s *goquery.Selection) {
		action, _ := s.Attr("action")
		method, _ := s.Attr("method")
		forms = append(forms, fmt.Sprintf("%s:%s", action, method))

		s.Find("input, select, textarea").Each(func(i int, field *goquery.Selection) {
			name, _ := field.Attr("name")
			inputType, _ := field.Attr("type")
			forms = append(forms, fmt.Sprintf("field:%s:%s", name, inputType))
		})
	})

	sort.Strings(forms)
	combined := strings.Join(forms, "|")

	return SegmentHash{
		Name:     SegmentForms,
		Hash:     computeSHA256(combined),
		Length:   len(combined),
		Elements: len(forms),
	}
}

func (d *Detector) computeFullHash(segments []SegmentHash) string {
	var hashes []string
	for _, seg := range segments {
		hashes = append(hashes, fmt.Sprintf("%s:%s", seg.Name, seg.Hash))
	}
	sort.Strings(hashes)
	return computeSHA256(strings.Join(hashes, "|"))
}

func (d *Detector) cleanHTML(html string) string {
	if html == "" {
		return ""
	}

	html = reMultipleSpaces.ReplaceAllString(html, " ")
	html = reComments.ReplaceAllString(html, "")
	html = reDataImages.ReplaceAllString(html, "DATA_IMAGE_REMOVED")
	html = reNonce.ReplaceAllString(html, "")
	html = reCSRFToken.ReplaceAllString(html, "")
	html = strings.TrimSpace(html)

	return html
}

func (d *Detector) SaveBaseline(url string, result *PageHashResult) error {
	return d.storage.SaveBaseline(url, result)
}

func (d *Detector) LoadBaseline(url string) (*PageHashResult, error) {
	return d.storage.LoadBaseline(url)
}

func (d *Detector) HasBaseline(url string) bool {
	return d.storage.HasBaseline(url)
}

func (d *Detector) CheckTampering(ctx context.Context, url string) (*TamperCheckResult, error) {
	currentHash, err := d.ComputePageHash(ctx, url)
	if err != nil {
		return nil, err
	}

	baseline, err := d.storage.LoadBaseline(url)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			result := &TamperCheckResult{
				URL:          url,
				CurrentHash:  currentHash,
				Tampered:     false,
				Status:       "failed",
				ErrorType:    "baseline",
				ErrorMessage: fmt.Sprintf("failed to load baseline: %v", err),
				Timestamp:    time.Now().Unix(),
			}

			record := &CheckRecord{
				ID:            fmt.Sprintf("%d", time.Now().UnixNano()),
				URL:           url,
				Tampered:      false,
				CurrentHash:   currentHash,
				Timestamp:     result.Timestamp,
				CheckType:     "baseline_error",
				DetectionMode: d.detectionMode,
			}
			if saveErr := d.storage.SaveCheckRecord(url, record); saveErr != nil {
				logger.Warnf("Failed to save check record: %v", saveErr)
			}

			return result, nil
		}

		// 没有基线，首次检测
		result := &TamperCheckResult{
			URL:         url,
			CurrentHash: currentHash,
			Tampered:    false,
			Status:      "no_baseline",
			Timestamp:   time.Now().Unix(),
		}

		// 保存首次检测记录
		record := &CheckRecord{
			ID:            fmt.Sprintf("%d", time.Now().UnixNano()),
			URL:           url,
			Tampered:      false,
			CurrentHash:   currentHash,
			Timestamp:     result.Timestamp,
			CheckType:     "first_check",
			DetectionMode: d.detectionMode,
		}
		if saveErr := d.storage.SaveCheckRecord(url, record); saveErr != nil {
			logger.Warnf("Failed to save check record: %v", saveErr)
		}

		return result, nil
	}

	result := &TamperCheckResult{
		URL:          url,
		CurrentHash:  currentHash,
		BaselineHash: baseline,
		Tampered:     false,
		Status:       "normal",
		Timestamp:    time.Now().Unix(),
	}

	checkType := "normal"

	suspiciousFlags := detectMaliciousContent(currentHash.RawHTML)
	result.SuspiciousFlags = suspiciousFlags

	// 根据检测模式决定是否基于恶意内容判定篡改
	if len(suspiciousFlags) > 0 {
		// 根据检测模式确定是否判定为篡改
		shouldMarkAsTampered := false

		switch d.detectionMode {
		case DetectionModeStrict:
			// 严格模式：只要有可疑内容就标记为篡改
			shouldMarkAsTampered = true
		case DetectionModeSecurity:
			// 安全模式：需要多个可疑特征或强信号才标记为篡改
			strongSignals := 0
			for _, flag := range suspiciousFlags {
				if flag == "hidden_iframe_detected" || flag == "dangerous_event_handler" {
					strongSignals++
				}
			}
			// 强信号或者多个可疑特征
			shouldMarkAsTampered = strongSignals > 0 || len(suspiciousFlags) >= 2
		case DetectionModePrecise:
			// 精确模式：只在有强信号时才标记为篡改
			for _, flag := range suspiciousFlags {
				if flag == "hidden_iframe_detected" || flag == "dangerous_event_handler" {
					shouldMarkAsTampered = true
					break
				}
			}
		case DetectionModeBalanced:
			// 平衡模式：需要多个可疑特征才标记为篡改
			shouldMarkAsTampered = len(suspiciousFlags) >= 2
		default: // DetectionModeRelaxed
			// 宽松模式：只在有强信号时才标记为篡改
			for _, flag := range suspiciousFlags {
				if flag == "hidden_iframe_detected" || flag == "dangerous_event_handler" {
					shouldMarkAsTampered = true
					break
				}
			}
		}

		if shouldMarkAsTampered {
			result.Tampered = true
			result.Status = "suspicious"
			result.TamperedSegments = []string{"malicious_content"}
			checkType = "suspicious"

			// 触发恶意内容告警
			if d.alertManager != nil {
				details := map[string]interface{}{
					"flags":          suspiciousFlags,
					"detection_mode": d.detectionMode,
				}
				d.alertManager.SendCritical(alerting.AlertTypeTamper,
					"检测到恶意内容",
					fmt.Sprintf("URL %s 检测到恶意内容", url),
					details,
					"tamper_detector",
					url)
			}
		}
	} else {
		simpleMD5Changed := false
		if baseline.SimpleMD5Hash != "" && currentHash.SimpleMD5Hash != "" {
			simpleMD5Changed = baseline.SimpleMD5Hash != currentHash.SimpleMD5Hash
		}

		tamperedSegments, changes := d.findChangedSegments(currentHash, baseline)
		result.TamperedSegments = tamperedSegments
		result.Changes = changes

		if simpleMD5Changed || len(changes) > 0 {
			if simpleMD5Changed || d.isMeaningfulTamper(changes) {
				result.Tampered = true
				result.Status = "tampered"
				checkType = "tampered"

				// 触发篡改告警
				if d.alertManager != nil {
					details := map[string]interface{}{
						"segments":           tamperedSegments,
						"changes":            len(changes),
						"detection_mode":     d.detectionMode,
						"simple_md5_changed": simpleMD5Changed,
					}
					d.alertManager.SendWarning(alerting.AlertTypeTamper,
						"检测到网页篡改",
						fmt.Sprintf("URL %s 检测到 %d 个分段被修改", url, len(tamperedSegments)),
						details,
						"tamper_detector",
						url)
				}
			} else {
				checkType = "normal_dynamic"
			}
		}
	}

	// 保存检测记录
	record := &CheckRecord{
		ID:               fmt.Sprintf("%d", time.Now().UnixNano()),
		URL:              url,
		Tampered:         result.Tampered,
		TamperedSegments: result.TamperedSegments,
		Changes:          result.Changes,
		CurrentHash:      currentHash,
		BaselineHash:     baseline,
		Timestamp:        result.Timestamp,
		CheckType:        checkType,
		DetectionMode:    d.detectionMode,
	}
	if saveErr := d.storage.SaveCheckRecord(url, record); saveErr != nil {
		logger.Warnf("Failed to save check record: %v", saveErr)
	}

	return result, nil
}

func (d *Detector) findChangedSegments(current, baseline *PageHashResult) ([]string, []SegmentChange) {
	var tamperedSegments []string
	var changes []SegmentChange

	currentMap := make(map[string]SegmentHash)
	for _, seg := range current.SegmentHashes {
		currentMap[seg.Name] = seg
	}

	baselineMap := make(map[string]SegmentHash)
	for _, seg := range baseline.SegmentHashes {
		baselineMap[seg.Name] = seg
	}

	for name, currentSeg := range currentMap {
		if baselineSeg, exists := baselineMap[name]; exists {
			if currentSeg.Hash != baselineSeg.Hash {
				tamperedSegments = append(tamperedSegments, name)
				changeType := "modified"
				if currentSeg.Elements != baselineSeg.Elements {
					changeType = "structure_changed"
				}
				changes = append(changes, SegmentChange{
					Segment:     name,
					OldHash:     baselineSeg.Hash,
					NewHash:     currentSeg.Hash,
					ChangeType:  changeType,
					Description: fmt.Sprintf("Segment '%s' has been modified", name),
				})
			}
		} else {
			if isCompatibilityOptionalSegment(name) {
				continue
			}
			tamperedSegments = append(tamperedSegments, name)
			changes = append(changes, SegmentChange{
				Segment:     name,
				OldHash:     "",
				NewHash:     currentSeg.Hash,
				ChangeType:  "added",
				Description: fmt.Sprintf("Segment '%s' is new", name),
			})
		}
	}

	for name, baselineSeg := range baselineMap {
		if _, exists := currentMap[name]; !exists {
			if isCompatibilityOptionalSegment(name) {
				continue
			}
			tamperedSegments = append(tamperedSegments, name)
			changes = append(changes, SegmentChange{
				Segment:     name,
				OldHash:     baselineSeg.Hash,
				NewHash:     "",
				ChangeType:  "removed",
				Description: fmt.Sprintf("Segment '%s' has been removed", name),
			})
		}
	}

	return tamperedSegments, changes
}

func (d *Detector) isMeaningfulTamper(changes []SegmentChange) bool {
	if len(changes) == 0 {
		return false
	}

	switch d.detectionMode {
	case DetectionModeStrict:
		return true

	case DetectionModeSecurity:
		// 安全模式：只关注安全相关的篡改
		for _, change := range changes {
			if change.Segment == SegmentScripts ||
				change.Segment == SegmentForms ||
				change.Segment == SegmentHead {
				return true
			}
		}
		return false

	case DetectionModePrecise:
		// 精确模式：只当核心内容发生变化时才判定
		criticalChanges := 0
		for _, change := range changes {
			if isCriticalStableSegment(change.Segment) {
				criticalChanges++
			}
		}
		return criticalChanges > 0

	case DetectionModeBalanced:
		// 平衡模式：稳定分段变化或核心分段变化
		stableModifiedCount := 0
		criticalChanges := 0

		for _, change := range changes {
			if !d.isStableSegment(change.Segment) {
				continue
			}

			if change.ChangeType == "added" || change.ChangeType == "removed" {
				return true
			}

			stableModifiedCount++
			if isCriticalStableSegment(change.Segment) {
				criticalChanges++
			}
		}

		// 平衡模式：单个核心分段变化或多个稳定分段变化
		return criticalChanges > 0 || stableModifiedCount >= 2

	default: // DetectionModeRelaxed
		// 宽松模式：仅当稳定分段出现新增/删除才判定为结构性变化
		stableModifiedCount := 0
		for _, change := range changes {
			if !d.isStableSegment(change.Segment) {
				continue
			}

			if change.ChangeType == "added" || change.ChangeType == "removed" {
				return true
			}

			stableModifiedCount++
			if isCriticalStableSegment(change.Segment) {
				return true
			}
		}

		// 宽松模式下，单个非核心稳定分段变化通常属于页面动态内容，不立即判为篡改
		return stableModifiedCount >= 2
	}
}

func isCriticalStableSegment(segment string) bool {
	switch segment {
	case SegmentMain, SegmentArticle, SegmentForms:
		return true
	default:
		return false
	}
}

func (d *Detector) isStableSegment(segment string) bool {
	if d.detectionMode == DetectionModeStrict {
		return true
	}
	_, volatile := relaxedVolatileSegments[segment]
	return !volatile
}

func isCompatibilityOptionalSegment(segment string) bool {
	_, optional := compatibilityOptionalSegments[segment]
	return optional
}

type tamperBatchCheckResult struct {
	index  int
	result TamperCheckResult
}

type tamperBatchCheckTask struct {
	detector   *Detector
	ctx        context.Context
	index      int
	targetURL  string
	resultChan chan<- tamperBatchCheckResult
	wg         *sync.WaitGroup
}

func (t *tamperBatchCheckTask) Execute() error {
	defer t.wg.Done()

	result, err := t.detector.CheckTampering(t.ctx, t.targetURL)
	if err != nil {
		t.resultChan <- tamperBatchCheckResult{
			index: t.index,
			result: TamperCheckResult{
				URL:          t.targetURL,
				Tampered:     false,
				Status:       "unreachable",
				ErrorType:    classifyTamperError(err.Error()),
				ErrorMessage: err.Error(),
				Timestamp:    time.Now().Unix(),
				CurrentHash: &PageHashResult{
					URL:    t.targetURL,
					Status: "error: " + err.Error(),
				},
			},
		}
		return nil
	}

	t.resultChan <- tamperBatchCheckResult{index: t.index, result: *result}
	return nil
}

type tamperBatchBaselineResult struct {
	index  int
	result PageHashResult
}

type tamperBatchBaselineTask struct {
	detector   *Detector
	ctx        context.Context
	index      int
	targetURL  string
	resultChan chan<- tamperBatchBaselineResult
	wg         *sync.WaitGroup
}

func (t *tamperBatchBaselineTask) Execute() error {
	defer t.wg.Done()

	hashResult, err := t.detector.ComputePageHash(t.ctx, t.targetURL)
	if err != nil {
		t.resultChan <- tamperBatchBaselineResult{
			index: t.index,
			result: PageHashResult{
				URL:    t.targetURL,
				Status: "error: " + err.Error(),
			},
		}
		return nil
	}

	if err := t.detector.SaveBaseline(t.targetURL, hashResult); err != nil {
		t.resultChan <- tamperBatchBaselineResult{
			index: t.index,
			result: PageHashResult{
				URL:    t.targetURL,
				Status: "error saving baseline: " + err.Error(),
			},
		}
		return nil
	}

	t.resultChan <- tamperBatchBaselineResult{index: t.index, result: *hashResult}
	return nil
}

func collectOrderedTamperCheckResults(resultChan <-chan tamperBatchCheckResult, size int) []TamperCheckResult {
	results := make([]TamperCheckResult, size)
	for item := range resultChan {
		if item.index < 0 || item.index >= size {
			continue
		}
		results[item.index] = item.result
	}
	return results
}

func collectOrderedTamperBaselineResults(resultChan <-chan tamperBatchBaselineResult, size int) []PageHashResult {
	results := make([]PageHashResult, size)
	for item := range resultChan {
		if item.index < 0 || item.index >= size {
			continue
		}
		results[item.index] = item.result
	}
	return results
}

func (d *Detector) BatchCheckTampering(ctx context.Context, urls []string, concurrency int) ([]TamperCheckResult, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no URLs provided")
	}

	if concurrency <= 0 {
		concurrency = 5
	}

	results := make([]TamperCheckResult, len(urls))
	pool := workerpool.NewPool(concurrency)
	pool.Start()

	var wg sync.WaitGroup
	resultChan := make(chan tamperBatchCheckResult, len(urls))

	for i, url := range urls {
		wg.Add(1)
		task := &tamperBatchCheckTask{
			detector:   d,
			ctx:        ctx,
			index:      i,
			targetURL:  url,
			resultChan: resultChan,
			wg:         &wg,
		}
		pool.Submit(task)
	}

	go func() {
		wg.Wait()
		pool.Stop()
		close(resultChan)
	}()

	results = collectOrderedTamperCheckResults(resultChan, len(urls))
	return results, nil
}

func (d *Detector) BatchSetBaseline(ctx context.Context, urls []string, concurrency int) ([]PageHashResult, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no URLs provided")
	}

	if concurrency <= 0 {
		concurrency = 5
	}

	results := make([]PageHashResult, len(urls))
	pool := workerpool.NewPool(concurrency)
	pool.Start()

	var wg sync.WaitGroup
	resultChan := make(chan tamperBatchBaselineResult, len(urls))

	for i, url := range urls {
		wg.Add(1)
		task := &tamperBatchBaselineTask{
			detector:   d,
			ctx:        ctx,
			index:      i,
			targetURL:  url,
			resultChan: resultChan,
			wg:         &wg,
		}
		pool.Submit(task)
	}

	go func() {
		wg.Wait()
		pool.Stop()
		close(resultChan)
	}()

	results = collectOrderedTamperBaselineResults(resultChan, len(urls))
	return results, nil
}

func (d *Detector) ListBaselines() ([]string, error) {
	return d.storage.ListBaselines()
}

func (d *Detector) DeleteBaseline(url string) error {
	return d.storage.DeleteBaseline(url)
}

// LoadCheckRecords 加载指定URL的检测记录
func (d *Detector) LoadCheckRecords(url string, limit int) ([]*CheckRecord, error) {
	return d.storage.LoadCheckRecords(url, limit)
}

// ListAllCheckRecords 列出所有URL的检测记录
func (d *Detector) ListAllCheckRecords() (map[string][]*CheckRecord, error) {
	return d.storage.ListAllCheckRecords()
}

// GetCheckStats 获取检测统计信息
func (d *Detector) GetCheckStats(url string) (map[string]interface{}, error) {
	return d.storage.GetCheckStats(url)
}

// DeleteCheckRecords 删除指定URL的所有检测记录
func (d *Detector) DeleteCheckRecords(url string) error {
	return d.storage.DeleteCheckRecords(url)
}

func computeSHA256(data string) string {
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

func sanitizeFilenameForStorage(url string) string {
	replacer := strings.NewReplacer(
		"http://", "",
		"https://", "",
		"/", "_",
		":", "_",
		"?", "_",
		"&", "_",
		"=", "_",
		".", "_",
	)
	return replacer.Replace(url)
}

func classifyTamperError(message string) string {
	msg := strings.ToLower(strings.TrimSpace(message))
	if msg == "" {
		return "unknown"
	}

	switch {
	case strings.Contains(msg, "baseline"):
		return "baseline"
	case strings.Contains(msg, "name_not_resolved") || strings.Contains(msg, "dns"):
		return "dns"
	case strings.Contains(msg, "timed out") || strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "ssl") || strings.Contains(msg, "tls") || strings.Contains(msg, "certificate"):
		return "tls"
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "connrefused"):
		return "connection_refused"
	case strings.Contains(msg, "connection reset"):
		return "connection_reset"
	default:
		return "network"
	}
}

// CheckRecord 检测记录
type CheckRecord struct {
	ID               string          `json:"id"`
	URL              string          `json:"url"`
	Tampered         bool            `json:"tampered"`
	DetectionMode    string          `json:"detection_mode,omitempty"`
	TamperedSegments []string        `json:"tampered_segments,omitempty"`
	Changes          []SegmentChange `json:"changes,omitempty"`
	CurrentHash      *PageHashResult `json:"current_hash"`
	BaselineHash     *PageHashResult `json:"baseline_hash,omitempty"`
	Timestamp        int64           `json:"timestamp"`
	CheckType        string          `json:"check_type"` // "first_check", "normal", "no_baseline"
}

// SaveCheckRecord 保存检测记录
func (s *HashStorage) SaveCheckRecord(url string, record *CheckRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 创建检测记录目录：hash_store/records/{url_safe_name}/
	recordsDir := filepath.Join(s.baseDir, "records", sanitizeFilenameForStorage(url))
	if err := os.MkdirAll(recordsDir, 0755); err != nil {
		return fmt.Errorf("failed to create records directory: %w", err)
	}

	// 生成记录ID和时间戳
	if record.ID == "" {
		record.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if record.Timestamp == 0 {
		record.Timestamp = time.Now().Unix()
	}

	// 文件名：{timestamp}_{check_type}.json
	filename := fmt.Sprintf("%s_%s.json", record.ID, record.CheckType)
	filePath := filepath.Join(recordsDir, filename)

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal check record: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to save check record: %w", err)
	}

	logger.Infof("Saved check record for %s to %s", url, filePath)
	return nil
}

// LoadCheckRecords 加载指定URL的检测记录
func (s *HashStorage) LoadCheckRecords(url string, limit int) ([]*CheckRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	recordsDir := filepath.Join(s.baseDir, "records", sanitizeFilenameForStorage(url))

	files, err := os.ReadDir(recordsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read records directory: %w", err)
	}

	// 按时间倒序排序
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() > files[j].Name()
	})

	if limit > 0 && len(files) > limit {
		files = files[:limit]
	}

	var records []*CheckRecord
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(recordsDir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Warnf("Failed to read check record %s: %v", filePath, err)
			continue
		}

		var record CheckRecord
		if err := json.Unmarshal(data, &record); err != nil {
			logger.Warnf("Failed to unmarshal check record %s: %v", filePath, err)
			continue
		}

		records = append(records, &record)
	}

	return records, nil
}

// ListAllCheckRecords 列出所有URL的检测记录
func (s *HashStorage) ListAllCheckRecords() (map[string][]*CheckRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	recordsBaseDir := filepath.Join(s.baseDir, "records")

	urlDirs, err := os.ReadDir(recordsBaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string][]*CheckRecord), nil
		}
		return nil, fmt.Errorf("failed to read records base directory: %w", err)
	}

	result := make(map[string][]*CheckRecord)
	for _, urlDir := range urlDirs {
		if !urlDir.IsDir() {
			continue
		}

		recordsDir := filepath.Join(recordsBaseDir, urlDir.Name())

		files, err := os.ReadDir(recordsDir)
		if err != nil {
			continue
		}

		// 按时间倒序排序
		sort.Slice(files, func(i, j int) bool {
			return files[i].Name() > files[j].Name()
		})

		for _, file := range files {
			if !strings.HasSuffix(file.Name(), ".json") {
				continue
			}

			filePath := filepath.Join(recordsDir, file.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			var record CheckRecord
			if err := json.Unmarshal(data, &record); err != nil {
				continue
			}

			// 使用记录中的实际URL作为键
			if record.URL != "" {
				result[record.URL] = append(result[record.URL], &record)
			}
		}
	}

	return result, nil
}

// GetCheckStats 获取检测统计信息
func (s *HashStorage) GetCheckStats(url string) (map[string]interface{}, error) {
	records, err := s.LoadCheckRecords(url, 0)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return map[string]interface{}{
			"total_checks":      0,
			"tampered_count":    0,
			"safe_count":        0,
			"first_check_count": 0,
		}, nil
	}

	var tamperedCount, safeCount, firstCheckCount int
	for _, r := range records {
		if r.CheckType == "first_check" {
			firstCheckCount++
		} else if r.Tampered {
			tamperedCount++
		} else {
			safeCount++
		}
	}

	return map[string]interface{}{
		"total_checks":      len(records),
		"tampered_count":    tamperedCount,
		"safe_count":        safeCount,
		"first_check_count": firstCheckCount,
		"last_check_time":   records[0].Timestamp,
		"first_check_time":  records[len(records)-1].Timestamp,
	}, nil
}

// DeleteCheckRecords 删除指定URL的所有检测记录
func (s *HashStorage) DeleteCheckRecords(url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	recordsDir := filepath.Join(s.baseDir, "records", sanitizeFilenameForStorage(url))
	return os.RemoveAll(recordsDir)
}
