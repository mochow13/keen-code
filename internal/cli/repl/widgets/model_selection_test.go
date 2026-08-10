package widgets

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mochow13/keen-code/internal/auth"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/providers"
)

func TestModelSelectionUsesDimFaintStyle(t *testing.T) {
	registry := &providers.Registry{Providers: []providers.Provider{
		{ID: config.ProviderOpenAI, Name: "OpenAI"},
		{ID: config.ProviderAnthropic, Name: "Anthropic"},
	}}
	m := New(registry, config.DefaultGlobalConfig(), config.NewLoader(), &config.ResolvedConfig{}, nil)

	view := m.renderProviderSelection()
	textPrefix := stylePrefix(repltheme.ModelSelectionTextStyle)
	if !strings.Contains(view, stylePrefix(repltheme.ModelSelectionTitleStyle)+"Select a provider:") || !strings.Contains(view, "  "+textPrefix+"Anthropic") {
		t.Fatalf("expected dim, faint provider-selection content, got %q", view)
	}
	if !strings.Contains(view, stylePrefix(repltheme.ModelSelectionCursorStyle)+"▶ ") || !strings.Contains(view, stylePrefix(repltheme.ModelSelectionSelectedTextStyle)+"OpenAI") {
		t.Fatalf("expected suggestion-style cursor and selected provider text, got %q", view)
	}
}

func stylePrefix(style lipgloss.Style) string {
	rendered := style.Render("x")
	if idx := strings.Index(rendered, "x"); idx >= 0 {
		return rendered[:idx]
	}
	return rendered
}

func TestModelSelectionTitlesUseBoldDimFaintStyle(t *testing.T) {
	m := &Model{ThinkingOptions: []string{"low", "medium"}}

	view := m.renderThinkingSelection()
	if !strings.Contains(view, stylePrefix(repltheme.ModelSelectionTitleStyle)+"Select thinking effort:") {
		t.Fatalf("expected bold, dim, faint thinking-effort title, got %q", view)
	}
}

func TestModelSelectionSelectSetsPairAndPromptsThinking(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	registry := &providers.Registry{Providers: []providers.Provider{{
		ID: config.ProviderOpenAI,
		Models: []providers.Model{{
			ID:              "gpt-5.4",
			ThinkingEfforts: []string{"low", "medium", "high"},
		}},
	}}}
	m := New(registry, config.DefaultGlobalConfig(), config.NewLoader(), &config.ResolvedConfig{}, nil)

	m, cmd, err := m.Select(config.ProviderOpenAI, "gpt-5.4")
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if cmd != nil {
		t.Fatal("Select() returned an unexpected command")
	}
	if m.SelectedProvider != config.ProviderOpenAI || m.SelectedModel != "gpt-5.4" {
		t.Fatalf("selected pair = %s/%s", m.SelectedProvider, m.SelectedModel)
	}
	if m.Step != StepThinking {
		t.Fatalf("step = %v, want StepThinking", m.Step)
	}
}

func TestModelSelectionSelectRejectsUnknownPair(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	registry := &providers.Registry{Providers: []providers.Provider{{ID: config.ProviderOpenAI}}}
	m := New(registry, config.DefaultGlobalConfig(), config.NewLoader(), &config.ResolvedConfig{}, nil)

	_, _, err := m.Select(config.ProviderOpenAI, "gpt-unknown")
	if err == nil || err.Error() != "unknown model: openai/gpt-unknown" {
		t.Fatalf("Select() error = %v", err)
	}
}

func TestModelSelectionHidesOpenAICompatibleProvider(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	registry := &providers.Registry{Providers: []providers.Provider{
		{ID: config.ProviderOpenAICompatible, Name: "OpenAI Compatible"},
		{ID: config.ProviderOpenAI, Name: "OpenAI"},
	}}
	m := New(registry, config.DefaultGlobalConfig(), config.NewLoader(), &config.ResolvedConfig{}, nil)
	if len(m.ProviderList) != 1 || m.ProviderList[0].ID != config.ProviderOpenAI {
		t.Fatalf("provider list = %+v, want only OpenAI", m.ProviderList)
	}
	if len(registry.Providers) != 2 {
		t.Fatalf("model selection mutated provider registry: %+v", registry.Providers)
	}
}

func TestIsValidBaseURL_Empty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := isValidBaseURL(""); err != nil {
		t.Errorf("expected empty URL to be valid, got %v", err)
	}
}

