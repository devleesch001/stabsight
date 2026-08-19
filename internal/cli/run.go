package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/devleesch001/stabsight/internal/config"
	"github.com/devleesch001/stabsight/internal/logging"
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

			// Initialize global structured logger with resolved log level
			logger := logging.Init(cfg.LogLevel)
			logger.Info().
				Str("log_level", cfg.LogLevel).
				Str("metrics_addr", cfg.MetricsAddr).
				Str("otlp_endpoint", cfg.OTLPEndpoint).
				Int("targets_count", len(cfg.Targets)).
				Msg("Starting stabsight agent")

			return nil
		},
	}

	return runCmd
}
