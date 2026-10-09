package widgets

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/decision"
)

// DecisionModel is the selection wizard for decision providers. It mirrors the
// /model flow but writes to GlobalConfig.Decision instead of the primary LLM
// provider config.
type DecisionModel struct {
	Step               Step
	SelectedProvider   string
	SelectedModel      string
	APIKeyInput        string
	ProviderCursor     int
	ModelCursor        int
	UpdateConfigCursor int
	ProviderList       []decision.Provider
	ModelList          []decision.Model
	ErrorMessage       string
	registry           *decision.Registry
	globalCfg          *config.GlobalConfig
	loader             *config.Loader
	onComplete         func(provider, model string) error
}

func NewDecision(registry *decision.Registry, globalCfg *config.GlobalConfig, loader *config.Loader, onComplete func(provider, model string) error) *DecisionModel {
	return &DecisionModel{
		Step:         StepProvider,
		ProviderList: registry.Providers,
		registry:     registry,
		globalCfg:    globalCfg,
		loader:       loader,
		onComplete:   onComplete,
	}
}

func (m *DecisionModel) Update(msg tea.Msg) (*DecisionModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKeyMsg(msg)
	case tea.PasteMsg:
		if m.Step == StepAPIKey && msg.Content != "" {
			m.APIKeyInput += msg.Content
		}
	}
	return m, nil
}

func (m *DecisionModel) handleKeyMsg(msg tea.KeyPressMsg) (*DecisionModel, tea.Cmd) {
	switch m.Step {
	case StepProvider:
		switch msg.String() {
		case "up":
			m.ProviderCursor = (m.ProviderCursor - 1 + len(m.ProviderList)) % len(m.ProviderList)
		case "down":
			m.ProviderCursor = (m.ProviderCursor + 1) % len(m.ProviderList)
		case "enter":
			m.SelectedProvider = m.ProviderList[m.ProviderCursor].ID
			m.APIKeyInput = ""
			m.ErrorMessage = ""
			m.ModelList = m.ProviderList[m.ProviderCursor].Models
			m.ModelCursor = 0
			m.Step = StepModel
		case "esc":
			return m, func() tea.Msg { return modelSelectionCancelMsg{} }
		}

	case StepModel:
		switch msg.String() {
		case "up":
			m.ModelCursor = (m.ModelCursor - 1 + len(m.ModelList)) % len(m.ModelList)
		case "down":
			m.ModelCursor = (m.ModelCursor + 1) % len(m.ModelList)
		case "enter":
			m.SelectedModel = m.ModelList[m.ModelCursor].ID
			if m.getExistingAPIKey() != "" {
				m.UpdateConfigCursor = 0
				m.Step = StepUpdateProviderConfigs
			} else {
				m.Step = StepAPIKey
			}
		case "esc":
			return m, func() tea.Msg { return modelSelectionCancelMsg{} }
		}

	case StepUpdateProviderConfigs:
		switch msg.String() {
		case "up", "down":
			m.UpdateConfigCursor = 1 - m.UpdateConfigCursor
		case "enter":
			if m.UpdateConfigCursor == 0 {
				return m.complete()
			}
			m.APIKeyInput = ""
			m.Step = StepAPIKey
		case "esc":
			return m, func() tea.Msg { return modelSelectionCancelMsg{} }
		}

	case StepAPIKey:
		switch msg.String() {
		case "enter":
			return m.complete()
		case "backspace":
			if len(m.APIKeyInput) > 0 {
				m.APIKeyInput = m.APIKeyInput[:len(m.APIKeyInput)-1]
			}
		case "esc":
			return m, func() tea.Msg { return modelSelectionCancelMsg{} }
		default:
			if len(msg.Text) > 0 {
				m.APIKeyInput += msg.Text
			}
		}
	}

	return m, nil
}

