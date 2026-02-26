package modelselection

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/user/keen-cli/configs/providers"
	"github.com/user/keen-cli/internal/config"
)

type Step int

const (
	StepProvider Step = iota
	StepModel
	StepAPIKey
)

type keyEnter struct{}
type keyCancel struct{}

type Model struct {
	Step             Step
	SelectedProvider string
	SelectedModel    string
	APIKeyInput      string
	ProviderCursor   int
	ModelCursor      int
	ProviderList     []providers.Provider
	ModelList        []providers.Model
	ErrorMessage     string
	registry         *providers.Registry
	globalCfg        *config.GlobalConfig
	loader           *config.Loader
	resolvedCfg      *config.ResolvedConfig
	onComplete       func(provider, model, apiKey string) error
}

func New(registry *providers.Registry, globalCfg *config.GlobalConfig, loader *config.Loader, resolvedCfg *config.ResolvedConfig, onComplete func(provider, model, apiKey string) error) *Model {
	return &Model{
		Step:         StepProvider,
		ProviderList: registry.Providers,
		registry:     registry,
		globalCfg:    globalCfg,
		loader:       loader,
		resolvedCfg:  resolvedCfg,
		onComplete:   onComplete,
	}
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKeyMsg(msg)
	}
	return m, nil
}

func (m *Model) handleKeyMsg(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.Step {
	case StepProvider:
		switch msg.String() {
		case "up", "k":
			if m.ProviderCursor > 0 {
				m.ProviderCursor--
			}
		case "down", "j":
			if m.ProviderCursor < len(m.ProviderList)-1 {
				m.ProviderCursor++
			}
		case "enter":
			m.SelectedProvider = m.ProviderList[m.ProviderCursor].ID
			provider, _ := m.registry.GetProvider(m.SelectedProvider)
			m.ModelList = provider.Models
			m.ModelCursor = 0
			m.Step = StepModel
		case "esc":
			return m, func() tea.Msg { return keyCancel{} }
		}

	case StepModel:
		switch msg.String() {
		case "up", "k":
			if m.ModelCursor > 0 {
				m.ModelCursor--
			}
		case "down", "j":
			if m.ModelCursor < len(m.ModelList)-1 {
				m.ModelCursor++
			}
		case "enter":
			m.SelectedModel = m.ModelList[m.ModelCursor].ID
			m.Step = StepAPIKey
		case "esc":
			return m, func() tea.Msg { return keyCancel{} }
		}

	case StepAPIKey:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return keyCancel{} }
		case "enter":
			return m.complete()
		case "backspace":
			if len(m.APIKeyInput) > 0 {
				m.APIKeyInput = m.APIKeyInput[:len(m.APIKeyInput)-1]
			}
		default:
			if len(msg.Text) > 0 {
				m.APIKeyInput += msg.Text
			}
		}
	}

	return m, nil
}

func (m *Model) complete() (tea.Model, tea.Cmd) {
	existingKey := ""
	if providerCfg, exists := m.globalCfg.GetProviderConfig(m.SelectedProvider); exists {
		existingKey = providerCfg.APIKey
	}

	apiKey := m.APIKeyInput
	if apiKey == "" && existingKey != "" {
		apiKey = existingKey
	}

	if apiKey == "" {
		m.ErrorMessage = "API key is required"
		return m, nil
	}

	m.globalCfg.ActiveProvider = m.SelectedProvider
	m.globalCfg.ActiveModel = m.SelectedModel

	providerCfg := config.ProviderConfig{
		APIKey: apiKey,
		Models: []string{m.SelectedModel},
	}
	m.globalCfg.SetProviderConfig(m.SelectedProvider, providerCfg)

	if err := m.loader.Save(m.globalCfg); err != nil {
		m.ErrorMessage = fmt.Sprintf("Failed to save config: %v", err)
		return m, nil
	}

	m.resolvedCfg.Provider = m.SelectedProvider
	m.resolvedCfg.Model = m.SelectedModel
	m.resolvedCfg.APIKey = apiKey

	if err := m.onComplete(m.SelectedProvider, m.SelectedModel, apiKey); err != nil {
		m.ErrorMessage = fmt.Sprintf("Failed to initialize LLM client: %v", err)
		return m, nil
	}

	return m, func() tea.Msg { return keyEnter{} }
}

func (m *Model) View() tea.View {
	return tea.NewView(m.ViewString())
}

func (m *Model) ViewString() string {
	switch m.Step {
	case StepProvider:
		return m.renderProviderSelection()
	case StepModel:
		return m.renderModelSelection()
	case StepAPIKey:
		return m.renderAPIKeyInput()
	}
	return ""
}

func (m *Model) renderProviderSelection() string {
	var view strings.Builder
	view.WriteString(titleStyle.Render("Select a provider:"))
	view.WriteString("\n\n")
	view.WriteString(m.renderList(m.ProviderCursor, func(i int) string { return m.ProviderList[i].Name }, len(m.ProviderList)))
	view.WriteString("\n" + hintStyle.Render("[↑/↓ to navigate, Enter to select, Esc to cancel]"))
	return view.String()
}

func (m *Model) renderModelSelection() string {
	var view strings.Builder
	providerName := m.getProviderName(m.SelectedProvider)
	view.WriteString(titleStyle.Render(fmt.Sprintf("Select a model for %s:", providerName)))
	view.WriteString("\n\n")
	view.WriteString(m.renderList(m.ModelCursor, func(i int) string { return m.ModelList[i].Name }, len(m.ModelList)))
	view.WriteString("\n" + hintStyle.Render("[↑/↓ to navigate, Enter to select, Esc to cancel]"))
	return view.String()
}

func (m *Model) renderAPIKeyInput() string {
	var view strings.Builder
	providerName := m.getProviderName(m.SelectedProvider)
	existingKey := m.getExistingAPIKey(m.SelectedProvider)

	title := fmt.Sprintf("Enter API key for %s", providerName)
	if existingKey != "" {
		title += "\n" + hintStyle.Render("(press Enter to keep existing key)")
	}
	view.WriteString(titleStyle.Render(title))
	view.WriteString("\n\n")

	maskedKey := strings.Repeat("•", len(m.APIKeyInput))
	view.WriteString(promptStyle.Render("> ") + maskedKey)
	view.WriteString("\n\n" + hintStyle.Render("[Enter to confirm, Esc to cancel]"))

	if m.ErrorMessage != "" {
		view.WriteString("\n" + errorStyle.Render(m.ErrorMessage))
	}
	return view.String()
}

func (m *Model) renderList(cursor int, getName func(int) string, count int) string {
	var view strings.Builder
	for i := 0; i < count; i++ {
		cursorStr := "  "
		style := normalStyle
		if i == cursor {
			cursorStr = "> "
			style = selectionStyle
		}
		view.WriteString(cursorStr + style.Render(getName(i)) + "\n")
	}
	return view.String()
}

func (m *Model) getProviderName(providerID string) string {
	if provider, ok := m.registry.GetProvider(providerID); ok {
		return provider.Name
	}
	return ""
}

func (m *Model) getExistingAPIKey(providerID string) string {
	if providerCfg, exists := m.globalCfg.GetProviderConfig(providerID); exists {
		return providerCfg.APIKey
	}
	return ""
}

func IsComplete(msg tea.Msg) bool {
	_, ok := msg.(keyEnter)
	return ok
}

func IsCancel(msg tea.Msg) bool {
	_, ok := msg.(keyCancel)
	return ok
}
