package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// ParseLevel converts a string log level to zerolog.Level.
func ParseLevel(level string) (zerolog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return zerolog.DebugLevel, nil
	case "info", "":
		return zerolog.InfoLevel, nil
	case "warn", "warning":
		return zerolog.WarnLevel, nil
	case "error":
		return zerolog.ErrorLevel, nil
	default:
		return zerolog.InfoLevel, fmt.Errorf("unknown log level: %q", level)
	}
}

// New creates a configured zerolog.Logger with the specified level and output writer.
func New(level string, w io.Writer) zerolog.Logger {
	lvl, err := ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	zerolog.TimeFieldFormat = time.RFC3339

	return zerolog.New(w).
		Level(lvl).
		With().
		Timestamp().
		Logger()
}

// Init configures the global zerolog.Logger with the given level and writes to os.Stdout.
func Init(level string) zerolog.Logger {
	logger := New(level, os.Stdout)
	log.Logger = logger
	zerolog.SetGlobalLevel(logger.GetLevel())
	return logger
}
