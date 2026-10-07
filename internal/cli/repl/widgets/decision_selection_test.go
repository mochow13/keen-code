package widgets

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/decision"
)

func newDecisionSelection(t *testing.T, globalCfg *config.GlobalConfig, onComplete func(provider, model string) error) *DecisionModel {
	t.Helper()
	registry, err := decision.Load()
	if err != nil {
		t.Fatal(err)
	}
	return NewDecision(registry, globalCfg, config.NewLoader(), onComplete)
}

func enterKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter}
}

func TestDecisionSelectionConfiguresProviderModelAndKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	globalCfg := config.DefaultGlobalConfig()

	var completedProvider, completedModel string
	m := newDecisionSelection(t, globalCfg, func(provider, model string) error {
		completedProvider, completedModel = provider, model
		return nil
	})

	if m.Step != StepProvider || len(m.ProviderList) != 2 {
		t.Fatalf("step = %v, providers = %d", m.Step, len(m.ProviderList))
	}

	// Providers are ordered: typesafe first, liquid second.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(enterKey())
	if m.SelectedProvider != "liquid" || m.Step != StepModel {
		t.Fatalf("provider = %q, step = %v", m.SelectedProvider, m.Step)
	}

	m, _ = m.Update(enterKey())
	if m.SelectedModel != "d1" || m.Step != StepAPIKey {
		t.Fatalf("model = %q, step = %v", m.SelectedModel, m.Step)
	}

	m.APIKeyInput = "liquid_test-key"
	m, cmd := m.Update(enterKey())
	if cmd == nil {
		t.Fatal("expected completion command")
	}
	if !IsComplete(cmd()) {
		t.Fatal("expected completion message")
	}
	if completedProvider != "liquid" || completedModel != "d1" {
		t.Fatalf("onComplete = %s/%s", completedProvider, completedModel)
	}

	decisionCfg := globalCfg.Decision
	if decisionCfg == nil || !decisionCfg.Enabled {
		t.Fatalf("decision config = %#v", decisionCfg)
	}
	if decisionCfg.ActiveProvider != "liquid" || decisionCfg.ActiveModel != "d1" {
		t.Fatalf("active = %s/%s", decisionCfg.ActiveProvider, decisionCfg.ActiveModel)
	}
	if got := decisionCfg.Providers["liquid"].APIKey; got != "liquid_test-key" {
		t.Fatalf("stored API key = %q", got)
	}
}

func TestDecisionSelectionPromptsToKeepExistingKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	globalCfg := config.DefaultGlobalConfig()
	globalCfg.Decision = &config.DecisionConfig{
		Providers: map[string]config.ProviderConfig{
			"typesafe": {APIKey: "old-key"},
		},
	}

	m := newDecisionSelection(t, globalCfg, func(provider, model string) error { return nil })
	m, _ = m.Update(enterKey()) // typesafe
	m, _ = m.Update(enterKey()) // jev-1.13.0
	if m.Step != StepUpdateProviderConfigs {
		t.Fatalf("step = %v, want StepUpdateProviderConfigs", m.Step)
	}

	// Default cursor keeps the existing key.
	m, cmd := m.Update(enterKey())
	if cmd == nil || !IsComplete(cmd()) {
		t.Fatalf("expected completion, cmd = %v", cmd)
	}
	if got := globalCfg.Decision.Providers["typesafe"].APIKey; got != "old-key" {
		t.Fatalf("stored API key = %q", got)
	}
}

func TestDecisionSelectionReplaceExistingKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	globalCfg := config.DefaultGlobalConfig()
	globalCfg.Decision = &config.DecisionConfig{
		Providers: map[string]config.ProviderConfig{
			"typesafe": {APIKey: "old-key"},
		},
	}

	m := newDecisionSelection(t, globalCfg, func(provider, model string) error { return nil })
	m, _ = m.Update(enterKey())
	m, _ = m.Update(enterKey())
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // choose "Yes, enter a new key"
	m, _ = m.Update(enterKey())
	if m.Step != StepAPIKey {
		t.Fatalf("step = %v, want StepAPIKey", m.Step)
	}

	m.APIKeyInput = "new-key"
	m, cmd := m.Update(enterKey())
	if cmd == nil || !IsComplete(cmd()) {
		t.Fatalf("expected completion, cmd = %v", cmd)
	}
	if got := globalCfg.Decision.Providers["typesafe"].APIKey; got != "new-key" {
		t.Fatalf("stored API key = %q", got)
	}
}

func TestDecisionSelectionRejectsEmptyKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	globalCfg := config.DefaultGlobalConfig()

	m := newDecisionSelection(t, globalCfg, func(provider, model string) error { return nil })
	m, _ = m.Update(enterKey())
	m, _ = m.Update(enterKey())
	m, cmd := m.Update(enterKey())
	if cmd != nil {
		t.Fatal("expected no completion without an API key")
	}
	if m.ErrorMessage == "" {
		t.Fatal("expected an error message")
	}
}

func TestDecisionSelectionCancel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := newDecisionSelection(t, config.DefaultGlobalConfig(), nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil || !IsCancel(cmd()) {
		t.Fatal("expected cancel message")
	}
}
