package repl

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
)

const taskHintNewTaskThreshold = 0.7

var taskHintChoices = []string{"Create a new session", "Compact current context", "Skip this time", "Skip this session"}

type taskHintState struct {
	display         string
	input           string
	active          bool
	cursor          int
	skipSession     bool
	afterCompaction string
}

func (m *replModel) taskHintEligible() bool {
	return !m.taskHint.skipSession && len(m.agentCore.GetMessages()) > 0
}

func (m *replModel) submitOrShowTaskHint(input string, classification routerClassification) (replModel, tea.Cmd) {
	if classification.err != nil || classification.result.NewTask < taskHintNewTaskThreshold || !m.taskHintEligible() {
		return m.submitPrompt(input, true, true)
	}
	m.stopLoading()
	m.taskHint.input, m.taskHint.active, m.taskHint.cursor = input, true, 0
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m, nil
}

func (m *replModel) handleTaskHintKeyMsg(msg tea.Msg) (replModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return *m, nil
	}
	choice := -1
	switch keyMsg.String() {
	case keyUp, keyShiftUp:
		m.taskHint.cursor = max(0, m.taskHint.cursor-1)
	case keyDown, keyShiftDown:
		m.taskHint.cursor = min(len(taskHintChoices)-1, m.taskHint.cursor+1)
	case keyEnter:
		choice = m.taskHint.cursor
	case keyEsc, keyCtrlC:
		choice = 2
	}
	if choice < 0 {
		m.updateViewportContent()
		return *m, nil
	}
	input, display := m.taskHint.input, m.taskHint.display
	m.taskHint.active, m.taskHint.input = false, ""
	switch choice {
	case 0:
		m.handleClearCommand()
		m.output.AddUserInput(display, repltheme.PromptStyle)
	case 1:
		m.taskHint.afterCompaction = input
		return m.startCompaction("")
	case 3:
		m.taskHint.skipSession = true
		m.output.AddStyledLine("  New-task hints disabled for this session.", repltheme.FaintedStyle)
		m.output.AddEmptyLine()
	}
	return m.submitPrompt(input, true, false)
}

func (m *replModel) takeTaskHintCompactionInput() string {
	input := m.taskHint.afterCompaction
	m.taskHint.afterCompaction = ""
	return input
}

func formatTaskHintCard(cursor, width int) string {
	ruleWidth := max(1, width-2)
	if width <= 0 {
		ruleWidth = 78
	}
	rule := "  " + repltheme.ModelSelectionRuleStyle.Render(strings.Repeat("─", ruleWidth))
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s\n", rule)
	fmt.Fprintf(&b, "  %s\n\n", repltheme.PrimaryBoldStyle.Render("Moving to a new task? You can save some cost."))
	for i, choice := range taskHintChoices {
		if i == cursor {
			fmt.Fprintf(&b, "  %s%s\n", repltheme.ModelSelectionCursorStyle.Render("▶ "), repltheme.ModelSelectionSelectedTextStyle.Render(choice))
		} else {
			fmt.Fprintf(&b, "    %s\n", repltheme.ModelSelectionTextStyle.Render(choice))
		}
	}
	fmt.Fprintf(&b, "\n  %s\n", repltheme.ModelSelectionTextStyle.Render("[↑/↓ navigate  Enter to select  Esc to skip]"))
	fmt.Fprintf(&b, "%s\n", rule)
	return b.String()
}
