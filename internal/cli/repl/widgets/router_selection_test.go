package widgets

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mochow13/keen-code/internal/auth"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/providers"
)

func TestRouterSelectionAuthenticatedAndPreselected(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	registry := &providers.Registry{Providers: []providers.Provider{
		{ID: "unconfigured", Models: []providers.Model{{ID: "one"}}},
		{ID: config.ProviderBedrock, Models: []providers.Model{{ID: "one"}}},
		{ID: "helper-only", Models: []providers.Model{{ID: "one"}}},
		{ID: config.ProviderOpenAI, Models: []providers.Model{{ID: "first"}, {ID: "saved", ThinkingEfforts: []string{"low", "medium", "high"}}}},
	}}
	global := config.DefaultGlobalConfig()
	global.ActiveProvider, global.ActiveModel = "original", "original-model"
	global.Providers["helper-only"] = config.ProviderConfig{APIKeyHelper: "false"}
	global.Providers[config.ProviderOpenAI] = config.ProviderConfig{APIKey: "test-key"}
	before := *global
	var selected config.RouterModel
	existing := config.RouterModel{Provider: config.ProviderOpenAI, Model: "saved", ThinkingEffort: "high"}
	m := NewRouterSelection(registry, global, config.RouterCategoryComplex, existing, func(choice config.RouterModel) error { selected = choice; return nil })
	if len(m.ProviderList) != 1 || m.ProviderList[0].ID != config.ProviderOpenAI {
		t.Fatalf("providers: %#v", m.ProviderList)
	}
	if !strings.Contains(m.ViewString(), "complex") {
		t.Fatal("category missing from selection")
	}
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	m, _ = m.Update(enter)
	if m.Step != StepModel || m.ModelCursor != 1 {
		t.Fatalf("model not preselected: %#v", m)
	}
	m, _ = m.Update(enter)
	if m.Step != StepThinking || m.ThinkingCursor != 2 {
		t.Fatal("thinking not preselected")
	}
	_, cmd := m.Update(enter)
	if cmd == nil || !IsComplete(cmd()) {
		t.Fatal("selection did not complete")
	}
	if selected != existing {
		t.Fatalf("selected %v, want %v", selected, existing)
	}
	if !reflect.DeepEqual(*global, before) {
		t.Fatal("router selection changed global configuration")
	}
	if config.NewLoader().Exists() {
		t.Fatal("category selection saved configuration prematurely")
	}
}

func TestRouterSelectionEmptyProvidersCanCancel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewRouterSelection(&providers.Registry{}, config.DefaultGlobalConfig(), config.RouterCategorySimple, config.RouterModel{}, func(config.RouterModel) error { t.Fatal("unexpected completion"); return nil })
	for _, code := range []rune{tea.KeyUp, tea.KeyDown, tea.KeyEnter} {
		m, _ = m.Update(tea.KeyPressMsg{Code: code})
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil || !IsCancel(cmd()) {
		t.Fatal("empty selection should cancel")
	}
}

func TestRouterSelectionIncludesStoredOAuth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	registry := &providers.Registry{Providers: []providers.Provider{{ID: config.ProviderOpenAICodex, Models: []providers.Model{{ID: "gpt-5.4"}}}}}
	global := config.DefaultGlobalConfig()
	newSelection := func() *Model {
		return NewRouterSelection(registry, global, config.RouterCategorySimple, config.RouterModel{}, func(config.RouterModel) error { return nil })
	}
	if len(newSelection().ProviderList) != 0 {
		t.Fatal("unauthenticated OAuth provider included")
	}
	if err := auth.NewStore().Set(config.ProviderOpenAICodex, auth.OAuthCredential{AccessToken: "test-token", RefreshToken: "test-refresh"}); err != nil {
		t.Fatal(err)
	}
	if len(newSelection().ProviderList) != 1 {
		t.Fatal("authenticated OAuth provider excluded")
	}
}

func TestRouterSelectionDescriptivePrompts(t *testing.T) {
	for _, tc := range []struct{ category, progress, description string }{
		{config.RouterCategorySimple, "1/3", "fast, cost-effective"},
		{config.RouterCategoryStandard, "2/3", "classification fails"},
		{config.RouterCategoryComplex, "3/3", "most capable model"},
	} {
		for _, step := range []Step{StepProvider, StepModel, StepThinking} {
			m := &Model{routerCategory: tc.category, Step: step}
			prompt := ansi.Strip(m.routerSelectionPrompt())
			for _, want := range []string{tc.progress, tc.category + " tasks", tc.description} {
				if !strings.Contains(prompt, want) {
					t.Errorf("category %s, step %v: missing %q in %q", tc.category, step, want, prompt)
				}
			}
			seen := map[string]bool{}
			for _, line := range strings.Split(prompt, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if seen[line] {
					t.Errorf("repeated router prompt line: %q", line)
				}
				seen[line] = true
			}
			if strings.Contains(prompt, "Choose the") {
				t.Errorf("router prompt should not contain redundant instruction text: %q", prompt)
			}
		}
	}
}
