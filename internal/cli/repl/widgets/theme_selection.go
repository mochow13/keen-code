package widgets

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
)

type ThemeSelectionCompleteMsg struct {
	Name string
}

type ThemeSelectionCancelMsg struct{}

type ThemeSelection struct {
	Options []repltheme.Theme
	Cursor  int
	Active  string
}

func NewThemeSelection(current string) *ThemeSelection {
	opts := append([]repltheme.Theme(nil), repltheme.Themes...)
	cursor := 0
	for i, opt := range opts {
		if opt.Name == current {
			cursor = i
			break
		}
	}
	return &ThemeSelection{Options: opts, Cursor: cursor, Active: current}
}

func (m *ThemeSelection) Update(msg tea.Msg) (*ThemeSelection, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "up", "k":
		if len(m.Options) > 0 {
			m.Cursor = (m.Cursor - 1 + len(m.Options)) % len(m.Options)
		}
		return m, nil
	case "down", "j":
		if len(m.Options) > 0 {
			m.Cursor = (m.Cursor + 1) % len(m.Options)
		}
		return m, nil
	case "enter":
		if len(m.Options) == 0 {
			return m, func() tea.Msg { return ThemeSelectionCancelMsg{} }
		}
		name := m.Options[m.Cursor].Name
		return m, func() tea.Msg { return ThemeSelectionCompleteMsg{Name: name} }
	case "esc":
		return m, func() tea.Msg { return ThemeSelectionCancelMsg{} }
	}
	return m, nil
}

func (m *ThemeSelection) SelectedName() string {
	if len(m.Options) == 0 {
		return ""
	}
	return m.Options[m.Cursor].Name
}

func (m *ThemeSelection) View(width int) string {
	ruleWidth := width
	if ruleWidth < 1 {
		ruleWidth = 1
	}
	rule := repltheme.ModelSelectionRuleStyle.Render(strings.Repeat("─", ruleWidth))
	var body strings.Builder
	body.WriteString(repltheme.ModelSelectionTitleStyle.Render("Select theme:"))
	body.WriteString("\n")
	body.WriteString(repltheme.ModelSelectionTextStyle.Render("[↑/↓ to navigate, Enter to select, Esc to cancel]"))
	body.WriteString("\n\n")
	maxVisible := 11
	start := 0
	if len(m.Options) > maxVisible {
		start = m.Cursor - maxVisible/2
		if start < 0 {
			start = 0
		}
		if start+maxVisible > len(m.Options) {
			start = len(m.Options) - maxVisible
		}
	}
	if start > 0 {
		body.WriteString(repltheme.ModelSelectionTextStyle.Render("  ↑"))
		body.WriteString("\n")
	}
	end := len(m.Options)
	if end-start > maxVisible {
		end = start + maxVisible
	}
	for i := start; i < end; i++ {
		opt := m.Options[i]
		selected := i == m.Cursor
		prefix := "  "
		label := repltheme.ModelSelectionTextStyle.Render(opt.Label)
		if selected {
			prefix = repltheme.ModelSelectionCursorStyle.Render("▶ ")
			label = repltheme.ModelSelectionSelectedTextStyle.Render(opt.Label)
		}
		swatches := lipgloss.NewStyle().Foreground(opt.Primary).Render("██") +
			lipgloss.NewStyle().Foreground(opt.PrimaryLight).Render("██") +
			lipgloss.NewStyle().Foreground(opt.Secondary).Render("██")
		line := prefix + swatches + " " + label
		if opt.Name == m.Active {
			line += lipgloss.NewStyle().Foreground(opt.Secondary).Render(" (active)")
		}
		body.WriteString(line)
		body.WriteString("\n")
		prompt := lipgloss.NewStyle().Foreground(opt.Primary).Bold(true).Render("▶ ")
		title := lipgloss.NewStyle().Foreground(opt.Secondary).Bold(true).Render("Title ")
		code := lipgloss.NewStyle().Foreground(opt.PrimaryLight).Bold(true).Render("code ")
		faintRule := lipgloss.NewStyle().Foreground(opt.Primary).Faint(true).Render("────")
		previewIndent := "  "
		if selected {
			previewIndent = repltheme.ModelSelectionCursorStyle.Render("  ")
		}
		body.WriteString(previewIndent)
		body.WriteString("  ")
		body.WriteString(prompt)
		body.WriteString(title)
		body.WriteString(code)
		body.WriteString(faintRule)
		body.WriteString("\n")
	}
	if end < len(m.Options) {
		body.WriteString(repltheme.ModelSelectionTextStyle.Render("  ↓"))
		body.WriteString("\n")
	}
	lines := strings.Split(strings.TrimRight(body.String(), "\n"), "\n")
	var out strings.Builder
	out.WriteString("\n")
	out.WriteString(rule)
	out.WriteString("\n")
	for _, line := range lines {
		if line == "" {
			out.WriteString("\n")
			continue
		}
		out.WriteString("  ")
		out.WriteString(line)
		out.WriteString("\n")
	}
	out.WriteString(rule)
	out.WriteString("\n")
	return out.String()
}
