package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// MetricsServer hosts the Prometheus pull /metrics HTTP endpoint.
type MetricsServer struct {
	server   *http.Server
	listener net.Listener
	reader   sdkmetric.Reader
	mu       sync.Mutex
	closed   bool
}

// NewPrometheusExporter creates a Prometheus metric reader for the OTel MeterProvider
// using a custom or default Prometheus Registerer.
func NewPrometheusExporter(reg prometheus.Registerer) (sdkmetric.Reader, error) {
	opts := []otelprom.Option{}
	if reg != nil {
		opts = append(opts, otelprom.WithRegisterer(reg))
	}

	reader, err := otelprom.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus exporter: %w", err)
	}
	return reader, nil
}

// StartMetricsServer binds to the given address and serves Prometheus /metrics.
func StartMetricsServer(addr string, reg *prometheus.Registry) (*MetricsServer, sdkmetric.Reader, error) {
	if reg == nil {
		reg = prometheus.NewRegistry()
	}

	reader, err := NewPrometheusExporter(reg)
	if err != nil {
		return nil, nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	// Health check endpoint
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to listen on %q: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	ms := &MetricsServer{
		server:   srv,
		listener: listener,
		reader:   reader,
	}

	go func() {
		if serveErr := srv.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			// Logged or handled upstream
			_ = serveErr
		}
	}()

	return ms, reader, nil
}

// Addr returns the bound address of the server.
func (ms *MetricsServer) Addr() string {
	if ms.listener != nil {
		return ms.listener.Addr().String()
	}
	return ""
}

// Reader returns the OTel metric reader associated with this Prometheus exporter.
func (ms *MetricsServer) Reader() sdkmetric.Reader {
	return ms.reader
}

// Shutdown gracefully shuts down the metrics HTTP server.
func (ms *MetricsServer) Shutdown(ctx context.Context) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.closed {
		return nil
	}
	ms.closed = true

	if ms.server != nil {
		return ms.server.Shutdown(ctx)
	}
	return nil
}
