package cli

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/user/keen-cli/configs/providers"
	"github.com/user/keen-cli/internal/config"
)

func testRegistry() *providers.Registry {
	return &providers.Registry{
		Providers: []providers.Provider{
			{
				ID:   "anthropic",
				Name: "Anthropic",
				Models: []providers.Model{
					{ID: "claude-3-sonnet", Name: "Claude 3 Sonnet"},
				},
			},
		},
	}
}

func testGlobalConfig() *config.GlobalConfig {
	cfg := config.DefaultGlobalConfig()
	cfg.ActiveProvider = "anthropic"
	cfg.ActiveModel = "claude-3-sonnet"
	cfg.SetProviderConfig("anthropic", config.ProviderConfig{
		APIKey: "test-key",
		Models: []string{"claude-3-sonnet"},
	})
	return cfg
}

func testResolvedConfig() *config.ResolvedConfig {
	return &config.ResolvedConfig{
		Provider: "anthropic",
		Model:    "claude-3-sonnet",
		APIKey:   "test-key",
	}
}

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
		done <- RunREPL("0.1.0", "/tmp", testResolvedConfig(), config.NewLoader(), testGlobalConfig(), testRegistry())
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
		done <- RunREPL("0.1.0", "/tmp", testResolvedConfig(), config.NewLoader(), testGlobalConfig(), testRegistry())
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
		done <- RunREPL("0.1.0", "/tmp", testResolvedConfig(), config.NewLoader(), testGlobalConfig(), testRegistry())
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

func TestHandleInput_ExitReturnsFalse(t *testing.T) {
	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	_, outW, _ := os.Pipe()
	os.Stdout = outW

	state := &replState{
		cfg:       testResolvedConfig(),
		globalCfg: testGlobalConfig(),
		registry:  testRegistry(),
		loader:    config.NewLoader(),
	}

	if state.handleInput("/exit") {
		t.Error("handleInput('/exit') should return false")
	}

	outW.Close()
}

func TestHandleInput_EmptyInputReturnsTrue(t *testing.T) {
	state := &replState{
		cfg:       testResolvedConfig(),
		globalCfg: testGlobalConfig(),
		registry:  testRegistry(),
		loader:    config.NewLoader(),
	}

	if !state.handleInput("") {
		t.Error("handleInput('') should return true")
	}
}

func TestHandleInput_RegularInputReturnsTrue(t *testing.T) {
	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	state := &replState{
		cfg:       testResolvedConfig(),
		globalCfg: testGlobalConfig(),
		registry:  testRegistry(),
		loader:    config.NewLoader(),
	}

	if !state.handleInput("hello") {
		t.Error("handleInput('hello') should return true")
	}

	outW.Close()
	output, _ := io.ReadAll(outR)
	if !strings.Contains(string(output), "hello") {
		t.Error("Output should contain echoed input")
	}
}

func TestHandleInput_HelpCommand(t *testing.T) {
	oldStdout := os.Stdout
	defer func() { os.Stdout = oldStdout }()

	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	state := &replState{
		cfg:       testResolvedConfig(),
		globalCfg: testGlobalConfig(),
		registry:  testRegistry(),
		loader:    config.NewLoader(),
	}

	if !state.handleInput("/help") {
		t.Error("handleInput('/help') should return true")
	}

	outW.Close()
	output, _ := io.ReadAll(outR)
	outputStr := string(output)

	if !strings.Contains(outputStr, "/help") {
		t.Error("Help output should contain /help command")
	}
	if !strings.Contains(outputStr, "/model") {
		t.Error("Help output should contain /model command")
	}
	if !strings.Contains(outputStr, "/exit") {
		t.Error("Help output should contain /exit command")
	}
}