func TestIsValidBaseURL_ValidHTTPS(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := []string{
		"https://api.example.com",
		"https://api.example.com/v1",
		"http://localhost:8080",
		"http://localhost:8080/v1/",
	}
	for _, c := range cases {
		if err := isValidBaseURL(c); err != nil {
			t.Errorf("expected %q to be valid, got %v", c, err)
		}
	}
}

func TestIsValidBaseURL_InvalidScheme(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := []string{
		"ftp://example.com",
		"example.com",
		"//example.com",
	}
	for _, c := range cases {
		if err := isValidBaseURL(c); err == nil {
			t.Errorf("expected %q to be invalid, got nil", c)
		}
	}
}

func TestIsValidBaseURL_MissingHost(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := isValidBaseURL("https://"); err == nil {
		t.Error("expected URL with no host to be invalid")
	}
}

func TestModelSelection_OpenAICodexSkipsAPIKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	registry := &providers.Registry{
		Providers: []providers.Provider{
			{
				ID:   config.ProviderOpenAICodex,
				Name: "Codex (ChatGPT OAuth)",
				Models: []providers.Model{
					{
						ID:              "gpt-5.4",
						Name:            "GPT-5.4",
						ThinkingEfforts: []string{"low", "medium", "high", "xhigh"},
					},
				},
			},
		},
	}
	global := config.DefaultGlobalConfig()
	resolved := &config.ResolvedConfig{}
	store := auth.NewStoreAt(t.TempDir() + "/auth.json")
	if err := store.Set(config.ProviderOpenAICodex, auth.OAuthCredential{
		Type:         "oauth",
		AccessToken:  "access",
		RefreshToken: "refresh",
	}); err != nil {
		t.Fatalf("seed auth store: %v", err)
	}
	manager := auth.NewOAuthManager(store)

	completed := false
	m := NewWithAuthManager(registry, global, config.NewLoader(), resolved, manager, func(provider, model, apiKey string) error {
		completed = true
		if apiKey != "" {
			t.Fatalf("expected empty API key, got %q", apiKey)
		}
		return nil
	})

	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("did not expect OAuth command for existing credentials")
	}
	if m.Step != StepModel {
		t.Fatalf("expected StepModel, got %v", m.Step)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Step != StepThinking {
		t.Fatalf("expected StepThinking, got %v", m.Step)
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !completed {
		t.Fatal("expected completion")
	}
	if cmd == nil {
		t.Fatal("expected completion command")
	}
	if resolved.Provider != config.ProviderOpenAICodex || resolved.Model != "gpt-5.4" {
		t.Fatalf("unexpected resolved config: %+v", resolved)
	}
	if resolved.APIKey != "" || resolved.BaseURL != "" {
		t.Fatalf("expected no API key/base URL for Codex, got %+v", resolved)
	}
}

func TestModelSelection_BedrockAPIKeyCanBeSkipped(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	registry := &providers.Registry{
		Providers: []providers.Provider{
			{
				ID:   config.ProviderBedrock,
				Name: "Amazon Bedrock",
				Models: []providers.Model{
					{ID: "global.anthropic.claude-sonnet-4-6", Name: "Claude Sonnet 4.6"},
				},
			},
		},
	}
	global := config.DefaultGlobalConfig()
	resolved := &config.ResolvedConfig{}

	completed := false
	m := New(registry, global, config.NewLoader(), resolved, func(provider, model, apiKey string) error {
		completed = true
		if apiKey != "" {
			t.Fatalf("expected empty Bedrock API key for AWS auth fallback, got %q", apiKey)
		}
		return nil
	})
	var cmd tea.Cmd
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Step != StepAPIKey {
		t.Fatalf("expected optional StepAPIKey, got %v", m.Step)
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !completed {
		t.Fatal("expected completion without API key")
	}
	if cmd == nil {
		t.Fatal("expected completion command")
	}
	if resolved.APIKey != "" {
		t.Fatalf("expected empty resolved API key, got %q", resolved.APIKey)
	}
	if resolved.AuthMode != config.AuthModeAWS {
		t.Fatalf("expected AWS auth mode, got %q", resolved.AuthMode)
	}
}

func TestModelSelection_UsesAPIKeyHelperForResolvedKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	registry := &providers.Registry{
		Providers: []providers.Provider{
			{
				ID:   config.ProviderAnthropic,
				Name: "Anthropic",
				Models: []providers.Model{
					{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6"},
				},
			},
		},
	}
	global := config.DefaultGlobalConfig()
	global.SetProviderConfig(config.ProviderAnthropic, config.ProviderConfig{
		APIKeyHelper: "printf helper-key",
		Models:       []string{"claude-sonnet-4-6"},
	})
	resolved := &config.ResolvedConfig{}

	var completedAPIKey string
	m := New(registry, global, config.NewLoader(), resolved, func(provider, model, apiKey string) error {
		completedAPIKey = apiKey
		return nil
	})

	var cmd tea.Cmd
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Step != StepBaseURL {
		t.Fatalf("expected StepBaseURL, got %v", m.Step)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Step != StepAPIKey {
		t.Fatalf("expected StepAPIKey, got %v", m.Step)
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected api key helper command")
	}
	if m.Step != StepAPIKeyHelper {
		t.Fatalf("expected StepAPIKeyHelper, got %v", m.Step)
	}
	if !strings.Contains(m.renderAPIKeyHelperStatus(), "Fetching credentials...") {
		t.Fatalf("expected fetching credentials status, got %q", m.renderAPIKeyHelperStatus())
	}

	m, cmd = m.Update(cmd())
	if cmd == nil {
		t.Fatal("expected completion command")
	}
	if !cmdCalled(cmd) {
		t.Fatal("expected completion message")
	}
	if resolved.APIKey != "helper-key" {
		t.Fatalf("expected resolved helper key, got %q", resolved.APIKey)
	}
	if completedAPIKey != "helper-key" {
		t.Fatalf("expected completion helper key, got %q", completedAPIKey)
	}
	saved, ok := global.GetProviderConfig(config.ProviderAnthropic)
	if !ok {
		t.Fatal("expected saved provider config")
	}
	if saved.APIKey != "" {
		t.Fatalf("expected helper output not to be persisted, got APIKey %q", saved.APIKey)
	}
	if saved.APIKeyHelper != "printf helper-key" {
		t.Fatalf("expected api key helper preserved, got %q", saved.APIKeyHelper)
	}
}

func TestModelSelection_APIKeyHelperFailureReturnsToInput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	registry := &providers.Registry{
		Providers: []providers.Provider{
			{
				ID:   config.ProviderAnthropic,
				Name: "Anthropic",
				Models: []providers.Model{
					{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6"},
				},
			},
		},
	}
	global := config.DefaultGlobalConfig()
	global.SetProviderConfig(config.ProviderAnthropic, config.ProviderConfig{
		APIKeyHelper: "exit 1",
		Models:       []string{"claude-sonnet-4-6"},
	})
	resolved := &config.ResolvedConfig{}

	m := New(registry, global, config.NewLoader(), resolved, func(provider, model, apiKey string) error {
		t.Fatal("completion should not be called on helper failure")
		return nil
	})

	var cmd tea.Cmd
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Step != StepAPIKey {
		t.Fatalf("expected StepAPIKey, got %v", m.Step)
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected api key helper command")
	}
	if m.Step != StepAPIKeyHelper {
		t.Fatalf("expected StepAPIKeyHelper, got %v", m.Step)
	}

	m, cmd = m.Update(cmd())
	if cmd != nil {
		t.Fatal("expected no command after helper failure")
	}
	if m.Step != StepAPIKey {
		t.Fatalf("expected StepAPIKey after helper failure, got %v", m.Step)
	}
	if m.ErrorMessage == "" {
		t.Fatal("expected error message after helper failure")
	}
}

func TestModelSelection_LongModelListScrollsWithCursor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	models := make([]providers.Model, 14)
	for i := range models {
		models[i] = providers.Model{
			ID:   fmt.Sprintf("model-%02d", i+1),
			Name: fmt.Sprintf("Model %02d", i+1),
		}
	}
	registry := &providers.Registry{
		Providers: []providers.Provider{
			{
				ID:     config.ProviderOpenCodeGo,
				Name:   "OpenCode Go",
				Models: models,
			},
		},
	}

	m := New(registry, config.DefaultGlobalConfig(), config.NewLoader(), &config.ResolvedConfig{}, func(provider, model, apiKey string) error {
		return nil
	})
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("did not expect command when entering model selection")
	}
	if m.Step != StepModel {
		t.Fatalf("expected StepModel, got %v", m.Step)
	}

	initial := m.renderModelSelection()
	if !strings.Contains(initial, "Model 01") {
		t.Fatalf("expected first model in initial view, got %q", initial)
	}
	if strings.Contains(initial, "Model 14") {
		t.Fatalf("did not expect final model before scrolling, got %q", initial)
	}
	if !strings.Contains(initial, "↓") {
		t.Fatalf("expected downward more indicator, got %q", initial)
	}

	for i := 0; i < 13; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}

	scrolled := m.renderModelSelection()
	if !strings.Contains(scrolled, "Model 14") {
		t.Fatalf("expected final model after scrolling, got %q", scrolled)
	}
	if strings.Contains(scrolled, "Model 01") {
		t.Fatalf("did not expect first model after scrolling to bottom, got %q", scrolled)
	}
	if !strings.Contains(scrolled, "↑") {
		t.Fatalf("expected upward more indicator, got %q", scrolled)
	}
}

