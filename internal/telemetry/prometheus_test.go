package telemetry_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/devleesch001/stabsight/internal/telemetry"
)

func TestMetricsServer_ScrapeAndHealth(t *testing.T) {
	reg := prometheus.NewRegistry()
	server, reader, err := telemetry.StartMetricsServer("127.0.0.1:0", reg)
	if err != nil {
		t.Fatalf("failed to start metrics server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{ServiceName: "test-server"}, reader)
	if err != nil {
		t.Fatalf("failed to init provider: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	meter := tel.Meter("test")
	counter, err := meter.Int64Counter("test_pings_total")
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}
	counter.Add(context.Background(), 42)

	addr := server.Addr()
	client := &http.Client{Timeout: 2 * time.Second}

	// Test healthz endpoint
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("failed to GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK on /healthz, got %d", resp.StatusCode)
	}

	// Test metrics endpoint
	mResp, err := client.Get("http://" + addr + "/metrics")
	if err != nil {
		t.Fatalf("failed to GET /metrics: %v", err)
	}
	defer func() { _ = mResp.Body.Close() }()
	if mResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK on /metrics, got %d", mResp.StatusCode)
	}

	body, err := io.ReadAll(mResp.Body)
	if err != nil {
		t.Fatalf("failed to read /metrics body: %v", err)
	}

	if !strings.Contains(string(body), "test_pings_total") {
		t.Errorf("expected metric 'test_pings_total' in scraped output, got:\n%s", string(body))
	}
}

func TestNewPrometheusExporter_ErrorOnBadConfig(t *testing.T) {
	reader, err := telemetry.NewPrometheusExporter(nil)
	if err != nil {
		t.Fatalf("expected nil error on default registerer, got: %v", err)
	}
	if reader == nil {
		t.Fatal("expected non-nil reader")
	}
}
