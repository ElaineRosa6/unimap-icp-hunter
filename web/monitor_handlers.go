package web

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"
)

// 预编译正则表达式，避免每次调用时重新编译
var reURLPattern = regexp.MustCompile(`^(https?://)?([\w.-]+)(:\d+)?(/.*)?$`)

// handleScreenshot 处理截图请求
func (s *Server) handleMonitorPage(w http.ResponseWriter, r *http.Request) {
	if !s.renderTemplateWithNonce(r, w, http.StatusInternalServerError, "monitor.html", map[string]interface{}{
		"staticVersion": s.staticVersion,
	}) {
		return
	}
}

// handleImportURLs 处理URL文件导入
func (s *Server) handleImportURLs(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	// 解析multipart表单
	maxMultipartMemory := int64(10 << 20)
	if s.config != nil && s.config.Web.RequestLimits.MaxMultipartMemory > 0 {
		maxMultipartMemory = s.config.Web.RequestLimits.MaxMultipartMemory
	}
	err := r.ParseMultipartForm(maxMultipartMemory)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_multipart_form", "failed to parse form", err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "file_required", "failed to get file", err.Error())
		return
	}
	defer file.Close()

	fileName := strings.ToLower(header.Filename)
	var urls []string

	if strings.HasSuffix(fileName, ".xlsx") || strings.HasSuffix(fileName, ".xls") {
		// 解析Excel文件
		urls, err = parseExcelFile(file)
	} else if strings.HasSuffix(fileName, ".csv") {
		// 解析CSV文件
		urls, err = parseCSVFile(file)
	} else if strings.HasSuffix(fileName, ".txt") {
		// 解析TXT文件
		urls, err = parseTXTFile(file)
	} else {
		writeAPIError(w, http.StatusBadRequest, "unsupported_file_format", "unsupported file format", nil)
		return
	}

	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "file_parse_failed", "failed to parse file", err.Error())
		return
	}

	// 过滤有效URL
	validUrls := filterValidURLs(urls)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":    len(urls),
		"valid":    len(validUrls),
		"urls":     validUrls,
		"filename": header.Filename,
	})
}

func (s *Server) handleURLReachability(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		URLs        []string `json:"urls"`
		Concurrency int      `json:"concurrency"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	if len(req.URLs) == 0 {
		writeAPIError(w, http.StatusBadRequest, "no_urls_provided", "no URLs provided", nil)
		return
	}

	// 检查所有URL是否指向内网地址
	for _, urlStr := range req.URLs {
		parsed, err := url.Parse(urlStr)
		if err != nil {
			continue
		}
		if isPrivateOrInternalIP(parsed.Hostname()) {
			writeAPIError(w, http.StatusForbidden, "blocked_url", "target url resolves to private/internal address", nil)
			return
		}
	}

	if s.monitorApp == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "monitor_service_unavailable", "monitor app service not initialized", nil)
		return
	}

	response, err := s.monitorApp.CheckURLReachability(r.Context(), req.URLs, req.Concurrency)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "reachability_check_failed", "url reachability check failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"summary": response.Summary,
		"results": response.Results,
	})
}

func (s *Server) handleURLPortScan(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		URLs        []string `json:"urls"`
		Ports       []int    `json:"ports"`
		Concurrency int      `json:"concurrency"`
	}

	if !decodeJSONBody(w, r, &req) {
		return
	}

	if len(req.URLs) == 0 {
		writeAPIError(w, http.StatusBadRequest, "no_urls_provided", "no URLs provided", nil)
		return
	}

	// 检查所有URL是否指向内网地址
	for _, urlStr := range req.URLs {
		parsed, err := url.Parse(urlStr)
		if err != nil {
			continue
		}
		if isPrivateOrInternalIP(parsed.Hostname()) {
			writeAPIError(w, http.StatusForbidden, "blocked_url", "target url resolves to private/internal address", nil)
			return
		}
	}

	if s.monitorApp == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "monitor_service_unavailable", "monitor app service not initialized", nil)
		return
	}

	response, err := s.monitorApp.ScanURLPorts(r.Context(), req.URLs, req.Ports, req.Concurrency)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "url_port_scan_failed", "url port scan failed", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"summary": response.Summary,
		"ports":   response.Ports,
		"results": response.Results,
	})
}

// parseExcelFile 解析Excel文件
func parseExcelFile(file io.Reader) ([]string, error) {
	f, err := excelize.OpenReader(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// 获取第一个工作表
	sheetName := f.GetSheetName(0)
	if sheetName == "" {
		return nil, fmt.Errorf("no sheet found")
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, err
	}

	var urls []string
	for i, row := range rows {
		if i == 0 {
			// 跳过表头
			continue
		}
		if len(row) > 0 && row[0] != "" {
			urls = append(urls, strings.TrimSpace(row[0]))
		}
	}

	return urls, nil
}

// parseCSVFile 解析CSV文件
func parseCSVFile(file io.Reader) ([]string, error) {
	reader := csv.NewReader(file)
	var urls []string
	isFirstRow := true

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if isFirstRow {
			isFirstRow = false
			// 检查是否是表头
			if len(record) > 0 && (strings.ToLower(record[0]) == "url" ||
				strings.ToLower(record[0]) == "address" ||
				strings.ToLower(record[0]) == "网址") {
				continue
			}
		}

		if len(record) > 0 && record[0] != "" {
			urls = append(urls, strings.TrimSpace(record[0]))
		}
	}

	return urls, nil
}

// parseTXTFile 解析TXT文件
func parseTXTFile(file io.Reader) ([]string, error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	var urls []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			urls = append(urls, line)
		}
	}

	return urls, nil
}

// filterValidURLs 过滤有效URL
func filterValidURLs(urls []string) []string {
	var valid []string
	seen := make(map[string]bool)

	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			continue
		}

		// 简单URL验证（使用预编译正则）
		if reURLPattern.MatchString(u) {
			valid = append(valid, u)
			seen[u] = true
		}
	}

	return valid
}