func TestModelSelection_SwitchProviderPreservesHeaders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	registry := &providers.Registry{
		Providers: []providers.Provider{
			{
				ID:   config.ProviderDeepSeek,
				Name: "DeepSeek",
				Models: []providers.Model{
					{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro"},
				},
			},
			{
				ID:   config.ProviderOpenAI,
				Name: "OpenAI",
				Models: []providers.Model{
					{ID: "gpt-4o", Name: "GPT-4o"},
				},
			},
		},
	}

	global := config.DefaultGlobalConfig()
	loader := config.NewLoader()
	resolved := &config.ResolvedConfig{}

	// Seed DeepSeek provider config with custom headers.
	global.SetProviderConfig(config.ProviderDeepSeek, config.ProviderConfig{
		APIKey: "ds-key",
		Models: []string{"deepseek-v4-pro"},
		Headers: map[string]string{
			"x-header-1": "val1",
			"x-header-2": "val2",
		},
	})
	if err := loader.Save(global); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	m := New(registry, global, loader, resolved, func(provider, model, apiKey string) error {
		return nil
	})

	// Confirm first provider (DeepSeek) selected.
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("did not expect command when entering provider selection")
	}
	if m.Step != StepModel {
		t.Fatalf("expected StepModel, got %v", m.Step)
	}
	// Confirm model, then decline to update the existing provider config.
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Step != StepUpdateProviderConfigs {
		t.Fatalf("expected StepUpdateProviderConfigs, got %v", m.Step)
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !cmdCalled(cmd) {
		t.Fatal("expected completion command")
	}
	if resolved.Provider != config.ProviderDeepSeek {
		t.Fatalf("expected provider %q, got %q", config.ProviderDeepSeek, resolved.Provider)
	}
	if len(resolved.Headers) != 2 || resolved.Headers["x-header-1"] != "val1" {
		t.Fatalf("expected headers preserved, got %+v", resolved.Headers)
	}
}

