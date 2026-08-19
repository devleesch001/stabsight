package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devleesch001/stabsight/internal/config"
)

func TestLoadFromFile_ExampleFile(t *testing.T) {
	// Look for config.example.yaml in project root
	cfgPath := filepath.Join("..", "..", "config.example.yaml")
	cfg, err := config.LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to load config.example.yaml: %v", err)
	}

	if cfg.LogLevel != "info" {
		t.Errorf("expected log_level 'info', got %q", cfg.LogLevel)
	}
	if cfg.MetricsAddr == "" {
		t.Error("expected non-empty metrics_addr")
	}
	if cfg.OTLPEndpoint == "" {
		t.Error("expected non-empty otlp_endpoint")
	}
	if len(cfg.Targets) != 5 {
		t.Fatalf("expected 5 targets, got %d", len(cfg.Targets))
	}

	// Verify google-dns target
	gTarget := cfg.Targets[0]
	if gTarget.Name != "google-dns" || gTarget.Host != "8.8.8.8" {
		t.Errorf("unexpected first target: %+v", gTarget)
	}
	if gTarget.Probes.ICMP == nil || gTarget.Probes.ICMP.Interval != time.Second {
		t.Errorf("expected ICMP interval 1s, got %+v", gTarget.Probes.ICMP)
	}
	if gTarget.Probes.DNS == nil || gTarget.Probes.DNS.Server != "8.8.8.8:53" {
		t.Errorf("expected DNS server 8.8.8.8:53, got %+v", gTarget.Probes.DNS)
	}
}

func TestLoadFromFile_NotFound(t *testing.T) {
	_, err := config.LoadFromFile("non_existent_file.yaml")
	if err == nil {
		t.Fatal("expected error loading non-existent file, got nil")
	}
}

func TestConfigValidation_LogLevel(t *testing.T) {
	cfg := config.NewDefaultConfig()
	cfg.LogLevel = "invalid_level"

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for invalid log level, got nil")
	}

	validLevels := []string{"debug", "info", "warn", "error", "DEBUG", "INFO"}
	for _, lvl := range validLevels {
		cfg.LogLevel = lvl
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected level %q to be valid, got: %v", lvl, err)
		}
	}
}

func TestConfigValidation_RequiredFields(t *testing.T) {
	cfg := config.NewDefaultConfig()
	cfg.MetricsAddr = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty metrics_addr, got nil")
	}

	cfg = config.NewDefaultConfig()
	cfg.OTLPEndpoint = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty otlp_endpoint, got nil")
	}
}

func TestConfigValidation_TargetFields(t *testing.T) {
	cfg := config.NewDefaultConfig()
	cfg.Targets = []config.Target{
		{Name: "", Host: "8.8.8.8"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for target with empty name")
	}

	cfg.Targets = []config.Target{
		{Name: "dns", Host: ""},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for target with empty host")
	}

	cfg.Targets = []config.Target{
		{Name: "dns", Host: "8.8.8.8", IPVersion: "invalid"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for invalid IPVersion")
	}
}

func TestConfigValidation_ProbeIntervals(t *testing.T) {
	tests := []struct {
		name   string
		target config.Target
	}{
		{
			name: "icmp negative interval",
			target: config.Target{
				Name: "t1", Host: "1.1.1.1",
				Probes: config.ProbesConfig{
					ICMP: &config.ICMPProbeConfig{Interval: -1 * time.Second, Timeout: time.Second},
				},
			},
		},
		{
			name: "dns zero timeout",
			target: config.Target{
				Name: "t2", Host: "1.1.1.1",
				Probes: config.ProbesConfig{
					DNS: &config.DNSProbeConfig{Interval: time.Second, Timeout: 0},
				},
			},
		},
		{
			name: "http zero interval",
			target: config.Target{
				Name: "t3", Host: "example.com",
				Probes: config.ProbesConfig{
					HTTP: &config.HTTPProbeConfig{Interval: 0, Timeout: time.Second},
				},
			},
		},
		{
			name: "tcp invalid port",
			target: config.Target{
				Name: "t4", Host: "1.1.1.1",
				Probes: config.ProbesConfig{
					TCP: &config.TCPProbeConfig{Interval: time.Second, Timeout: time.Second, Port: 70000},
				},
			},
		},
		{
			name: "mtr negative timeout",
			target: config.Target{
				Name: "t5", Host: "1.1.1.1",
				Probes: config.ProbesConfig{
					MTR: &config.MTRProbeConfig{Interval: time.Second, Timeout: -time.Second},
				},
			},
		},
		{
			name: "speedtest zero interval",
			target: config.Target{
				Name: "t6", Host: "speedtest",
				Probes: config.ProbesConfig{
					Speedtest: &config.SpeedtestProbeConfig{Interval: 0, Timeout: time.Second},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewDefaultConfig()
			cfg.Targets = []config.Target{tt.target}
			if err := cfg.Validate(); err == nil {
				t.Errorf("expected validation error for %s, got nil", tt.name)
			}
		})
	}
}

func TestLoadFromFile_InvalidYAML(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(tempFile, []byte("invalid: yaml: [content"), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_, err := config.LoadFromFile(tempFile)
	if err == nil {
		t.Fatal("expected unmarshal error on invalid YAML, got nil")
	}
}
