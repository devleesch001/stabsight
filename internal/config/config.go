package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Default operational values.
const (
	DefaultLogLevel     = "info"
	DefaultMetricsAddr  = ":9090"
	DefaultOTLPEndpoint = "localhost:4317"
)

// Config represents the complete application configuration.
type Config struct {
	// Operational settings (can be overridden by CLI flags and env vars)
	LogLevel     string `mapstructure:"log_level" yaml:"log_level"`
	MetricsAddr  string `mapstructure:"metrics_addr" yaml:"metrics_addr"`
	OTLPEndpoint string `mapstructure:"otlp_endpoint" yaml:"otlp_endpoint"`

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
		LogLevel:     DefaultLogLevel,
		MetricsAddr:  DefaultMetricsAddr,
		OTLPEndpoint: DefaultOTLPEndpoint,
		Targets:      []Target{},
	}
}

// LoadFromFile loads and parses a YAML configuration file into Config.
func LoadFromFile(filePath string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(filePath)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file or directory") {
			return nil, fmt.Errorf("config file not found: %w", err)
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := NewDefaultConfig()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
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
