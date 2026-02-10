package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/service"
)

func main() {
	// Parse flags
	queryPtr := flag.String("q", "", "Query string (e.g., 'country=\"CN\"')")
	enginesPtr := flag.String("e", "", "Comma-separated list of engines to use (e.g., 'fofa,hunter')")
	limitPtr := flag.Int("l", 100, "Limit number of results")
	outputPtr := flag.String("o", "", "Output file path (e.g., 'results.csv' or 'results.json')")
	configPtr := flag.String("c", "configs/config.yaml", "Configuration file path")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n  %s -q 'app=\"Apache\"' -e fofa,hunter -o results.csv\n", os.Args[0])
	}

	flag.Parse()

	if *queryPtr == "" {
		fmt.Println("Error: Query string is required")
		flag.Usage()
		os.Exit(1)
	}

	// Load config
	cfgManager := config.NewManager(*configPtr)
	if err := cfgManager.Load(); err != nil {
		log.Printf("Warning: Failed to load config from %s: %v. Using defaults.", *configPtr, err)
	}
	cfg := cfgManager.GetConfig()

	// Init service
	svc := service.NewUnifiedService()

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
		log.Fatalf("Query failed: %v", err)
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
			log.Printf("Failed to save results: %v", err)
		} else {
			fmt.Printf("Results saved to %s\n", *outputPtr)
		}
	} else {
		// Print to stdout (simple)
		for _, asset := range resp.Assets {
			fmt.Printf("%s\t%s:%d\t%s\n", asset.IP, asset.Host, asset.Port, asset.Title)
		}
	}
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
}

func saveResults(assets []model.UnifiedAsset, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if strings.HasSuffix(strings.ToLower(path), ".json") {
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		return enc.Encode(assets)
	}

	// CSV default
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
