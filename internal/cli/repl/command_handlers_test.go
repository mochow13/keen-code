package repl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	replappstate "github.com/user/keen-code/internal/cli/repl/appstate"
	replcommands "github.com/user/keen-code/internal/cli/repl/commands"
	replwidgets "github.com/user/keen-code/internal/cli/repl/widgets"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/llm"
	keenmcp "github.com/user/keen-code/internal/mcp"
	"github.com/user/keen-code/internal/mcpskills"
	"github.com/user/keen-code/internal/providers"
	"github.com/user/keen-code/internal/skills"
)

func TestHandleEnterKey_EmptyInput(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("")

	newM, cmd := m.handleEnterKey()

	if cmd != nil {
		t.Error("expected nil cmd for empty input")
	}
	if len(newM.output.GetLines()) != 0 {
		t.Error("expected no output for empty input")
	}
}

func TestHandleEnterKey_ActiveStream(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("some input")
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	newM, cmd := m.handleEnterKey()

	if cmd != nil {
		t.Error("expected nil cmd when queueable input is sent during active stream")
	}
	if newM.suggestion.IsModelMode() {
		t.Fatal("expected bare /model to open provider selection")
	}
	if newM.textarea.Value() != "" {
		t.Error("expected textarea to be reset after queueing input during active stream")
	}
	if len(newM.queuedInputs) != 1 || newM.queuedInputs[0] != "some input" {
		t.Errorf("expected input to be queued, got %v", newM.queuedInputs)
	}
}

func TestHandleEnterKey_ActiveStream_AdjustsViewportHeight(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("some input")
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")
	m.adjustTextareaHeight()

	before := m.viewport.Height()
	newM, _ := m.handleEnterKey()
	after := newM.viewport.Height()

	if after != before-newM.queuedHeight() {
		t.Errorf("expected viewport height to decrease by queued height %d, got before=%d after=%d", newM.queuedHeight(), before, after)
	}
}

func TestHandleBangCommand_ReturnsBeforeCommandCompletes(t *testing.T) {
	m := newTestModel()

	start := time.Now()
	newM, cmd := m.handleBangCommand("!sleep 2")
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("handleBangCommand blocked for %s", elapsed)
	}
	if cmd == nil {
		t.Fatal("expected command waiter")
	}
	if !newM.bang.active {
		t.Fatal("expected bang command to be active")
	}
	if !strings.Contains(newM.output.Join(), "$ sleep 2") {
		t.Fatal("expected command to be rendered immediately")
	}

	newM.cancelBangCommand()
	for i := 0; i < 20 && cmd != nil; i++ {
		newM, cmd = processCmd(newM, cmd)
	}
	if newM.bang.active {
		t.Fatal("expected bang command to stop after cancellation")
	}
}

func TestHandleEnterKey_ExitCommand(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue(replcommands.Exit)

	newM, cmd := m.handleEnterKey()

	if !newM.quitting {
		t.Error("expected quitting to be true")
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}
}

func TestHandleEnterKey_HelpCommand(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue(replcommands.Help)

	newM, _ := m.handleEnterKey()

	if !strings.Contains(newM.output.Join(), "Available Commands") {
		t.Error("expected help text in output")
	}
	if newM.textarea.Value() != "" {
		t.Error("expected textarea to be reset after help command")
	}
}

func TestHandleEnterKey_CleanupCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.textarea.SetValue(replcommands.Cleanup)

	newM, cmd := m.handleEnterKey()
	if !newM.loading.showSpinner || newM.loading.text != "Cleaning up..." {
		t.Fatalf("expected cleanup spinner, got %#v", newM.loading)
	}
	if cmd == nil {
		t.Fatal("expected cleanup command")
	}

	newM, _ = processCmd(newM, cmd)
	if newM.loading.showSpinner {
		t.Fatal("expected cleanup spinner to stop")
	}
	if !strings.Contains(newM.output.Join(), "Cleanup completed") {
		t.Fatalf("expected completion message, got %q", newM.output.Join())
	}
}

func TestHandleEnterKey_ModelCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.ctx.registry = &providers.Registry{Providers: []providers.Provider{}}
	m.ctx.globalCfg = &config.GlobalConfig{}
	m.ctx.loader = config.NewLoader()
	m.textarea.SetValue(replcommands.Model)

	newM, _ := m.handleEnterKey()

	if newM.modelSelection == nil {
		t.Error("expected model selection to be started")
	}
	if newM.textarea.Value() != "" {
		t.Error("expected textarea to be reset")
	}
}

func TestHandleSuggestionEnter_ExpandsModelCommandToModelPairs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.ctx.registry = &providers.Registry{Providers: []providers.Provider{{
		ID:     config.ProviderOpenAI,
		Models: []providers.Model{{ID: "gpt-5.4"}},
	}}}
	m.textarea.SetValue("/mo")
	m.suggestion.Refresh("/mo")

	handled, newM, cmd := m.handleSuggestionKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled || cmd != nil {
		t.Fatalf("handled = %t, cmd = %v", handled, cmd)
	}
	if newM.modelSelection != nil {
		t.Fatal("did not expect provider selection")
	}
	if newM.textarea.Value() != replcommands.Model {
		t.Fatalf("textarea = %q, want %q", newM.textarea.Value(), replcommands.Model)
	}
	if !newM.suggestion.IsModelMode() || newM.suggestion.Current() == nil {
		t.Fatal("expected provider/model suggestions")
	}
}

func TestHandleSuggestionEnter_SelectsModelPairAfterCursorMove(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue(replcommands.Model)
	m.suggestion.RefreshModels(replcommands.Model, []string{"anthropic/claude-sonnet-4-6", "openai/gpt-5.4"})
	m.suggestion.MoveDown()
	m.suggestion.MoveDown()

	handled, newM, cmd := m.handleSuggestionKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled || cmd != nil {
		t.Fatalf("handled = %t, cmd = %v", handled, cmd)
	}
	if newM.modelSelection != nil {
		t.Fatal("did not expect provider selection")
	}
	if newM.textarea.Value() != "/model openai/gpt-5.4" {
		t.Fatalf("textarea = %q", newM.textarea.Value())
	}
}

func TestHandleSuggestionEnter_PicksSupportedProviders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.ctx.registry = &providers.Registry{}
	m.ctx.globalCfg = &config.GlobalConfig{}
	m.ctx.loader = config.NewLoader()
	m.textarea.SetValue(replcommands.Model)
	m.suggestion.RefreshModels(replcommands.Model, []string{"openai/gpt-5.4"})
	if current := m.suggestion.Current(); current == nil || current.Name != "Pick from supported providers" {
		t.Fatalf("unexpected first suggestion: %#v", current)
	}

	handled, newM, cmd := m.handleSuggestionKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled || cmd != nil {
		t.Fatalf("handled = %t, cmd = %v", handled, cmd)
	}
	if newM.modelSelection == nil {
		t.Fatal("expected bare /model to open model selection")
	}
	if newM.textarea.Value() != "" {
		t.Fatalf("textarea = %q, want empty", newM.textarea.Value())
	}
}

func TestHandleEnterKey_ModelPairCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.ctx.registry = &providers.Registry{Providers: []providers.Provider{{
		ID: config.ProviderOpenAI,
		Models: []providers.Model{{
			ID:              "gpt-5.4",
			ThinkingEfforts: []string{"low", "medium", "high"},
		}},
	}}}
	m.ctx.globalCfg = config.DefaultGlobalConfig()
	m.ctx.loader = config.NewLoader()
	m.textarea.SetValue("/model openai/gpt-5.4")

	newM, _ := m.handleEnterKey()
	if newM.modelSelection == nil {
		t.Fatal("expected model selection")
	}
	if newM.modelSelection.SelectedProvider != config.ProviderOpenAI || newM.modelSelection.SelectedModel != "gpt-5.4" {
		t.Fatalf("selected pair = %s/%s", newM.modelSelection.SelectedProvider, newM.modelSelection.SelectedModel)
	}
	if newM.modelSelection.Step != replwidgets.StepThinking {
		t.Fatalf("step = %v, want StepThinking", newM.modelSelection.Step)
	}
}

