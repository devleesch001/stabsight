package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newRunCmd(flags *GlobalFlags) *cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Start monitoring probes according to configuration",
		Long:  `Run starts all configured network probes and exports metrics via OpenTelemetry and Prometheus endpoint.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			// Runner implementation will be wired with config/scheduler in subsequent tasks
			fmt.Printf("Starting stabsight (config: %s, log-level: %s, metrics-addr: %s, otlp-endpoint: %s)\n",
				flags.ConfigFile, flags.LogLevel, flags.MetricsAddr, flags.OTLPEndpoint)
			return nil
		},
	}

	return runCmd
}
