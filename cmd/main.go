package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	clicmd "github.com/user/keen-code/internal/cli/cmd"
	"github.com/user/keen-code/internal/logging"
	"github.com/user/keen-code/internal/telemetry"
)

const version = "0.42.0"

var (
	telemetryMeasurementID string
	telemetryAPISecret     string
)

func main() {
	os.Exit(run())
}

func run() int {
	cleanup, logFile, err := logging.Init()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing logging: %v\n", err)
		return 1
	}
	defer cleanup()

	slog.Debug("Logging initialized", "file", logFile)

	rootCmd := clicmd.NewRootCommand(version)
	var reporter *telemetry.Reporter
	rootCmd.PersistentPreRun = func(command *cobra.Command, _ []string) {
		mode := "interactive"
		if command.Name() == "run" {
			mode = "headless"
		}
		reporter = telemetry.New(telemetry.Config{
			MeasurementID: telemetryMeasurementID,
			APISecret:     telemetryAPISecret,
			Version:       version,
			Mode:          mode,
		})
	}
	defer func() {
		reporter.Close()
	}()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}
