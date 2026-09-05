package tamper

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unimap/project/internal/alerting"
	"github.com/unimap/project/internal/logger"
	"github.com/unimap/project/internal/tamper/fingerprint"
	"github.com/unimap/project/internal/utils"
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

// userAgentPool UA 池 — 每次请求随机选择一个，避免单一 UA 被目标站点识别和屏蔽
var userAgentPool = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36 OPR/105.0.0.0",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
}

// randomUA 从 UA 池随机选择一个
func randomUA() string {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(userAgentPool))))
	if err != nil {
		return userAgentPool[0]
	}
	return userAgentPool[n.Int64()]
}

// volatileHTTPHeaders HTTP 响应头中每次请求可能变化、不应纳入指纹的字段
var volatileHTTPHeaders = map[string]struct{}{
	"Date":              {},
	"Age":               {},
	"Expires":           {},
	"Set-Cookie":        {},
	"X-Request-Id":      {},
	"X-Trace-Id":        {},
	"X-Runtime":         {},
	"X-RateLimit-Reset": {},
	"Cf-Cache-Status":   {},
	"X-Cache":           {},
	"X-Served-By":       {},
	"X-Timer":           {},
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

	// 分段 hasher 内部归一化正则（Phase 0 误报修复）
	reVersionedFile   = regexp.MustCompile(`[a-f0-9]{8,}([-.][a-f0-9]{8,})*\.(js|css)`)
	reCacheBust       = regexp.MustCompile(`[?&](?:v|ver|version|t|ts|_|cb|nocache|rand)=\d+`)
	reSSRHydration    = regexp.MustCompile(`window\.__(?:NEXT_DATA__|NUXT__|INITIAL_STATE__|DATA__|RENDER_DATA__|PRELOADED_STATE__)\s*=\s*\{[\s\S]*?\}\s*;?`)
	reAnalyticsScript = regexp.MustCompile(`(?:var\s+)?(?:_hmt|_gaq|gtag|fbq|_paq|wa_t)\s*[=\(][\s\S]*`)
	reTimestamp       = regexp.MustCompile(`\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?`)
	reUUID            = regexp.MustCompile(`\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
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
		"xxx", "porn", "sex", "adult", "casino", "gambling",
		"bet", "lottery", "crypto", "bitcoin", "mining",
		"coin-hive", "coinhive", "cryptonight",
	}

	suspiciousPathKeywords = []string{
		"shell", "backdoor", "webshell", "hacked", "deface",
		"phishing", "fake", "login", "admin",
	}

	hiddenIframePattern   = regexp.MustCompile(`(?i)<iframe[^>]*style\s*=\s*["'][^"']*(display\s*:\s*none|visibility\s*:\s*hidden|width\s*:\s*0|height\s*:\s*0)[^"']*["']`)
	dangerousEventPattern = regexp.MustCompile(`(?i)on(?:error|load|mouseover|click|keyup|keydown|submit)\s*=\s*["'][^"']*(?:eval\(|document\.write\(|Function\(|atob\(|btoa\(|unescape\(|decodeURIComponent\(|String\.fromCharCode\(|crypto|miner|coin-hive|coinhive|cryptonight)[^"']*["']`)
	maliciousKeywordRes   = compileWordBoundaryRegexes(maliciousScriptKeywords)
	domainKeywordRes      = compileWordBoundaryRegexes(suspiciousDomainKeywords)
	pathKeywordRes        = compileWordBoundaryRegexes(suspiciousPathKeywords)
)

// compiledKeyword pairs a human-readable keyword with its regex.
type compiledKeyword struct {
	keyword string
	re      *regexp.Regexp
}

// compileWordBoundaryRegexes builds word-boundary regexps from keyword list.
func compileWordBoundaryRegexes(keywords []string) []compiledKeyword {
	res := make([]compiledKeyword, 0, len(keywords))
	for _, kw := range keywords {
		quoted := regexp.QuoteMeta(kw)
		if len(kw) > 0 && isWordChar(kw[len(kw)-1]) {
			res = append(res, compiledKeyword{keyword: kw, re: regexp.MustCompile(`(?i)\b` + quoted + `\b`)})
		} else {
			res = append(res, compiledKeyword{keyword: kw, re: regexp.MustCompile(`(?i)\b` + quoted)})
		}
	}
	return res
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func detectMaliciousContent(html string) []string {
	var flags []string
	for _, item := range maliciousKeywordRes {
		if item.re.MatchString(html) {
			flags = append(flags, fmt.Sprintf("suspicious_script: %s", item.keyword))
		}
	}
	domainMatches := 0
	for _, item := range domainKeywordRes {
		if item.re.MatchString(html) {
			domainMatches++
			if domainMatches >= 2 {
				flags = append(flags, "suspicious_domain_keywords: multiple matches")
				break
			}
		}
	}
	pathMatches := 0
	for _, item := range pathKeywordRes {
		if item.re.MatchString(html) {
			pathMatches++
			if pathMatches >= 2 {
				flags = append(flags, "suspicious_path_keywords: multiple matches")
				break
			}
		}
	}
	if hiddenIframePattern.MatchString(html) {
		flags = append(flags, "hidden_iframe_detected")
	}
	if dangerousEventPattern.MatchString(html) {
		flags = append(flags, "dangerous_event_handler")
	}
	return flags
}

// SegmentHash represents a hash of a specific page segment.
type SegmentHash struct {
	Name     string `json:"name"`
	Hash     string `json:"hash"`
	Content  string `json:"content,omitempty"`
	Length   int    `json:"length"`
	Elements int    `json:"elements"`
}

// PageHashResult holds the full hash computation result for a page.
type PageHashResult struct {
	URL                       string                          `json:"url"`
	Title                     string                          `json:"title"`
	FullHash                  string                          `json:"full_hash"`
	SimpleMD5Hash             string                          `json:"simple_md5_hash"`
	SegmentHashes             []SegmentHash                   `json:"segment_hashes"`
	Timestamp                 int64                           `json:"timestamp"`
	HTMLLength                int                             `json:"html_length"`
	Status                    string                          `json:"status"`
	RawHTML                   string                          `json:"-"`
	HTTPHeaders               map[string]string               `json:"http_headers,omitempty"`
	Fingerprints              []fingerprint.FingerprintResult `json:"fingerprints,omitempty"`
	NormalizedHTTPFingerprint string                          `json:"normalized_http_fingerprint,omitempty"`
	FinalURL                  string                          `json:"final_url,omitempty"`     // 最终落地 URL（跟随重定向后）
	RedirectURLs              []string                        `json:"redirect_urls,omitempty"` // 重定向链
	OpenPorts                 []int                           `json:"open_ports,omitempty"`    // 主机开放端口
}

// FingerprintChange 记录指纹变化（新增/消失）
type FingerprintChange struct {
	RuleName string `json:"rule_name"`
	Category string `json:"category"`
	Change   string `json:"change"` // "added" | "removed"
}

// PortChange 记录端口变化
type PortChange struct {
	Port   int    `json:"port"`
	Change string `json:"change"` // "opened" | "closed"
}

// TamperCheckResult is the result of a single tamper check.
type TamperCheckResult struct {
	URL                string              `json:"url"`
	CurrentHash        *PageHashResult     `json:"current_hash"`
	BaselineHash       *PageHashResult     `json:"baseline_hash,omitempty"`
	Tampered           bool                `json:"tampered"`
	Status             string              `json:"status"`
	ErrorType          string              `json:"error_type,omitempty"`
	ErrorMessage       string              `json:"error_message,omitempty"`
	TamperedSegments   []string            `json:"tampered_segments,omitempty"`
	Changes            []SegmentChange     `json:"changes,omitempty"`
	SuspiciousFlags    []string            `json:"suspicious_flags,omitempty"`
	FingerprintChanges []FingerprintChange `json:"fingerprint_changes,omitempty"`
	PortChanges        []PortChange        `json:"port_changes,omitempty"`
	Timestamp          int64               `json:"timestamp"`
}

// SegmentChange describes a single segment-level change.
type SegmentChange struct {
	Segment     string `json:"segment"`
	OldHash     string `json:"old_hash"`
	NewHash     string `json:"new_hash"`
	ChangeType  string `json:"change_type"`
	Description string `json:"description"`
}

// HashStorage manages on-disk baseline and check-record persistence.
type HashStorage struct {
	historyOwner *historyDatabaseOwner
	baseDir      string
	mu           sync.RWMutex
}

type cacheEntry struct {
	result    *PageHashResult
	timestamp time.Time
}

// Detector is the main tamper-detection engine.
type Detector struct {
	storage            *HashStorage
	browserPageLoader  BrowserPageLoader
	detectionMode      string
	performanceMode    string
	alertManager       *alerting.Manager
	fpEngine           *fingerprint.Engine
	insecureSkipVerify bool
	portScanEnabled    bool
	portScanList       []int
	portScanTimeout    time.Duration
	cache              map[string]*cacheEntry
	cacheMu            sync.RWMutex
	mu                 sync.Mutex
}

// BrowserPageLoader renders one URL through the application's guarded browser
// path and returns the final title and HTML. Implementations must apply their
// navigation security policy before making a connection.
type BrowserPageLoader interface {
	LoadPage(context.Context, string) (title string, html string, err error)
}

// BrowserPageLoaderFunc adapts a function to BrowserPageLoader.
type BrowserPageLoaderFunc func(context.Context, string) (string, string, error)

func (f BrowserPageLoaderFunc) LoadPage(ctx context.Context, targetURL string) (string, string, error) {
	return f(ctx, targetURL)
}

// DetectorConfig holds configuration for creating a new Detector.
type DetectorConfig struct {
	// Storage optionally shares a service-owned store. The service owns its lifetime.
	Storage            *HashStorage
	BaseDir            string
	DetectionMode      string
	PerformanceMode    string
	AlertManager       *alerting.Manager
	InsecureSkipVerify bool          // 跳过 SSL 证书验证（内网/自签证书场景）
	PortScanEnabled    bool          // 巡检时附带端口扫描
	PortScanList       []int         // 扫描端口列表（为空时使用默认 Top 20）
	PortScanTimeout    time.Duration // 单端口超时（默认 800ms）
}

// CheckRecord represents a single tamper check record persisted to disk.
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
	CheckType        string          `json:"check_type"`
}

// NewHashStorage creates a new HashStorage with the given base directory.
func NewHashStorage(baseDir string) *HashStorage {
	if baseDir == "" {
		baseDir = utils.HashStoreDir()
	}
	return &HashStorage{baseDir: baseDir}
}

// SaveBaseline persists a page hash baseline to disk.
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

// LoadBaseline loads a page hash baseline from disk.
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

// HasBaseline checks whether a baseline exists for the given URL.
func (s *HashStorage) HasBaseline(url string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	safeFilename := sanitizeFilenameForStorage(url)
	filePath := filepath.Join(s.baseDir, safeFilename+".json")

	_, err := os.Stat(filePath)
	return err == nil
}

// ListBaselines lists all stored baseline URLs.
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

// DeleteBaseline removes a baseline from disk.
func (s *HashStorage) DeleteBaseline(url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	safeFilename := sanitizeFilenameForStorage(url)
	filePath := filepath.Join(s.baseDir, safeFilename+".json")

	return os.Remove(filePath)
}

// SaveCheckRecord persists a check record to disk.
func (s *HashStorage) SaveCheckRecord(url string, record *CheckRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	recordsDir := filepath.Join(s.baseDir, "records", sanitizeFilenameForStorage(url))
	if err := os.MkdirAll(recordsDir, 0755); err != nil {
		return fmt.Errorf("failed to create records directory: %w", err)
	}

	if record.ID == "" {
		record.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if record.Timestamp == 0 {
		record.Timestamp = time.Now().Unix()
	}

	filename := fmt.Sprintf("%s_%s.json", record.ID, record.CheckType)
	filePath := filepath.Join(recordsDir, filename)

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal check record: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to save check record: %w", err)
	}
	if err := s.indexCheckRecord(record); err != nil {
		logger.Warnf("Failed to index check record %s: %v", record.ID, err)
	}

	logger.Infof("Saved check record for %s to %s", url, filePath)
	return nil
}

// LoadCheckRecords loads check records for a URL.
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
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			logger.Warnf("Failed to read check record %s: %v", filePath, readErr)
			continue
		}

		var record CheckRecord
		if unmarshalErr := json.Unmarshal(data, &record); unmarshalErr != nil {
			logger.Warnf("Failed to unmarshal check record %s: %v", filePath, unmarshalErr)
			continue
		}

		records = append(records, &record)
	}

	return records, nil
}