func TestModelSelection_ExistingAPIKeyPromptsBeforeUpdatingProviderConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	registry := &providers.Registry{Providers: []providers.Provider{{
		ID:   config.ProviderDeepSeek,
		Name: "DeepSeek",
		Models: []providers.Model{
			{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro"},
			{ID: "deepseek-v4-chat", Name: "DeepSeek V4 Chat"},
		},
	}}}
	global := config.DefaultGlobalConfig()
	global.SetProviderConfig(config.ProviderDeepSeek, config.ProviderConfig{
		APIKey:  "existing-key",
		BaseURL: "https://api.deepseek.example/v1",
		Models:  []string{"deepseek-v4-pro"},
	})
	resolved := &config.ResolvedConfig{}
	completed := false
	m := New(registry, global, config.NewLoader(), resolved, func(provider, model, apiKey string) error {
		completed = true
		return nil
	})

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Step != StepUpdateProviderConfigs {
		t.Fatalf("expected StepUpdateProviderConfigs, got %v", m.Step)
	}
	if view := m.ViewString(); !strings.Contains(view, "Update provider configs?") {
		t.Fatalf("expected update config prompt, got %q", view)
	}

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !cmdCalled(cmd) || !completed {
		t.Fatal("expected model selection to complete")
	}
	if resolved.APIKey != "existing-key" || resolved.BaseURL != "https://api.deepseek.example/v1" {
		t.Fatalf("expected existing configs to be retained, got %+v", resolved)
	}
	if resolved.Model != "deepseek-v4-chat" {
		t.Fatalf("expected selected model, got %q", resolved.Model)
	}
}

