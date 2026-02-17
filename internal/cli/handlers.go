package cli

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/keen-cli/internal/cli/modelselection"
	"github.com/user/keen-cli/internal/llm"
)

const (
	keyEnter     = "enter"
	keyCtrlJ     = "ctrl+j"
	keyCtrlC     = "ctrl+c"
	keyUp        = "up"
	keyDown      = "down"
	keyPageUp    = "pgup"
	keyPageDown  = "pgdown"
	keyHome      = "home"
	keyEnd       = "end"
	keyShiftUp   = "shift+up"
	keyShiftDown = "shift+down"
)

func (m *replModel) handleLLMChunk(chunk string) (replModel, tea.Cmd) {
	m.showSpinner = false
	if !m.userScrolled {
		m.scrollToBottom()
	}
	return *m, m.streamHandler.HandleChunk(chunk)
}

func (m *replModel) handleLLMDone() (replModel, tea.Cmd) {
	m.showSpinner = false
	responseLines, fullResponse := m.streamHandler.HandleDone()
	m.appState.AddMessage(llm.RoleAssistant, fullResponse)
	for _, line := range responseLines {
		m.output.AddLine(line)
	}
	m.output.AddEmptyLine()
	if !m.userScrolled {
		m.scrollToBottom()
	}
	return *m, nil
}

func (m *replModel) handleLLMError(err error) (replModel, tea.Cmd) {
	m.showSpinner = false
	errMsg := m.streamHandler.HandleError(err)
	m.output.AddError(errMsg, errorStyle)
	return *m, nil
}

func (m *replModel) handleKeyMsg(msg tea.Msg) (replModel, tea.Cmd) {
	if m.modelSelection != nil {
		newModel, cmd := m.modelSelection.Update(msg)
		m.modelSelection = newModel.(*modelselection.Model)

		if modelselection.IsComplete(msg) {
			successMsg := "✓ Updated to " + m.modelSelection.SelectedProvider + " / " + m.modelSelection.SelectedModel
			m.output.AddStyledLine("  "+successMsg, highlightStyle)
			m.output.AddEmptyLine()
			m.modelSelection = nil
			return *m, nil
		}

		if modelselection.IsCancel(msg) {
			cancelStyle := lipgloss.NewStyle().Foreground(mutedColor)
			m.output.AddStyledLine("  Model selection cancelled", cancelStyle)
			m.output.AddEmptyLine()
			m.modelSelection = nil
			return *m, nil
		}

		return *m, cmd
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return *m, nil
	}

	switch keyMsg.String() {
	case keyEnter:
		return m.handleEnterKey()
	case keyCtrlJ:
		return m.handleCtrlJ()
	case keyCtrlC:
		m.quitting = true
		return *m, tea.Quit
	case keyUp, keyShiftUp:
		if m.isAtTopOfInput() {
			m.scrollUp(1)
			return *m, nil
		}
	case keyDown, keyShiftDown:
		if m.isAtBottomOfInput() {
			m.scrollDown(1)
			return *m, nil
		}
	case keyPageUp:
		availableHeight := m.height - m.inputAreaHeight() - 1
		m.scrollUp(availableHeight / 2)
		return *m, nil
	case keyPageDown:
		availableHeight := m.height - m.inputAreaHeight() - 1
		m.scrollDown(availableHeight / 2)
		return *m, nil
	case keyHome:
		m.scrollOffset = 0
		return *m, nil
	case keyEnd:
		m.scrollToBottom()
		return *m, nil
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(keyMsg)
	m.adjustTextareaHeight()
	return *m, cmd
}
