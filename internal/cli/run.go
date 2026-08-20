package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/devleesch001/stabsight/internal/config"
	"github.com/devleesch001/stabsight/internal/diagnostic"
	"github.com/devleesch001/stabsight/internal/logging"
	"github.com/devleesch001/stabsight/internal/probes"
	"github.com/devleesch001/stabsight/internal/scheduler"
	"github.com/devleesch001/stabsight/internal/telemetry"
)

func newRunCmd(flags *GlobalFlags) *cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Start monitoring probes according to configuration",
		Long:  `Run starts all configured network probes and exports metrics via OpenTelemetry and Prometheus endpoint.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Load and merge configuration (flags > env > file > defaults)
			cfg, err := config.Load(nil, flags.ConfigFile, cmd.Flags())
			if err != nil {
				return fmt.Errorf("configuration error: %w", err)
			}

			// Initialize global structured logger with resolved log level and format
			logger := logging.Init(cfg.LogLevel, cfg.LogFormat)
			logger.Info().
				Str("log_level", cfg.LogLevel).
				Str("log_format", cfg.LogFormat).
				Str("metrics_addr", cfg.MetricsAddr).
				Str("otlp_endpoint", cfg.OTLPEndpoint).
				Int("targets_count", len(cfg.Targets)).
				Msg("Starting stabsight agent")

			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			return RunApp(ctx, cfg)
		},
	}

	return runCmd
}

// RunApp wires all subsystems, spawns probes and manages the application lifecycle.
func RunApp(ctx context.Context, cfg *config.Config) error {
	logger := logging.Get()

	var readers []sdkmetric.Reader
	var metricsServer *telemetry.MetricsServer

	// 1. Prometheus metrics server (Pull mode)
	if cfg.MetricsAddr != "" {
		srv, reader, err := telemetry.StartMetricsServer(cfg.MetricsAddr, nil)
		if err != nil {
			return fmt.Errorf("failed to start prometheus metrics server on %s: %w", cfg.MetricsAddr, err)
		}
		metricsServer = srv
		readers = append(readers, reader)
		logger.Info().Str("addr", cfg.MetricsAddr).Msg("Prometheus metrics endpoint ready at /metrics")
	}

	// 2. OTLP Exporter (Push mode)
	if cfg.OTLPEndpoint != "" {
		otlpReader, err := telemetry.NewOTLPExporter(ctx, telemetry.OTLPConfig{
			Endpoint: cfg.OTLPEndpoint,
		})
		if err != nil {
			return fmt.Errorf("failed to initialize OTLP exporter to %s: %w", cfg.OTLPEndpoint, err)
		}
		readers = append(readers, otlpReader)
		logger.Info().Str("endpoint", cfg.OTLPEndpoint).Msg("OTLP push metric exporter configured")
	}

	// 3. OpenTelemetry Provider
	telProvider, err := telemetry.NewProvider(telemetry.ProviderConfig{
		ServiceName:      "stabsight",
		HistogramMode:    cfg.HistogramMode,
		HistogramBuckets: cfg.HistogramBuckets,
	}, readers...)
	if err != nil {
		return fmt.Errorf("failed to initialize telemetry provider: %w", err)
	}
	defer func() {
		shutdownCtx, sCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer sCancel()
		_ = telProvider.Shutdown(shutdownCtx)
	}()

	// 4. Metrics Instruments
	metrics, err := telemetry.NewMetrics(telProvider.Meter("stabsight"))
	if err != nil {
		return fmt.Errorf("failed to initialize metrics instruments: %w", err)
	}

	// 5. Diagnostic Engine
	diagEngine := diagnostic.NewEngine(logger, 0.05, 250*time.Millisecond)

	// 6. Probe Scheduler
	sched := scheduler.NewScheduler()

	// 7. Instantiate and register probe workers
	for _, target := range cfg.Targets {
		if target.Probes.ICMP != nil {
			probe := probes.NewICMPProbe(target.Name, target.Host, target.Probes.ICMP, target.IPVersion, metrics, nil)
			if err := sched.RegisterWorker(probe); err != nil {
				logger.Warn().Err(err).Str("probe", probe.Name()).Msg("Failed to register probe")
			}
		}

		if target.Probes.DNS != nil {
			probe := probes.NewDNSProbe(target.Name, target.Host, target.Probes.DNS, target.IPVersion, metrics, nil)
			if err := sched.RegisterWorker(probe); err != nil {
				logger.Warn().Err(err).Str("probe", probe.Name()).Msg("Failed to register probe")
			}
		}

		if target.Probes.HTTP != nil {
			probe := probes.NewHTTPProbe(target.Name, target.Host, target.Probes.HTTP, target.IPVersion, metrics, nil)
			if err := sched.RegisterWorker(probe); err != nil {
				logger.Warn().Err(err).Str("probe", probe.Name()).Msg("Failed to register probe")
			}
		}

		if target.Probes.TCP != nil {
			probe := probes.NewTCPProbe(target.Name, target.Host, target.Probes.TCP, target.IPVersion, metrics, nil)
			if err := sched.RegisterWorker(probe); err != nil {
				logger.Warn().Err(err).Str("probe", probe.Name()).Msg("Failed to register probe")
			}
		}

		if target.Probes.MTR != nil {
			targetName := target.Name
			onResult := func(res *probes.MTRResult) {
				diagEngine.Analyze(targetName, 0, 0, res)
			}
			probe := probes.NewMTRProbe(target.Name, target.Host, target.Probes.MTR, metrics, nil, onResult)
			if err := sched.RegisterWorker(probe); err != nil {
				logger.Warn().Err(err).Str("probe", probe.Name()).Msg("Failed to register probe")
			}
		}

		if target.Probes.Speedtest != nil {
			probe := probes.NewSpeedtestProbe(target.Name, target.Probes.Speedtest, sched, metrics, nil)
			if err := sched.RegisterWorker(probe); err != nil {
				logger.Warn().Err(err).Str("probe", probe.Name()).Msg("Failed to register probe")
			}
		}
	}

	logger.Info().Int("worker_count", sched.WorkerCount()).Msg("All probe workers registered, starting scheduler")

	// 8. Start Scheduler
	if err := sched.Start(ctx); err != nil {
		return fmt.Errorf("failed to start probe scheduler: %w", err)
	}

	// 9. Block until context cancellation / OS signal
	<-ctx.Done()
	logger.Info().Msg("Shutdown signal received, gracefully terminating stabsight agent...")

	// 10. Graceful shutdown
	sched.Stop()

	if metricsServer != nil {
		shutdownCtx, sCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer sCancel()
		_ = metricsServer.Shutdown(shutdownCtx)
	}

	logger.Info().Msg("stabsight agent shutdown complete")
	return nil
}
