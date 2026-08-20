package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Default operational values.
const (
	DefaultLogLevel      = "info"
	DefaultLogFormat     = "json"
	DefaultHistogramMode = "explicit"
	DefaultMetricsAddr   = ":9090"
	DefaultOTLPEndpoint  = "localhost:4317"
	EnvPrefix            = "INTERNET_MONITOR"
)

// Config represents the complete application configuration.
type Config struct {
	// Operational settings (can be overridden by CLI flags and env vars)
	LogLevel         string    `mapstructure:"log_level" yaml:"log_level"`
	LogFormat        string    `mapstructure:"log_format" yaml:"log_format"`
	HistogramMode    string    `mapstructure:"histogram_mode" yaml:"histogram_mode"`       // "explicit" (default) or "exponential"
	HistogramBuckets []float64 `mapstructure:"histogram_buckets" yaml:"histogram_buckets"` // Custom bucket boundaries in seconds
	MetricsAddr      string    `mapstructure:"metrics_addr" yaml:"metrics_addr"`
	OTLPEndpoint     string    `mapstructure:"otlp_endpoint" yaml:"otlp_endpoint"`

	// Network monitoring targets (strictly configured via YAML only)
	Targets []Target `mapstructure:"targets" yaml:"targets"`
}

// Target defines a host to monitor with its attached probes.
type Target struct {
	Name      string       `mapstructure:"name" yaml:"name"`
	Host      string       `mapstructure:"host" yaml:"host"`
	IPVersion string       `mapstructure:"ip_version" yaml:"ip_version"` // "ipv4", "ipv6", or "" (auto)
	Probes    ProbesConfig `mapstructure:"probes" yaml:"probes"`
}

// ProbesConfig aggregates probe configurations for a target.
type ProbesConfig struct {
	ICMP      *ICMPProbeConfig      `mapstructure:"icmp" yaml:"icmp,omitempty"`
	DNS       *DNSProbeConfig       `mapstructure:"dns" yaml:"dns,omitempty"`
	HTTP      *HTTPProbeConfig      `mapstructure:"http" yaml:"http,omitempty"`
	TCP       *TCPProbeConfig       `mapstructure:"tcp" yaml:"tcp,omitempty"`
	MTR       *MTRProbeConfig       `mapstructure:"mtr" yaml:"mtr,omitempty"`
	Speedtest *SpeedtestProbeConfig `mapstructure:"speedtest" yaml:"speedtest,omitempty"`
}

// ICMPProbeConfig configures ICMP ping probe.
type ICMPProbeConfig struct {
	Interval time.Duration `mapstructure:"interval" yaml:"interval"`
	Timeout  time.Duration `mapstructure:"timeout" yaml:"timeout"`
	Count    int           `mapstructure:"count" yaml:"count"`
}

// DNSProbeConfig configures DNS resolution probe.
type DNSProbeConfig struct {
	Interval   time.Duration `mapstructure:"interval" yaml:"interval"`
	Timeout    time.Duration `mapstructure:"timeout" yaml:"timeout"`
	Server     string        `mapstructure:"server" yaml:"server"`
	RecordType string        `mapstructure:"record_type" yaml:"record_type"`
}

// HTTPProbeConfig configures HTTP availability and latency probe.
type HTTPProbeConfig struct {
	Interval      time.Duration `mapstructure:"interval" yaml:"interval"`
	Timeout       time.Duration `mapstructure:"timeout" yaml:"timeout"`
	URL           string        `mapstructure:"url" yaml:"url"`
	Method        string        `mapstructure:"method" yaml:"method"`
	ExpectedCode  int           `mapstructure:"expected_code" yaml:"expected_code"`
	TLSSkipVerify bool          `mapstructure:"tls_skip_verify" yaml:"tls_skip_verify"`
}

// TCPProbeConfig configures TCP connect probe.
type TCPProbeConfig struct {
	Interval time.Duration `mapstructure:"interval" yaml:"interval"`
	Timeout  time.Duration `mapstructure:"timeout" yaml:"timeout"`
	Port     int           `mapstructure:"port" yaml:"port"`
}

