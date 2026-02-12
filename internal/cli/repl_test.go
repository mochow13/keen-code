package cli

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/user/keen-cli/internal/config"
)

func TestAbbreviateHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory")
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "path in home",
			input:    home + "/projects/myapp",
			expected: "~/projects/myapp",
		},
		{
			name:     "home directory",
			input:    home,
			expected: "~",
		},
		{
			name:     "path outside home",
			input:    "/etc/passwd",
			expected: "/etc/passwd",
		},
		{
			name:     "empty path",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := abbreviateHome(tt.input)
			if got != tt.expected {
				t.Errorf("abbreviateHome(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRunREPL_ExitCommand(t *testing.T) {
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	r, w, _ := os.Pipe()
	os.Stdin = r

	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	done := make(chan error)
	go func() {
		cfg := &config.ResolvedConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet",
			APIKey:   "test-key",
		}
		done <- RunREPL("0.1.0", "/tmp", cfg)
	}()

	w.WriteString("/exit\n")
	w.Close()

	err := <-done
	if err != nil {
		t.Errorf("RunREPL returned error: %v", err)
	}

	outW.Close()
	output, _ := io.ReadAll(outR)
	outputStr := string(output)

	if !strings.Contains(outputStr, "Keen v0.1.0") {
		t.Error("Output should contain version")
	}
	if !strings.Contains(outputStr, "anthropic") {
		t.Error("Output should contain provider")
	}
	if !strings.Contains(outputStr, "Goodbye!") {
		t.Error("Output should contain 'Goodbye!'")
	}
}

func TestRunREPL_EchoInput(t *testing.T) {
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	r, w, _ := os.Pipe()
	os.Stdin = r

	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	done := make(chan error)
	go func() {
		cfg := &config.ResolvedConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet",
			APIKey:   "test-key",
		}
		done <- RunREPL("0.1.0", "/tmp", cfg)
	}()

	w.WriteString("hello world\n/exit\n")
	w.Close()

	err := <-done
	if err != nil {
		t.Errorf("RunREPL returned error: %v", err)
	}

	outW.Close()
	output, _ := io.ReadAll(outR)
	outputStr := string(output)

	if !strings.Contains(outputStr, "hello world") {
		t.Error("Output should echo user input")
	}
}

func TestRunREPL_EmptyInput(t *testing.T) {
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	r, w, _ := os.Pipe()
	os.Stdin = r

	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	done := make(chan error)
	go func() {
		cfg := &config.ResolvedConfig{
			Provider: "anthropic",
			Model:    "claude-3-sonnet",
			APIKey:   "test-key",
		}
		done <- RunREPL("0.1.0", "/tmp", cfg)
	}()

	w.WriteString("\n/exit\n")
	w.Close()

	err := <-done
	if err != nil {
		t.Errorf("RunREPL returned error: %v", err)
	}

	outW.Close()
	io.ReadAll(outR)
}
