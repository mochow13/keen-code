package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/config"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
)

func TestNewRootCommand(t *testing.T) {
	cmd := NewRootCommand("0.1.0")

	if cmd.Use != "keen" {
		t.Errorf("command Use = %q, want 'keen'", cmd.Use)
	}

	if cmd.Version != "0.1.0" {
		t.Errorf("command Version = %q, want '0.1.0'", cmd.Version)
	}

	if cmd.Short == "" {
		t.Error("command Short should not be empty")
	}

	if cmd.Long == "" {
		t.Error("command Long should not be empty")
	}
}

func TestNewRootCommand_HasRunCommand(t *testing.T) {
	cmd := NewRootCommand("0.1.0")

	runCmd, _, err := cmd.Find([]string{"run"})
	if err != nil {
		t.Fatalf("Find(run) error = %v", err)
	}
	if runCmd == nil || runCmd.Name() != "run" {
		t.Fatalf("expected run command, got %#v", runCmd)
	}
}

func TestNewRootCommand_HasResumeFlag(t *testing.T) {
	cmd := NewRootCommand("0.1.0")

	if cmd.Flags().Lookup("resume") == nil {
		t.Fatal("expected root command to have --resume flag")
	}
}

func TestNewRootCommand_RunCommandHasModelProviderFlags(t *testing.T) {
	cmd := NewRootCommand("0.1.0")

	runCmd, _, err := cmd.Find([]string{"run"})
	if err != nil {
		t.Fatalf("Find(run) error = %v", err)
	}

	for _, name := range []string{"model", "provider"} {
		if runCmd.Flags().Lookup(name) == nil {
			t.Fatalf("expected run command to have --%s flag", name)
		}
	}
}

func TestStartMCPRuntimeStartsWithE2EConfigAndCloses(t *testing.T) {
	previous := newMCPManager
	defer func() { newMCPManager = previous }()

	fake := &fakeMCPRuntime{}
	var gotOptions int
	newMCPManager = func(opts ...keenmcp.Option) (keenmcp.Runtime, error) {
		gotOptions = len(opts)
		return fake, nil
	}

	manager, closeMCP, err := startMCPRuntime(context.Background())
	closeMCP()
	if err != nil {
		t.Fatalf("startMCPRuntime() error = %v", err)
	}

	if manager != fake {
		t.Fatalf("manager = %#v, want fake", manager)
	}
	if gotOptions != 0 {
		t.Fatalf("options length = %d, want 0", gotOptions)
	}
	if fake.starts != 1 {
		t.Fatalf("starts = %d, want 1", fake.starts)
	}
	if fake.closes != 1 {
		t.Fatalf("closes = %d, want 1", fake.closes)
	}
}

func TestStartMCPRuntimeIsBestEffortOnCreateError(t *testing.T) {
	previous := newMCPManager
	defer func() { newMCPManager = previous }()

	newMCPManager = func(opts ...keenmcp.Option) (keenmcp.Runtime, error) {
		return nil, errors.New("boom")
	}

	manager, closeMCP, err := startMCPRuntime(context.Background())
	closeMCP()
	if manager != nil {
		t.Fatalf("manager = %#v, want nil", manager)
	}
	if err == nil {
		t.Fatal("startMCPRuntime() error = nil, want error")
	}
}

func TestStartMCPRuntimeClosesAfterStartError(t *testing.T) {
	previous := newMCPManager
	defer func() { newMCPManager = previous }()

	fake := &fakeMCPRuntime{startErr: errors.New("boom")}
	newMCPManager = func(opts ...keenmcp.Option) (keenmcp.Runtime, error) {
		return fake, nil
	}

	manager, closeMCP, err := startMCPRuntime(context.Background())
	closeMCP()
	if manager != nil {
		t.Fatalf("manager = %#v, want nil", manager)
	}
	if err == nil {
		t.Fatal("startMCPRuntime() error = nil, want error")
	}

	if fake.starts != 1 {
		t.Fatalf("starts = %d, want 1", fake.starts)
	}
	if fake.closes != 1 {
		t.Fatalf("closes = %d, want 1", fake.closes)
	}
}

