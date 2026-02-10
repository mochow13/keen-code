package logger

import (
	"fmt"
	"log/slog"
	"os"
)

type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

type Logger struct {
	*slog.Logger
}

type Config struct {
	Level  Level
	Format string
	File   string
}

func levelToSlog(l Level) (slog.Level, error) {
	switch l {
	case LevelDebug:
		return slog.LevelDebug, nil
	case LevelInfo:
		return slog.LevelInfo, nil
	case LevelWarn:
		return slog.LevelWarn, nil
	case LevelError:
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level: %s", l)
	}
}

func New(cfg Config) (*Logger, error) {
	level, err := levelToSlog(cfg.Level)
	if err != nil {
		return nil, err
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}

	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return &Logger{
		Logger: slog.New(handler),
	}, nil
}

func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		Logger: l.Logger.With(args...),
	}
}
