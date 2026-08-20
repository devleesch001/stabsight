package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// BuildInfo stores version and build metadata.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// GlobalFlags holds global CLI flag values.
type GlobalFlags struct {
	ConfigFile   string
	LogLevel     string
	LogFormat    string
	OTLPEndpoint string
	MetricsAddr  string
}

// NewRootCmd creates and returns the root cobra Command.
func NewRootCmd(info BuildInfo) *cobra.Command {
	flags := &GlobalFlags{}

	rootCmd := &cobra.Command{
		Use:   "stabsight",
		Short: "stabsight is a network observability and diagnostic agent",
		Long: `stabsight is an autonomous, modern network observability agent designed
to finely diagnose connectivity degradations by correlating multiple types of measurements
(ICMP, DNS, HTTP, TCP, MTR, Speedtest) with native OpenTelemetry integration.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global persistent flags
	rootCmd.PersistentFlags().StringVarP(&flags.ConfigFile, "config", "c", "config.yaml", "Path to YAML configuration file")
	rootCmd.PersistentFlags().StringVarP(&flags.LogLevel, "log-level", "l", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&flags.LogFormat, "log-format", "json", "Log format (json, common)")
	rootCmd.PersistentFlags().StringVar(&flags.OTLPEndpoint, "otlp-endpoint", "localhost:4317", "OpenTelemetry collector endpoint (gRPC/HTTP)")
	rootCmd.PersistentFlags().StringVar(&flags.MetricsAddr, "metrics-addr", ":9090", "Address to expose Prometheus pull /metrics endpoint")

	// Add subcommands
	rootCmd.AddCommand(newRunCmd(flags))
	rootCmd.AddCommand(newVersionCmd(info))

	return rootCmd
}

// Execute runs the root CLI command.
func Execute(info BuildInfo) {
	rootCmd := NewRootCmd(info)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
