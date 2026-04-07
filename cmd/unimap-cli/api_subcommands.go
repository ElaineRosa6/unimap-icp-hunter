package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"strings"
	"time"

	"github.com/unimap-icp-hunter/project/internal/model"
)

type apiQueryResponse struct {
	Query       string               `json:"query"`
	Assets      []model.UnifiedAsset `json:"assets"`
	TotalCount  int                  `json:"totalCount"`
	EngineStats map[string]int       `json:"engineStats"`
	Errors      []string             `json:"errors"`
}

type apiTamperResponse struct {
	Success bool                     `json:"success"`
	Mode    string                   `json:"mode"`
	Summary map[string]int           `json:"summary"`
	Results []map[string]interface{} `json:"results"`
}

type apiScreenshotBatchResponse struct {
	BatchID string                   `json:"batch_id"`
	Total   int                      `json:"total"`
	Success int                      `json:"success"`
	Failed  int                      `json:"failed"`
	Results []map[string]interface{} `json:"results"`
}

func runAPISubcommand(command string, args []string) bool {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "query":
		runAPIQuery(args)
		return true
	case "tamper-check":
		runAPITamperCheck(args)
		return true
	case "screenshot-batch":
		runAPIScreenshotBatch(args)
		return true
	default:
		return false
	}
}

func runAPIQuery(args []string) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	query := fs.String("q", "", "UQL query string")
	engines := fs.String("e", "", "Comma-separated engines, e.g. fofa,hunter")
	limit := fs.Int("l", 100, "Result limit")
	apiBase := fs.String("api-base", "http://127.0.0.1:8448", "Web API base URL")
	timeoutSec := fs.Int("timeout", 60, "HTTP timeout in seconds")
	output := fs.String("o", "", "Output file path (csv/json/xlsx)")
	_ = fs.Parse(args)

	if strings.TrimSpace(*query) == "" {
		fmt.Fprintln(os.Stderr, "Error: -q is required")
		os.Exit(1)
	}

	values := neturl.Values{}
	values.Set("query", *query)
	if strings.TrimSpace(*engines) != "" {
		values.Set("engines", *engines)
	}
	if *limit > 0 {
		values.Set("page_size", fmt.Sprintf("%d", *limit))
	}

	var resp apiQueryResponse
	if err := doFormRequest(*apiBase, "/api/query", *timeoutSec, values, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "API query failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d results.\n", resp.TotalCount)
	for engine, count := range resp.EngineStats {
		fmt.Printf("  %s: %d\n", engine, count)
	}
	for _, errMsg := range resp.Errors {
		fmt.Printf("  Error: %s\n", errMsg)
	}

	if strings.TrimSpace(*output) != "" {
		if err := saveResults(resp.Assets, *output); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save results: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Results saved to %s\n", *output)
		return
	}

	for _, asset := range resp.Assets {
		fmt.Printf("%s\t%s:%d\t%s\n", asset.IP, asset.Host, asset.Port, asset.Title)
	}
}

func runAPITamperCheck(args []string) {
	fs := flag.NewFlagSet("tamper-check", flag.ExitOnError)
	urlsText := fs.String("urls", "", "Comma-separated URLs")
	concurrency := fs.Int("concurrency", 5, "Concurrency")
	mode := fs.String("mode", "relaxed", "Tamper mode: relaxed|strict")
	apiBase := fs.String("api-base", "http://127.0.0.1:8448", "Web API base URL")
	timeoutSec := fs.Int("timeout", 120, "HTTP timeout in seconds")
	output := fs.String("o", "", "Output JSON file path")
	_ = fs.Parse(args)

	urls := splitCSVText(*urlsText)
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "Error: --urls is required")
		os.Exit(1)
	}

	payload := map[string]interface{}{
		"urls":        urls,
		"concurrency": *concurrency,
		"mode":        strings.ToLower(strings.TrimSpace(*mode)),
	}
	var resp apiTamperResponse
	if err := doJSONRequest(*apiBase, "/api/tamper/check", *timeoutSec, payload, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "API tamper-check failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Tamper check completed (mode=%s).\n", resp.Mode)
	for k, v := range resp.Summary {
		fmt.Printf("  %s: %d\n", k, v)
	}
	fmt.Printf("  results: %d\n", len(resp.Results))

	if strings.TrimSpace(*output) != "" {
		if err := writeJSONFile(*output, resp); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save output: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Output saved to %s\n", *output)
	}
}

func runAPIScreenshotBatch(args []string) {
	fs := flag.NewFlagSet("screenshot-batch", flag.ExitOnError)
	urlsText := fs.String("urls", "", "Comma-separated URLs")
	batchID := fs.String("batch-id", "", "Batch id (optional)")
	concurrency := fs.Int("concurrency", 5, "Concurrency")
	apiBase := fs.String("api-base", "http://127.0.0.1:8448", "Web API base URL")
	timeoutSec := fs.Int("timeout", 300, "HTTP timeout in seconds")
	output := fs.String("o", "", "Output JSON file path")
	_ = fs.Parse(args)

	urls := splitCSVText(*urlsText)
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "Error: --urls is required")
		os.Exit(1)
	}

	payload := map[string]interface{}{
		"urls":        urls,
		"batch_id":    strings.TrimSpace(*batchID),
		"concurrency": *concurrency,
	}
	var resp apiScreenshotBatchResponse
	if err := doJSONRequest(*apiBase, "/api/screenshot/batch-urls", *timeoutSec, payload, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "API screenshot-batch failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Screenshot batch completed: batch_id=%s total=%d success=%d failed=%d\n", resp.BatchID, resp.Total, resp.Success, resp.Failed)

	if strings.TrimSpace(*output) != "" {
		if err := writeJSONFile(*output, resp); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save output: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Output saved to %s\n", *output)
	}
}

func doFormRequest(apiBase, path string, timeoutSec int, values neturl.Values, out interface{}) error {
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if base == "" {
		return fmt.Errorf("api base is empty")
	}
	client := &http.Client{Timeout: time.Duration(maxInt(timeoutSec, 1)) * time.Second}
	resp, err := client.PostForm(base+path, values)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response failed: %w", err)
	}
	return nil
}

func doJSONRequest(apiBase, path string, timeoutSec int, payload interface{}, out interface{}) error {
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if base == "" {
		return fmt.Errorf("api base is empty")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: time.Duration(maxInt(timeoutSec, 1)) * time.Second}
	resp, err := client.Post(base+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode response failed: %w", err)
	}
	return nil
}

func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func splitCSVText(raw string) []string {
	parts := strings.Split(strings.ReplaceAll(raw, "\n", ","), ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
