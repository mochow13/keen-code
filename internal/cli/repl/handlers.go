package repl

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/user/keen-code/internal/cli/modelselection"
	"github.com/user/keen-code/internal/llm"
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
	m.showSpinner = true
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

	if m.streamHandler != nil && m.streamHandler.HasPendingPermission() {
		switch keyMsg.String() {
		case "up", "k", "down", "j", keyEnter, keyEsc:
			return m.handlePermissionKeyMsg(keyMsg)
		}
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

func (m *replModel) handlePermissionKeyMsg(msg tea.KeyPressMsg) (replModel, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.streamHandler.MovePendingCursor(-1)
		m.updateViewportContent()
		if !m.userScrolled {
			m.viewport.GotoBottom()
		}
	case "down", "j":
		m.streamHandler.MovePendingCursor(1)
		m.updateViewportContent()
		if !m.userScrolled {
			m.viewport.GotoBottom()
		}
	case keyEnter:
		req := m.streamHandler.GetPendingPermissionRequest()
		if req == nil {
			return *m, nil
		}
		choice := m.streamHandler.GetPendingChoice()
		var status PermissionStatus
		switch choice {
		case PermissionChoiceAllow:
			status = PermissionStatusAllowed
		case PermissionChoiceAllowSession:
			status = PermissionStatusAllowedSession
		case PermissionChoiceDeny:
			status = PermissionStatusDenied
		}
		m.streamHandler.ResolvePendingPermission(status)
		m.permissionRequester.SendResponse(choice, req.ToolName)
		m.updateViewportContent()
		if !m.userScrolled {
			m.viewport.GotoBottom()
		}
	case keyEsc:
		req := m.streamHandler.GetPendingPermissionRequest()
		if req == nil {
			return *m, nil
		}
		m.streamHandler.ResolvePendingPermission(PermissionStatusDenied)
		m.permissionRequester.SendResponse(PermissionChoiceDeny, req.ToolName)
		m.updateViewportContent()
		if !m.userScrolled {
			m.viewport.GotoBottom()
		}
	}
	return *m, nil
}

func (m replModel) handleLLMStreamMsg(msg tea.Msg) (replModel, tea.Cmd, bool) {
	if m.streamHandler == nil || !m.streamHandler.IsActive() {
		switch msg.(type) {
		case llmChunkMsg, llmDoneMsg, llmErrorMsg, llmToolStartMsg, llmToolEndMsg:
			return m, nil, true
		}
	}

	switch msg := msg.(type) {
	case llmChunkMsg:
		updated, cmd := m.handleLLMChunk(string(msg))
		return updated, cmd, true
	case llmDoneMsg:
		updated, cmd := m.handleLLMDone()
		return updated, cmd, true
	case llmErrorMsg:
		updated, cmd := m.handleLLMError(msg.err)
		return updated, cmd, true
	case llmToolStartMsg:
		updated, cmd := m.handleToolStart(msg.toolCall)
		return updated, cmd, true
	case llmToolEndMsg:
		updated, cmd := m.handleToolEnd(msg.toolCall)
		if updated.showSpinner {
			return updated, tea.Batch(cmd, updated.spinner.Tick), true
		}
		return updated, cmd, true
	default:
		return m, nil, false
	}
}
