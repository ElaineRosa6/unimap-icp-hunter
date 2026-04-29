package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/appversion"
	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/exporter"
	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/service"
)

func main() {
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		if runAPISubcommand(os.Args[1], os.Args[2:]) {
			return
		}
	}

	// Parse flags
	queryPtr := flag.String("q", "", "Query string (e.g., 'country=\"CN\"')")
	enginesPtr := flag.String("e", "", "Comma-separated list of engines to use (e.g., 'fofa,hunter')")
	limitPtr := flag.Int("l", 100, "Limit number of results")
	outputPtr := flag.String("o", "", "Output file path (e.g., 'results.csv' or 'results.json')")
	configPtr := flag.String("c", "configs/config.yaml", "Configuration file path")
	fofaCookiePtr := flag.String("cookie-fofa", "", "FOFA cookie header (e.g., 'session=xxx; token=yyy')")
	hunterCookiePtr := flag.String("cookie-hunter", "", "Hunter cookie header (e.g., 'session=xxx; token=yyy')")
	quakeCookiePtr := flag.String("cookie-quake", "", "Quake cookie header (e.g., 'session=xxx; token=yyy')")
	zoomeyeCookiePtr := flag.String("cookie-zoomeye", "", "ZoomEye cookie header (e.g., 'session=xxx; token=yyy')")
	versionPtr := flag.Bool("version", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "UniMap CLI %s\n\n", appversion.Full())
		fmt.Fprintf(os.Stderr, "API-first subcommands:\n")
		fmt.Fprintf(os.Stderr, "  %s query -q '<uql>' [-e fofa,hunter] [-l 100] [--api-base http://127.0.0.1:8448]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s tamper-check --urls 'https://a.com,https://b.com' [--mode relaxed]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s screenshot-batch --urls 'https://a.com,https://b.com'\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Legacy direct-engine mode:\n")
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n  %s -q 'app=\"Apache\"' -e fofa,hunter -o results.csv\n", os.Args[0])
	}

	flag.Parse()

	if *versionPtr {
		fmt.Printf("UniMap CLI %s\n", appversion.Full())
		return
	}

	if *queryPtr == "" {
		fmt.Println("Error: Query string is required")
		flag.Usage()
		os.Exit(1)
	}

	// Load config
	cfgManager := config.NewManager(*configPtr)
	if err := cfgManager.Load(); err != nil {
		logger.Warnf("Failed to load config from %s: %v. Using defaults.", *configPtr, err)
	}
	cfg := cfgManager.GetConfig()
	if cfg != nil {
		if applyCookiesFromFlags(cfg, *fofaCookiePtr, *hunterCookiePtr, *quakeCookiePtr, *zoomeyeCookiePtr) {
			if err := cfgManager.Save(); err != nil {
				logger.Warnf("Failed to save cookies to %s: %v", *configPtr, err)
			}
		}
	}

	// Init service
	svc := service.NewUnifiedServiceWithConfig(cfg)

	// Register engines
	registerEngines(svc, cfg)

	// Prepare request
	var engines []string
	if *enginesPtr != "" {
		rawEngines := strings.Split(*enginesPtr, ",")
		for _, e := range rawEngines {
			e = strings.TrimSpace(e)
			if e != "" {
				engines = append(engines, e)
			}
		}
	}
	if len(engines) == 0 {
		engines = getEnabledEngines(cfg)
	}
	if len(engines) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no engines configured/enabled. Please set API keys and enable engines in %s, or specify -e.\n", *configPtr)
		os.Exit(1)
	}

	fmt.Printf("Querying with engines: %v\n", engines)

	ctx := context.Background()
	req := service.QueryRequest{
		Query:       *queryPtr,
		Engines:     engines,
		PageSize:    *limitPtr,
		ProcessData: true,
	}

	// Execute query
	resp, err := svc.Query(ctx, req)
	if err != nil {
		logger.Errorf("Query failed: %v", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d results.\n", resp.TotalCount)
	for engine, count := range resp.EngineStats {
		fmt.Printf("  %s: %d\n", engine, count)
	}
	for _, errMsg := range resp.Errors {
		fmt.Printf("  Error: %s\n", errMsg)
	}

	// Output results
	if *outputPtr != "" {
		err := saveResults(resp.Assets, *outputPtr)
		if err != nil {
			logger.Errorf("Failed to save results: %v", err)
		} else {
			fmt.Printf("Results saved to %s\n", *outputPtr)
		}
	} else {
		// Print to stdout (simple)
		for _, asset := range resp.Assets {
			fmt.Printf("%s\t%s:%d\t%s\n", asset.IP, asset.Host, asset.Port, asset.Title)
		}
	}

	// 关闭服务
	if err := svc.Shutdown(); err != nil {
		logger.Warnf("Error during shutdown: %v", err)
	}
}

func applyCookiesFromFlags(cfg *config.Config, fofa, hunter, quake, zoomeye string) bool {
	changed := false
	if strings.TrimSpace(fofa) != "" {
		cfg.Engines.Fofa.Cookies = config.ParseCookieHeader(fofa, config.DefaultCookieDomain("fofa"))
		changed = true
	}
	if strings.TrimSpace(hunter) != "" {
		cfg.Engines.Hunter.Cookies = config.ParseCookieHeader(hunter, config.DefaultCookieDomain("hunter"))
		changed = true
	}
	if strings.TrimSpace(quake) != "" {
		cfg.Engines.Quake.Cookies = config.ParseCookieHeader(quake, config.DefaultCookieDomain("quake"))
		changed = true
	}
	if strings.TrimSpace(zoomeye) != "" {
		cfg.Engines.Zoomeye.Cookies = config.ParseCookieHeader(zoomeye, config.DefaultCookieDomain("zoomeye"))
		changed = true
	}
	return changed
}

func getEnabledEngines(cfg *config.Config) []string {
	var list []string
	if cfg.Engines.Fofa.Enabled {
		list = append(list, "fofa")
	}
	if cfg.Engines.Hunter.Enabled {
		list = append(list, "hunter")
	}
	if cfg.Engines.Quake.Enabled {
		list = append(list, "quake")
	}
	if cfg.Engines.Zoomeye.Enabled {
		list = append(list, "zoomeye")
	}
	if cfg.Engines.Shodan.Enabled {
		list = append(list, "shodan")
	}
	return list
}

func registerEngines(svc *service.UnifiedService, cfg *config.Config) {
	if cfg.Engines.Fofa.Enabled {
		svc.RegisterAdapter(adapter.NewFofaAdapter(
			cfg.Engines.Fofa.BaseURL,
			cfg.Engines.Fofa.APIKey,
			cfg.Engines.Fofa.Email,
			cfg.Engines.Fofa.QPS,
			time.Duration(cfg.Engines.Fofa.Timeout)*time.Second,
		))
	}
	if cfg.Engines.Hunter.Enabled {
		svc.RegisterAdapter(adapter.NewHunterAdapter(
			cfg.Engines.Hunter.BaseURL,
			cfg.Engines.Hunter.APIKey,
			cfg.Engines.Hunter.QPS,
			time.Duration(cfg.Engines.Hunter.Timeout)*time.Second,
		))
	}
	if cfg.Engines.Zoomeye.Enabled {
		svc.RegisterAdapter(adapter.NewZoomEyeAdapter(
			cfg.Engines.Zoomeye.BaseURL,
			cfg.Engines.Zoomeye.APIKey,
			cfg.Engines.Zoomeye.QPS,
			time.Duration(cfg.Engines.Zoomeye.Timeout)*time.Second,
		))
	}
	if cfg.Engines.Quake.Enabled {
		svc.RegisterAdapter(adapter.NewQuakeAdapter(
			cfg.Engines.Quake.BaseURL,
			cfg.Engines.Quake.APIKey,
			cfg.Engines.Quake.QPS,
			time.Duration(cfg.Engines.Quake.Timeout)*time.Second,
		))
	}
	if cfg.Engines.Shodan.Enabled {
		svc.RegisterAdapter(adapter.NewShodanAdapter(
			cfg.Engines.Shodan.BaseURL,
			cfg.Engines.Shodan.APIKey,
			cfg.Engines.Shodan.QPS,
			time.Duration(cfg.Engines.Shodan.Timeout)*time.Second,
		))
	}
}

func saveResults(assets []model.UnifiedAsset, path string) error {
	// 根据文件扩展名选择导出格式
	lowerPath := strings.ToLower(path)

	switch {
	case strings.HasSuffix(lowerPath, ".json"):
		exp := exporter.NewJSONExporter()
		return exp.Export(assets, path)
	case strings.HasSuffix(lowerPath, ".xlsx") || strings.HasSuffix(lowerPath, ".xls"):
		exp := exporter.NewExcelExporter()
		return exp.Export(assets, path)
	default:
		// CSV default
		return saveResultsCSV(assets, path)
	}
}

// saveResultsCSV 保存为CSV格式
func saveResultsCSV(assets []model.UnifiedAsset, path string) error {
	// Use O_CREATE|O_EXCL to prevent overwriting existing files
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("file %q already exists, refusing to overwrite: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{"IP", "Port", "Protocol", "Domain", "Title", "Country", "City", "ISP", "Source"}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, asset := range assets {
		record := []string{
			asset.IP,
			fmt.Sprintf("%d", asset.Port),
			asset.Protocol,
			asset.Host,
			asset.Title,
			asset.CountryCode,
			asset.City,
			asset.ISP,
			asset.Source,
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	return nil
}
