package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"

	"github.com/devleesch001/stabsight/internal/config"
)

func createTempConfigFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}
	return filePath
}

func TestPriority_Defaults(t *testing.T) {
	v := config.NewViper()
	cfg, err := config.Load(v, "", nil)
	if err != nil {
		t.Fatalf("failed to load default config: %v", err)
	}

	if cfg.LogLevel != config.DefaultLogLevel {
		t.Errorf("expected log_level %q, got %q", config.DefaultLogLevel, cfg.LogLevel)
	}
	if cfg.MetricsAddr != config.DefaultMetricsAddr {
		t.Errorf("expected metrics_addr %q, got %q", config.DefaultMetricsAddr, cfg.MetricsAddr)
	}
	if cfg.OTLPEndpoint != config.DefaultOTLPEndpoint {
		t.Errorf("expected otlp_endpoint %q, got %q", config.DefaultOTLPEndpoint, cfg.OTLPEndpoint)
	}
}

func TestPriority_FileOverridesDefault(t *testing.T) {
	fileContent := `
log_level: "warn"
metrics_addr: ":9091"
otlp_endpoint: "collector.internal:4317"
`
	cfgFile := createTempConfigFile(t, fileContent)
	v := config.NewViper()
	cfg, err := config.Load(v, cfgFile, nil)
	if err != nil {
		t.Fatalf("failed to load config from file: %v", err)
	}

	if cfg.LogLevel != "warn" {
		t.Errorf("expected log_level 'warn', got %q", cfg.LogLevel)
	}
	if cfg.MetricsAddr != ":9091" {
		t.Errorf("expected metrics_addr ':9091', got %q", cfg.MetricsAddr)
	}
	if cfg.OTLPEndpoint != "collector.internal:4317" {
		t.Errorf("expected otlp_endpoint 'collector.internal:4317', got %q", cfg.OTLPEndpoint)
	}
}

func TestPriority_EnvOverridesFile(t *testing.T) {
	fileContent := `
log_level: "warn"
metrics_addr: ":9091"
otlp_endpoint: "collector.internal:4317"
`
	cfgFile := createTempConfigFile(t, fileContent)

	t.Setenv("INTERNET_MONITOR_LOG_LEVEL", "debug")
	t.Setenv("INTERNET_MONITOR_METRICS_ADDR", ":9092")
	t.Setenv("INTERNET_MONITOR_OTLP_ENDPOINT", "env-collector:4317")

	v := config.NewViper()
	cfg, err := config.Load(v, cfgFile, nil)
	if err != nil {
		t.Fatalf("failed to load config with env vars: %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("expected log_level 'debug' from env, got %q", cfg.LogLevel)
	}
	if cfg.MetricsAddr != ":9092" {
		t.Errorf("expected metrics_addr ':9092' from env, got %q", cfg.MetricsAddr)
	}
	if cfg.OTLPEndpoint != "env-collector:4317" {
		t.Errorf("expected otlp_endpoint 'env-collector:4317' from env, got %q", cfg.OTLPEndpoint)
	}
}

func TestPriority_FlagOverridesEnvAndFile(t *testing.T) {
	fileContent := `
log_level: "warn"
metrics_addr: ":9091"
otlp_endpoint: "collector.internal:4317"
`
	cfgFile := createTempConfigFile(t, fileContent)

	t.Setenv("INTERNET_MONITOR_LOG_LEVEL", "debug")
	t.Setenv("INTERNET_MONITOR_METRICS_ADDR", ":9092")
	t.Setenv("INTERNET_MONITOR_OTLP_ENDPOINT", "env-collector:4317")

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.String("log-level", "error", "")
	flags.String("metrics-addr", ":9093", "")
	flags.String("otlp-endpoint", "flag-collector:4317", "")

	// Parse flags to simulate CLI argument passing
	if err := flags.Parse([]string{
		"--log-level", "error",
		"--metrics-addr", ":9093",
		"--otlp-endpoint", "flag-collector:4317",
	}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	v := config.NewViper()
	cfg, err := config.Load(v, cfgFile, flags)
	if err != nil {
		t.Fatalf("failed to load config with flags: %v", err)
	}

	if cfg.LogLevel != "error" {
		t.Errorf("expected log_level 'error' from flag, got %q", cfg.LogLevel)
	}
	if cfg.MetricsAddr != ":9093" {
		t.Errorf("expected metrics_addr ':9093' from flag, got %q", cfg.MetricsAddr)
	}
	if cfg.OTLPEndpoint != "flag-collector:4317" {
		t.Errorf("expected otlp_endpoint 'flag-collector:4317' from flag, got %q", cfg.OTLPEndpoint)
	}
}

func TestPriority_TargetsUntouchedByEnv(t *testing.T) {
	fileContent := `
targets:
  - name: "yaml-target"
    host: "1.1.1.1"
`
	cfgFile := createTempConfigFile(t, fileContent)

	// Attempt to inject targets through env var
	t.Setenv("INTERNET_MONITOR_TARGETS", `[{"name":"injected","host":"evil.com"}]`)

	v := config.NewViper()
	cfg, err := config.Load(v, cfgFile, nil)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(cfg.Targets) != 1 || cfg.Targets[0].Name != "yaml-target" {
		t.Errorf("targets were improperly modified by env var: %+v", cfg.Targets)
	}
}