func TestHandleEnterKey_UnknownModelPair(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.ctx.registry = &providers.Registry{Providers: []providers.Provider{{ID: config.ProviderOpenAI}}}
	m.ctx.globalCfg = config.DefaultGlobalConfig()
	m.ctx.loader = config.NewLoader()
	m.textarea.SetValue("/model openai/gpt-unknown")

	newM, _ := m.handleEnterKey()
	if newM.modelSelection != nil {
		t.Fatal("expected model selection to close")
	}
	if !strings.Contains(newM.output.Join(), "unknown model: openai/gpt-unknown") {
		t.Fatalf("expected unknown model error, got %q", newM.output.Join())
	}
}

func TestHandleEnterKey_SessionsCommand_EmptyState(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	m := newTestModel()
	m.sessions = newReplSessionState(filepath.Join(tmp, "project"))
	m.textarea.SetValue(replcommands.Sessions)

	newM, cmd := m.handleEnterKey()

	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	if newM.sessionPicker != nil {
		t.Fatal("expected no session picker for empty state")
	}
	if !strings.Contains(newM.output.Join(), "No saved sessions for this directory.") {
		t.Fatalf("expected empty state message, got %q", newM.output.Join())
	}
}

func TestHandleEnterKey_CompactCommandStartsCompaction(t *testing.T) {
	m := newTestModel()
	m.ctx.cfg = &config.ResolvedConfig{APIKey: "key", Model: "model"}
	m.appState = replappstate.New(&mockLLMClient{}, "")
	m.appState.AddMessage(llm.RoleUser, "hello")
	m.textarea.SetValue("/compact Keep business logic details")

	newM, cmd := m.handleEnterKey()

	if !newM.compaction.active {
		t.Fatal("expected compaction mode to start")
	}
	if !newM.loading.showSpinner {
		t.Fatal("expected spinner to be visible during compaction")
	}
	if newM.loading.text != "Compacting..." {
		t.Fatalf("expected compaction loading text, got %q", newM.loading.text)
	}
	if newM.textarea.Value() != "" {
		t.Fatal("expected textarea to be reset")
	}
	if newM.compaction.cancel == nil {
		t.Fatal("expected compaction cancel func to be set")
	}
	if !newM.stream.handler.IsActive() {
		t.Fatal("expected compaction to use the stream handler")
	}
	if cmd == nil {
		t.Fatal("expected async compaction command")
	}
}

func TestHandleEnterKey_ClientNotReady(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("hello there")

	newM, _ := m.handleEnterKey()

	found := false
	for _, line := range newM.output.GetLines() {
		if strings.Contains(line, "LLM client not initialized") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error about LLM client not initialized")
	}
	if newM.textarea.Value() != "" {
		t.Error("expected textarea to be reset")
	}
}

func TestGetHelpText(t *testing.T) {
	text := getHelpText(80)

	if !strings.Contains(text, "/compact") {
		t.Error("expected /compact in help text")
	}
	if !strings.Contains(text, "/help") {
		t.Error("expected /help in help text")
	}
	if !strings.Contains(text, "/model") {
		t.Error("expected /model in help text")
	}
	if !strings.Contains(text, "/mode") {
		t.Error("expected /mode in help text")
	}
	if !strings.Contains(text, "/exit") {
		t.Error("expected /exit in help text")
	}
	if !strings.Contains(text, "/resume") {
		t.Error("expected /resume in help text")
	}
	if !strings.Contains(text, "/sessions") {
		t.Error("expected /sessions in help text")
	}
	if !strings.Contains(text, "/skills") {
		t.Error("expected /skills in help text")
	}
}

func TestGetHelpTextWrapsCommandDescriptions(t *testing.T) {
	text := getHelpText(48)
	stripped := ansi.Strip(text)

	if !strings.Contains(stripped, "  Available Commands") {
		t.Fatalf("expected padded title, got %q", stripped)
	}
	if strings.Contains(stripped, "\n/show-thinking") {
		t.Fatalf("expected /show-thinking to stay in command column, got %q", stripped)
	}

	for _, line := range strings.Split(stripped, "\n") {
		if lipgloss.Width(line) > 46 {
			t.Fatalf("help line exceeds padded width (%d > %d): %q", lipgloss.Width(line), 46, line)
		}
	}
}

func TestDispatchCommand_UnknownCommandFallsThrough(t *testing.T) {
	m := newTestModel()

	_, _, handled := m.dispatchCommand("hello world")

	if handled {
		t.Error("expected unknown input to not be handled by dispatchCommand")
	}
}

func TestHandleMCPCommandStatus(t *testing.T) {
	m := newTestModel()
	m.ctx.mcp = &fakeMCPRuntime{
		statuses: []keenmcp.ServerStatus{
			{Name: "deepwiki", State: keenmcp.StateConnected, AuthType: keenmcp.AuthNone, ToolCount: 3},
			{Name: "posthog", State: keenmcp.StateAuthRequired, AuthType: keenmcp.AuthOAuth, LastError: "mcp authentication required"},
			{Name: "context7", State: keenmcp.StateDisconnected, AuthType: keenmcp.AuthAPIKey, LastError: "connection refused"},
		},
	}

	result, cmd := m.handleMCPCommand("/mcp status")

	if cmd != nil {
		t.Fatal("expected nil command")
	}
	out := ansi.Strip(result.output.Join())
	if !strings.Contains(out, "deepwiki") || !strings.Contains(out, "posthog") {
		t.Fatalf("output = %q, want MCP statuses", out)
	}
	for _, expected := range []string{"Server", "Status", "Auth", "Detail", "────", "✓ connected", "✗ auth_required"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q in MCP status output, got %q", expected, out)
		}
	}
	if !strings.Contains(result.output.Join(), "38;2;239;83;80m✗ disconnected") {
		t.Fatalf("expected error-colored disconnected status, got %q", result.output.Join())
	}
}

func TestHandleMCPCommandConnectUnknown(t *testing.T) {
	m := newTestModel()
	m.ctx.mcp = &fakeMCPRuntime{statuses: []keenmcp.ServerStatus{{Name: "deepwiki", State: keenmcp.StateConnected}}}

	result, cmd := m.handleMCPCommand("/mcp connect missing")

	if cmd != nil {
		t.Fatal("expected nil command")
	}
	if !strings.Contains(ansi.Strip(result.output.Join()), "unknown MCP server or tool") {
		t.Fatalf("output = %q, want unknown error", result.output.Join())
	}
}

func TestConnectMCPCmdPassesOAuthReauthOption(t *testing.T) {
	m := newTestModel()
	fake := &fakeMCPRuntime{statuses: []keenmcp.ServerStatus{{Name: "posthog", State: keenmcp.StateAuthRequired}}}
	m.ctx.mcp = fake

	msg := m.connectMCPCmd("posthog")().(mcpConnectDoneMsg)

	if msg.Server != "posthog" || fake.connected != "posthog" {
		t.Fatalf("connected = %q, msg server = %q; want posthog", fake.connected, msg.Server)
	}
	if fake.refreshOptCount != 4 {
		t.Fatalf("Refresh option count = %d, want 4", fake.refreshOptCount)
	}
}