// MTRProbeConfig configures traceroute/MTR probe.
type MTRProbeConfig struct {
	Interval time.Duration `mapstructure:"interval" yaml:"interval"`
	Timeout  time.Duration `mapstructure:"timeout" yaml:"timeout"`
	MaxHops  int           `mapstructure:"max_hops" yaml:"max_hops"`
}

// SpeedtestProbeConfig configures bandwidth probe.
type SpeedtestProbeConfig struct {
	Interval time.Duration `mapstructure:"interval" yaml:"interval"`
	Timeout  time.Duration `mapstructure:"timeout" yaml:"timeout"`
	ServerID string        `mapstructure:"server_id" yaml:"server_id"`
}

// NewDefaultConfig returns a Config with default operational values.
func NewDefaultConfig() *Config {
	return &Config{
		LogLevel:         DefaultLogLevel,
		LogFormat:        DefaultLogFormat,
		HistogramMode:    DefaultHistogramMode,
		HistogramBuckets: nil,
		MetricsAddr:      DefaultMetricsAddr,
		OTLPEndpoint:     DefaultOTLPEndpoint,
		Targets:          []Target{},
	}
}

// NewViper creates and configures a Viper instance with defaults, env prefix and key replacers.
func NewViper() *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	v.SetDefault("log_level", DefaultLogLevel)
	v.SetDefault("log_format", DefaultLogFormat)
	v.SetDefault("histogram_mode", DefaultHistogramMode)
	v.SetDefault("metrics_addr", DefaultMetricsAddr)
	v.SetDefault("otlp_endpoint", DefaultOTLPEndpoint)

	return v
}

// BindFlags binds Cobra/pflag flags to Viper operational keys only.
func BindFlags(v *viper.Viper, flags *pflag.FlagSet) error {
	if flags == nil {
		return nil
	}

	flagBindings := map[string]string{
		"log_level":      "log-level",
		"log_format":     "log-format",
		"histogram_mode": "histogram-mode",
		"metrics_addr":   "metrics-addr",
		"otlp_endpoint":  "otlp-endpoint",
	}

	for viperKey, flagName := range flagBindings {
		if flag := flags.Lookup(flagName); flag != nil {
			if err := v.BindPFlag(viperKey, flag); err != nil {
				return fmt.Errorf("failed to bind flag %q to %q: %w", flagName, viperKey, err)
			}
		}
	}

	return nil
}

// Load loads, merges, and validates configuration from Viper instance and optional config file.
func Load(v *viper.Viper, configFile string, flags *pflag.FlagSet) (*Config, error) {
	if v == nil {
		v = NewViper()
	}

	if flags != nil {
		if err := BindFlags(v, flags); err != nil {
			return nil, err
		}
	}

	cfg := NewDefaultConfig()

	if configFile != "" {
		v.SetConfigFile(configFile)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file or directory") {
				return nil, fmt.Errorf("config file not found: %w", err)
			}
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		// Strictly decode targets and custom histogram buckets from YAML file content
		fileData, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file for targets: %w", err)
		}

		type yamlDoc struct {
			HistogramBuckets []float64 `yaml:"histogram_buckets"`
			Targets          []Target  `yaml:"targets"`
		}
		var doc yamlDoc
		if err := yaml.Unmarshal(fileData, &doc); err != nil {
			return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
		}
		if len(doc.HistogramBuckets) > 0 {
			cfg.HistogramBuckets = doc.HistogramBuckets
		}
		if doc.Targets != nil {
			cfg.Targets = doc.Targets
		}
	}

	// Operational settings are resolved by Viper (flag > env > file > default)
	cfg.LogLevel = v.GetString("log_level")
	cfg.LogFormat = v.GetString("log_format")
	cfg.HistogramMode = v.GetString("histogram_mode")
	cfg.MetricsAddr = v.GetString("metrics_addr")
	cfg.OTLPEndpoint = v.GetString("otlp_endpoint")

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// LoadFromFile loads and parses a YAML configuration file into Config.
func LoadFromFile(filePath string) (*Config, error) {
	v := NewViper()
	return Load(v, filePath, nil)
}

