package logging

import (
	"fmt"
	"io"
	stdlog "log"
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

// New creates a configured zerolog.Logger with the specified level, format ("json" or "common"), and output writer.
func New(level, format string, w io.Writer) zerolog.Logger {
	lvl, err := ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	zerolog.TimeFieldFormat = time.RFC3339

	var out = w
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "common", "console":
		out = zerolog.ConsoleWriter{
			Out:        w,
			TimeFormat: time.RFC3339,
		}
	case "json", "":
		out = w
	}

	logger := zerolog.New(out).
		Level(lvl).
		With().
		Timestamp().
		Logger()

	if err != nil {
		logger.Warn().Err(err).Msgf("Invalid log level, defaulting to %s", lvl.String())
	}

	return logger
}

// Init configures the global zerolog.Logger with the given level, format, and writes to os.Stdout.
// It also redirects standard library logging (net/http etc.) to Zerolog.
func Init(level, format string) zerolog.Logger {
	logger := New(level, format, os.Stdout)
	log.Logger = logger
	zerolog.SetGlobalLevel(logger.GetLevel())

	stdlog.SetFlags(0)
	stdlog.SetOutput(logger)

	return logger
}

// Get returns the global zerolog.Logger.
func Get() zerolog.Logger {
	return log.Logger
}
