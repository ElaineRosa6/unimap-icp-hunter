package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unimap-icp-hunter/project/internal/proxypool"
	"github.com/unimap-icp-hunter/project/internal/util/workerpool"
)

var defaultScanPorts = []int{80, 81, 443, 8000, 8080, 8443, 9000}

type URLPortScanSummary struct {
	Total         int `json:"total"`
	FormatValid   int `json:"formatValid"`
	InvalidFormat int `json:"invalidFormat"`
	CDNExcluded   int `json:"cdnExcluded"`
	Scanned       int `json:"scanned"`
	ResolveFailed int `json:"resolveFailed"`
	ScanFailed    int `json:"scanFailed"`
}

type URLPortScanResult struct {
	Input       string           `json:"input"`
	URL         string           `json:"url,omitempty"`
	Host        string           `json:"host,omitempty"`
	Status      string           `json:"status"`
	Reason      string           `json:"reason,omitempty"`
	CDNDetected bool             `json:"cdn_detected"`
	CDNReasons  []string         `json:"cdn_reasons,omitempty"`
	ResolvedIPs []string         `json:"resolved_ips,omitempty"`
	ScannedIPs  []string         `json:"scanned_ips,omitempty"`
	OpenPorts   map[string][]int `json:"open_ports,omitempty"`
}

type URLPortScanResponse struct {
	Summary URLPortScanSummary  `json:"summary"`
	Ports   []int               `json:"ports"`
	Results []URLPortScanResult `json:"results"`
}

type portScanTaskPayload struct {
	index int
	item  URLPortScanResult
}

type portScanTask struct {
	ctx        context.Context
	index      int
	input      string
	ports      []int
	proxyPool  *proxypool.Pool
	resultChan chan<- portScanTaskPayload
	wg         *sync.WaitGroup
}

func (t *portScanTask) Execute() error {
	defer t.wg.Done()

	normalizedURL, normalizeErr := normalizeMonitorURLForService(t.input)
	if normalizeErr != nil {
		t.resultChan <- portScanTaskPayload{index: t.index, item: URLPortScanResult{
			Input:       t.input,
			Status:      "invalid_format",
			CDNDetected: false,
			Reason:      normalizeErr.Error(),
		}}
		return nil
	}

	parsed, err := url.Parse(normalizedURL)
	if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		t.resultChan <- portScanTaskPayload{index: t.index, item: URLPortScanResult{
			Input:       t.input,
			URL:         normalizedURL,
			Status:      "invalid_format",
			CDNDetected: false,
			Reason:      "missing host",
		}}
		return nil
	}

	host := strings.TrimSpace(parsed.Hostname())
	resolveCtx, resolveCancel := context.WithTimeout(t.ctx, 6*time.Second)
	defer resolveCancel()

	ips, resolveErr := resolveIPv4Addresses(resolveCtx, host)
	if resolveErr != nil {
		t.resultChan <- portScanTaskPayload{index: t.index, item: URLPortScanResult{
			Input:       t.input,
			URL:         normalizedURL,
			Host:        host,
			Status:      "resolve_failed",
			CDNDetected: false,
			Reason:      resolveErr.Error(),
		}}
		return nil
	}

	cdnDetected, cdnReasons := detectCDNForTarget(t.ctx, normalizedURL, host, ips, t.proxyPool)
	if cdnDetected {
		t.resultChan <- portScanTaskPayload{index: t.index, item: URLPortScanResult{
			Input:       t.input,
			URL:         normalizedURL,
			Host:        host,
			Status:      "cdn_excluded",
			Reason:      "cdn detected, port scan excluded",
			CDNDetected: true,
			CDNReasons:  cdnReasons,
			ResolvedIPs: ips,
		}}
		return nil
	}

	scanCtx, scanCancel := context.WithTimeout(t.ctx, 20*time.Second)
	defer scanCancel()

	openPorts, scanErr := scanHostPorts(scanCtx, ips, t.ports)
	if scanErr != nil {
		t.resultChan <- portScanTaskPayload{index: t.index, item: URLPortScanResult{
			Input:       t.input,
			URL:         normalizedURL,
			Host:        host,
			Status:      "scan_failed",
			Reason:      scanErr.Error(),
			CDNDetected: false,
			ResolvedIPs: ips,
		}}
		return nil
	}

	scannedIPs := make([]string, 0, len(openPorts))
	for ip := range openPorts {
		scannedIPs = append(scannedIPs, ip)
	}
	sort.Strings(scannedIPs)

	t.resultChan <- portScanTaskPayload{index: t.index, item: URLPortScanResult{
		Input:       t.input,
		URL:         normalizedURL,
		Host:        host,
		Status:      "scanned",
		CDNDetected: false,
		ResolvedIPs: ips,
		ScannedIPs:  scannedIPs,
		OpenPorts:   openPorts,
	}}
	return nil
}

// ScanURLPorts executes URL->IP resolution, CDN exclusion and port scanning for non-CDN targets.
func (s *MonitorAppService) ScanURLPorts(ctx context.Context, urls []string, ports []int, concurrency int) (*URLPortScanResponse, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no URLs provided")
	}
	if concurrency <= 0 || concurrency > 10 {
		concurrency = 3
	}

	normalizedPorts := normalizeScanPorts(ports)
	results := make([]URLPortScanResult, len(urls))

	pool := workerpool.NewPool(concurrency)
	pool.Start()

	resultChan := make(chan portScanTaskPayload, len(urls))
	var wg sync.WaitGroup

	for i, rawURL := range urls {
		wg.Add(1)
		pool.Submit(&portScanTask{
			ctx:        ctx,
			index:      i,
			input:      rawURL,
			ports:      normalizedPorts,
			proxyPool:  s.proxyPool,
			resultChan: resultChan,
			wg:         &wg,
		})
	}

	go func() {
		wg.Wait()
		pool.Stop()
		close(resultChan)
	}()

	for item := range resultChan {
		results[item.index] = item.item
	}

	summary := URLPortScanSummary{Total: len(results)}
	for _, result := range results {
		switch result.Status {
		case "invalid_format":
			summary.InvalidFormat++
		case "resolve_failed":
			summary.FormatValid++
			summary.ResolveFailed++
		case "cdn_excluded":
			summary.FormatValid++
			summary.CDNExcluded++
		case "scan_failed":
			summary.FormatValid++
			summary.ScanFailed++
		case "scanned":
			summary.FormatValid++
			summary.Scanned++
		}
	}

	return &URLPortScanResponse{Summary: summary, Ports: normalizedPorts, Results: results}, nil
}

