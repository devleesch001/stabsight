package telemetry_test

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/devleesch001/stabsight/internal/telemetry"
)

func TestNewProvider_Defaults(t *testing.T) {
	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{})
	if err != nil {
		t.Fatalf("failed to create telemetry provider: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tel.Shutdown(ctx)
	}()

	meter := tel.Meter("")
	if meter == nil {
		t.Fatal("expected non-nil meter")
	}

	if tel.MeterProvider() == nil {
		t.Fatal("expected non-nil MeterProvider")
	}

	// Verify global provider is set
	globalMeter := otel.GetMeterProvider().Meter("test")
	if globalMeter == nil {
		t.Fatal("expected non-nil global meter")
	}
}

func TestNewProvider_CustomAttributes(t *testing.T) {
	tel, err := telemetry.NewProvider(telemetry.ProviderConfig{
		ServiceName:    "custom-agent",
		ServiceVersion: "1.2.3",
		Environment:    "staging",
		ExtraAttrs: []attribute.KeyValue{
			attribute.String("region", "eu-west-3"),
		},
	})
	if err != nil {
		t.Fatalf("failed to create custom telemetry provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := tel.Shutdown(ctx); err != nil {
		t.Fatalf("unexpected error on shutdown: %v", err)
	}

	// Idempotent shutdown
	if err := tel.Shutdown(ctx); err != nil {
		t.Fatalf("unexpected error on second shutdown: %v", err)
	}
}
