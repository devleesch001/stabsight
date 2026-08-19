package telemetry

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const (
	// DefaultExportInterval is the default period between OTLP metric flushes.
	DefaultExportInterval = 5 * time.Second
	// DefaultExportTimeout is the default timeout for OTLP export calls.
	DefaultExportTimeout = 5 * time.Second
)

// OTLPConfig defines connection settings for push metric export via OTLP.
type OTLPConfig struct {
	Endpoint string        // e.g., "localhost:4317" or "http://localhost:4318"
	Protocol string        // "grpc" or "http/protobuf" / "http"
	Insecure bool          // use plaintext / disable TLS
	Interval time.Duration // export flush interval
	Timeout  time.Duration // export call timeout
}

// NewOTLPExporter instantiates an OTLP metric exporter and wraps it in a PeriodicReader.
func NewOTLPExporter(ctx context.Context, cfg OTLPConfig) (sdkmetric.Reader, error) {
	interval := cfg.Interval
	if interval <= 0 {
		interval = DefaultExportInterval
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultExportTimeout
	}

	protocol := strings.ToLower(strings.TrimSpace(cfg.Protocol))
	if protocol == "" {
		// Detect protocol from environment variable if present
		if envProto := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"); envProto != "" {
			protocol = strings.ToLower(strings.TrimSpace(envProto))
		} else if envMetricProto := os.Getenv("OTEL_EXPORTER_OTLP_METRICS_PROTOCOL"); envMetricProto != "" {
			protocol = strings.ToLower(strings.TrimSpace(envMetricProto))
		}
	}

	// Auto-detect based on endpoint format/port if still unspecified
	if protocol == "" {
		if strings.HasPrefix(cfg.Endpoint, "http://") || strings.HasPrefix(cfg.Endpoint, "https://") || strings.HasSuffix(cfg.Endpoint, ":4318") {
			protocol = "http"
		} else {
			protocol = "grpc"
		}
	}

	var exporter sdkmetric.Exporter
	var err error

	switch protocol {
	case "http", "http/protobuf", "http/json":
		opts := []otlpmetrichttp.Option{
			otlpmetrichttp.WithTimeout(timeout),
		}
		if cfg.Endpoint != "" {
			if strings.HasPrefix(cfg.Endpoint, "http://") {
				opts = append(opts, otlpmetrichttp.WithEndpoint(strings.TrimPrefix(cfg.Endpoint, "http://")), otlpmetrichttp.WithInsecure())
			} else if strings.HasPrefix(cfg.Endpoint, "https://") {
				opts = append(opts, otlpmetrichttp.WithEndpoint(strings.TrimPrefix(cfg.Endpoint, "https://")))
			} else {
				opts = append(opts, otlpmetrichttp.WithEndpoint(cfg.Endpoint))
				if cfg.Insecure {
					opts = append(opts, otlpmetrichttp.WithInsecure())
				}
			}
		} else if cfg.Insecure {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}
		exporter, err = otlpmetrichttp.New(ctx, opts...)

	case "grpc":
		opts := []otlpmetricgrpc.Option{
			otlpmetricgrpc.WithTimeout(timeout),
		}
		if cfg.Endpoint != "" {
			endpoint := cfg.Endpoint
			endpoint = strings.TrimPrefix(endpoint, "http://")
			endpoint = strings.TrimPrefix(endpoint, "https://")
			opts = append(opts, otlpmetricgrpc.WithEndpoint(endpoint))
		}
		if cfg.Insecure || !strings.HasPrefix(cfg.Endpoint, "https://") {
			opts = append(opts, otlpmetricgrpc.WithInsecure())
		}
		exporter, err = otlpmetricgrpc.New(ctx, opts...)

	default:
		return nil, fmt.Errorf("unsupported OTLP protocol: %q", protocol)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP metric exporter (%s): %w", protocol, err)
	}

	reader := sdkmetric.NewPeriodicReader(
		exporter,
		sdkmetric.WithInterval(interval),
		sdkmetric.WithTimeout(timeout),
	)

	return reader, nil
}
