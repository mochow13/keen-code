package cli

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/keen-cli/configs/providers"
	"github.com/user/keen-cli/internal/config"
)

type selectionStep int

const (
	stepProvider selectionStep = iota
	stepModel
	stepAPIKey
)

type modelSelectionState struct {
	step             selectionStep
	selectedProvider string
	selectedModel    string
	apiKeyInput      string
	providerCursor   int
	modelCursor      int
	providerList     []providers.Provider
	modelList        []providers.Model
	errorMessage     string
}

func (m replModel) startModelSelection() replModel {
	m.modelSelection = &modelSelectionState{
		step:           stepProvider,
		providerCursor: 0,
		providerList:   m.state.registry.Providers,
	}
	return m
}

func (m replModel) handleModelSelectionUpdate(msg tea.KeyMsg) (replModel, tea.Cmd) {
	if m.modelSelection == nil {
		return m, nil
	}

	switch m.modelSelection.step {
	case stepProvider:
		switch msg.String() {
		case "up", "k":
			if m.modelSelection.providerCursor > 0 {
				m.modelSelection.providerCursor--
			}
		case "down", "j":
			if m.modelSelection.providerCursor < len(m.modelSelection.providerList)-1 {
				m.modelSelection.providerCursor++
			}
		case "enter":
			m.modelSelection.selectedProvider = m.modelSelection.providerList[m.modelSelection.providerCursor].ID
			provider, _ := m.state.registry.GetProvider(m.modelSelection.selectedProvider)
			m.modelSelection.modelList = provider.Models
			m.modelSelection.modelCursor = 0
			m.modelSelection.step = stepModel
		case "esc":
			return m.cancelModelSelection(), nil
		}

	case stepModel:
		switch msg.String() {
		case "up", "k":
			if m.modelSelection.modelCursor > 0 {
				m.modelSelection.modelCursor--
			}
		case "down", "j":
			if m.modelSelection.modelCursor < len(m.modelSelection.modelList)-1 {
				m.modelSelection.modelCursor++
			}
		case "enter":
			m.modelSelection.selectedModel = m.modelSelection.modelList[m.modelSelection.modelCursor].ID
			m.modelSelection.step = stepAPIKey
		case "esc":
			return m.cancelModelSelection(), nil
		}

	case stepAPIKey:
		switch msg.String() {
		case "esc":
			return m.cancelModelSelection(), nil
		case "enter":
			return m.completeModelSelection()
		case "backspace":
			if len(m.modelSelection.apiKeyInput) > 0 {
				m.modelSelection.apiKeyInput = m.modelSelection.apiKeyInput[:len(m.modelSelection.apiKeyInput)-1]
			}
		default:
			if len(msg.String()) == 1 {
				m.modelSelection.apiKeyInput += msg.String()
			}
		}
	}

	return m, nil
}

func (m replModel) renderModelSelection() string {
	if m.modelSelection == nil {
		return ""
	}

	var view strings.Builder

	selectionStyle := lipgloss.NewStyle().Foreground(primaryColor).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
		Light: "#374151",
		Dark:  "#9CA3AF",
	})
	hintStyle := lipgloss.NewStyle().Foreground(mutedColor).Italic(true)

	switch m.modelSelection.step {
	case stepProvider:
		view.WriteString(titleStyle.Render("Select a provider:"))
		view.WriteString("\n\n")
		for i, provider := range m.modelSelection.providerList {
			cursor := "  "
			style := normalStyle
			if i == m.modelSelection.providerCursor {
				cursor = "> "
				style = selectionStyle
			}
			view.WriteString(cursor + style.Render(provider.Name) + "\n")
		}
		view.WriteString("\n" + hintStyle.Render("[↑/↓ to navigate, Enter to select, Esc to cancel]"))

	case stepModel:
		providerName := ""
		if provider, ok := m.state.registry.GetProvider(m.modelSelection.selectedProvider); ok {
			providerName = provider.Name
		}
		view.WriteString(titleStyle.Render(fmt.Sprintf("Select a model for %s:", providerName)))
		view.WriteString("\n\n")
		for i, model := range m.modelSelection.modelList {
			cursor := "  "
			style := normalStyle
			if i == m.modelSelection.modelCursor {
				cursor = "> "
				style = selectionStyle
			}
			view.WriteString(cursor + style.Render(model.Name) + "\n")
		}
		view.WriteString("\n" + hintStyle.Render("[↑/↓ to navigate, Enter to select, Esc to cancel]"))

	case stepAPIKey:
		providerName := ""
		if provider, ok := m.state.registry.GetProvider(m.modelSelection.selectedProvider); ok {
			providerName = provider.Name
		}

		existingKey := ""
		if providerCfg, exists := m.state.globalCfg.GetProviderConfig(m.modelSelection.selectedProvider); exists {
			existingKey = providerCfg.APIKey
		}

		title := fmt.Sprintf("Enter API key for %s", providerName)
		if existingKey != "" {
			title += "\n" + hintStyle.Render("(press Enter to keep existing key)")
		}
		view.WriteString(titleStyle.Render(title))
		view.WriteString("\n\n")

		maskedKey := strings.Repeat("•", len(m.modelSelection.apiKeyInput))
		view.WriteString(promptStyle.Render("> ") + maskedKey)
		view.WriteString("\n\n" + hintStyle.Render("[Enter to confirm, Esc to cancel]"))
	}

	return view.String()
}

func (m replModel) completeModelSelection() (replModel, tea.Cmd) {
	if m.modelSelection == nil {
		return m, nil
	}

	existingKey := ""
	if providerCfg, exists := m.state.globalCfg.GetProviderConfig(m.modelSelection.selectedProvider); exists {
		existingKey = providerCfg.APIKey
	}

	apiKey := m.modelSelection.apiKeyInput
	if apiKey == "" && existingKey != "" {
		apiKey = existingKey
	}

	if apiKey == "" {
		m.modelSelection.errorMessage = "API key is required"
		return m, nil
	}

	m.state.globalCfg.ActiveProvider = m.modelSelection.selectedProvider
	m.state.globalCfg.ActiveModel = m.modelSelection.selectedModel

	providerCfg := config.ProviderConfig{
		APIKey: apiKey,
		Models: []string{m.modelSelection.selectedModel},
	}
	m.state.globalCfg.SetProviderConfig(m.modelSelection.selectedProvider, providerCfg)

	if err := m.state.loader.Save(m.state.globalCfg); err != nil {
		m.outputLines = append(m.outputLines, outputStyle.Render(fmt.Sprintf("  ✗ Failed to save config: %v", err)))
		m.outputLines = append(m.outputLines, "")
		m.modelSelection = nil
		return m, nil
	}

	m.state.cfg.Provider = m.modelSelection.selectedProvider
	m.state.cfg.Model = m.modelSelection.selectedModel
	m.state.cfg.APIKey = apiKey

	successMsg := fmt.Sprintf("✓ Updated to %s / %s",
		m.modelSelection.selectedProvider,
		m.modelSelection.selectedModel)
	m.outputLines = append(m.outputLines, highlightStyle.Render("  "+successMsg))
	m.outputLines = append(m.outputLines, "")

	m.modelSelection = nil
	return m, nil
}

func (m replModel) cancelModelSelection() replModel {
	cancelStyle := lipgloss.NewStyle().Foreground(mutedColor)
	m.outputLines = append(m.outputLines, cancelStyle.Render("  Model selection cancelled"))
	m.outputLines = append(m.outputLines, "")
	m.modelSelection = nil
	return m
}