func TestApplyRunOverrides(t *testing.T) {
	globalCfg := &config.GlobalConfig{
		Providers: map[string]config.ProviderConfig{
			config.ProviderAnthropic: {
				APIKey:  "anthropic-key",
				Models:  []string{"claude-3"},
				BaseURL: "https://anthropic.example",
			},
			config.ProviderOpenCodeGo: {
				APIKey:  "opencode-key",
				Models:  []string{"kimi-k2.6"},
				BaseURL: "https://opencode.example",
			},
		},
	}
	resolvedCfg := &config.ResolvedConfig{
		Provider: config.ProviderAnthropic,
		APIKey:   "anthropic-key",
		Model:    "claude-3",
		BaseURL:  "https://anthropic.example",
		AuthMode: config.AuthModeAPIKey,
	}

	err := applyRunOverrides(globalCfg, resolvedCfg, config.ProviderOpenCodeGo, "qwen3.6-plus")
	if err != nil {
		t.Fatalf("applyRunOverrides() error = %v", err)
	}

	if resolvedCfg.Provider != config.ProviderOpenCodeGo {
		t.Fatalf("Provider = %q, want %q", resolvedCfg.Provider, config.ProviderOpenCodeGo)
	}
	if resolvedCfg.APIKey != "opencode-key" {
		t.Fatalf("APIKey = %q, want opencode-key", resolvedCfg.APIKey)
	}
	if resolvedCfg.BaseURL != "https://opencode.example" {
		t.Fatalf("BaseURL = %q, want https://opencode.example", resolvedCfg.BaseURL)
	}
	if resolvedCfg.Model != "qwen3.6-plus" {
		t.Fatalf("Model = %q, want qwen3.6-plus", resolvedCfg.Model)
	}
}

type fakeMCPRuntime struct {
	startErr error
	closeErr error
	starts   int
	closes   int
}

func (f *fakeMCPRuntime) Start(context.Context) error {
	f.starts++
	return f.startErr
}

func (f *fakeMCPRuntime) Close() error {
	f.closes++
	return f.closeErr
}

func (f *fakeMCPRuntime) Servers() []keenmcp.ServerStatus {
	return nil
}

func (f *fakeMCPRuntime) Status(server string) keenmcp.ServerStatus {
	return keenmcp.ServerStatus{Name: server}
}

func (f *fakeMCPRuntime) WaitInitialScan(context.Context) error {
	return nil
}

func (f *fakeMCPRuntime) ListTools(context.Context, string) ([]keenmcp.Tool, error) {
	return nil, nil
}

func (f *fakeMCPRuntime) Refresh(context.Context, string, ...keenmcp.RefreshOption) error {
	return nil
}

func (f *fakeMCPRuntime) CallTool(context.Context, string, string, map[string]any) (*keenmcp.ToolResult, error) {
	return &keenmcp.ToolResult{}, nil
}

func TestApplyRunOverrides_ProviderUsesFirstConfiguredModel(t *testing.T) {
	globalCfg := &config.GlobalConfig{
		Providers: map[string]config.ProviderConfig{
			config.ProviderOpenCodeGo: {
				APIKey: "opencode-key",
				Models: []string{"kimi-k2.6"},
			},
		},
	}
	resolvedCfg := &config.ResolvedConfig{
		Provider: config.ProviderAnthropic,
		Model:    "claude-3",
	}

	err := applyRunOverrides(globalCfg, resolvedCfg, config.ProviderOpenCodeGo, "")
	if err != nil {
		t.Fatalf("applyRunOverrides() error = %v", err)
	}

	if resolvedCfg.Model != "kimi-k2.6" {
		t.Fatalf("Model = %q, want kimi-k2.6", resolvedCfg.Model)
	}
}