// Validate checks the configuration for semantic and type correctness.
func (c *Config) Validate() error {
	// Validate log level
	lvl := strings.ToLower(c.LogLevel)
	switch lvl {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log_level %q: must be one of debug, info, warn, error", c.LogLevel)
	}

	// Validate log format
	format := strings.ToLower(strings.TrimSpace(c.LogFormat))
	switch format {
	case "json", "common", "console", "":
	default:
		return fmt.Errorf("invalid log_format %q: must be one of json, common", c.LogFormat)
	}

	// Validate histogram mode
	mode := strings.ToLower(strings.TrimSpace(c.HistogramMode))
	switch mode {
	case "explicit", "exponential", "":
	default:
		return fmt.Errorf("invalid histogram_mode %q: must be one of explicit, exponential", c.HistogramMode)
	}

	// Validate custom histogram buckets
	for i, b := range c.HistogramBuckets {
		if b <= 0 {
			return fmt.Errorf("histogram_bucket at index %d must be > 0 (got %f)", i, b)
		}
		if i > 0 && b <= c.HistogramBuckets[i-1] {
			return fmt.Errorf("histogram_buckets must be strictly monotonically increasing (got %f <= %f)", b, c.HistogramBuckets[i-1])
		}
	}

	if strings.TrimSpace(c.MetricsAddr) == "" {
		return errors.New("metrics_addr cannot be empty")
	}

	if strings.TrimSpace(c.OTLPEndpoint) == "" {
		return errors.New("otlp_endpoint cannot be empty")
	}

	// Validate targets
	for i, target := range c.Targets {
		if strings.TrimSpace(target.Name) == "" {
			return fmt.Errorf("target at index %d has empty name", i)
		}
		if strings.TrimSpace(target.Host) == "" {
			return fmt.Errorf("target %q has empty host", target.Name)
		}
		if target.IPVersion != "" && target.IPVersion != "ipv4" && target.IPVersion != "ipv6" {
			return fmt.Errorf("target %q has invalid ip_version %q: must be 'ipv4' or 'ipv6'", target.Name, target.IPVersion)
		}

		if err := validateProbes(target.Name, &target.Probes); err != nil {
			return err
		}
	}

	return nil
}

func validateProbes(targetName string, probes *ProbesConfig) error {
	if probes.ICMP != nil {
		if probes.ICMP.Interval <= 0 {
			return fmt.Errorf("target %q icmp probe: interval must be > 0", targetName)
		}
		if probes.ICMP.Timeout <= 0 {
			return fmt.Errorf("target %q icmp probe: timeout must be > 0", targetName)
		}
	}

	if probes.DNS != nil {
		if probes.DNS.Interval <= 0 {
			return fmt.Errorf("target %q dns probe: interval must be > 0", targetName)
		}
		if probes.DNS.Timeout <= 0 {
			return fmt.Errorf("target %q dns probe: timeout must be > 0", targetName)
		}
	}

	if probes.HTTP != nil {
		if probes.HTTP.Interval <= 0 {
			return fmt.Errorf("target %q http probe: interval must be > 0", targetName)
		}
		if probes.HTTP.Timeout <= 0 {
			return fmt.Errorf("target %q http probe: timeout must be > 0", targetName)
		}
	}

	if probes.TCP != nil {
		if probes.TCP.Interval <= 0 {
			return fmt.Errorf("target %q tcp probe: interval must be > 0", targetName)
		}
		if probes.TCP.Timeout <= 0 {
			return fmt.Errorf("target %q tcp probe: timeout must be > 0", targetName)
		}
		if probes.TCP.Port <= 0 || probes.TCP.Port > 65535 {
			return fmt.Errorf("target %q tcp probe: invalid port %d", targetName, probes.TCP.Port)
		}
	}

	if probes.MTR != nil {
		if probes.MTR.Interval <= 0 {
			return fmt.Errorf("target %q mtr probe: interval must be > 0", targetName)
		}
		if probes.MTR.Timeout <= 0 {
			return fmt.Errorf("target %q mtr probe: timeout must be > 0", targetName)
		}
	}

	if probes.Speedtest != nil {
		if probes.Speedtest.Interval <= 0 {
			return fmt.Errorf("target %q speedtest probe: interval must be > 0", targetName)
		}
		if probes.Speedtest.Timeout <= 0 {
			return fmt.Errorf("target %q speedtest probe: timeout must be > 0", targetName)
		}
	}

	return nil
}
