package tamper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/unimap-icp-hunter/project/internal/logger"
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
	SegmentForms       = "forms"
	SegmentFullContent = "full_content"
)

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
	SegmentHashes []SegmentHash `json:"segment_hashes"`
	Timestamp     int64         `json:"timestamp"`
	HTMLLength    int           `json:"html_length"`
	Status        string        `json:"status"`
}

type TamperCheckResult struct {
	URL              string          `json:"url"`
	CurrentHash      *PageHashResult `json:"current_hash"`
	BaselineHash     *PageHashResult `json:"baseline_hash,omitempty"`
	Tampered         bool            `json:"tampered"`
	TamperedSegments []string        `json:"tampered_segments,omitempty"`
	Changes          []SegmentChange `json:"changes,omitempty"`
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

type Detector struct {
	storage     *HashStorage
	allocCtx    context.Context
	allocCancel context.CancelFunc
	mu          sync.Mutex
}

type DetectorConfig struct {
	BaseDir string
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

	data, err := json.MarshalIndent(result, "", "  ")
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
			urls = append(urls, strings.TrimSuffix(file.Name(), ".json"))
		}
	}
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
		storage: storage,
	}
}

func (d *Detector) SetAllocator(ctx context.Context, allocCtx context.Context, allocCancel context.CancelFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.allocCtx = allocCtx
	d.allocCancel = allocCancel
}

func (d *Detector) ComputePageHash(ctx context.Context, targetURL string) (*PageHashResult, error) {
	var html string
	var title string

	if err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Title(&title),
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	); err != nil {
		return nil, fmt.Errorf("failed to load page: %w", err)
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
	}

	segmentHashes := d.computeSegmentHashes(doc, html)
	result.SegmentHashes = segmentHashes

	result.FullHash = d.computeFullHash(segmentHashes)

	return result, nil
}

func (d *Detector) computeSegmentHashes(doc *goquery.Document, html string) []SegmentHash {
	var segments []SegmentHash

	segments = append(segments, d.computeElementHash(doc, "head", SegmentHead))
	segments = append(segments, d.computeElementHash(doc, "body", SegmentBody))
	segments = append(segments, d.computeElementHash(doc, "header", SegmentHeader))
	segments = append(segments, d.computeElementHash(doc, "nav", SegmentNav))
	segments = append(segments, d.computeElementHash(doc, "main", SegmentMain))
	segments = append(segments, d.computeElementHash(doc, "article", SegmentArticle))
	segments = append(segments, d.computeElementHash(doc, "section", SegmentSection))
	segments = append(segments, d.computeElementHash(doc, "aside", SegmentAside))
	segments = append(segments, d.computeElementHash(doc, "footer", SegmentFooter))

	segments = append(segments, d.computeScriptHash(doc))
	segments = append(segments, d.computeStyleHash(doc))
	segments = append(segments, d.computeMetaHash(doc))
	segments = append(segments, d.computeLinkHash(doc))
	segments = append(segments, d.computeImageHash(doc))
	segments = append(segments, d.computeFormHash(doc))

	cleanHTML := d.cleanHTML(html)
	fullContentHash := SegmentHash{
		Name:     SegmentFullContent,
		Hash:     computeSHA256(cleanHTML),
		Length:   len(cleanHTML),
		Elements: 1,
	}
	segments = append(segments, fullContentHash)

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
		content := s.Text()
		scripts = append(scripts, src+":"+content)
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
		alt, _ := s.Attr("alt")
		images = append(images, fmt.Sprintf("%s:%s", src, alt))
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

	html = regexp.MustCompile(`(?i)\s+`).ReplaceAllString(html, " ")
	html = regexp.MustCompile(`(?i)<!--.*?-->`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)data:image/[^"']*`).ReplaceAllString(html, "DATA_IMAGE_REMOVED")
	html = regexp.MustCompile(`(?i)nonce="[^"]*"`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?i)csrf[^"]*_token["']?\s*[:=]\s*["'][^"']*["']`).ReplaceAllString(html, "")
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
		// 没有基线，首次检测
		result := &TamperCheckResult{
			URL:         url,
			CurrentHash: currentHash,
			Tampered:    false,
			Timestamp:   time.Now().Unix(),
		}

		// 保存首次检测记录
		record := &CheckRecord{
			ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
			URL:         url,
			Tampered:    false,
			CurrentHash: currentHash,
			Timestamp:   result.Timestamp,
			CheckType:   "first_check",
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
		Timestamp:    time.Now().Unix(),
	}

	// 确定检测类型
	checkType := "normal"
	if currentHash.FullHash != baseline.FullHash {
		result.Tampered = true
		result.TamperedSegments, result.Changes = d.findChangedSegments(currentHash, baseline)
		checkType = "tampered"
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

func (d *Detector) BatchCheckTampering(ctx context.Context, urls []string, concurrency int) ([]TamperCheckResult, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no URLs provided")
	}

	if concurrency <= 0 {
		concurrency = 5
	}

	results := make([]TamperCheckResult, len(urls))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, concurrency)

	for i, url := range urls {
		wg.Add(1)
		go func(index int, targetURL string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result, err := d.CheckTampering(ctx, targetURL)
			if err != nil {
				results[index] = TamperCheckResult{
					URL:       targetURL,
					Tampered:  false,
					Timestamp: time.Now().Unix(),
				}
				results[index].CurrentHash = &PageHashResult{
					URL:    targetURL,
					Status: "error: " + err.Error(),
				}
			} else {
				results[index] = *result
			}
		}(i, url)
	}

	wg.Wait()
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
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, concurrency)
	var mu sync.Mutex

	for i, url := range urls {
		wg.Add(1)
		go func(index int, targetURL string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			hashResult, err := d.ComputePageHash(ctx, targetURL)
			if err != nil {
				mu.Lock()
				results[index] = PageHashResult{
					URL:    targetURL,
					Status: "error: " + err.Error(),
				}
				mu.Unlock()
				return
			}

			if err := d.SaveBaseline(targetURL, hashResult); err != nil {
				mu.Lock()
				results[index] = PageHashResult{
					URL:    targetURL,
					Status: "error saving baseline: " + err.Error(),
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			results[index] = *hashResult
			mu.Unlock()
		}(i, url)
	}

	wg.Wait()
	return results, nil
}

func (d *Detector) ListBaselines() ([]string, error) {
	return d.storage.ListBaselines()
}

func (d *Detector) DeleteBaseline(url string) error {
	return d.storage.DeleteBaseline(url)
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

// CheckRecord 检测记录
type CheckRecord struct {
	ID               string          `json:"id"`
	URL              string          `json:"url"`
	Tampered         bool            `json:"tampered"`
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

	data, err := json.MarshalIndent(record, "", "  ")
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

		url := urlDir.Name()
		recordsDir := filepath.Join(recordsBaseDir, url)

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

			result[url] = append(result[url], &record)
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
