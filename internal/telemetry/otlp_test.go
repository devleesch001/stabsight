package telemetry_test

import (
	"context"
	"testing"
	"time"

	"github.com/devleesch001/stabsight/internal/telemetry"
)

func TestNewOTLPExporter_GRPC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	reader, err := telemetry.NewOTLPExporter(ctx, telemetry.OTLPConfig{
		Endpoint: "localhost:4317",
		Protocol: "grpc",
		Insecure: true,
		Interval: 1 * time.Second,
		Timeout:  1 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create gRPC OTLP exporter: %v", err)
	}
	defer func() {
		_ = reader.Shutdown(context.Background())
	}()

	if reader == nil {
		t.Fatal("expected non-nil reader")
	}
}

func TestNewOTLPExporter_HTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	reader, err := telemetry.NewOTLPExporter(ctx, telemetry.OTLPConfig{
		Endpoint: "http://localhost:4318",
		Protocol: "http",
		Interval: 1 * time.Second,
		Timeout:  1 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create HTTP OTLP exporter: %v", err)
	}
	defer func() {
		_ = reader.Shutdown(context.Background())
	}()

	if reader == nil {
		t.Fatal("expected non-nil reader")
	}
}

func TestNewOTLPExporter_AutoDetectProtocol(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Detects http from prefix
	readerHTTP, err := telemetry.NewOTLPExporter(ctx, telemetry.OTLPConfig{
		Endpoint: "http://localhost:4318",
	})
	if err != nil {
		t.Fatalf("failed to create auto-detected HTTP exporter: %v", err)
	}
	_ = readerHTTP.Shutdown(context.Background())

	// Detects grpc from standard host:port
	readerGRPC, err := telemetry.NewOTLPExporter(ctx, telemetry.OTLPConfig{
		Endpoint: "localhost:4317",
	})
	if err != nil {
		t.Fatalf("failed to create auto-detected gRPC exporter: %v", err)
	}
	_ = readerGRPC.Shutdown(context.Background())
}

func TestNewOTLPExporter_EnvOverride(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	reader, err := telemetry.NewOTLPExporter(ctx, telemetry.OTLPConfig{
		Endpoint: "localhost:4318",
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("failed to create exporter with env override: %v", err)
	}
	_ = reader.Shutdown(context.Background())
}

func TestNewOTLPExporter_InvalidProtocol(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := telemetry.NewOTLPExporter(ctx, telemetry.OTLPConfig{
		Protocol: "invalid_proto",
	})
	if err == nil {
		t.Fatal("expected error on invalid protocol, got nil")
	}
}