func TestHandleMCPConnectDoneGeneratesAndEnablesSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	m := newTestModel()
	m.appState = replappstate.New(nil, work)
	if err := m.appState.SetSkillStatus("mcp:deepwiki", skills.StatusDisabled); err != nil {
		t.Fatalf("disable deepwiki: %v", err)
	}
	m.ctx.mcp = &fakeMCPRuntime{
		statuses: []keenmcp.ServerStatus{{Name: "deepwiki", Description: "Ask questions about DeepWiki."}},
		tools: map[string][]keenmcp.Tool{
			"deepwiki": {{Name: "ask", Description: "Ask DeepWiki", InputSchema: map[string]any{"type": "object"}}},
		},
	}
	m.loading.showSpinner = true

	m.handleMCPConnectDone(mcpConnectDoneMsg{Server: "deepwiki"})

	if m.loading.showSpinner {
		t.Fatal("expected spinner to stop")
	}
	if !strings.Contains(ansi.Strip(m.output.Join()), "MCP server connected: deepwiki") {
		t.Fatalf("output = %q, want success", m.output.Join())
	}
	if _, ok := skills.Find(m.appState.GetSkills().Skills, "mcp:deepwiki"); !ok {
		t.Fatalf("expected mcp:deepwiki skill to be reloaded")
	}
	if !m.appState.GetSkillsConfig().Enabled("mcp:deepwiki") {
		t.Fatalf("expected mcp:deepwiki skill to be enabled")
	}
	data, err := os.ReadFile(filepath.Join(home, ".keen", "skills", "mcp:deepwiki", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(data), "description: Ask questions about DeepWiki.") {
		t.Fatalf("SKILL.md missing MCP server description: %s", string(data))
	}
}

func TestHandleMCPConnectDoneFailureDisablesSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	m := newTestModel()
	m.appState = replappstate.New(nil, work)
	if err := mcpskills.Generate("deepwiki", "", []keenmcp.Tool{{Name: "ask", Description: "Ask DeepWiki"}}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	m.appState.ReloadSkills()

	m.handleMCPConnectDone(mcpConnectDoneMsg{Server: "deepwiki", Err: errors.New("connection failed")})

	if m.appState.GetSkillsConfig().Enabled("mcp:deepwiki") {
		t.Fatalf("expected mcp:deepwiki skill to be disabled")
	}
	if m.appState.SkillsCatalog() != "" && strings.Contains(m.appState.SkillsCatalog(), "mcp:deepwiki") {
		t.Fatalf("expected mcp:deepwiki to be hidden from catalog")
	}
}

