package repl

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	replcommands "github.com/user/keen-code/internal/cli/repl/commands"
	repltheme "github.com/user/keen-code/internal/cli/repl/theme"
)

const maxQueuedInputs = 5

func (m *replModel) isQueueable(input string) bool {
	if !strings.HasPrefix(input, "/") {
		return true
	}
	fields := strings.Fields(strings.TrimPrefix(input, "/"))
	if len(fields) == 0 {
		return false
	}
	_, ok := m.appState.FindEnabledSkill(fields[0])
	return ok
}

func (m *replModel) drainQueuedInput() (replModel, tea.Cmd) {
	if len(m.queuedInputs) == 0 {
		return *m, nil
	}
	input := m.queuedInputs[0]
	m.queuedInputs = m.queuedInputs[1:]
	if input == replcommands.Adversary || strings.HasPrefix(input, replcommands.Adversary+" ") {
		m.history.Push(input)
		return m.handleAdversaryCommand(input)
	}
	return m.submitInput(input, true)
}

func (m *replModel) queuedHeight() int {
	if len(m.queuedInputs) == 0 {
		return 0
	}
	return 1 + len(m.queuedInputs)
}

func (m replModel) renderQueuedInputs() string {
	if len(m.queuedInputs) == 0 {
		return ""
	}
	chip := repltheme.QueueChipStyle.Render("queue")
	chipWidth := lipgloss.Width(chip)
	maxWidth := m.width - chipWidth - 3
	if maxWidth < 10 {
		maxWidth = 10
	}
	var b strings.Builder
	b.WriteString("\n")
	for _, qi := range m.queuedInputs {
		display := qi
		if idx := strings.Index(qi, "\n"); idx >= 0 {
			display = qi[:idx]
		}
		if lipgloss.Width(display) > maxWidth {
			display = ansi.Truncate(display, maxWidth-1, "…")
		}
		b.WriteString(" ")
		b.WriteString(chip)
		b.WriteString(" ")
		b.WriteString(repltheme.QueueItemStyle.Render(display))
		b.WriteString("\n")
	}
	return b.String()
}
