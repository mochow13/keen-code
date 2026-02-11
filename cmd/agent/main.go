package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/user/keen-cli/internal/cli"
)

const version = "0.1.0"

func parseLogLevel() slog.Level {
	switch strings.ToLower(os.Getenv("KEEN_LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
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