func TestHandleMCPStartupStatusGeneratesConnectedSkillsWithoutChangingStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	m := newTestModel()
	m.appState = replappstate.New(nil, work)
	if err := m.appState.SetSkillStatus("mcp:deepwiki", skills.StatusDisabled); err != nil {
		t.Fatalf("disable deepwiki: %v", err)
	}
	m.ctx.mcp = &fakeMCPRuntime{tools: map[string][]keenmcp.Tool{
		"deepwiki": {{Name: "ask", Description: "Ask DeepWiki", InputSchema: map[string]any{"type": "object"}}},
	}}

	m.handleMCPStartupStatus(mcpStartupStatusMsg{Statuses: []keenmcp.ServerStatus{{Name: "deepwiki", State: keenmcp.StateConnected, Description: "Ask questions about DeepWiki."}}})

	if _, ok := skills.Find(m.appState.GetSkills().Skills, "mcp:deepwiki"); !ok {
		t.Fatalf("expected mcp:deepwiki skill to be reloaded")
	}
	if m.appState.GetSkillsConfig().Enabled("mcp:deepwiki") {
		t.Fatalf("expected mcp:deepwiki skill to remain disabled")
	}
	data, err := os.ReadFile(filepath.Join(home, ".keen", "skills", "mcp:deepwiki", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if !strings.Contains(string(data), "description: Ask questions about DeepWiki.") {
		t.Fatalf("SKILL.md missing MCP server description: %s", string(data))
	}
}

func TestHandleMCPStartupStatusDisablesFailedSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	m := newTestModel()
	m.appState = replappstate.New(nil, work)
	if err := mcpskills.Generate("posthog", "", []keenmcp.Tool{{Name: "query", Description: "Query PostHog"}}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	m.appState.ReloadSkills()

	m.handleMCPStartupStatus(mcpStartupStatusMsg{Statuses: []keenmcp.ServerStatus{{Name: "posthog", State: keenmcp.StateAuthRequired, LastError: "auth required"}}})

	if m.appState.GetSkillsConfig().Enabled("mcp:posthog") {
		t.Fatalf("expected mcp:posthog skill to be disabled")
	}
	if _, err := os.Stat(filepath.Join(home, ".keen", "skills", "mcp:posthog", "SKILL.md")); err != nil {
		t.Fatalf("expected mcp:posthog files to remain: %v", err)
	}
	if m.appState.SkillsCatalog() != "" && strings.Contains(m.appState.SkillsCatalog(), "mcp:posthog") {
		t.Fatalf("expected mcp:posthog to be hidden from catalog")
	}
}

func TestHandleMCPStartupStatusRemovesUnconfiguredSkillStatuses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	m := newTestModel()
	m.appState = replappstate.New(nil, work)
	m.ctx.mcp = &fakeMCPRuntime{tools: map[string][]keenmcp.Tool{
		"context7": {{Name: "resolve", Description: "Resolve docs"}},
	}}
	if err := mcpskills.Generate("deepwiki", "", []keenmcp.Tool{{Name: "ask", Description: "Ask DeepWiki"}}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := m.appState.SetSkillStatus("mcp:deepwiki", skills.StatusEnabled); err != nil {
		t.Fatalf("enable deepwiki: %v", err)
	}
	if err := m.appState.SetSkillStatus("mcp:disabled", skills.StatusDisabled); err != nil {
		t.Fatalf("disable stale skill: %v", err)
	}
	if err := m.appState.SetSkillStatus("debug", skills.StatusEnabled); err != nil {
		t.Fatalf("enable debug: %v", err)
	}
	m.appState.ReloadSkills()

	m.handleMCPStartupStatus(mcpStartupStatusMsg{Statuses: []keenmcp.ServerStatus{{Name: "context7", State: keenmcp.StateConnected}}})

	cfg := m.appState.GetSkillsConfig()
	if _, ok := cfg.IsEnabled["mcp:deepwiki"]; ok {
		t.Fatalf("expected unconfigured mcp:deepwiki status to be removed")
	}
	if _, ok := cfg.IsEnabled["mcp:disabled"]; ok {
		t.Fatalf("expected unconfigured mcp:disabled status to be removed")
	}
	if _, err := os.Stat(filepath.Join(home, ".keen", "skills", "mcp:deepwiki")); !os.IsNotExist(err) {
		t.Fatalf("expected unconfigured mcp:deepwiki skill files to be removed, err = %v", err)
	}
	if !cfg.Enabled("debug") {
		t.Fatalf("expected non-MCP skill to remain enabled")
	}
	if _, ok := skills.Find(m.appState.GetSkills().Skills, "mcp:context7"); !ok {
		t.Fatalf("expected configured mcp:context7 skill to be generated")
	}
}

func TestHandleMCPStartupStatusShowsFailures(t *testing.T) {
	m := newTestModel()
	m.width = 50
	m.viewport.SetWidth(50)
	m.output.SetWidth(50)

	m.handleMCPStartupStatus(mcpStartupStatusMsg{Statuses: []keenmcp.ServerStatus{
		{Name: "deepwiki", State: keenmcp.StateConnected},
		{Name: "posthog", State: keenmcp.StateAuthRequired, LastError: "mcp authentication required because the stored token is missing or expired"},
	}})

	out := ansi.Strip(m.output.Join())
	if strings.Contains(out, "deepwiki") {
		t.Fatalf("output = %q, did not expect connected server notice", out)
	}
	compactOut := strings.Join(strings.Fields(out), " ")
	if !strings.Contains(compactOut, "/mcp connect posthog") {
		t.Fatalf("output = %q, want connect hint", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > 46 {
			t.Fatalf("MCP startup notice exceeds content width (%d > 46): %q", lipgloss.Width(line), line)
		}
	}
}

func TestHandleMCPStartupStatusShowsWaitError(t *testing.T) {
	m := newTestModel()
	m.width = 50
	m.viewport.SetWidth(50)
	m.output.SetWidth(50)

	m.handleMCPStartupStatus(mcpStartupStatusMsg{Err: context.DeadlineExceeded})

	out := ansi.Strip(m.output.Join())
	if !strings.Contains(out, "timed out") {
		t.Fatalf("output = %q, want timeout notice", out)
	}
}

func TestHandleModeCommand(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("/mode plan")

	newM, cmd := m.handleEnterKey()
	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	if newM.currentMode() != llm.ModePlan {
		t.Fatalf("expected plan mode, got %q", newM.currentMode())
	}
	if newM.appState.Mode() != llm.ModePlan {
		t.Fatalf("expected app state plan mode, got %q", newM.appState.Mode())
	}

	newM.textarea.SetValue("/mode build")
	newM, _ = newM.handleEnterKey()
	if newM.currentMode() != llm.ModeBuild {
		t.Fatalf("expected build mode, got %q", newM.currentMode())
	}
}

func TestHandleSkillsCommandList(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(work, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo skill\n---\nBody"), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	m := newTestModel()
	m.ctx.workingDir = work
	m.appState = replappstate.New(nil, work)
	m.textarea.SetValue("/skills list")
	newM, _ := m.handleEnterKey()

	out := newM.output.Join()
	stripped := ansi.Strip(out)
	if !strings.Contains(out, "\x1b[1;38;2;189;189;189m  Available Skills") || strings.Contains(stripped, "Available Skills:") {
		t.Fatalf("expected header-colored skills title without colon, got %q", out)
	}
	if !strings.Contains(out, "38;2;92;107;192mdemo") {
		t.Fatalf("expected primary-colored skill name, got %q", out)
	}
	for _, expected := range []string{"Skill", "Status", "Description", "────", "demo", "✓ enabled", "Demo skill"} {
		if !strings.Contains(stripped, expected) {
			t.Fatalf("expected %q in skills list output, got %q", expected, stripped)
		}
	}
}

func TestHandleSkillsCommandListStylesDisabledStatus(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(work, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo skill\n---\nBody"), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	cfg := skills.Config{IsEnabled: map[string]bool{"demo": false}}
	if err := skills.SaveConfig(cfg); err != nil {
		t.Fatalf("save skills config: %v", err)
	}

	m := newTestModel()
	m.ctx.workingDir = work
	m.appState = replappstate.New(nil, work)
	m.textarea.SetValue("/skills list")
	newM, _ := m.handleEnterKey()

	out := newM.output.Join()
	if !strings.Contains(out, "38;2;255;179;0m✗ disabled") {
		t.Fatalf("expected accent-colored disabled status, got %q", out)
	}
	if !strings.Contains(ansi.Strip(out), "✗ disabled") {
		t.Fatalf("expected disabled status in skills list, got %q", out)
	}
}

func TestHandleSkillsCommandListWrapsLongDescriptions(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(work, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	description := "This skill has a very long description that should wrap within the viewport boundary instead of overflowing horizontally."
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: "+description+"\n---\nBody"), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	m := newTestModel()
	m.ctx.workingDir = work
	m.appState = replappstate.New(nil, work)
	m.width = 50
	m.viewport.SetWidth(50)
	m.output.SetWidth(50)
	m.textarea.SetValue("/skills list")
	newM, _ := m.handleEnterKey()

	for _, line := range strings.Split(ansi.Strip(newM.output.Join()), "\n") {
		if !strings.Contains(line, "demo") && !strings.Contains(line, description[:10]) {
			continue
		}
		if lipgloss.Width(line) > 48 {
			t.Fatalf("skill line exceeds padded width (%d > %d): %q", lipgloss.Width(line), 48, line)
		}
	}
}

func TestHandleSkillsCommandListTruncatesLongDescriptions(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(work, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	words := make([]string, 0, 55)
	for i := range 55 {
		words = append(words, fmt.Sprintf("word%d", i+1))
	}
	description := strings.Join(words, " ")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: "+description+"\n---\nBody"), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	m := newTestModel()
	m.ctx.workingDir = work
	m.appState = replappstate.New(nil, work)
	m.textarea.SetValue("/skills list")
	newM, _ := m.handleEnterKey()

	stripped := ansi.Strip(newM.output.Join())
	if !strings.Contains(stripped, "word50...") {
		t.Fatalf("expected truncated description with ellipsis, got %q", stripped)
	}
	if strings.Contains(stripped, "word51") {
		t.Fatalf("expected description to be truncated before word51, got %q", stripped)
	}
}

func TestHandleSkillsCommandDisable(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(work, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo skill\n---\nBody"), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	m := newTestModel()
	m.ctx.workingDir = work
	m.appState = replappstate.New(nil, work)
	m.textarea.SetValue("/skills disable demo")
	newM, _ := m.handleEnterKey()

	if !strings.Contains(newM.output.Join(), "Skill \"demo\" disabled") {
		t.Fatalf("expected disable confirmation, got %q", newM.output.Join())
	}
	if newM.appState.GetSkillsConfig().Enabled("demo") {
		t.Fatal("expected appstate config to disable skill")
	}
}

func TestHandleSubagentsCommandList(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	agentsDir := filepath.Join(work, ".agents", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte("---\nname: reviewer\ndescription: Review scoped inputs\n---\nBody"), 0644); err != nil {
		t.Fatalf("write reviewer: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "hidden.md"), []byte("---\nname: hidden\ndescription: Hidden agent\nhidden: true\n---\nBody"), 0644); err != nil {
		t.Fatalf("write hidden: %v", err)
	}

	m := newTestModel()
	m.ctx.workingDir = work
	m.appState = replappstate.New(nil, work)
	m.textarea.SetValue("/subagents list")
	newM, _ := m.handleEnterKey()

	out := ansi.Strip(newM.output.Join())
	for _, expected := range []string{"Available Subagents", "Subagent", "Description", "reviewer", "Review scoped inputs"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q in subagents list output, got %q", expected, out)
		}
	}
	if strings.Contains(out, "hidden") {
		t.Fatalf("expected hidden subagent to be omitted, got %q", out)
	}
}

func TestHandleSubagentsCommandRootLists(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	agentsDir := filepath.Join(work, ".agents", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte("---\nname: reviewer\ndescription: Review scoped inputs\n---\nBody"), 0644); err != nil {
		t.Fatalf("write reviewer: %v", err)
	}

	m := newTestModel()
	m.ctx.workingDir = work
	m.appState = replappstate.New(nil, work)
	m.textarea.SetValue("/subagents")
	newM, _ := m.handleEnterKey()

	out := ansi.Strip(newM.output.Join())
	for _, expected := range []string{"Available Subagents", "reviewer", "Review scoped inputs"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected %q in subagents output, got %q", expected, out)
		}
	}
}

func TestHandleSubagentsCommandInvalidArgs(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("/subagents foo")
	newM, _ := m.handleEnterKey()

	out := ansi.Strip(newM.output.Join())
	if !strings.Contains(out, "Usage: /subagents [list]") {
		t.Fatalf("expected usage output, got %q", out)
	}
}

func TestParseSubagentArgs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "root", input: "/subagents", want: nil},
		{name: "list", input: "/subagents list", want: []string{"list"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSubagentArgs(tt.input)
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Fatalf("parseSubagentArgs(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSkillArgs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "root", input: "/skills", want: nil},
		{name: "list", input: "/skills list", want: []string{"list"}},
		{name: "status", input: "/skills status", want: []string{"status"}},
		{name: "reload", input: "/skills reload", want: []string{"reload"}},
		{name: "enable", input: "/skills enable demo", want: []string{"enable", "demo"}},
		{name: "disable", input: "/skills disable demo", want: []string{"disable", "demo"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSkillArgs(tt.input)
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Fatalf("parseSkillArgs(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestHandleSkillsCommandStatus(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(work, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo skill\n---\nBody"), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	m := newTestModel()
	m.ctx.workingDir = work
	m.appState = replappstate.New(nil, work)
	m.textarea.SetValue("/skills status")
	newM, _ := m.handleEnterKey()

	output := ansi.Strip(newM.output.Join())
	for _, expected := range []string{"Available Skills", "Skill", "Status", "Description", "demo", "✓ enabled", "Demo skill"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in skills status output, got %q", expected, output)
		}
	}
}

func TestHandleSkillsCommandRejectsNameFirstStatus(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(work, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo skill\n---\nBody"), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	m := newTestModel()
	m.ctx.workingDir = work
	m.appState = replappstate.New(nil, work)
	m.textarea.SetValue("/skills demo enable")
	newM, _ := m.handleEnterKey()

	output := ansi.Strip(newM.output.Join())
	for _, expected := range []string{"Usage:", "/skills list|status", "/skills reload", "enable|disable", "<name>"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected usage error containing %q, got %q", expected, output)
		}
	}
	if strings.Contains(output, "Skill \"demo\" enabled") {
		t.Fatalf("expected name-first status command to be rejected, got %q", output)
	}
}

func TestHandleSkillsCommandReload(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)

	m := newTestModel()
	m.ctx.workingDir = work
	m.appState = replappstate.New(nil, work)

	writeSkillDir := filepath.Join(work, ".agents", "skills", "demo")
	if err := os.MkdirAll(writeSkillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(writeSkillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo skill\n---\nBody"), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	m.textarea.SetValue("/skills reload")
	newM, _ := m.handleEnterKey()

	if !strings.Contains(newM.output.Join(), "Skills reloaded") {
		t.Fatalf("expected reload confirmation, got %q", newM.output.Join())
	}
	if _, ok := skills.Find(newM.appState.GetSkills().Skills, "demo"); !ok {
		t.Fatal("expected reloaded appstate to include new skill")
	}
}

func TestDispatchCommand_SlashPrefixedNonCommandFallsThrough(t *testing.T) {
	m := newTestModel()

	_, _, handled := m.dispatchCommand("/unknown")

	if handled {
		t.Error("expected unknown slash command to not be handled by dispatchCommand")
	}
}

func TestHandleEnterKey_ClearCommand(t *testing.T) {
	m := newTestModel()
	client := &mockLLMClient{}
	m.appState = replappstate.New(client, "")
	m.appState.AddMessage(llm.RoleUser, "previous")
	m.textarea.SetValue(replcommands.Clear)

	newM, cmd := m.handleEnterKey()

	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	if !strings.Contains(newM.output.Join(), "New session started") {
		t.Fatalf("expected new session message, got %q", newM.output.Join())
	}
	if newM.textarea.Value() != "" {
		t.Error("expected textarea to be reset")
	}
	if len(newM.appState.GetMessages()) != 0 {
		t.Fatal("expected messages to be cleared")
	}
	if client.resetCount != 1 {
		t.Fatalf("expected LLM client reset once, got %d", client.resetCount)
	}
}

func TestHandleEnterKey_LogoutCommand_NoProvider(t *testing.T) {
	m := newTestModel()
	m.ctx.cfg = &config.ResolvedConfig{Provider: ""}
	m.textarea.SetValue(replcommands.Logout)

	newM, _ := m.handleEnterKey()

	found := false
	for _, line := range newM.output.GetLines() {
		if strings.Contains(line, "No provider is configured") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error about no provider configured")
	}
}

func TestHandleEnterKey_NewCommand(t *testing.T) {
	m := newTestModel()
	client := &mockLLMClient{}
	m.appState = replappstate.New(client, "")
	m.textarea.SetValue(replcommands.New)

	newM, cmd := m.handleEnterKey()

	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	if !strings.Contains(newM.output.Join(), "New session started") {
		t.Fatalf("expected new session message, got %q", newM.output.Join())
	}
	if client.resetCount != 1 {
		t.Fatalf("expected LLM client reset once, got %d", client.resetCount)
	}
}

func TestHandleEnterKey_CompactCommandClientNotReady(t *testing.T) {
	m := newTestModel()
	m.ctx.cfg = &config.ResolvedConfig{}
	m.textarea.SetValue(replcommands.Compact)

	newM, cmd := m.handleEnterKey()

	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	found := false
	for _, line := range newM.output.GetLines() {
		if strings.Contains(line, "LLM client not initialized") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error about LLM client not initialized for /compact")
	}
}

func TestHandleEnterKey_ThinkingCommandNoSupport(t *testing.T) {
	m := newTestModel()
	m.ctx.registry = &providers.Registry{
		Providers: []providers.Provider{
			{
				ID: "openai",
				Models: []providers.Model{
					{ID: "gpt-4", ContextWindow: 2000},
				},
			},
		},
	}
	m.ctx.cfg = &config.ResolvedConfig{Provider: "openai", Model: "gpt-4"}
	m.textarea.SetValue("/thinking high")

	newM, _ := m.handleEnterKey()

	found := false
	for _, line := range newM.output.GetLines() {
		if strings.Contains(line, "does not support configurable thinking") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error about model not supporting thinking")
	}
}

func TestHandleEnterKey_SessionsCommandWithSessions(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	workingDir := filepath.Join(tmp, "project")

	m := newTestModel()
	m.ctx.registry = &providers.Registry{Providers: []providers.Provider{}}
	m.ctx.globalCfg = &config.GlobalConfig{}
	m.ctx.loader = config.NewLoader()
	m.sessions = newReplSessionState(workingDir)
	if err := m.sessions.appendUserMessage("saved prompt"); err != nil {
		t.Fatalf("append user message: %v", err)
	}
	m.sessions.resetSession()
	m.textarea.SetValue(replcommands.Sessions)

	newM, cmd := m.handleEnterKey()

	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	if newM.sessionPicker == nil {
		t.Fatal("expected session picker for saved sessions")
	}
	if newM.textarea.Value() != "" {
		t.Fatal("expected textarea to be reset")
	}
}

func TestHandleEnterKey_ResumeCommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	m := newTestModel()
	m.sessions = newReplSessionState(filepath.Join(tmp, "project"))
	m.textarea.SetValue(replcommands.Resume)

	newM, _ := m.handleEnterKey()

	if !strings.Contains(newM.output.Join(), "No saved sessions for this directory.") {
		t.Fatalf("expected empty state message for /resume, got %q", newM.output.Join())
	}
}

func TestStartModelSelection_CallsOnComplete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newTestModel()
	m.ctx.registry = &providers.Registry{Providers: []providers.Provider{}}
	m.ctx.globalCfg = &config.GlobalConfig{}
	m.ctx.loader = config.NewLoader()

	result := m.startModelSelection()
	if result.modelSelection == nil {
		t.Fatal("expected model selection to be set")
	}

	// Verify the model selection widget was initialized
	ms := result.modelSelection
	if ms.Step != replwidgets.StepProvider {
		t.Fatalf("expected model selection to start at provider step, got %d", ms.Step)
	}
}

func TestStartAdversaryModelSelectionRestoresPrimaryThinkingEffort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := newTestModel()
	m.ctx.globalCfg = &config.GlobalConfig{
		ActiveProvider: config.ProviderAnthropic,
		ActiveModel:    "claude-sonnet-5",
		ThinkingEffort: "high",
		Providers: map[string]config.ProviderConfig{
			config.ProviderMoonshotAI: {APIKey: "moonshot-key"},
		},
	}
	m.ctx.loader = config.NewLoader()
	m.ctx.registry = &providers.Registry{Providers: []providers.Provider{
		{
			ID: config.ProviderMoonshotAI,
			Models: []providers.Model{{
				ID:              "kimi-k2.6",
				ThinkingEfforts: []string{"enabled", "disabled"},
			}},
		},
	}}

	m = m.startAdversaryModelSelection()
	var cmd tea.Cmd
	m.adversary.modelSelection, cmd = m.adversary.modelSelection.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.adversary.modelSelection.Step != replwidgets.StepModel {
		t.Fatalf("expected model selection after provider selection, got step %v", m.adversary.modelSelection.Step)
	}
	m.adversary.modelSelection, cmd = m.adversary.modelSelection.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.adversary.modelSelection.Step != replwidgets.StepThinking {
		t.Fatalf("expected thinking selection after model selection, got step %v", m.adversary.modelSelection.Step)
	}
	m.adversary.modelSelection, cmd = m.adversary.modelSelection.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.adversary.modelSelection.Step != replwidgets.StepUpdateProviderConfigs {
		t.Fatalf("expected provider-config confirmation after thinking selection, got step %v", m.adversary.modelSelection.Step)
	}
	m.adversary.modelSelection, cmd = m.adversary.modelSelection.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected completion command")
	}

	if m.ctx.globalCfg.ActiveProvider != config.ProviderAnthropic || m.ctx.globalCfg.ActiveModel != "claude-sonnet-5" {
		t.Fatalf("primary model changed to %s/%s", m.ctx.globalCfg.ActiveProvider, m.ctx.globalCfg.ActiveModel)
	}
	if m.ctx.globalCfg.ThinkingEffort != "high" {
		t.Fatalf("primary thinking effort = %q, want high", m.ctx.globalCfg.ThinkingEffort)
	}
	if m.ctx.globalCfg.AdversaryProvider != config.ProviderMoonshotAI || m.ctx.globalCfg.AdversaryModel != "kimi-k2.6" {
		t.Fatalf("unexpected adversary model: %s/%s", m.ctx.globalCfg.AdversaryProvider, m.ctx.globalCfg.AdversaryModel)
	}
}

func TestHandleShowThinkingCommand_On(t *testing.T) {
	m := newTestModel()
	m.showThinking = false
	m.stream.handler.showThinking = false

	result := m.handleShowThinkingCommand("/show-thinking on")

	if !result.showThinking {
		t.Error("expected showThinking to be true after /show-thinking on")
	}
	if !result.stream.handler.showThinking {
		t.Error("expected stream.handler.showThinking to be true after /show-thinking on")
	}
	if !strings.Contains(result.output.Join(), "Thinking tokens shown") {
		t.Fatalf("expected confirmation message, got %q", result.output.Join())
	}
}

func TestHandleShowThinkingCommand_Off(t *testing.T) {
	m := newTestModel()

	result := m.handleShowThinkingCommand("/show-thinking off")

	if result.showThinking {
		t.Error("expected showThinking to be false after /show-thinking off")
	}
	if result.stream.handler.showThinking {
		t.Error("expected stream.handler.showThinking to be false after /show-thinking off")
	}
	if !strings.Contains(result.output.Join(), "Thinking tokens hidden") {
		t.Fatalf("expected confirmation message, got %q", result.output.Join())
	}
}

func TestHandleShowThinkingCommand_NoArgShowsStatus(t *testing.T) {
	m := newTestModel()

	result := m.handleShowThinkingCommand("/show-thinking")
	if !strings.Contains(result.output.Join(), "shown") {
		t.Fatalf("expected status message for shown state, got %q", result.output.Join())
	}

	m2 := newTestModel()
	m2.showThinking = false
	m2.stream.handler.showThinking = false

	result2 := m2.handleShowThinkingCommand("/show-thinking")
	if !strings.Contains(result2.output.Join(), "hidden") {
		t.Fatalf("expected status message for hidden state, got %q", result2.output.Join())
	}
}

func TestHandleShowThinkingCommand_PersistsToGlobalConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	m := newTestModel()
	m.ctx.globalCfg = &config.GlobalConfig{}
	m.ctx.loader = config.NewLoader()

	_ = m.handleShowThinkingCommand("/show-thinking off")

	if m.ctx.globalCfg.ShowThinking == nil || *m.ctx.globalCfg.ShowThinking {
		t.Error("expected globalCfg.ShowThinking to be false after /show-thinking off")
	}

	_ = m.handleShowThinkingCommand("/show-thinking on")

	if m.ctx.globalCfg.ShowThinking == nil || !*m.ctx.globalCfg.ShowThinking {
		t.Error("expected globalCfg.ShowThinking to be true after /show-thinking on")
	}
}

func TestHandleEnterKey_BtwCommandStartsStream(t *testing.T) {
	m := newTestModel()
	m.ctx.cfg = &config.ResolvedConfig{APIKey: "key", Model: "model"}
	m.appState = replappstate.New(&mockLLMClient{}, "")
	m.appState.AddMessage(llm.RoleUser, "context message")
	m.btw.streamHandler = NewStreamHandler(nil)
	m.textarea.SetValue("/btw what is this?")

	newM, cmd := m.handleEnterKey()

	if !newM.btw.showSpinner {
		t.Fatal("expected btw spinner to be visible")
	}
	if newM.btw.question != "what is this?" {
		t.Fatalf("expected btw question %q, got %q", "what is this?", newM.btw.question)
	}
	if newM.textarea.Value() != "" {
		t.Fatal("expected textarea to be reset")
	}
	if cmd == nil {
		t.Fatal("expected async btw command")
	}
}

func TestHandleEnterKey_BtwCommandDuringActiveStream(t *testing.T) {
	m := newTestModel()
	m.ctx.cfg = &config.ResolvedConfig{APIKey: "key", Model: "model"}
	m.appState = replappstate.New(&mockLLMClient{}, "")
	m.btw.streamHandler = NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")
	m.textarea.SetValue("/btw quick question")

	newM, cmd := m.handleEnterKey()

	if !newM.btw.showSpinner {
		t.Fatal("expected btw to work even during active main stream")
	}
	if cmd == nil {
		t.Fatal("expected async btw command")
	}
}

func TestHandleEnterKey_BtwCommandNoQuestion(t *testing.T) {
	m := newTestModel()
	m.btw.streamHandler = NewStreamHandler(nil)
	m.textarea.SetValue("/btw")

	newM, cmd := m.handleEnterKey()

	if newM.btw.showSpinner {
		t.Fatal("expected btw spinner not to show without question")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd for /btw without question")
	}
	found := false
	for _, line := range newM.output.GetLines() {
		if strings.Contains(line, "Usage:") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected usage hint for /btw without question")
	}
}

func TestHandleEnterKey_BtwCommandNoQuestionShowsUsage(t *testing.T) {
	m := newTestModel()
	m.btw.streamHandler = NewStreamHandler(nil)
	m.textarea.SetValue("/btw")

	newM, cmd := m.handleEnterKey()

	if newM.btw.showSpinner {
		t.Fatal("expected btw spinner not to show without question")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd for /btw without question")
	}
	found := false
	for _, line := range newM.output.GetLines() {
		if strings.Contains(line, "Usage:") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected usage hint for /btw without question")
	}
}

type fakeMCPRuntime struct {
	statuses        []keenmcp.ServerStatus
	tools           map[string][]keenmcp.Tool
	connectErr      error
	connected       string
	refreshOptCount int
}

func (f *fakeMCPRuntime) Start(context.Context) error {
	return nil
}

func (f *fakeMCPRuntime) Close() error {
	return nil
}

func (f *fakeMCPRuntime) Servers() []keenmcp.ServerStatus {
	return f.statuses
}

func (f *fakeMCPRuntime) Status(server string) keenmcp.ServerStatus {
	for _, status := range f.statuses {
		if status.Name == server {
			return status
		}
	}
	return keenmcp.ServerStatus{Name: server}
}

func (f *fakeMCPRuntime) WaitInitialScan(context.Context) error {
	return nil
}

func (f *fakeMCPRuntime) ListTools(_ context.Context, server string) ([]keenmcp.Tool, error) {
	if f.tools == nil {
		return nil, errors.New("no tools")
	}
	return f.tools[server], nil
}

func (f *fakeMCPRuntime) Refresh(_ context.Context, server string, opts ...keenmcp.RefreshOption) error {
	f.connected = server
	f.refreshOptCount = len(opts)
	return f.connectErr
}

func (f *fakeMCPRuntime) CallTool(_ context.Context, _, _ string, _ map[string]any) (*keenmcp.ToolResult, error) {
	return &keenmcp.ToolResult{}, nil
}

func TestHandleEnterKey_BtwCommandClientNotReady(t *testing.T) {
	m := newTestModel()
	m.ctx.cfg = &config.ResolvedConfig{}
	m.btw.streamHandler = NewStreamHandler(nil)
	m.textarea.SetValue("/btw question")

	newM, cmd := m.handleEnterKey()

	if newM.btw.showSpinner {
		t.Fatal("expected btw not to activate when client is not ready")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	found := false
	for _, line := range newM.output.GetLines() {
		if strings.Contains(line, "LLM client not initialized") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error about LLM client not initialized for /btw")
	}
}

func TestCancelBtwStream_ClearsState(t *testing.T) {
	m := newTestModel()
	m.btw.showSpinner = true
	m.btw.streamHandler = NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	m.btw.streamHandler.Start(eventCh, "Loading...")
	m.btw.lines = []string{"some lines"}

	m.cancelBtwStream()

	if m.btw.showSpinner {
		t.Fatal("expected btw spinner to be cleared")
	}
	if m.btw.lines != nil {
		t.Fatal("expected btw lines to be nil")
	}
}

func TestCancelBtwStream_CancelsContext(t *testing.T) {
	m := newTestModel()
	m.btw.streamHandler = NewStreamHandler(nil)
	cancelled := false
	m.btw.streamCancel = func() {
		cancelled = true
	}

	m.cancelBtwStream()

	if !cancelled {
		t.Fatal("expected btw cancel function to be called")
	}
	if m.btw.streamCancel != nil {
		t.Fatal("expected btw streamCancel to be nil after cancel")
	}
}

func TestHandleEnterKey_QueueFullNotification(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")
	m.queuedInputs = []string{"msg1", "msg2", "msg3", "msg4", "msg5"}

	m.textarea.SetValue("overflow")
	newM, cmd := m.handleEnterKey()

	if newM.notification.text != "Queue is full" {
		t.Errorf("expected 'Queue is full' notification, got %q", newM.notification.text)
	}
	if cmd == nil {
		t.Error("expected clear notification cmd")
	}
	if newM.textarea.Value() != "overflow" {
		t.Error("expected textarea to be preserved when queue is full")
	}
	if len(newM.queuedInputs) != 5 {
		t.Errorf("expected queue to remain at 5, got %d", len(newM.queuedInputs))
	}
}

func TestHandleEnterKey_NonQueueableSlashCommandNotification(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	m.textarea.SetValue("/clear")
	newM, _ := m.handleEnterKey()

	if newM.notification.text != "Operation not permitted" {
		t.Errorf("expected 'Operation not permitted' for known command, got %q", newM.notification.text)
	}
	if newM.textarea.Value() != "/clear" {
		t.Error("expected textarea to be preserved for non-queueable command")
	}
	if len(newM.queuedInputs) != 0 {
		t.Errorf("expected empty queue, got %v", newM.queuedInputs)
	}
}

func TestHandleEnterKey_UnknownSkillNotification(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	m.textarea.SetValue("/nosuchskill arg")
	newM, _ := m.handleEnterKey()

	if newM.notification.text != "No such skill found" {
		t.Errorf("expected 'No such skill found' for unknown skill, got %q", newM.notification.text)
	}
	if newM.textarea.Value() != "/nosuchskill arg" {
		t.Error("expected textarea to be preserved for unknown skill")
	}
}

func TestHandleEnterKey_MultilineNormalPromptQueued(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	m.textarea.SetValue("line1\nline2")
	newM, _ := m.handleEnterKey()

	if len(newM.queuedInputs) != 1 || newM.queuedInputs[0] != "line1\nline2" {
		t.Errorf("expected multiline normal prompt to be queued, got %v", newM.queuedInputs)
	}
	if newM.notification.text != "" {
		t.Errorf("expected no notification for queued multiline, got %q", newM.notification.text)
	}
}

func TestHandleEnterKey_MultilineSlashNotQueued(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	m.textarea.SetValue("/skill\ntext")
	newM, _ := m.handleEnterKey()

	if len(newM.queuedInputs) != 0 {
		t.Errorf("expected multiline slash input to not be queued, got %v", newM.queuedInputs)
	}
	if newM.notification.text != "No such skill found" {
		t.Errorf("expected 'No such skill found' for multiline slash, got %q", newM.notification.text)
	}
}

func TestDrainQueuedInput_SubmitsFirstMessage(t *testing.T) {
	m := newTestModel()
	m.queuedInputs = []string{"first", "second"}

	newM, _ := m.drainQueuedInput()

	if len(newM.queuedInputs) != 1 || newM.queuedInputs[0] != "second" {
		t.Errorf("expected second to remain in queue, got %v", newM.queuedInputs)
	}
	if !strings.Contains(newM.output.Join(), "first") {
		t.Error("expected drained input to be added to output")
	}
}

func TestDrainQueuedInput_EmptyQueueNoOp(t *testing.T) {
	m := newTestModel()

	newM, cmd := m.drainQueuedInput()

	if len(newM.queuedInputs) != 0 {
		t.Errorf("expected empty queue, got %v", newM.queuedInputs)
	}
	if cmd != nil {
		t.Error("expected nil cmd for empty queue")
	}
}

func TestHandleLLMError_CanceledPreservesQueue(t *testing.T) {
	m := newTestModel()
	m.queuedInputs = []string{"next msg"}
	m.stream.handler.Start(make(chan llm.StreamEvent), "Loading...")
	m.startLoading("Loading...")

	newM, _ := m.handleLLMError(context.Canceled)

	if len(newM.queuedInputs) != 1 {
		t.Errorf("expected queue to be preserved after canceled error, got %v", newM.queuedInputs)
	}
}

func TestIsKnownCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/clear", true},
		{"/model", true},
		{"/help", true},
		{"/emptyq", true},
		{"/cleanup", true},
		{"/mcp connect foo", true},
		{"/skills enable demo", true},
		{"hello", false},
		{"/nosuchskill", false},
		{"!shell", false},
		{"", false},
	}
	for _, tt := range tests {
		got := replcommands.IsKnownCommand(tt.input)
		if got != tt.want {
			t.Errorf("IsKnownCommand(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestQueuedHeight(t *testing.T) {
	m := newTestModel()
	if h := m.queuedHeight(); h != 0 {
		t.Errorf("expected 0 for empty queue, got %d", h)
	}
	m.queuedInputs = []string{"one"}
	if h := m.queuedHeight(); h != 2 {
		t.Errorf("expected 2 for 1 item (1 header + 1 line), got %d", h)
	}
	m.queuedInputs = []string{"one", "two", "three"}
	if h := m.queuedHeight(); h != 4 {
		t.Errorf("expected 4 for 3 items, got %d", h)
	}
}

func TestRenderQueuedInputs_TruncatesLongMessages(t *testing.T) {
	m := newTestModel()
	m.width = 30
	m.queuedInputs = []string{"this is a very long message that exceeds the terminal width"}

	rendered := m.renderQueuedInputs()
	stripped := ansi.Strip(rendered)

	for _, line := range strings.Split(stripped, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Errorf("line exceeds terminal width (%d > %d): %q", lipgloss.Width(line), m.width, line)
		}
	}
}

func TestHandleEnterKey_EmptyQueueClearsQueue(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")
	m.queuedInputs = []string{"msg1", "msg2"}

	m.textarea.SetValue("/emptyq")
	newM, _ := m.handleEnterKey()

	if len(newM.queuedInputs) != 0 {
		t.Errorf("expected queue to be cleared, got %v", newM.queuedInputs)
	}
	if newM.notification.text != "Queue cleared" {
		t.Errorf("expected 'Queue cleared' notification, got %q", newM.notification.text)
	}
}

func TestHandleEnterKey_EmptyQueueEmpty(t *testing.T) {
	m := newTestModel()
	m.queuedInputs = nil

	m.textarea.SetValue("/emptyq")
	newM, _ := m.handleEnterKey()

	if newM.notification.text != "Queue is empty" {
		t.Errorf("expected 'Queue is empty' notification, got %q", newM.notification.text)
	}
}

func TestHandleEnterKey_AdversaryQueuedWhenBusy(t *testing.T) {
	m := newTestModel()
	m.loading.showSpinner = true
	m.queuedInputs = nil

	m.textarea.SetValue("/adversary focus on error handling")
	newM, cmd := m.handleEnterKey()

	if len(newM.queuedInputs) != 1 || newM.queuedInputs[0] != "/adversary focus on error handling" {
		t.Errorf("expected /adversary to be queued, got %v", newM.queuedInputs)
	}
	if newM.textarea.Value() != "" {
		t.Error("expected textarea to be reset after queueing /adversary")
	}
	if cmd != nil {
		t.Error("expected nil cmd when queueing /adversary")
	}
}

func TestHandleEnterKey_AdversaryQueuedWhenBusy_AdjustsViewportHeight(t *testing.T) {
	m := newTestModel()
	m.loading.showSpinner = true
	m.adjustTextareaHeight()

	before := m.viewport.Height()
	m.textarea.SetValue("/adversary focus on error handling")
	newM, _ := m.handleEnterKey()
	after := newM.viewport.Height()

	if after != before-newM.queuedHeight() {
		t.Errorf("expected viewport height to decrease by queued height %d, got before=%d after=%d", newM.queuedHeight(), before, after)
	}
}

func TestHandleEnterKey_AdversaryQueueFull(t *testing.T) {
	m := newTestModel()
	m.loading.showSpinner = true
	m.queuedInputs = []string{"msg1", "msg2", "msg3", "msg4", "msg5"}

	m.textarea.SetValue("/adversary review")
	newM, _ := m.handleEnterKey()

	if newM.notification.text != "Queue is full" {
		t.Errorf("expected 'Queue is full' for /adversary, got %q", newM.notification.text)
	}
	if len(newM.queuedInputs) != 5 {
		t.Errorf("expected queue to remain at 5, got %d", len(newM.queuedInputs))
	}
}

func TestHandleMemoryCommand_NoFilesFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()

	m := newTestModel()
	m.ctx.workingDir = work
	result := m.handleMemoryCommand("/memory", "/memory show")
	out := ansi.Strip(result.output.Join())
	if !strings.Contains(out, "No memory files found.") {
		t.Fatalf("expected 'No memory files found.', got %q", out)
	}
}

func TestHandleMemoryCommand_ListPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".keen", "memory", "global"), 0755)
	os.WriteFile(filepath.Join(home, ".keen", "memory", "global", "MEMORY.md"), []byte("- brief"), 0644)

	m := newTestModel()
	m.ctx.workingDir = work
	result := m.handleMemoryCommand("/memory", "/memory show")
	out := ansi.Strip(result.output.Join())
	if !strings.Contains(out, "Global memory:") || !strings.Contains(out, "exists") {
		t.Fatalf("expected global memory exists, got %q", out)
	}
	if !strings.Contains(out, "Project memory:") || !strings.Contains(out, "missing") {
		t.Fatalf("expected project memory missing, got %q", out)
	}
}

func TestHandleMemoryCommand_ShowEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()

	m := newTestModel()
	m.ctx.workingDir = work
	result := m.handleMemoryCommand("/memory show", "/memory show")
	out := ansi.Strip(result.output.Join())
	if !strings.Contains(out, "Memory is empty.") {
		t.Fatalf("expected 'Memory is empty.', got %q", out)
	}
}

