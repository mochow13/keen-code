package main

import (
	"fmt"
	"log/slog"
	"os"

	clicmd "github.com/user/keen-code/internal/cli/cmd"
	"github.com/user/keen-code/internal/logging"
)

const version = "0.1.3"

func main() {
	cleanup, logFile, err := logging.Init()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing logging: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	slog.Debug("Logging initialized", "file", logFile)

	rootCmd := clicmd.NewRootCommand(version)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