// ListAllCheckRecords lists all check records grouped by URL.
func (s *HashStorage) ListAllCheckRecords() (map[string][]*CheckRecord, error) {
	markerPath := filepath.Join(s.baseDir, ".check_records_indexed")
	if _, err := os.Stat(markerPath); err == nil {
		indexed, indexErr := s.listIndexedCheckRecords()
		if indexErr != nil {
			return nil, fmt.Errorf("read check record index: %w", indexErr)
		}
		return indexed, nil
	}
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

		files, readErr := os.ReadDir(recordsDir)
		if readErr != nil {
			continue
		}

		sort.Slice(files, func(i, j int) bool {
			return files[i].Name() > files[j].Name()
		})

		for _, file := range files {
			if !strings.HasSuffix(file.Name(), ".json") {
				continue
			}

			filePath := filepath.Join(recordsDir, file.Name())
			data, readErr := os.ReadFile(filePath)
			if readErr != nil {
				continue
			}

			var record CheckRecord
			if unmarshalErr := json.Unmarshal(data, &record); unmarshalErr != nil {
				continue
			}

			if record.URL != "" {
				result[record.URL] = append(result[record.URL], &record)
			}
		}
	}

	for _, records := range result {
		for _, record := range records {
			if err := s.indexCheckRecord(record); err != nil {
				return nil, fmt.Errorf("index legacy check record: %w", err)
			}
		}
	}
	if err := os.WriteFile(markerPath, []byte("indexed\n"), 0o600); err != nil {
		return nil, fmt.Errorf("write check record index marker: %w", err)
	}
	return s.listIndexedCheckRecords()
}

