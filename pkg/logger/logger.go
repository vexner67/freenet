package logger

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/reqid"
)

func New(cfg Config) (*slog.Logger, error) {
	level, err := parseLevel(cfg.LogLevel)
	if err != nil {
		return nil, errs.Wrap("parseLevel", err)
	}

	opts := &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	}

	var handler slog.Handler

	switch cfg.LogFormat {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		return nil, fmt.Errorf("unsupported log format: %q", cfg.LogFormat)
	}

	// Every line logged with a request's context says which request it was.
	return slog.New(reqid.LogHandler(handler)), nil
}

func parseLevel(level string) (slog.Level, error) {
	switch level {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level: %q", level)
	}
}
