package repl

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	replcommands "github.com/mochow13/keen-code/internal/cli/repl/commands"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	replwidgets "github.com/mochow13/keen-code/internal/cli/repl/widgets"
)

const themeUsage = "Usage: /theme | /theme <name>"

func (m *replModel) startThemeSelection() replModel {
	m.themeSelection = replwidgets.NewThemeSelection(repltheme.CurrentThemeName)
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func (m *replModel) applyTheme(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	t, ok := repltheme.ThemeByName(name)
	if !ok {
		return false
	}
	repltheme.ApplyTheme(t.Name)
	m.syncSpinnerStyles()
	if m.ctx != nil && m.ctx.globalCfg != nil && m.ctx.loader != nil {
		m.ctx.globalCfg.Theme = t.Name
		if err := m.ctx.loader.Save(m.ctx.globalCfg); err != nil {
			m.output.AddError("Failed to save theme: "+err.Error(), repltheme.ErrorStyle)
			m.updateViewportContent()
			m.viewport.GotoBottom()
		}
	}
	return true
}

func (m *replModel) syncSpinnerStyles() {
	m.loading.spinner.Style = lipgloss.NewStyle().Foreground(repltheme.PrimaryColor)
	m.btw.spinner.Style = lipgloss.NewStyle().Foreground(repltheme.AccentColor)
	m.adversary.spinner.Style = lipgloss.NewStyle().Foreground(repltheme.SecondaryColor)
}

func (m *replModel) addThemeSelectedLine(t repltheme.Theme) {
	m.output.AddStyledLine("  Selected theme "+t.Label, repltheme.HighlightStyle)
	m.output.AddEmptyLine()
}

func (m *replModel) handleThemeCommand(input string) replModel {
	arg := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(input, replcommands.Theme)))
	if arg == "" {
		return m.startThemeSelection()
	}
	if m.applyTheme(arg) {
		t, _ := repltheme.ThemeByName(arg)
		m.addThemeSelectedLine(t)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m
	}
	m.output.AddError(themeUsage+" (available: "+strings.Join(repltheme.ThemeNames(), "|")+")", repltheme.ErrorStyle)
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m
}

func (m replModel) consumeThemeSelectionResult(msg tea.Msg) (replModel, tea.Cmd, bool) {
	if m.themeSelection == nil {
		return m, nil, false
	}
	switch msg := msg.(type) {
	case replwidgets.ThemeSelectionCompleteMsg:
		name := strings.ToLower(strings.TrimSpace(msg.Name))
		m.themeSelection = nil
		if !m.applyTheme(name) {
			m.output.AddError("Unknown theme: "+msg.Name, repltheme.ErrorStyle)
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return m, nil, true
		}
		t, _ := repltheme.ThemeByName(name)
		m.addThemeSelectedLine(t)
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return m, nil, true
	case replwidgets.ThemeSelectionCancelMsg:
		m.themeSelection = nil
		m.output.AddStyledLine("  Theme selection cancelled", repltheme.MutedStyle)
		m.output.AddEmptyLine()
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return m, nil, true
	}
	return m, nil, false
}