// CheckStats is the typed report returned by GetCheckStats.
type CheckStats struct {
	TotalChecks     int   `json:"total_checks"`
	TamperedCount   int   `json:"tampered_count"`
	SafeCount       int   `json:"safe_count"`
	FirstCheckCount int   `json:"first_check_count"`
	LastCheckTime   int64 `json:"last_check_time,omitempty"`
	FirstCheckTime  int64 `json:"first_check_time,omitempty"`
}

// GetCheckStats returns aggregate statistics for a URL's check records.
func (s *HashStorage) GetCheckStats(url string) (CheckStats, error) {
	records, err := s.LoadCheckRecords(url, 0)
	if err != nil {
		return CheckStats{}, err
	}

	if len(records) == 0 {
		return CheckStats{}, nil
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

	return CheckStats{
		TotalChecks:     len(records),
		TamperedCount:   tamperedCount,
		SafeCount:       safeCount,
		FirstCheckCount: firstCheckCount,
		LastCheckTime:   records[0].Timestamp,
		FirstCheckTime:  records[len(records)-1].Timestamp,
	}, nil
}

// DeleteCheckRecords deletes all check records for a URL.
func (s *HashStorage) DeleteCheckRecords(url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	recordsDir := filepath.Join(s.baseDir, "records", sanitizeFilenameForStorage(url))
	if err := os.RemoveAll(recordsDir); err != nil {
		return err
	}
	if err := s.deleteIndexedCheckRecords(url); err != nil {
		return fmt.Errorf("delete indexed check records: %w", err)
	}
	return nil
}

// NewDetector creates a new Detector with the given configuration.
func NewDetector(cfg DetectorConfig) *Detector {
	storage := cfg.Storage
	if storage == nil {
		storage = NewHashStorage(cfg.BaseDir)
	}

	// 初始化指纹引擎（失败不影响核心功能，仅记录日志）
	fpEngine, err := fingerprint.NewEngine()
	if err != nil {
		logger.Warnf("Failed to initialize fingerprint engine: %v", err)
	}

	return &Detector{
		storage:            storage,
		detectionMode:      NormalizeDetectionMode(cfg.DetectionMode),
		performanceMode:    normalizePerformanceMode(cfg.PerformanceMode),
		alertManager:       cfg.AlertManager,
		fpEngine:           fpEngine,
		insecureSkipVerify: cfg.InsecureSkipVerify,
		portScanEnabled:    cfg.PortScanEnabled,
		portScanList:       normalizePortList(cfg.PortScanList),
		portScanTimeout:    normalizePortTimeout(cfg.PortScanTimeout),
		cache:              make(map[string]*cacheEntry),
	}
}

// NormalizeDetectionMode maps raw user input to one of the supported detection
// modes, defaulting to DetectionModeRelaxed for unknown values. Exported so the
// service layer can validate mode strings without re-implementing the mapping.
func NormalizeDetectionMode(raw string) string {
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

// defaultPortScanList 默认巡检扫描频次最高的服务端口
var defaultPortScanList = []int{21, 22, 25, 53, 80, 81, 110, 143, 443, 993, 995, 1433, 1521, 2181, 2375, 3128, 3306, 3389, 5432, 5672, 6379, 8000, 8080, 8443, 9000, 9090, 9200, 11211, 27017}

// maxPortScanList caps user-defined port lists to prevent resource exhaustion.
const maxPortScanList = 100

func normalizePortList(ports []int) []int {
	if len(ports) == 0 {
		return defaultPortScanList
	}
	if len(ports) > maxPortScanList {
		ports = ports[:maxPortScanList]
	}
	seen := make(map[int]struct{}, len(ports))
	var deduped []int
	for _, p := range ports {
		if p < 1 || p > 65535 {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		deduped = append(deduped, p)
	}
	sort.Ints(deduped)
	return deduped
}

func normalizePortTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 800 * time.Millisecond
	}
	if timeout < 100*time.Millisecond {
		return 100 * time.Millisecond
	}
	if timeout > 5*time.Second {
		return 5 * time.Second
	}
	return timeout
}