func TestModelSelection_ExistingAPIKeyCanUpdateProviderConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	registry := &providers.Registry{Providers: []providers.Provider{{
		ID:   config.ProviderDeepSeek,
		Name: "DeepSeek",
		Models: []providers.Model{
			{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro"},
		},
	}}}
	global := config.DefaultGlobalConfig()
	global.SetProviderConfig(config.ProviderDeepSeek, config.ProviderConfig{
		APIKey:  "existing-key",
		BaseURL: "https://api.deepseek.example/v1",
		Models:  []string{"deepseek-v4-pro"},
	})
	m := New(registry, global, config.NewLoader(), &config.ResolvedConfig{}, func(provider, model, apiKey string) error {
		return nil
	})

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Step != StepUpdateProviderConfigs {
		t.Fatalf("expected StepUpdateProviderConfigs, got %v", m.Step)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Step != StepBaseURL {
		t.Fatalf("expected StepBaseURL after selecting Yes, got %v", m.Step)
	}
	if m.BaseURLInput != "https://api.deepseek.example/v1" {
		t.Fatalf("expected existing base URL, got %q", m.BaseURLInput)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Step != StepAPIKey {
		t.Fatalf("expected StepAPIKey after confirming base URL, got %v", m.Step)
	}
}

func cmdCalled(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	_, ok := msg.(modelSelectionCompleteMsg)
	return ok
}

func TestVisibleListRangeKeepsCursorVisible(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tests := []struct {
		name       string
		cursor     int
		count      int
		maxVisible int
		wantStart  int
		wantEnd    int
	}{
		{name: "empty", cursor: 0, count: 0, maxVisible: 7, wantStart: 0, wantEnd: 0},
		{name: "short list", cursor: 2, count: 4, maxVisible: 7, wantStart: 0, wantEnd: 4},
		{name: "top", cursor: 0, count: 14, maxVisible: 7, wantStart: 0, wantEnd: 7},
		{name: "middle", cursor: 6, count: 14, maxVisible: 7, wantStart: 3, wantEnd: 10},
		{name: "bottom", cursor: 13, count: 14, maxVisible: 7, wantStart: 7, wantEnd: 14},
	}

	for _, tt := range tests {
		gotStart, gotEnd := visibleListRange(tt.cursor, tt.count, tt.maxVisible)
		if gotStart != tt.wantStart || gotEnd != tt.wantEnd {
			t.Fatalf("%s: expected %d:%d, got %d:%d", tt.name, tt.wantStart, tt.wantEnd, gotStart, gotEnd)
		}
		if tt.count > 0 && tt.count > tt.maxVisible && (tt.cursor < gotStart || tt.cursor >= gotEnd) {
			t.Fatalf("%s: cursor %d outside visible range %d:%d", tt.name, tt.cursor, gotStart, gotEnd)
		}
	}
}

func TestModelSelectionNavigationWrapsAndCancels(t *testing.T) {
	registry := &providers.Registry{Providers: []providers.Provider{
		{ID: config.ProviderAnthropic, Name: "Anthropic", Models: []providers.Model{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}},
		{ID: config.ProviderOpenAI, Name: "OpenAI"},
	}}
	m := New(registry, config.DefaultGlobalConfig(), config.NewLoader(), &config.ResolvedConfig{}, nil)

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.ProviderCursor != 1 {
		t.Fatalf("provider cursor = %d, want wrapped cursor 1", m.ProviderCursor)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.ProviderCursor != 0 {
		t.Fatalf("provider cursor = %d, want wrapped cursor 0", m.ProviderCursor)
	}

	m.Step = StepModel
	m.ModelList = registry.Providers[0].Models
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.ModelCursor != 1 {
		t.Fatalf("model cursor = %d, want wrapped cursor 1", m.ModelCursor)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.ModelCursor != 0 {
		t.Fatalf("model cursor = %d, want wrapped cursor 0", m.ModelCursor)
	}

	m.Step = StepThinking
	m.ThinkingOptions = []string{"low", "high"}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.ThinkingCursor != 1 {
		t.Fatalf("thinking cursor = %d, want wrapped cursor 1", m.ThinkingCursor)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.ThinkingCursor != 0 {
		t.Fatalf("thinking cursor = %d, want wrapped cursor 0", m.ThinkingCursor)
	}

	for _, step := range []Step{StepProvider, StepModel, StepThinking, StepUpdateProviderConfigs, StepBaseURL, StepAPIKey, StepAPIKeyHelper} {
		m.Step = step
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		if cmd == nil || !IsCancel(cmd()) {
			t.Fatalf("step %v did not return cancellation", step)
		}
	}
}

func TestModelSelectionOAuthCancelInvokesCancel(t *testing.T) {
	cancelled := false
	m := &Model{Step: StepOAuth, oauthCancel: func() { cancelled = true }}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !cancelled {
		t.Fatal("OAuth cancellation function was not called")
	}
	if cmd == nil || !IsCancel(cmd()) {
		t.Fatal("OAuth escape did not return cancellation message")
	}
}

func TestModelSelectionEditsAndPastesProviderInputs(t *testing.T) {
	m := &Model{Step: StepBaseURL, BaseURLError: "old error"}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "https://example.test"})
	if m.BaseURLInput != "https://example.test" || m.BaseURLError != "" {
		t.Fatalf("base URL input = %q, error = %q", m.BaseURLInput, m.BaseURLError)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.BaseURLInput != "https://example.tes" {
		t.Fatalf("base URL after backspace = %q", m.BaseURLInput)
	}
	m.BaseURLError = "old error"
	m, _ = m.Update(tea.PasteMsg{Content: "t/v1"})
	if m.BaseURLInput != "https://example.test/v1" || m.BaseURLError != "" {
		t.Fatalf("base URL after paste = %q, error = %q", m.BaseURLInput, m.BaseURLError)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Step != StepAPIKey || m.BaseURLError != "" {
		t.Fatalf("valid base URL left step=%v error=%q", m.Step, m.BaseURLError)
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "secret"})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m, _ = m.Update(tea.PasteMsg{Content: "t"})
	if m.APIKeyInput != "secret" {
		t.Fatalf("API key input = %q, want secret", m.APIKeyInput)
	}
}

func TestModelSelectionRejectsInvalidBaseURL(t *testing.T) {
	m := &Model{Step: StepBaseURL, BaseURLInput: "example.test"}
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.Step != StepBaseURL || m.BaseURLError == "" {
		t.Fatalf("invalid URL produced step=%v error=%q cmd=%v", m.Step, m.BaseURLError, cmd)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "x"})
	if m.BaseURLError != "" {
		t.Fatalf("editing did not clear base URL error %q", m.BaseURLError)
	}
}

func TestModelSelectionOAuthCompletion(t *testing.T) {
	registry := &providers.Registry{Providers: []providers.Provider{{
		ID:     config.ProviderOpenAICodex,
		Models: []providers.Model{{ID: "gpt", Name: "GPT"}},
	}}}
	m := &Model{
		Step:             StepOAuth,
		SelectedProvider: config.ProviderOpenAICodex,
		OAuthStatus:      "waiting",
		OAuthURL:         "https://example.test/auth",
		registry:         registry,
	}

	m, cmd := m.Update(modelSelectionOAuthCompleteMsg{err: fmt.Errorf("denied")})
	if cmd != nil || m.ErrorMessage != "denied" || m.OAuthStatus != "Authentication failed." {
		t.Fatalf("OAuth error state = error %q status %q cmd=%v", m.ErrorMessage, m.OAuthStatus, cmd)
	}

	m.ErrorMessage = ""
	m, cmd = m.Update(modelSelectionOAuthCompleteMsg{})
	if cmd != nil || m.Step != StepModel || len(m.ModelList) != 1 || m.OAuthStatus != "" || m.OAuthURL != "" {
		t.Fatalf("OAuth success state = step %v models %d status %q URL %q", m.Step, len(m.ModelList), m.OAuthStatus, m.OAuthURL)
	}
}

func TestModelSelectionIgnoresStaleAPIKeyHelperResult(t *testing.T) {
	m := &Model{Step: StepProvider}
	got, cmd := m.Update(modelSelectionAPIKeyHelperResultMsg{apiKey: "secret"})
	if got != m || cmd != nil || m.Step != StepProvider {
		t.Fatal("stale API key helper result changed model")
	}
}

func TestModelSelectionResolveThinkingCursor(t *testing.T) {
	tests := []struct {
		name     string
		resolved *config.ResolvedConfig
		options  []string
		want     int
	}{
		{name: "defaults to medium", resolved: &config.ResolvedConfig{}, options: []string{"low", "medium", "high"}, want: 1},
		{name: "defaults to first without medium", resolved: &config.ResolvedConfig{}, options: []string{"low", "high"}, want: 0},
		{name: "uses current", resolved: &config.ResolvedConfig{ThinkingEffort: "high"}, options: []string{"low", "high"}, want: 1},
		{name: "unknown falls back to medium", resolved: &config.ResolvedConfig{ThinkingEffort: "other"}, options: []string{"low", "medium"}, want: 1},
		{name: "unknown falls back to first", resolved: &config.ResolvedConfig{ThinkingEffort: "other"}, options: []string{"low", "high"}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Model{resolvedCfg: tt.resolved}
			if got := m.resolveThinkingCursor(tt.options); got != tt.want {
				t.Fatalf("resolveThinkingCursor() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestModelSelectionViewStates(t *testing.T) {
	global := config.DefaultGlobalConfig()
	global.SetProviderConfig(config.ProviderAnthropic, config.ProviderConfig{
		APIKey:  "existing",
		BaseURL: "https://api.example.test",
	})
	registry := &providers.Registry{Providers: []providers.Provider{{ID: config.ProviderAnthropic, Name: "Anthropic"}}}
	m := &Model{
		registry:         registry,
		globalCfg:        global,
		SelectedProvider: config.ProviderAnthropic,
		BaseURLInput:     "https://new.example.test",
		BaseURLError:     "invalid URL",
		APIKeyInput:      "secret",
		ErrorMessage:     "credential error",
		OAuthStatus:      "Authentication failed.",
		OAuthURL:         "https://example.test/auth",
	}

	tests := []struct {
		step Step
		want []string
	}{
		{StepBaseURL, []string{"Base URL for Anthropic", "current: https://api.example.test", "invalid URL"}},
		{StepAPIKey, []string{"Enter API key for Anthropic", "keep existing key", "credential error"}},
		{StepAPIKeyHelper, []string{"Fetching credentials for Anthropic", "Please wait"}},
		{StepOAuth, []string{"Sign in with OpenAI", "Authentication failed.", "https://example.test/auth", "credential error"}},
	}
	for _, tt := range tests {
		m.Step = tt.step
		view := m.ViewString()
		for _, want := range tt.want {
			if !strings.Contains(view, want) {
				t.Fatalf("step %v view %q missing %q", tt.step, view, want)
			}
		}
	}
	m.Step = Step(999)
	if view := m.ViewString(); view != "" {
		t.Fatalf("unknown step view = %q", view)
	}
}

func TestModelSelectionMessagePredicates(t *testing.T) {
	if !IsComplete(modelSelectionCompleteMsg{}) || IsComplete(modelSelectionCancelMsg{}) {
		t.Fatal("IsComplete returned incorrect result")
	}
	if !IsCancel(modelSelectionCancelMsg{}) || IsCancel(modelSelectionCompleteMsg{}) {
		t.Fatal("IsCancel returned incorrect result")
	}
}

func TestModelSelectionLookupHelpersHandleMissingState(t *testing.T) {
	m := &Model{}
	if m.getProviderName("missing") != "" || m.getExistingAPIKey("missing") != "" || m.getExistingBaseURL("missing") != "" {
		t.Fatal("lookup helper returned data without configuration")
	}
	m.registry = &providers.Registry{}
	m.globalCfg = config.DefaultGlobalConfig()
	if m.getProviderName("missing") != "" || m.getExistingAPIKey("missing") != "" || m.getExistingBaseURL("missing") != "" {
		t.Fatal("lookup helper returned data for unknown provider")
	}
}

func TestModelSelectionReportsClientInitializationFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	registry := &providers.Registry{Providers: []providers.Provider{{
		ID:     config.ProviderAnthropic,
		Models: []providers.Model{{ID: "claude", Name: "Claude"}},
	}}}
	m := New(registry, config.DefaultGlobalConfig(), config.NewLoader(), &config.ResolvedConfig{}, func(_, _, _ string) error {
		return fmt.Errorf("client unavailable")
	})
	m.SelectedProvider = config.ProviderAnthropic
	m.SelectedModel = "claude"
	m.APIKeyInput = "secret"

	m, cmd := m.complete()
	if cmd != nil || !strings.Contains(m.ErrorMessage, "client unavailable") {
		t.Fatalf("complete() error = %q cmd=%v", m.ErrorMessage, cmd)
	}
}
