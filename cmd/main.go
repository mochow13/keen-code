package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	clicmd "github.com/mochow13/keen-code/internal/cli/cmd"
	"github.com/mochow13/keen-code/internal/logging"
	"github.com/mochow13/keen-code/internal/telemetry"
	"github.com/spf13/cobra"
)

const version = "0.50.1"

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
		var exitCoder interface{ ExitCode() int }
		code := 1
		if errors.As(err, &exitCoder) {
			code = exitCoder.ExitCode()
		}
		if code == 2 {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		return code
	}
	return 0
}
