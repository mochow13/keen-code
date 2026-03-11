package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     int
	}{
		{"debug", "debug", -4},
		{"info", "info", 0},
		{"warn", "warn", 4},
		{"warning", "warning", 4},
		{"error", "error", 8},
		{"empty", "", 0},
		{"default", "unknown", 0},
		{"uppercase", "DEBUG", -4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(logLevelEnvVar, tt.envValue)
				defer os.Unsetenv(logLevelEnvVar)
			}

			level := parseLogLevel()
			if int(level) != tt.want {
				t.Errorf("parseLogLevel() = %v, want %v", level, tt.want)
			}
		})
	}
}

func TestGetLogDirectory(t *testing.T) {
	dir, err := getLogDirectory()
	if err != nil {
		t.Fatalf("getLogDirectory() error = %v", err)
	}

	if !strings.Contains(dir, ".keen-code") {
		t.Errorf("getLogDirectory() = %v, want to contain '.keen-code'", dir)
	}

	if !strings.Contains(dir, "logs") {
		t.Errorf("getLogDirectory() = %v, want to contain 'logs'", dir)
	}
}

func TestCreateLogFile(t *testing.T) {
	file, logFile, err := createLogFile()
	if err != nil {
		t.Fatalf("createLogFile() error = %v", err)
	}
	defer os.RemoveAll(filepath.Dir(logFile))
	defer file.Close()

	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Errorf("createLogFile() did not create file: %v", logFile)
	}

	if !strings.HasPrefix(filepath.Base(logFile), "keen-code-") {
		t.Errorf("createLogFile() filename = %v, want prefix 'keen-code-'", logFile)
	}

	if !strings.HasSuffix(logFile, ".log") {
		t.Errorf("createLogFile() filename = %v, want suffix '.log'", logFile)
	}
}

func TestInit(t *testing.T) {
	cleanup, logFile, err := Init()
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer os.RemoveAll(filepath.Dir(logFile))
	defer cleanup()

	if logFile == "" {
		t.Error("Init() logFile is empty")
	}

	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("Init() created file not accessible: %v", err)
	}

	if info.IsDir() {
		t.Error("Init() created path is a directory, not a file")
	}
}

func TestTimestampFormat(t *testing.T) {
	timestamp := time.Now().Format("2006-01-02-15-04-05")
	parts := strings.Split(timestamp, "-")
	if len(parts) != 6 {
		t.Errorf("timestamp format incorrect: got %d parts, want 6", len(parts))
	}
}