func (m *DecisionModel) complete() (*DecisionModel, tea.Cmd) {
	if m.globalCfg.Decision == nil {
		m.globalCfg.Decision = &config.DecisionConfig{}
	}
	decisionCfg := m.globalCfg.Decision

	providerCfg := config.ProviderConfig{}
	if existing, exists := decisionCfg.Providers[m.SelectedProvider]; exists {
		providerCfg = existing
	}
	if m.APIKeyInput != "" {
		providerCfg.APIKey = m.APIKeyInput
	}
	providerCfg.Models = []string{m.SelectedModel}

	if providerCfg.APIKeyHelper == "" {
		if _, err := config.ResolveProviderAPIKey(m.SelectedProvider, providerCfg); err != nil {
			m.ErrorMessage = err.Error()
			return m, nil
		}
	}

	if decisionCfg.Providers == nil {
		decisionCfg.Providers = make(map[string]config.ProviderConfig)
	}
	decisionCfg.Providers[m.SelectedProvider] = providerCfg
	decisionCfg.ActiveProvider = m.SelectedProvider
	decisionCfg.ActiveModel = m.SelectedModel

	if err := m.loader.Save(m.globalCfg); err != nil {
		m.ErrorMessage = fmt.Sprintf("Failed to save config: %v", err)
		return m, nil
	}
	if err := m.onComplete(m.SelectedProvider, m.SelectedModel); err != nil {
		m.ErrorMessage = fmt.Sprintf("Failed to initialize decision evaluator: %v", err)
		return m, nil
	}

	return m, func() tea.Msg { return modelSelectionCompleteMsg{} }
}

func (m *DecisionModel) ViewString() string {
	switch m.Step {
	case StepProvider:
		return m.renderProviderSelection()
	case StepModel:
		return m.renderModelSelection()
	case StepUpdateProviderConfigs:
		return m.renderUpdateProviderConfigs()
	case StepAPIKey:
		return m.renderAPIKeyInput()
	}
	return ""
}

func (m *DecisionModel) renderProviderSelection() string {
	var view strings.Builder
	view.WriteString(repltheme.ModelSelectionTitleStyle.Render("Select a decision provider:"))
	view.WriteString("\n\n")
	view.WriteString(renderList(m.ProviderCursor, func(i int) string { return m.ProviderList[i].Name }, len(m.ProviderList)))
	view.WriteString("\n")
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("[↑/↓ to navigate, Enter to select, Esc to cancel]"))
	return view.String()
}

func (m *DecisionModel) renderModelSelection() string {
	var view strings.Builder
	view.WriteString(repltheme.ModelSelectionTitleStyle.Render(fmt.Sprintf("Select a decision model for %s:", m.getProviderName())))
	view.WriteString("\n\n")
	view.WriteString(renderList(m.ModelCursor, func(i int) string { return m.ModelList[i].Name }, len(m.ModelList)))
	view.WriteString("\n")
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("[↑/↓ to navigate, Enter to select, Esc to cancel]"))
	return view.String()
}

func (m *DecisionModel) renderUpdateProviderConfigs() string {
	var view strings.Builder
	view.WriteString(repltheme.ModelSelectionTitleStyle.Render("An API key is already configured for " + m.getProviderName() + ". Replace it?"))
	view.WriteString("\n\n")
	view.WriteString(renderList(m.UpdateConfigCursor, func(i int) string {
		return []string{"No, keep existing key", "Yes, enter a new key"}[i]
	}, 2))
	view.WriteString("\n")
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("[↑/↓ to navigate, Enter to select, Esc to cancel]"))
	return view.String()
}

func (m *DecisionModel) renderAPIKeyInput() string {
	var view strings.Builder
	title := fmt.Sprintf("Enter API key for %s", m.getProviderName())
	if m.getExistingAPIKey() != "" {
		title += "\n" + repltheme.ModelSelectionTextStyle.Render("(Press enter to keep existing key)")
	}
	view.WriteString(repltheme.ModelSelectionTextStyle.Render(title))
	view.WriteString("\n\n")

	view.WriteString(repltheme.ModelSelectionTextStyle.Render(" ▶ "))
	view.WriteString(strings.Repeat("•", len(m.APIKeyInput)))
	view.WriteString("\n\n")
	view.WriteString(repltheme.ModelSelectionTextStyle.Render("[Enter to confirm, Esc to cancel]"))

	if m.ErrorMessage != "" {
		view.WriteString("\n")
		view.WriteString(repltheme.ErrorStyle.Render(m.ErrorMessage))
	}
	return view.String()
}

func (m *DecisionModel) getProviderName() string {
	if m.registry != nil {
		if provider, ok := m.registry.GetProvider(m.SelectedProvider); ok {
			return provider.Name
		}
	}
	return m.SelectedProvider
}

func (m *DecisionModel) getExistingAPIKey() string {
	if m.globalCfg == nil || m.globalCfg.Decision == nil {
		return ""
	}
	if providerCfg, exists := m.globalCfg.Decision.Providers[m.SelectedProvider]; exists {
		return providerCfg.APIKey
	}
	return ""
}