func normalizeScanPorts(ports []int) []int {
	if len(ports) == 0 {
		out := make([]int, len(defaultScanPorts))
		copy(out, defaultScanPorts)
		return out
	}

	seen := make(map[int]struct{}, len(ports))
	out := make([]int, 0, len(ports))
	for _, p := range ports {
		if p < 1 || p > 65535 {
			continue
		}
		if _, exists := seen[p]; exists {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}

	if len(out) == 0 {
		out = append(out, defaultScanPorts...)
	}
	sort.Ints(out)
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

func resolveIPv4Addresses(ctx context.Context, host string) ([]string, error) {
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no ipv4 address resolved")
	}
	out := make([]string, 0, len(ips))
	seen := make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		ipText := strings.TrimSpace(ip.String())
		if ipText == "" {
			continue
		}
		if _, exists := seen[ipText]; exists {
			continue
		}
		seen[ipText] = struct{}{}
		out = append(out, ipText)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("no ipv4 address resolved")
	}
	return out, nil
}

func detectCDNForTarget(ctx context.Context, targetURL, host string, ips []string, pool *proxypool.Pool) (bool, []string) {
	reasons := make([]string, 0, 4)

	if cname, err := net.LookupCNAME(host); err == nil {
		lower := strings.ToLower(strings.TrimSpace(cname))
		if lower != "" && isLikelyCDNString(lower) {
			reasons = append(reasons, "cname indicates cdn")
		}
	}

	if len(ips) >= 4 {
		reasons = append(reasons, "multiple edge ips resolved")
	}

	for _, ip := range ips {
		if isLikelyCDNIP(ip) {
			reasons = append(reasons, "ip in known cdn range")
			break
		}
	}

	if cdnByHeader := detectCDNByHTTPHeaders(ctx, targetURL, pool); cdnByHeader {
		reasons = append(reasons, "http headers indicate cdn")
	}

	return len(reasons) > 0, dedupeStrings(reasons)
}

func detectCDNByHTTPHeaders(ctx context.Context, targetURL string, pool *proxypool.Pool) bool {
	selectedProxy := ""
	if pool != nil {
		if proxyAddr, ok := pool.Select(); ok {
			selectedProxy = proxyAddr
		}
	}

	client, err := buildReachabilityHTTPClient(selectedProxy)
	if err != nil {
		if pool != nil && selectedProxy != "" {
			pool.Report(selectedProxy, false)
		}
		return false
	}

	success := false
	defer func() {
		if pool != nil && selectedProxy != "" {
			pool.Report(selectedProxy, success)
		}
	}()

	headCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, reqErr := http.NewRequestWithContext(headCtx, http.MethodHead, targetURL, nil)
	if reqErr != nil {
		return false
	}
	resp, doErr := client.Do(req)
	if doErr != nil {
		return false
	}
	defer resp.Body.Close()
	success = true

	for k, values := range resp.Header {
		lowerKey := strings.ToLower(strings.TrimSpace(k))
		if isLikelyCDNString(lowerKey) {
			return true
		}
		for _, v := range values {
			if isLikelyCDNString(strings.ToLower(strings.TrimSpace(v))) {
				return true
			}
		}
	}
	if isLikelyCDNString(strings.ToLower(strings.TrimSpace(resp.Header.Get("Server")))) {
		return true
	}
	return false
}

func scanHostPorts(ctx context.Context, ips []string, ports []int) (map[string][]int, error) {
	result := make(map[string][]int, len(ips))
	for _, ip := range ips {
		open := make([]int, 0, len(ports))
		for _, port := range ports {
			if isTCPPortOpen(ctx, ip, port, 1200*time.Millisecond) {
				open = append(open, port)
			}
		}
		sort.Ints(open)
		result[ip] = open
	}
	return result, nil
}

func isTCPPortOpen(ctx context.Context, ip string, port int, timeout time.Duration) bool {
	target := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, item := range in {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func isLikelyCDNString(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	needle := []string{
		"cdn", "cloudflare", "cloudfront", "akamai", "fastly", "edgekey", "edgesuite",
		"incapsula", "sucuri", "stackpath", "qcloud", "aliyuncs", "tencent", "wangsu",
		"chinacache", "cache", "cf-ray", "x-cache", "x-served-by", "via",
	}
	lower := strings.ToLower(text)
	for _, k := range needle {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

func isLikelyCDNIP(ipText string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipText))
	if ip == nil {
		return false
	}
	cidrs := []string{
		"104.16.0.0/12",  // Cloudflare
		"172.64.0.0/13",  // Cloudflare
		"23.235.32.0/20", // Fastly
		"151.101.0.0/16", // Fastly
		"13.32.0.0/15",   // CloudFront
		"13.224.0.0/14",  // CloudFront
		"23.0.0.0/12",    // Akamai common edge
	}
	for _, cidrText := range cidrs {
		_, cidr, err := net.ParseCIDR(cidrText)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}