func TestBuildRunPrompt(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		stdin string
		want  string
	}{
		{name: "args only", args: []string{"hello", "there"}, want: "hello there"},
		{name: "stdin only", stdin: " from stdin\n", want: "from stdin"},
		{name: "args and stdin", args: []string{"hello"}, stdin: "from stdin\n", want: "hello\nfrom stdin"},
		{name: "empty", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildRunPrompt(tt.args, tt.stdin)
			if got != tt.want {
				t.Fatalf("buildRunPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunExitError(t *testing.T) {
	err := &runExitError{err: errors.New("failed"), exitCode: 2}
	if err.Error() != "failed" || err.ExitCode() != 2 {
		t.Fatalf("unexpected runExitError %q, %d", err.Error(), err.ExitCode())
	}
}

func TestApplyRunOverridesRejectsUnknownProvider(t *testing.T) {
	err := applyRunOverrides(&config.GlobalConfig{}, &config.ResolvedConfig{}, "missing", "")
	if err == nil {
		t.Fatal("applyRunOverrides accepted an unknown provider")
	}
}

func TestApplyRunOverridesChangesOnlyModel(t *testing.T) {
	resolved := &config.ResolvedConfig{Provider: config.ProviderAnthropic}
	if err := applyRunOverrides(&config.GlobalConfig{}, resolved, "", "model"); err != nil {
		t.Fatalf("applyRunOverrides() error = %v", err)
	}
	if resolved.Provider != config.ProviderAnthropic || resolved.Model != "model" {
		t.Fatalf("unexpected resolved config %#v", resolved)
	}
}

func TestLoadRootRuntimeWithoutConfigNeedsSetup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	registry, loader, globalCfg, resolvedCfg, needsSetup, err := loadRootRuntime()
	if err != nil {
		t.Fatalf("loadRootRuntime() error = %v", err)
	}
	if registry == nil || loader == nil || globalCfg == nil || resolvedCfg == nil {
		t.Fatal("loadRootRuntime() returned a nil runtime component")
	}
	if !needsSetup {
		t.Fatal("needsSetup = false, want true")
	}
	if resolvedCfg.Provider != "" {
		t.Fatalf("provider = %q, want empty", resolvedCfg.Provider)
	}
}

func TestLoadRootRuntimeResolvesConfiguredProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeRootConfig(t, home, `{
		"active_provider": "anthropic",
		"thinking_effort": "high",
		"providers": {
			"anthropic": {
				"models": ["claude-sonnet-4-5"],
				"api_key": "test-key",
				"base_url": "https://example.invalid",
				"headers": {"X-Test": "value"}
			}
		}
	}`)

	_, _, globalCfg, resolvedCfg, needsSetup, err := loadRootRuntime()
	if err != nil {
		t.Fatalf("loadRootRuntime() error = %v", err)
	}
	if needsSetup {
		t.Fatal("needsSetup = true, want false")
	}
	if globalCfg.ActiveProvider != config.ProviderAnthropic {
		t.Fatalf("active provider = %q", globalCfg.ActiveProvider)
	}
	if resolvedCfg.Provider != config.ProviderAnthropic || resolvedCfg.Model != "claude-sonnet-4-5" {
		t.Fatalf("resolved provider/model = %q/%q", resolvedCfg.Provider, resolvedCfg.Model)
	}
	if resolvedCfg.APIKey != "test-key" || resolvedCfg.BaseURL != "https://example.invalid" {
		t.Fatalf("resolved credentials = %#v", resolvedCfg)
	}
	if resolvedCfg.ThinkingEffort != "high" || resolvedCfg.Headers["X-Test"] != "value" {
		t.Fatalf("resolved options = %#v", resolvedCfg)
	}
}

func TestLoadRootRuntimeConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "invalid json", content: `{`, want: "failed to load config"},
		{
			name: "unknown provider",
			content: `{
				"active_provider": "unknown",
				"providers": {"unknown": {"api_key": "key"}}
			}`,
			want: `configured provider "unknown" not found`,
		},
		{
			name:    "missing provider config",
			content: `{"active_provider": "anthropic", "providers": {}}`,
			want:    `failed to get provider config for "anthropic"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			writeRootConfig(t, home, tt.content)

			_, _, _, _, _, err := loadRootRuntime()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("loadRootRuntime() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRunCommandRejectsMissingConfiguration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmd := newRunCommand()
	cmd.SetArgs([]string{"hello"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "LLM client not initialized") {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestRunCommandRejectsUnknownProviderOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmd := newRunCommand()
	cmd.SetArgs([]string{"--provider", "unknown", "hello"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `provider "unknown" is not configured`) {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestShouldReadStdin(t *testing.T) {
	regularPath := filepath.Join(t.TempDir(), "stdin.txt")
	regular, err := os.Create(regularPath)
	if err != nil {
		t.Fatalf("create regular file: %v", err)
	}
	defer regular.Close()
	if !shouldReadStdin(regular) {
		t.Fatal("shouldReadStdin(regular file) = false, want true")
	}

	closed, err := os.Create(filepath.Join(t.TempDir(), "closed.txt"))
	if err != nil {
		t.Fatalf("create closed file: %v", err)
	}
	if err := closed.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	if shouldReadStdin(closed) {
		t.Fatal("shouldReadStdin(closed file) = true, want false")
	}
}

func writeRootConfig(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".keen")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create config directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "configs.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
