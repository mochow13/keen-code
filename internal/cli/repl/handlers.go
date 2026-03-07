package repl

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/user/keen-cli/internal/cli/modelselection"
	"github.com/user/keen-cli/internal/llm"
)

const (
	keyEnter     = "enter"
	keyCtrlC     = "ctrl+c"
	keyCtrlD     = "ctrl+d"
	keyEsc       = "esc"
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
	cmd := m.streamHandler.HandleChunk(chunk)
	m.updateViewportContent()
	if !m.userScrolled {
		m.viewport.GotoBottom()
	}
	return *m, cmd
}

func (m *replModel) handleLLMDone() (replModel, tea.Cmd) {
	m.showSpinner = false
	m.clearStreamCancel()
	responseLines, fullResponse := m.streamHandler.HandleDone()
	m.appState.AddMessage(llm.RoleAssistant, fullResponse)
	for _, line := range responseLines {
		m.output.AddLine(line)
	}
	m.output.AddEmptyLine()
	m.updateViewportContent()
	if !m.userScrolled {
		m.viewport.GotoBottom()
	}
	return *m, nil
}

func (m *replModel) handleLLMError(err error) (replModel, tea.Cmd) {
	m.showSpinner = false
	m.clearStreamCancel()
	pendingLines, errMsg := m.streamHandler.HandleError(err)
	for _, line := range pendingLines {
		m.output.AddLine(line)
	}
	if errors.Is(err, context.Canceled) {
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}
	m.output.AddError(errMsg, errorStyle)
	m.updateViewportContent()
	m.viewport.GotoBottom()
	return *m, nil
}

func (m *replModel) handleToolStart(toolCall *llm.ToolCall) (replModel, tea.Cmd) {
	m.showSpinner = false
	var cmd tea.Cmd
	if toolCall.Name == "bash" {
		command, _ := toolCall.Input["command"].(string)
		summary, _ := toolCall.Input["summary"].(string)
		cmd = m.streamHandler.HandleBashStart(command, summary)
	} else {
		cmd = m.streamHandler.HandleToolStart(toolCall)
	}
	m.updateViewportContent()
	if !m.userScrolled {
		m.viewport.GotoBottom()
	}
	return *m, cmd
}

func (m *replModel) handleToolEnd(toolCall *llm.ToolCall) (replModel, tea.Cmd) {
	m.showSpinner = false
	var cmd tea.Cmd
	if toolCall.Name == "bash" {
		cmd = m.streamHandler.HandleBashEnd(toolCall)
	} else {
		cmd = m.streamHandler.HandleToolEnd(toolCall)
	}
	m.updateViewportContent()
	if !m.userScrolled {
		m.viewport.GotoBottom()
	}
	return *m, cmd
}

func (m *replModel) handleKeyMsg(msg tea.Msg) (replModel, tea.Cmd) {
	if m.modelSelection != nil {
		newModel, cmd := m.modelSelection.Update(msg)
		if ms, ok := newModel.(*modelselection.Model); ok {
			m.modelSelection = ms
		}

		if modelselection.IsComplete(msg) {
			successMsg := "✓ Updated to " + m.modelSelection.SelectedProvider + " / " + m.modelSelection.SelectedModel
			m.output.AddStyledLine("  "+successMsg, highlightStyle)
			m.output.AddEmptyLine()
			m.modelSelection = nil
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m, nil
		}

		if modelselection.IsCancel(msg) {
			cancelStyle := lipgloss.NewStyle().Foreground(mutedColor)
			m.output.AddStyledLine("  Model selection cancelled", cancelStyle)
			m.output.AddEmptyLine()
			m.modelSelection = nil
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m, nil
		}

		return *m, cmd
	}

	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return *m, nil
	}

	switch keyMsg.String() {
	case keyEnter:
		return m.handleEnterKey()
	case keyCtrlC, keyCtrlD:
		if m.textarea.Value() != "" {
			m.textarea.Reset()
			m.adjustTextareaHeight()
			return *m, nil
		}
		m.quitting = true
		return *m, tea.Quit
	case keyEsc:
		if m.streamHandler != nil && m.streamHandler.IsActive() {
			m.interruptStream("Interrupted...what should the agent do instead?")
		}
		return *m, nil
	case keyUp, keyShiftUp:
		if m.isAtTopOfInput() {
			m.viewport.ScrollUp(1)
			m.userScrolled = !m.viewport.AtBottom()
			return *m, nil
		}
	case keyDown, keyShiftDown:
		if m.isAtBottomOfInput() {
			m.viewport.ScrollDown(1)
			m.userScrolled = !m.viewport.AtBottom()
			return *m, nil
		}
	case keyPageUp:
		m.viewport.HalfPageUp()
		m.userScrolled = !m.viewport.AtBottom()
		return *m, nil
	case keyPageDown:
		m.viewport.HalfPageDown()
		m.userScrolled = !m.viewport.AtBottom()
		return *m, nil
	case keyHome:
		m.viewport.GotoTop()
		m.userScrolled = true
		return *m, nil
	case keyEnd:
		m.viewport.GotoBottom()
		m.userScrolled = false
		return *m, nil
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(keyMsg)
	m.adjustTextareaHeight()
	return *m, cmd
}

func (m *replModel) interruptStream(message string) {
	if m.streamCancel != nil {
		m.streamCancel()
		m.clearStreamCancel()
	}

	m.showSpinner = false
	for _, line := range m.streamHandler.HandleInterrupt() {
		m.output.AddLine(line)
	}
	m.output.AddStyledLine("\n  "+message, interruptedStyle)
	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.viewport.GotoBottom()
}

func (m *replModel) handlePermissionKeyMsg(msg tea.Msg) (replModel, tea.Cmd) {
	newModel, cmd := m.permissionSelector.Update(msg)
	if ps, ok := newModel.(*PermissionSelector); ok {
		m.permissionSelector = ps
	}

	if IsPermissionComplete(msg) {
		choice := m.permissionSelector.GetChoice()
		toolName := m.permissionSelector.toolName
		m.permissionRequester.SendResponse(choice, toolName)

		switch choice {
		case PermissionChoiceAllow:
			successMsg := "✓ Permission granted for " + toolName
			m.output.AddStyledLine("  "+successMsg, highlightStyle)
		case PermissionChoiceAllowSession:
			successMsg := "✓ Permission granted for " + toolName + " (this session)"
			m.output.AddStyledLine("  "+successMsg, highlightStyle)
		case PermissionChoiceDeny:
			cancelStyle := lipgloss.NewStyle().Foreground(mutedColor)
			m.output.AddStyledLine("  Permission denied for "+toolName, cancelStyle)
		}
		m.output.AddEmptyLine()
		m.permissionSelector = nil
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	if IsPermissionCancel(msg) {
		toolName := m.permissionSelector.toolName
		m.permissionRequester.SendResponse(PermissionChoiceDeny, toolName)
		cancelStyle := lipgloss.NewStyle().Foreground(mutedColor)
		m.output.AddStyledLine("  Permission denied for "+toolName, cancelStyle)
		m.output.AddEmptyLine()
		m.permissionSelector = nil
		m.updateViewportContent()
		m.viewport.GotoBottom()
		return *m, nil
	}

	return *m, cmd
}
