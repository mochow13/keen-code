package widgets

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/providers"
)

func NewRouterSelection(registry *providers.Registry, global *config.GlobalConfig, category string, existing config.RouterModel, onComplete func(config.RouterModel) error) *Model {
	m := New(registry, global, nil, &config.ResolvedConfig{ThinkingEffort: existing.ThinkingEffort}, nil)
	m.routerCategory, m.routerExisting, m.routerComplete = category, existing, onComplete
	m.ProviderList = nil
	for _, provider := range registry.Providers {
		cfg, _ := global.GetProviderConfig(provider.ID)
		authenticated := strings.TrimSpace(cfg.APIKey) != ""
		if config.AuthModeForProvider(provider.ID) == config.AuthModeOAuth {
			authenticated = m.authManager.HasCredential(provider.ID)
		}
		if authenticated && len(provider.Models) > 0 {
			if provider.ID == existing.Provider {
				m.ProviderCursor = len(m.ProviderList)
			}
			m.ProviderList = append(m.ProviderList, provider)
		}
	}
	return m
}

func (m *Model) completeRouterSelection() (*Model, tea.Cmd) {
	effort := m.SelectedThinking
	meta, ok := m.registry.GetModel(m.SelectedProvider, m.SelectedModel)
	if !ok || !meta.SupportsThinkingEffort() {
		effort = ""
	}
	if err := m.routerComplete(config.RouterModel{Provider: m.SelectedProvider, Model: m.SelectedModel, ThinkingEffort: effort}); err != nil {
		m.ErrorMessage = err.Error()
		return m, nil
	}
	return m, func() tea.Msg { return modelSelectionCompleteMsg{} }
}

func (m *Model) routerSelectionView() string {
	var list string
	switch m.Step {
	case StepProvider:
		list = renderList(m.ProviderCursor, func(i int) string { return m.ProviderList[i].Name }, len(m.ProviderList))
	case StepModel:
		list = renderList(m.ModelCursor, func(i int) string { return m.ModelList[i].Name }, len(m.ModelList))
	case StepThinking:
		list = renderList(m.ThinkingCursor, func(i int) string { return m.ThinkingOptions[i] }, len(m.ThinkingOptions))
	}
	view := m.routerSelectionPrompt() + "\n\n" + list
	if len(m.ProviderList) == 0 {
		view += repltheme.ErrorStyle.Render("No authenticated providers.") + "\n" +
			repltheme.HintStyle.Render("Use /model to authenticate a provider first.") + "\n"
	}
	if m.ErrorMessage != "" {
		view += "\n" + repltheme.ErrorStyle.Render(m.ErrorMessage) + "\n"
	}
	return view + "\n" + repltheme.HintStyle.Render("↑/↓ navigate  ·  Enter select  ·  Esc cancel")
}

func (m *Model) routerSelectionPrompt() string {
	var number int
	var description, recommendation string
	headerStyle := repltheme.PrimaryBoldStyle
	switch m.routerCategory {
	case config.RouterCategorySimple:
		number = 1
		headerStyle = headerStyle.Foreground(repltheme.SecondaryColor)
		description = "Quick questions, small edits, and straightforward tasks."
		recommendation = "Choose a fast, cost-effective model for routine work."
	case config.RouterCategoryStandard:
		number = 2
		description = "Everyday coding, debugging, and multi-step tasks."
		recommendation = "Choose a balanced model. This also handles prompts when classification fails."
	case config.RouterCategoryComplex:
		number = 3
		headerStyle = headerStyle.Foreground(repltheme.AccentColor)
		description = "Challenging debugging, architectural changes, and deep reasoning tasks."
		recommendation = "Choose your most capable model for demanding work."
	}
	header := headerStyle.Render(fmt.Sprintf("Router setup (%d/3): %s tasks", number, m.routerCategory))
	context := ""
	if m.SelectedProvider != "" {
		context = "\n\n" + repltheme.HighlightStyle.Render(m.getProviderName(m.SelectedProvider))
		if m.SelectedModel != "" {
			context += repltheme.HintStyle.Render(" / ") + repltheme.HighlightStyle.Render(m.SelectedModel)
		}
	}
	return header + "\n\n" + repltheme.AssistantStyle.Render(description) +
		"\n" + repltheme.HintStyle.Render(recommendation) + context
}
