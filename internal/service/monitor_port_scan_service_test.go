package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNormalizeScanPorts(t *testing.T) {
	t.Run("default ports", func(t *testing.T) {
		ports := normalizeScanPorts(nil)
		if len(ports) == 0 {
			t.Fatalf("expected default ports")
		}
	})

	t.Run("dedupe and trim invalid", func(t *testing.T) {
		ports := normalizeScanPorts([]int{8080, 8080, -1, 0, 65536, 443})
		if len(ports) != 2 {
			t.Fatalf("expected 2 valid ports, got %d", len(ports))
		}
		if ports[0] != 443 || ports[1] != 8080 {
			t.Fatalf("unexpected normalized ports: %+v", ports)
		}
	})
}

func TestCDNHelpers(t *testing.T) {
	if !isLikelyCDNString("cloudflare edge") {
		t.Fatalf("expected cloudflare marker to be detected")
	}
	if isLikelyCDNString("internal-app") {
		t.Fatalf("unexpected CDN marker for normal text")
	}
	if !isLikelyCDNIP("104.16.1.2") {
		t.Fatalf("expected known cloudflare range to match")
	}
	if isLikelyCDNIP("127.0.0.1") {
		t.Fatalf("localhost should not be treated as CDN")
	}
}

func TestScanURLPorts_LocalhostScanned(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{Handler: mux}
	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	target := fmt.Sprintf("http://127.0.0.1:%d", port)

	app := NewMonitorAppService(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := app.ScanURLPorts(ctx, []string{target}, []int{port}, 1)
	if err != nil {
		t.Fatalf("ScanURLPorts failed: %v", err)
	}
	if resp.Summary.Scanned != 1 {
		t.Fatalf("expected scanned=1, got %+v", resp.Summary)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected one result, got %d", len(resp.Results))
	}
	if resp.Results[0].Status != "scanned" {
		t.Fatalf("expected status scanned, got %s", resp.Results[0].Status)
	}

	open := resp.Results[0].OpenPorts["127.0.0.1"]
	if len(open) != 1 || open[0] != port {
		t.Fatalf("expected port %d open on 127.0.0.1, got %+v", port, resp.Results[0].OpenPorts)
	}
}

func TestScanURLPorts_CDNExcludedByHeader(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Via", "cloudflare")
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{Handler: mux}
	go func() {
		_ = srv.Serve(ln)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	target := fmt.Sprintf("http://127.0.0.1:%d", port)

	app := NewMonitorAppService(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := app.ScanURLPorts(ctx, []string{target}, []int{port}, 1)
	if err != nil {
		t.Fatalf("ScanURLPorts failed: %v", err)
	}
	if resp.Summary.CDNExcluded != 1 {
		t.Fatalf("expected cdnExcluded=1, got %+v", resp.Summary)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected one result, got %d", len(resp.Results))
	}
	if resp.Results[0].Status != "cdn_excluded" {
		t.Fatalf("expected status cdn_excluded, got %s", resp.Results[0].Status)
	}
	if !resp.Results[0].CDNDetected {
		t.Fatalf("expected CDNDetected=true")
	}
}
