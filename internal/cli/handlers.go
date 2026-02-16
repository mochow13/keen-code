package cli

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/keen-cli/internal/cli/modelselection"
	"github.com/user/keen-cli/internal/llm"
)

const (
	keyEnter = "enter"
	keyCtrlJ = "ctrl+j"
	keyCtrlC = "ctrl+c"
)

func (m replModel) handleLLMChunk(chunk string) (replModel, tea.Cmd) {
	m.showSpinner = false
	return m, m.streamHandler.HandleChunk(chunk)
}

func (m replModel) handleLLMDone() (replModel, tea.Cmd) {
	m.showSpinner = false
	responseLines, fullResponse := m.streamHandler.HandleDone()
	m.state.messages = append(m.state.messages, llm.Message{
		Role:    llm.RoleAssistant,
		Content: fullResponse,
	})
	for _, line := range responseLines {
		m.outputLines = append(m.outputLines, m.contentStyle().Render(assistantStyle.Render(line)))
	}
	m.outputLines = append(m.outputLines, "")
	return m, nil
}

func (m replModel) handleLLMError(err error) (replModel, tea.Cmd) {
	m.showSpinner = false
	errMsg := m.streamHandler.HandleError(err)
	m.outputLines = append(m.outputLines, m.contentStyle().Render(errorStyle.Render("  Error: "+errMsg)))
	m.outputLines = append(m.outputLines, "")
	return m, nil
}

func (m replModel) handleKeyMsg(msg tea.Msg) (replModel, tea.Cmd) {
	if m.modelSelection != nil {
		newModel, cmd := m.modelSelection.Update(msg)
		m.modelSelection = newModel.(*modelselection.Model)

		if modelselection.IsComplete(msg) {
			successMsg := "✓ Updated to " + m.modelSelection.SelectedProvider + " / " + m.modelSelection.SelectedModel
			m.outputLines = append(m.outputLines, highlightStyle.Render("  "+successMsg))
			m.outputLines = append(m.outputLines, "")
			m.modelSelection = nil
			return m, nil
		}

		if modelselection.IsCancel(msg) {
			cancelStyle := lipgloss.NewStyle().Foreground(mutedColor)
			m.outputLines = append(m.outputLines, cancelStyle.Render("  Model selection cancelled"))
			m.outputLines = append(m.outputLines, "")
			m.modelSelection = nil
			return m, nil
		}

		return m, cmd
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case keyEnter:
		return m.handleEnterKey()
	case keyCtrlJ:
		return m.handleCtrlJ()
	case keyCtrlC:
		m.quitting = true
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(keyMsg)
	m.adjustTextareaHeight()
	return m, cmd
}
