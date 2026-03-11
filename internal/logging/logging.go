package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	logLevelEnvVar = "KEEN_LOG_LEVEL"

	logLevelDebug   = "debug"
	logLevelInfo    = "info"
	logLevelWarn    = "warn"
	logLevelWarning = "warning"
	logLevelError   = "error"
)

func parseLogLevel() slog.Level {
	switch strings.ToLower(os.Getenv(logLevelEnvVar)) {
	case logLevelDebug:
		return slog.LevelDebug
	case logLevelInfo:
		return slog.LevelInfo
	case logLevelWarn, logLevelWarning:
		return slog.LevelWarn
	case logLevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func getLogDirectory() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(homeDir, ".keen-code", "logs"), nil
}

func createLogFile() (*os.File, string, error) {
	logDir, err := getLogDirectory()
	if err != nil {
		return nil, "", err
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, "", fmt.Errorf("creating log directory: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02-15-04-05")
	logFile := filepath.Join(logDir, fmt.Sprintf("keen-code-%s.log", timestamp))

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, "", fmt.Errorf("opening log file: %w", err)
	}

	return file, logFile, nil
}

type prettyHandler struct {
	w     io.Writer
	level slog.Level
}

func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	timestamp := r.Time.Format("2006-01-02 15:04:05.000")

	level := r.Level.String()
	switch r.Level {
	case slog.LevelDebug:
		level = "DEBUG"
	case slog.LevelInfo:
		level = "INFO "
	case slog.LevelWarn:
		level = "WARN "
	case slog.LevelError:
		level = "ERROR"
	}

	msg := r.Message

	var attrs []string
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, formatAttr(a))
		return true
	})

	if len(attrs) > 0 {
		fmt.Fprintf(h.w, "[%s] %s %s\n", timestamp, level, msg)
		for _, attr := range attrs {
			fmt.Fprintf(h.w, "  %s\n", attr)
		}
	} else {
		fmt.Fprintf(h.w, "[%s] %s %s\n", timestamp, level, msg)
	}

	return nil
}

func formatAttr(a slog.Attr) string {
	key := a.Key
	value := a.Value.Any()

	formatted := formatValue(value, "    ")
	return fmt.Sprintf("%s: %s", key, formatted)
}

func formatValue(value any, indent string) string {
	if value == nil {
		return "null"
	}

	switch v := value.(type) {
	case string:
		var jsonData any
		if err := json.Unmarshal([]byte(v), &jsonData); err == nil {
			jsonBytes, _ := json.MarshalIndent(jsonData, indent, "  ")
			return string(jsonBytes)
		}
		return v
	case []byte:
		return formatValue(string(v), indent)
	case map[string]any:
		cleaned := cleanupMap(v)
		jsonBytes, err := json.MarshalIndent(cleaned, indent, "  ")
		if err == nil {
			return string(jsonBytes)
		}
	case []any:
		// Clean up the slice to handle nested byte arrays
		cleaned := cleanupSlice(v)
		jsonBytes, err := json.MarshalIndent(cleaned, indent, "  ")
		if err == nil {
			return string(jsonBytes)
		}
	}

	return fmt.Sprintf("%v", value)
}

func cleanupMap(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case []byte:
			result[k] = string(val)
		case map[string]any:
			result[k] = cleanupMap(val)
		case []any:
			result[k] = cleanupSlice(val)
		default:
			result[k] = val
		}
	}
	return result
}

func cleanupSlice(s []any) []any {
	result := make([]any, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case []byte:
			result[i] = string(val)
		case map[string]any:
			result[i] = cleanupMap(val)
		case []any:
			result[i] = cleanupSlice(val)
		default:
			result[i] = val
		}
	}
	return result
}

func (h *prettyHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *prettyHandler) WithGroup(_ string) slog.Handler {
	return h
}

func Init() (func(), string, error) {
	file, logFile, err := createLogFile()
	if err != nil {
		return nil, "", err
	}

	handler := &prettyHandler{
		w:     file,
		level: parseLogLevel(),
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	cleanup := func() {
		file.Close()
	}

	return cleanup, logFile, nil
}
