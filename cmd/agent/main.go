package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/user/keen-cli/internal/cli"
)

const version = "0.1.0"

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

func main() {
	opts := &slog.HandlerOptions{Level: parseLogLevel()}
	logger := slog.New(slog.NewTextHandler(os.Stderr, opts))
	slog.SetDefault(logger)

	rootCmd := cli.NewRootCommand(version)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
