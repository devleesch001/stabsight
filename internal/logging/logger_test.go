package logging_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/devleesch001/stabsight/internal/logging"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected zerolog.Level
		hasErr   bool
	}{
		{"debug", zerolog.DebugLevel, false},
		{"DEBUG", zerolog.DebugLevel, false},
		{"info", zerolog.InfoLevel, false},
		{"", zerolog.InfoLevel, false},
		{"warn", zerolog.WarnLevel, false},
		{"warning", zerolog.WarnLevel, false},
		{"error", zerolog.ErrorLevel, false},
		{"invalid", zerolog.InfoLevel, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lvl, err := logging.ParseLevel(tt.input)
			if (err != nil) != tt.hasErr {
				t.Fatalf("expected error: %v, got: %v", tt.hasErr, err)
			}
			if lvl != tt.expected {
				t.Errorf("expected level %v, got %v", tt.expected, lvl)
			}
		})
	}
}

func TestLoggerFiltering_JSON(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New("info", "json", &buf)

	logger.Debug().Msg("debug message should not appear")
	if buf.Len() > 0 {
		t.Errorf("expected no log output for debug at info level, got: %s", buf.String())
	}

	logger.Info().Str("key", "val").Msg("info message")
	if buf.Len() == 0 {
		t.Fatal("expected log output for info message, got empty")
	}

	var data map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &data); err != nil {
		t.Fatalf("expected valid JSON output, got error: %v (raw: %s)", err, buf.String())
	}

	if data["level"] != "info" {
		t.Errorf("expected level 'info', got %v", data["level"])
	}
	if data["message"] != "info message" {
		t.Errorf("expected message 'info message', got %v", data["message"])
	}
	if data["key"] != "val" {
		t.Errorf("expected key 'val', got %v", data["key"])
	}
	if data["time"] == nil {
		t.Errorf("expected timestamp field 'time' to be present")
	}
}

func TestLoggerFormat_Common(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New("info", "common", &buf)

	logger.Info().Str("target", "google-dns").Msg("ping success")
	output := buf.String()

	if !strings.Contains(output, "INF") && !strings.Contains(output, "info") {
		t.Errorf("expected console level indicator in output, got: %s", output)
	}
	if !strings.Contains(output, "ping success") {
		t.Errorf("expected message 'ping success' in output, got: %s", output)
	}
	if !strings.Contains(output, "target=google-dns") && !strings.Contains(output, "google-dns") {
		t.Errorf("expected target field in output, got: %s", output)
	}
}

func TestInit(t *testing.T) {
	logger := logging.Init("warn", "json")
	if logger.GetLevel() != zerolog.WarnLevel {
		t.Errorf("expected level WarnLevel, got %v", logger.GetLevel())
	}
}