func TestHandleMemoryCommand_ShowContents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".keen", "memory", "global"), 0755)
	os.WriteFile(filepath.Join(home, ".keen", "memory", "global", "MEMORY.md"), []byte("- prefers brief"), 0644)
	os.MkdirAll(filepath.Join(work, ".keen"), 0755)
	os.WriteFile(filepath.Join(work, ".keen", "MEMORY.md"), []byte("- run go test -race"), 0644)

	m := newTestModel()
	m.ctx.workingDir = work
	result := m.handleMemoryCommand("/memory show", "/memory show")
	out := ansi.Strip(result.output.Join())
	if !strings.Contains(out, "Global memory") || !strings.Contains(out, "prefers brief") {
		t.Fatalf("expected global memory content, got %q", out)
	}
	if !strings.Contains(out, "Project memory") || !strings.Contains(out, "go test -race") {
		t.Fatalf("expected project memory content, got %q", out)
	}
}

func TestHandleMemoryCommand_ShowProjectOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	os.MkdirAll(filepath.Join(work, ".keen"), 0755)
	os.WriteFile(filepath.Join(work, ".keen", "MEMORY.md"), []byte("- project note"), 0644)

	m := newTestModel()
	m.ctx.workingDir = work
	result := m.handleMemoryCommand("/memory show project", "/memory show")
	out := ansi.Strip(result.output.Join())
	if !strings.Contains(out, "project note") {
		t.Fatalf("expected project memory content, got %q", out)
	}
	if strings.Contains(out, "Global memory") {
		t.Fatalf("did not expect global memory in project-only show, got %q", out)
	}
}

func TestDispatchCommand_MemoryRouting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()

	m := newTestModel()
	m.ctx.workingDir = work
	m.textarea.SetValue("/memory")
	_, _, handled := m.dispatchCommand("/memory")
	if !handled {
		t.Fatal("expected /memory to be handled")
	}
}

func TestIsKnownCommand_MemoryExists(t *testing.T) {
	if !replcommands.IsKnownCommand("/memory") {
		t.Fatal("expected /memory to be known")
	}
	if !replcommands.IsKnownCommand("/memory show") {
		t.Fatal("expected /memory show to be known")
	}
}
