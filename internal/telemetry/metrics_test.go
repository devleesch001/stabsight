package telemetry_test

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/devleesch001/stabsight/internal/telemetry"
)

func TestNewMetrics_NilMeter(t *testing.T) {
	_, err := telemetry.NewMetrics(nil)
	if err == nil {
		t.Fatal("expected error creating metrics with nil meter, got nil")
	}
}

func TestMetrics_RecordInstruments(t *testing.T) {
	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{ServiceName: "metrics-test"})
	if err != nil {
		t.Fatalf("failed to create telemetry provider: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	metrics, err := telemetry.NewMetrics(tel.Meter("test"))
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	ctx := context.Background()

	// Record RTT
	metrics.RecordRTT(ctx, 0.0125, "google-dns", "icmp", "ipv4")
	metrics.RecordRTT(ctx, 0.0250, "google-dns", "dns", "ipv4", attribute.String("record_type", "A"))

	// Record Jitter
	metrics.RecordJitter(ctx, 0.0012, "google-dns", "icmp", "ipv4")

	// Record Packet Loss
	metrics.RecordPacketLoss(ctx, 0.0, "google-dns", "icmp", "ipv4")
	metrics.RecordPacketLoss(ctx, 0.05, "flaky-target", "icmp", "ipv4")

	// Record Speedtest
	metrics.RecordSpeedtest(ctx, 125000000.0, "isp-speed", "download")
	metrics.RecordSpeedtest(ctx, 50000000.0, "isp-speed", "upload")
}
