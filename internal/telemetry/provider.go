package telemetry

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	// DefaultServiceName is the default OpenTelemetry service name.
	DefaultServiceName = "stabsight"
	// ScopeName is the instrumentation scope name.
	ScopeName = "github.com/devleesch001/stabsight"
)

// ProviderConfig configures the OpenTelemetry telemetry provider.
type ProviderConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	ExtraAttrs     []attribute.KeyValue
}

// Telemetry wraps the OpenTelemetry MeterProvider and handles shutdown.
type Telemetry struct {
	provider *sdkmetric.MeterProvider
	mu       sync.Mutex
	closed   bool
}

// NewProvider creates and configures a new OpenTelemetry MeterProvider with Exponential Histogram aggregation.
func NewProvider(cfg ProviderConfig, readers ...sdkmetric.Reader) (*Telemetry, error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = DefaultServiceName
	}

	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
	}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}
	if cfg.Environment != "" {
		attrs = append(attrs, attribute.String("environment", cfg.Environment))
	}
	attrs = append(attrs, cfg.ExtraAttrs...)

	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(attrs...),
		resource.WithSchemaURL(semconv.SchemaURL),
	)
	if err != nil && !errors.Is(err, resource.ErrPartialResource) {
		return nil, fmt.Errorf("failed to create otel resource: %w", err)
	}

	// Exponential/Native Histograms view for high precision latency/jitter without predefined linear buckets (NFR3)
	exponentialHistogramView := sdkmetric.NewView(
		sdkmetric.Instrument{Kind: sdkmetric.InstrumentKindHistogram},
		sdkmetric.Stream{
			Aggregation: sdkmetric.AggregationBase2ExponentialHistogram{
				MaxSize:  160,
				MaxScale: 20,
			},
		},
	)

	opts := []sdkmetric.Option{
		sdkmetric.WithResource(res),
		sdkmetric.WithView(exponentialHistogramView),
	}

	for _, reader := range readers {
		if reader != nil {
			opts = append(opts, sdkmetric.WithReader(reader))
		}
	}

	mp := sdkmetric.NewMeterProvider(opts...)
	otel.SetMeterProvider(mp)

	return &Telemetry{
		provider: mp,
	}, nil
}

// Meter returns a named OpenTelemetry Meter.
func (t *Telemetry) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	if name == "" {
		name = ScopeName
	}
	return t.provider.Meter(name, opts...)
}

// MeterProvider returns the underlying sdkmetric.MeterProvider.
func (t *Telemetry) MeterProvider() *sdkmetric.MeterProvider {
	return t.provider
}

// Shutdown shuts down the MeterProvider, flushing all telemetry readers.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	if t.provider != nil {
		return t.provider.Shutdown(ctx)
	}
	return nil
}
