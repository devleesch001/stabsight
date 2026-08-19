package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/devleesch001/stabsight/internal/cli"
)

func TestRootCmdFlagsDefaults(t *testing.T) {
	cmd := cli.NewRootCmd(cli.BuildInfo{Version: "1.0.0", Commit: "abc", Date: "2026-08-19"})

	configFlag := cmd.PersistentFlags().Lookup("config")
	if configFlag == nil || configFlag.DefValue != "config.yaml" {
		t.Errorf("expected default config flag to be 'config.yaml', got %v", configFlag)
	}

	logLevelFlag := cmd.PersistentFlags().Lookup("log-level")
	if logLevelFlag == nil || logLevelFlag.DefValue != "info" {
		t.Errorf("expected default log-level flag to be 'info', got %v", logLevelFlag)
	}

	otlpFlag := cmd.PersistentFlags().Lookup("otlp-endpoint")
	if otlpFlag == nil || otlpFlag.DefValue != "localhost:4317" {
		t.Errorf("expected default otlp-endpoint flag to be 'localhost:4317', got %v", otlpFlag)
	}

	metricsFlag := cmd.PersistentFlags().Lookup("metrics-addr")
	if metricsFlag == nil || metricsFlag.DefValue != ":9090" {
		t.Errorf("expected default metrics-addr flag to be ':9090', got %v", metricsFlag)
	}
}

func TestRootCmdSubcommands(t *testing.T) {
	cmd := cli.NewRootCmd(cli.BuildInfo{Version: "1.0.0", Commit: "abc", Date: "2026-08-19"})

	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}

	if !subcommands["run"] {
		t.Errorf("expected 'run' subcommand to be present")
	}
	if !subcommands["version"] {
		t.Errorf("expected 'version' subcommand to be present")
	}
}

func TestVersionCmdExecution(t *testing.T) {
	cmd := cli.NewRootCmd(cli.BuildInfo{
		Version: "v1.2.3",
		Commit:  "deadbeef",
		Date:    "2026-08-19T12:00:00Z",
	})

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing version cmd: %v", err)
	}
}

func TestRunCmdExecution(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test-config.yaml")
	if err := os.WriteFile(cfgPath, []byte("targets:\n  - name: t1\n    host: 1.1.1.1\n"), 0600); err != nil {
		t.Fatalf("failed to create temp config: %v", err)
	}

	cmd := cli.NewRootCmd(cli.BuildInfo{Version: "dev"})

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"run", "--config", cfgPath, "--log-level", "debug", "--metrics-addr", ":9100", "--otlp-endpoint", "collector:4317"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error executing run cmd: %v", err)
	}
}
