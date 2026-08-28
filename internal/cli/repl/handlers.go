package repl

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mochow13/keen-code/internal/cli/repl/appstate"
	replcommands "github.com/mochow13/keen-code/internal/cli/repl/commands"
	reploutput "github.com/mochow13/keen-code/internal/cli/repl/output"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	replwidgets "github.com/mochow13/keen-code/internal/cli/repl/widgets"
	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/tools"
)

const (
	keyEnter     = "enter"
	keyCtrlC     = "ctrl+c"
	keyCtrlD     = "ctrl+d"
	keyEsc       = "esc"
	keyTab       = "tab"
	keyUp        = "up"
	keyDown      = "down"
	keyPageUp    = "pgup"
	keyPageDown  = "pgdown"
	keyHome      = "home"
	keyEnd       = "end"
	keyShiftUp   = "shift+up"
	keyShiftDown = "shift+down"
)

func (m *replModel) handleLLMUsage(usage *llm.TokenUsage) (replModel, tea.Cmd) {
	if m.appState != nil && usage != nil {
		m.appState.SetLastUsage(usage)
		m.contextStatus.AddUsage(usage)
		m.refreshContextStatus()
	}
	return *m, m.waitForAsyncEvent()
}

func (m *replModel) handleLLMChunk(chunk string) (replModel, tea.Cmd) {
	m.stream.handler.HandleChunk(chunk)
	return *m, tea.Batch(m.afterStreamUpdate(), m.waitForAsyncEvent())
}

func (m *replModel) handleLLMReasoningChunk(chunk string) (replModel, tea.Cmd) {
	m.stream.handler.HandleReasoningChunk(chunk)
	return *m, tea.Batch(m.afterStreamUpdate(), m.waitForAsyncEvent())
}

func (m *replModel) drainSubagentActivity() {
	for m.subagentActivity != nil {
		select {
		case activity := <-m.subagentActivity:
			m.stream.handler.HandleSubagentActivity(activity)
		default:
			return
		}
	}
}

func (m *replModel) handleLLMDone() (replModel, tea.Cmd) {
	m.drainSubagentActivity()
	m.flushStreamRender()
	if m.compaction.active && m.compaction.mode != compactionAutomatic {
		return m.handleCompactionDone()
	}
	segments := cloneStreamSegments(m.stream.handler.segments)
	m.recordHistoricalToolActivity(segments)
	m.stopLoading()
	m.clearStreamCancel()
	m.adjustTextareaHeight()
	m.appendResolvedAskUserSegment()
	responseLines, response := m.stream.handler.HandleDone()
	assistantMessage := llm.Message{
		Role:       llm.RoleAssistant,
		Content:    response,
		TurnMemory: m.consumeTurnMemory(),
	}
	m.appState.AppendMessage(assistantMessage)
	if err := m.sessions.appendAssistantTurn(segments, assistantMessage, false, ""); err != nil {
		m.handleSessionPersistenceError(err)
	}
	m.refreshContextStatus()
	for _, line := range responseLines {
		m.output.AddLine(line)
	}
	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
	return m.drainQueuedInput()
}

func (m *replModel) handleLLMIncomplete(err error) (replModel, tea.Cmd) {
	m.flushStreamRender()
	m.clearAskUser()
	segments := cloneStreamSegments(m.stream.handler.segments)
	m.recordHistoricalToolActivity(segments)
	partialResponse := m.stream.handler.GetResponse()
	m.stopLoading()
	m.clearStreamCancel()
	turnMemory := m.consumeTurnMemory()
	m.adjustTextareaHeight()
	pendingLines, errMsg := m.stream.handler.HandleError(err)
	assistantMessage := llm.Message{
		Role:       llm.RoleAssistant,
		Content:    partialResponse,
		TurnMemory: turnMemory,
	}
	if persistErr := m.sessions.appendAssistantTurn(segments, assistantMessage, false, errMsg); persistErr != nil {
		m.handleSessionPersistenceError(persistErr)
	}
	for _, line := range pendingLines {
		m.output.AddLine(line)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		m.output.AddError(errMsg, repltheme.ErrorStyle)
	}
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
	return m.drainQueuedInput()
}

func (m *replModel) handleLLMError(err error) (replModel, tea.Cmd) {
	m.flushStreamRender()
	m.clearAskUser()
	if m.compaction.active && m.compaction.mode != compactionAutomatic {
		return m.handleCompactionError(err)
	}
	segments := cloneStreamSegments(m.stream.handler.segments)
	m.recordHistoricalToolActivity(segments)
	partialResponse := m.stream.handler.GetResponse()
	m.stopLoading()
	m.clearStreamCancel()
	turnMemory := m.consumeTurnMemory()
	m.adjustTextareaHeight()
	pendingLines, errMsg := m.stream.handler.HandleError(err)
	assistantMessage := llm.Message{
		Role:       llm.RoleAssistant,
		Content:    partialResponse,
		TurnMemory: turnMemory,
	}
	if partialResponse != "" || (turnMemory != nil && !turnMemory.IsEmpty()) {
		m.appState.AppendMessage(assistantMessage)
		if persistErr := m.sessions.appendAssistantTurn(segments, assistantMessage, false, errMsg); persistErr != nil {
			m.handleSessionPersistenceError(persistErr)
		}
	}
	for _, line := range pendingLines {
		m.output.AddLine(line)
	}
	if errors.Is(err, context.Canceled) {
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return *m, nil
	}
	m.output.AddError(errMsg, repltheme.ErrorStyle)
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
	return *m, nil
}

func (m *replModel) handleLLMRetry(err error, attempt int) (replModel, tea.Cmd) {
	m.flushStreamRender()
	m.stream.handler.RewindForRetry()
	m.loading.text = fmt.Sprintf("Retrying (attempt %d)...", attempt)
	m.stream.handler.SetLoadingText(m.loading.text)
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
	return *m, m.waitForAsyncEvent()
}

func (m *replModel) restoreAutomaticCompactionLoader() {
	m.compaction.active = false
	m.compaction.mode = compactionNone
	m.compaction.cancel = nil
	m.loading.text = nextLoadingText()
	m.stream.handler.SetLoadingText(m.loading.text)
}

func (m *replModel) handleAutoCompactionStarted(event *llm.AutoCompactionEvent) (replModel, tea.Cmd) {
	m.compaction.active = true
	m.compaction.mode = compactionAutomatic
	if event != nil {
		m.compaction.cancel = event.Cancel
	}
	m.loading.text = "Compacting..."
	m.stream.handler.SetLoadingText(m.loading.text)
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
	return *m, m.waitForAsyncEvent()
}

func (m *replModel) handleAutoCompactionApplied(event *llm.AutoCompactionEvent) (replModel, tea.Cmd) {
	if event == nil || len(event.Replacement) == 0 {
		return m.handleAutoCompactionStopped()
	}

	m.flushStreamRender()
	segments := cloneStreamSegments(m.stream.handler.segments)
	m.recordHistoricalToolActivity(segments)

	var turnMemory *llm.TurnMemory
	if m.turnMemory != nil {
		turnMemory = m.turnMemory.Build()
	}
	checkpoint := llm.Message{
		Role:       llm.RoleAssistant,
		Content:    m.stream.handler.GetResponse(),
		TurnMemory: turnMemory,
	}

	persistedReplacement := appstate.WithoutSystemMessages(event.Replacement)
	if err := m.sessions.appendAutoCompaction(segments, checkpoint, persistedReplacement); err != nil {
		m.handleSessionPersistenceError(err)
		return m.handleAutoCompactionStopped()
	}

	m.clearTurnMemory()
	lines, _, _ := m.stream.handler.Checkpoint()
	m.appState.ReplaceMessages(persistedReplacement)
	m.startAssistantTurnMemory()
	m.appState.ClearContextMetrics()
	m.refreshContextStatus()
	m.restoreAutomaticCompactionLoader()
	for _, line := range lines {
		m.output.AddLine(line)
	}
	if len(lines) > 0 {
		m.output.AddEmptyLine()
	}
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
	return *m, tea.Batch(m.showNotification("Context compacted automatically."), m.waitForAsyncEvent())
}

func (m *replModel) handleAutoCompactionStopped() (replModel, tea.Cmd) {
	m.restoreAutomaticCompactionLoader()
	m.updateViewportContent()
	return *m, m.waitForAsyncEvent()
}

func (m *replModel) handleCompactionDone() (replModel, tea.Cmd) {
	m.flushStreamRender()
	segments := cloneStreamSegments(m.stream.handler.segments)
	responseLines, summary := m.stream.handler.HandleDone()
	m.compaction.active = false
	m.compaction.mode = compactionNone
	m.stopLoading()
	m.compaction.cancel = nil
	m.clearStreamCancel()
	if err := m.appState.ApplyCompaction(summary); err != nil {
		return m.handleCompactionError(err)
	}
	m.refreshContextStatus()
	for _, line := range responseLines {
		m.output.AddLine(line)
	}
	if len(responseLines) > 0 {
		m.output.AddEmptyLine()
	}
	if err := m.sessions.appendCompaction(segments, m.appState.GetMessages(), "Context compacted."); err != nil {
		m.handleSessionPersistenceError(err)
	}
	m.adjustTextareaHeight()
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
	return m.drainQueuedInput()
}

func (m *replModel) handleCompactionError(err error) (replModel, tea.Cmd) {
	m.flushStreamRender()
	if m.stream.handler != nil && m.stream.handler.IsActive() {
		responseLines, _ := m.stream.handler.HandleError(err)
		for _, line := range responseLines {
			m.output.AddLine(line)
		}
		if len(responseLines) > 0 {
			m.output.AddEmptyLine()
		}
	}
	m.compaction.active = false
	m.compaction.mode = compactionNone
	m.stopLoading()
	m.compaction.cancel = nil
	m.clearStreamCancel()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			reploutput.AddCompactionCancelledStatus(m.output, "Compaction cancelled.")
		} else {
			status := "Compaction failed: " + err.Error()
			reploutput.AddCompactionErrorStatus(m.output, status)
		}
	}
	m.adjustTextareaHeight()
	m.refreshContextStatus()
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
	return m.drainQueuedInput()
}

func (m *replModel) handleToolStart(toolCall *llm.ToolCall) (replModel, tea.Cmd) {
	m.flushStreamRender()
	if toolCall != nil && toolCall.Name == tools.AskUserToolName {
		m.stream.handler.HandleToolStart(toolCall)
		return *m, m.waitForAsyncEvent()
	}
	if toolCall.Name == "bash" {
		command, _ := toolCall.Input["command"].(string)
		summary, _ := toolCall.Input["summary"].(string)
		m.stream.handler.HandleBashStart(command, summary)
	} else {
		m.stream.handler.HandleToolStart(toolCall)
	}
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
	return *m, m.waitForAsyncEvent()
}

func (m *replModel) handleToolEnd(toolCall *llm.ToolCall) (replModel, tea.Cmd) {
	m.flushStreamRender()
	toolCall = sanitizeDelegateToolCall(toolCall)
	if toolCall != nil && toolCall.Name == tools.AskUserToolName {
		m.stream.handler.HandleToolEnd(toolCall)
		return *m, m.waitForAsyncEvent()
	}
	if toolCall.Name == "bash" {
		m.stream.handler.HandleBashEnd(toolCall)
	} else {
		m.stream.handler.HandleToolEnd(toolCall)
	}
	m.loading.text = nextLoadingText()
	m.stream.handler.SetLoadingText(m.loading.text)
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
	return *m, m.waitForAsyncEvent()
}

func sanitizeDelegateToolCall(toolCall *llm.ToolCall) *llm.ToolCall {
	if toolCall == nil || toolCall.Name != "delegate_task" {
		return toolCall
	}
	cloned := *toolCall
	output, ok := toolCall.Output.(map[string]any)
	if !ok {
		return &cloned
	}
	clonedOutput := make(map[string]any, 4)
	for _, key := range []string{"completed", "failed", "completed_by_agent", "failed_by_agent"} {
		if value, exists := output[key]; exists {
			clonedOutput[key] = value
		}
	}
	cloned.Output = clonedOutput
	return &cloned
}

// extractAtToken scans backwards from cursorPos in input to find a @<token>.
// The @ must be at the start of input or preceded by a space.
// Returns the token text (without @), the start index of @, and found=true if valid.
func extractAtToken(input string, cursorPos int) (token string, startIdx int, found bool) {
	if cursorPos <= 0 || cursorPos > len(input) {
		return "", 0, false
	}
	sub := input[:cursorPos]
	atIdx := strings.LastIndex(sub, "@")
	if atIdx < 0 {
		return "", 0, false
	}
	if atIdx > 0 && input[atIdx-1] != ' ' {
		return "", 0, false
	}
	tok := sub[atIdx+1:]
	if len(tok) == 0 {
		return "", 0, false
	}
	if strings.ContainsRune(tok, ' ') {
		return "", 0, false
	}
	return tok, atIdx, true
}

func (m *replModel) handleFileModeSelection() (replModel, tea.Cmd) {
	var item *replwidgets.SuggestionItem
	if cur := m.suggestion.Current(); cur != nil {
		item = cur
	} else if first := m.suggestion.First(); first != nil {
		item = first
	}
	if item != nil {
		val := m.textarea.Value()
		linesBefore := strings.Split(val, "\n")
		cursorByte := 0
		for i, ln := range linesBefore {
			if i == m.textarea.Line() {
				cursorByte += m.textarea.Column()
				break
			}
			cursorByte += len(ln) + 1
		}
		if _, atIdx, found := extractAtToken(val, cursorByte); found {
			replacement := "@" + item.Name + " "
			newVal := val[:atIdx] + replacement + val[cursorByte:]
			m.textarea.SetValue(newVal)
			m.textarea.MoveToEnd()
		}
	}
	m.suggestion.Hide()
	m.adjustTextareaHeight()
	return *m, nil
}

func suggestionValue(item *replwidgets.SuggestionItem) string {
	if item.Value != "" {
		return item.Value
	}
	return item.Name
}

func (m *replModel) handleSuggestionKeyMsg(keyMsg tea.KeyPressMsg) (bool, replModel, tea.Cmd) {
	switch keyMsg.String() {
	case keyEnter, keyTab:
		if m.suggestion.IsFileMode() {
			result, cmd := m.handleFileModeSelection()
			return true, result, cmd
		}
		if keyMsg.String() == keyEnter && !m.suggestion.IsModelMode() && (m.textarea.Value() == replcommands.Model || m.suggestion.Current().Name == replcommands.Model) {
			m.textarea.SetValue(replcommands.Model)
			m.suggestion.Hide()
			m.refreshSuggestions(m.textarea.Value())
			m.adjustTextareaHeight()
			return true, *m, nil
		}
		if keyMsg.String() == keyEnter && m.textarea.Value() == replcommands.Model && m.suggestion.IsModelMode() && m.suggestion.IsFirstSelected() {
			m.suggestion.Hide()
			m.adjustTextareaHeight()
			result, cmd := m.handleEnterKey()
			return true, result, cmd
		}
		if cur := m.suggestion.Current(); cur != nil {
			m.textarea.SetValue(suggestionValue(cur))
		} else if first := m.suggestion.First(); first != nil {
			m.textarea.SetValue(suggestionValue(first))
		}
		if keyMsg.String() == keyEnter {
			m.suggestion.Hide()
		} else {
			m.refreshSuggestions(m.textarea.Value())
		}
		m.adjustTextareaHeight()
		return true, *m, nil
	case keyUp, keyShiftUp:
		m.suggestion.MoveUp()
		return true, *m, nil
	case keyDown, keyShiftDown:
		m.suggestion.MoveDown()
		return true, *m, nil
	case keyEsc:
		if m.stream.handler == nil || !m.stream.handler.IsActive() {
			m.suggestion.Refresh("")
			m.adjustTextareaHeight()
			return true, *m, nil
		}
	}
	return false, *m, nil
}

func (m *replModel) handleAskUserKeyMsg(msg tea.KeyPressMsg) (replModel, tea.Cmd) {
	s := &m.askUser
	question := s.request.Questionnaire.Questions[s.index]
	var cmd tea.Cmd
	switch msg.String() {
	case keyCtrlC, keyEsc:
		s.resolve(s.requester, true)
	case keyUp:
		s.move(-1)
	case keyDown:
		s.move(1)
	case keyEnter:
		if s.selected < len(question.Options) {
			if s.answer(question.Options[s.selected]) {
				s.resolve(s.requester, false)
			}
		} else if s.editing {
			if value := s.input.Value(); strings.TrimSpace(value) != "" && s.answer(value) {
				s.resolve(s.requester, false)
			}
		} else {
			s.editing = true
			s.input.Focus()
		}
	default:
		if s.editing || msg.Text != "" {
			s.selected = len(question.Options)
			s.editing = true
			s.input.Focus()
			s.input, cmd = s.input.Update(msg)
		}
	}
	if s.active() {
		m.stream.handler.SetAskUser(s)
	} else {
		m.appendResolvedAskUserSegment()
	}
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
	return *m, cmd
}

func (m *replModel) handleAskUserPasteMsg(msg tea.PasteMsg) (replModel, tea.Cmd) {
	s := &m.askUser
	s.selected = len(s.request.Questionnaire.Questions[s.index].Options)
	s.editing = true
	s.input.Focus()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	m.stream.handler.SetAskUser(s)
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
	return *m, cmd
}

func (m *replModel) handleKeyMsg(msg tea.Msg) (replModel, tea.Cmd) {
	m.flushStreamRender()
	if m.sessionPicker != nil {
		return m.handleSessionPickerKeyMsg(msg)
	}

	if m.modelSelection != nil {
		var cmd tea.Cmd
		m.modelSelection, cmd = m.modelSelection.Update(msg)
		m.updateViewportContent()
		return *m, cmd
	}

	if m.adversary.modelSelection != nil {
		var cmd tea.Cmd
		m.adversary.modelSelection, cmd = m.adversary.modelSelection.Update(msg)
		m.updateViewportContent()
		return *m, cmd
	}

	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return *m, nil
	}

	if m.compaction.active && m.compaction.mode != compactionAutomatic {
		switch keyMsg.String() {
		case keyEsc:
			if m.compaction.cancel != nil {
				m.compaction.cancel()
				m.compaction.cancel = nil
			}
			return *m, nil
		case keyCtrlC, keyCtrlD:
			return *m, nil
		}
	}
	if m.compaction.mode == compactionAutomatic && keyMsg.String() == keyEsc {
		if m.compaction.cancel != nil {
			m.compaction.cancel()
			m.compaction.cancel = nil
		}
		return *m, nil
	}

	if m.stream.handler != nil && m.stream.handler.HasPendingPermission() {
		switch keyMsg.String() {
		case "up", "down", keyEnter, keyEsc:
			return m.handlePermissionKeyMsg(keyMsg)
		}
	}

	if m.askUser.active() {
		return m.handleAskUserKeyMsg(keyMsg)
	}

	if m.suggestion.Visible() {
		if handled, result, cmd := m.handleSuggestionKeyMsg(keyMsg); handled {
			return result, cmd
		}
	} else if keyMsg.String() == "shift+tab" {
		m.toggleMode()
		return *m, nil
	} else if keyMsg.String() == keyTab {
		return *m, m.toggleInputFocus()
	}

	if !m.textarea.Focused() {
		if handled := m.handleViewportFocusKeyMsg(keyMsg); handled {
			return *m, nil
		}
		if keyMsg.Text != "" {
			cmd := m.focusInput()
			var textCmd tea.Cmd
			m.textarea, textCmd = m.textarea.Update(keyMsg)
			input := m.textarea.Value()
			m.refreshSuggestions(input)
			m.adjustTextareaHeight()
			return *m, tea.Batch(cmd, textCmd)
		}
	}

	switch keyMsg.String() {
	case keyEnter:
		return m.handleEnterKey()
	case keyCtrlC, keyCtrlD:
		if m.adversary.streamHandler != nil && m.adversary.streamHandler.IsActive() {
			m.cancelAdversaryStream()
			m.updateViewportContent()
			m.scrollToBottomIfFollowing()
			return *m, nil
		}
		if m.btw.streamHandler != nil && m.btw.streamHandler.IsActive() {
			m.cancelBtwStream()
			m.updateViewportContent()
			m.scrollToBottomIfFollowing()
			return *m, nil
		}
		if m.bang.active {
			m.cancelBangCommand()
			return *m, nil
		}
		if m.stream.handler != nil && m.stream.handler.IsActive() {
			m.interruptStream(interruptedPromptText)
			return m.drainQueuedInput()
		}
		if len(m.queuedInputs) > 0 {
			m.queuedInputs = nil
			m.updateViewportContent()
			m.adjustTextareaHeight()
			return *m, m.showNotification("Queue cleared")
		}
		if m.textarea.Value() != "" {
			m.textarea.Reset()
			m.adjustTextareaHeight()
			return *m, nil
		}
		m.quitting = true
		_ = m.history.Flush()
		return *m, tea.Quit
	case keyEsc:
		if m.adversary.streamHandler != nil && m.adversary.streamHandler.IsActive() {
			m.cancelAdversaryStream()
			m.updateViewportContent()
			m.scrollToBottomIfFollowing()
			return *m, nil
		}
		if m.btw.streamHandler != nil && m.btw.streamHandler.IsActive() {
			m.cancelBtwStream()
			m.updateViewportContent()
			m.scrollToBottomIfFollowing()
			return *m, nil
		}
		if m.bang.active {
			m.cancelBangCommand()
			return *m, nil
		}
		if m.stream.handler != nil && m.stream.handler.IsActive() {
			m.interruptStream(interruptedPromptText)
			return m.drainQueuedInput()
		}
		if len(m.queuedInputs) > 0 {
			m.queuedInputs = nil
			m.updateViewportContent()
			m.adjustTextareaHeight()
			return *m, m.showNotification("Queue cleared")
		}
		if m.textarea.Value() != "" {
			m.textarea.Reset()
			m.adjustTextareaHeight()
			return *m, nil
		}
		return *m, nil
	case keyUp, keyShiftUp:
		if m.isAtTopOfInput() {
			if !m.history.IsNavigating() && m.textarea.Column() > 0 {
				m.textarea.MoveToBegin()
				return *m, nil
			}
			if val, ok := m.history.NavigateUp(m.textarea.Value()); ok {
				m.textarea.SetValue(val)
				m.textarea.MoveToEnd()
				m.adjustTextareaHeight()
			}
			return *m, nil
		}
	case keyDown, keyShiftDown:
		if m.isAtBottomOfInput() {
			if val, ok := m.history.NavigateDown(); ok {
				m.textarea.SetValue(val)
				m.textarea.MoveToEnd()
				m.adjustTextareaHeight()
			}
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
	input := m.textarea.Value()
	m.refreshSuggestions(input)
	m.adjustTextareaHeight()
	return *m, cmd
}

func (m *replModel) refreshSuggestions(input string) {
	if input == "" {
		m.suggestion.Hide()
		return
	}
	if m.refreshFileSuggestions(input) {
		return
	}
	if strings.HasPrefix(input, replcommands.Model) && (input == replcommands.Model || strings.HasPrefix(input, replcommands.Model+" ")) {
		m.suggestion.RefreshModels(input, m.modelPairs())
		return
	}
	if strings.HasPrefix(input, "/") {
		m.suggestion.RefreshWithSkills(input, m.skillSuggestions())
	}
}

func (m *replModel) modelPairs() []string {
	if m.ctx == nil || m.ctx.registry == nil {
		return nil
	}
	pairs := make([]string, 0)
	for _, provider := range m.ctx.registry.Providers {
		for _, model := range provider.Models {
			pairs = append(pairs, provider.ID+"/"+model.ID)
		}
	}
	return pairs
}

func (m *replModel) skillSuggestions() []replwidgets.SuggestionItem {
	skillList := m.appState.SkillSuggestions()
	items := make([]replwidgets.SuggestionItem, 0, len(skillList))
	for _, skill := range skillList {
		items = append(items, replwidgets.SuggestionItem{Name: "/" + skill.Name, Description: skill.Description})
	}
	return items
}

func (m *replModel) refreshFileSuggestions(input string) bool {
	if m.fileSearcher == nil {
		m.suggestion.Hide()
		return false
	}
	linesBefore := strings.Split(input, "\n")
	cursorByte := 0
	for i, ln := range linesBefore {
		if i == m.textarea.Line() {
			cursorByte += m.textarea.Column()
			break
		}
		cursorByte += len(ln) + 1
	}
	if tok, _, found := extractAtToken(input, cursorByte); found {
		paths := m.fileSearcher.Search(tok, 10)
		m.suggestion.RefreshFiles(paths)
		return true
	}
	m.suggestion.Hide()
	return false
}

func (m *replModel) interruptStream(message string) {
	m.flushStreamRender()
	m.clearAskUser()
	if m.stream.cancel != nil {
		m.stream.cancel()
		m.clearStreamCancel()
	}

	m.stopLoading()

	segments := cloneStreamSegments(m.stream.handler.segments)
	m.recordHistoricalToolActivity(segments)
	partialResponse := m.stream.handler.GetResponse()
	turnMemory := m.consumeTurnMemory()

	for _, line := range m.stream.handler.HandleInterrupt() {
		m.output.AddLine(line)
	}
	m.output.AddStyledLine("\n  "+message, repltheme.InterruptedStyle)
	m.output.AddEmptyLine()

	assistantMessage := llm.Message{
		Role:       llm.RoleAssistant,
		Content:    partialResponse,
		TurnMemory: turnMemory,
	}
	if persistErr := m.sessions.appendAssistantTurn(segments, assistantMessage, true, ""); persistErr != nil {
		m.handleSessionPersistenceError(persistErr)
	}

	m.adjustTextareaHeight()
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
}

func (m *replModel) handleSessionPickerKeyMsg(msg tea.Msg) (replModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok || m.sessionPicker == nil {
		return *m, nil
	}

	switch keyMsg.String() {
	case keyUp, keyShiftUp:
		m.sessionPicker.Move(-1)
		m.updateViewportContent()
	case keyDown, keyShiftDown:
		m.sessionPicker.Move(1)
		m.updateViewportContent()
	case keyEnter:
		selected := m.sessionPicker.Current()
		if selected == nil {
			return *m, nil
		}
		loaded, err := m.sessions.load(*selected)
		if err != nil {
			m.sessionPicker = nil
			m.handleSessionPersistenceError(err)
			m.updateViewportContent()
			m.viewport.GotoBottom()
			return *m, nil
		}
		m.replayLoadedSession(loaded)
	case keyEsc:
		m.sessionPicker = nil
		m.updateViewportContent()
		m.viewport.GotoBottom()
	}

	return *m, nil
}

func (m *replModel) handlePermissionKeyMsg(msg tea.KeyPressMsg) (replModel, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.stream.handler.MovePendingCursor(-1)
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
	case "down":
		m.stream.handler.MovePendingCursor(1)
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
	case keyEnter:
		req := m.stream.handler.GetPendingPermissionRequest()
		if req == nil {
			return *m, nil
		}
		choice := m.stream.handler.GetPendingChoice()
		if choice == replpermissions.ChoiceAskWhatToDo {
			m.stream.handler.ResolvePendingPermission(replpermissions.StatusRedirected)
			m.permissionRequester.SendResponse(replpermissions.ChoiceDeny, req.ToolName)
			m.interruptStream(interruptedPromptText)
			return m.drainQueuedInput()
		}
		var status replpermissions.Status
		switch choice {
		case replpermissions.ChoiceAllow:
			status = replpermissions.StatusAllowed
		case replpermissions.ChoiceAllowSession:
			status = replpermissions.StatusAllowedSession
		case replpermissions.ChoiceDeny:
			status = replpermissions.StatusDenied
		}
		m.stream.handler.ResolvePendingPermission(status)
		m.permissionRequester.SendResponse(choice, req.ToolName)
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
	case keyEsc:
		req := m.stream.handler.GetPendingPermissionRequest()
		if req == nil {
			return *m, nil
		}
		m.stream.handler.ResolvePendingPermission(replpermissions.StatusDenied)
		m.permissionRequester.SendResponse(replpermissions.ChoiceDeny, req.ToolName)
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
	}
	return *m, nil
}

func (m replModel) handleLLMStreamMsg(msg tea.Msg) (replModel, tea.Cmd, bool) {
	if streamMsg, ok := msg.(mainStreamMsg); ok {
		if m.stream.handler == nil || m.stream.handler.eventCh != streamMsg.eventCh {
			return m, nil, true
		}
		if streamMsg.closed {
			msg = llmDoneMsg{}
		} else {
			switch streamMsg.event.Type {
			case llm.StreamEventTypeChunk:
				msg = llmChunkMsg(streamMsg.event.Content)
			case llm.StreamEventTypeReasoningChunk:
				msg = llmReasoningChunkMsg(streamMsg.event.Content)
			case llm.StreamEventTypeDone:
				msg = llmDoneMsg{}
			case llm.StreamEventTypeError:
				msg = llmErrorMsg{err: streamMsg.event.Error}
			case llm.StreamEventTypeIncomplete:
				msg = llmIncompleteMsg{err: streamMsg.event.Error}
			case llm.StreamEventTypeToolStart:
				msg = llmToolStartMsg{toolCall: streamMsg.event.ToolCall}
			case llm.StreamEventTypeToolEnd:
				msg = llmToolEndMsg{toolCall: streamMsg.event.ToolCall}
			case llm.StreamEventTypeUsage:
				msg = llmUsageMsg{usage: streamMsg.event.Usage}
			case llm.StreamEventTypeRetry:
				msg = llmRetryMsg{err: streamMsg.event.Error, attempt: streamMsg.event.Attempt}
			case llm.StreamEventTypeAutoCompactionStarted:
				msg = llmAutoCompactionStartedMsg{event: streamMsg.event.AutoCompaction}
			case llm.StreamEventTypeAutoCompactionApplied:
				msg = llmAutoCompactionAppliedMsg{event: streamMsg.event.AutoCompaction}
			case llm.StreamEventTypeAutoCompactionCancelled:
				msg = llmAutoCompactionCancelledMsg{event: streamMsg.event.AutoCompaction}
			case llm.StreamEventTypeAutoCompactionFailed:
				msg = llmAutoCompactionFailedMsg{event: streamMsg.event.AutoCompaction}
			default:
				msg = llmDoneMsg{}
			}
		}
	}

	if updated, cmd, handled := m.handleBtwStreamMsg(msg); handled {
		return updated, cmd, true
	}

	if updated, cmd, handled := m.handleAdversaryStreamMsg(msg); handled {
		return updated, cmd, true
	}

	switch msg.(type) {
	case streamRenderMsg:
		m.flushStreamRender()
		return m, nil, true
	}

	if m.stream.handler == nil || !m.stream.handler.IsActive() {
		switch msg.(type) {
		case
			llmChunkMsg,
			llmReasoningChunkMsg,
			llmDoneMsg,
			llmIncompleteMsg,
			llmErrorMsg,
			llmRetryMsg,
			llmToolStartMsg,
			llmToolEndMsg,
			llmUsageMsg,
			llmAutoCompactionStartedMsg,
			llmAutoCompactionAppliedMsg,
			llmAutoCompactionCancelledMsg,
			llmAutoCompactionFailedMsg:
			return m, nil, true
		}
	}

	switch msg := msg.(type) {
	case llmUsageMsg:
		updated, cmd := m.handleLLMUsage(msg.usage)
		return updated, cmd, true
	case llmAutoCompactionStartedMsg:
		updated, cmd := m.handleAutoCompactionStarted(msg.event)
		return updated, cmd, true
	case llmAutoCompactionAppliedMsg:
		updated, cmd := m.handleAutoCompactionApplied(msg.event)
		return updated, cmd, true
	case llmAutoCompactionCancelledMsg, llmAutoCompactionFailedMsg:
		updated, cmd := m.handleAutoCompactionStopped()
		return updated, cmd, true
	case llmChunkMsg:
		updated, cmd := m.handleLLMChunk(string(msg))
		return updated, cmd, true
	case llmReasoningChunkMsg:
		updated, cmd := m.handleLLMReasoningChunk(string(msg))
		return updated, cmd, true
	case llmDoneMsg:
		updated, cmd := m.handleLLMDone()
		return updated, cmd, true
	case llmIncompleteMsg:
		updated, cmd := m.handleLLMIncomplete(msg.err)
		return updated, cmd, true
	case llmErrorMsg:
		updated, cmd := m.handleLLMError(msg.err)
		return updated, cmd, true
	case llmRetryMsg:
		updated, cmd := m.handleLLMRetry(msg.err, msg.attempt)
		return updated, cmd, true
	case llmToolStartMsg:
		updated, cmd := m.handleToolStart(msg.toolCall)
		return updated, cmd, true
	case llmToolEndMsg:
		updated, cmd := m.handleToolEnd(msg.toolCall)
		return updated, cmd, true
	default:
		return m, nil, false
	}
}

func (m *replModel) handleUpdateCheckMsg(msg updateCheckMsg) {
	if msg.latest == "" {
		return
	}
	m.output.AddEmptyLine()
	m.output.AddStyledLine("  Update available: v"+msg.latest, repltheme.UpdateAvailableStyle)
	m.output.AddEmptyLine()
	updateCmd := "  npm update -g keen-code\n  or\n  curl -fsSL https://raw.githubusercontent.com/mochow13/keen-code/main/scripts/install.sh | bash"
	m.output.AddStyledLine(updateCmd, repltheme.UpdateCommandStyle)
	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
}

func (m replModel) handleBtwStreamMsg(msg tea.Msg) (replModel, tea.Cmd, bool) {
	if m.btw.streamHandler == nil || !m.btw.streamHandler.IsActive() {
		switch msg.(type) {
		case btwChunkMsg, btwDoneMsg, btwErrorMsg:
			return m, nil, true
		}
		return m, nil, false
	}

	switch msg := msg.(type) {
	case btwChunkMsg:
		m.btw.streamHandler.HandleChunk(string(msg))
		return m, tea.Batch(m.afterStreamUpdate(), waitForBtwEvent(m.btw.streamHandler.eventCh)), true
	case btwDoneMsg:
		m.flushStreamRender()
		responseLines, _ := m.btw.streamHandler.HandleDone()
		m.btw.showSpinner = false
		m.btw.lines = responseLines
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return m, nil, true
	case btwErrorMsg:
		m.flushStreamRender()
		pendingLines, errMsg := m.btw.streamHandler.HandleError(msg.err)
		m.btw.showSpinner = false
		lines := pendingLines
		if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
			lines = append(lines, "  "+repltheme.ErrorStyle.Render(errMsg))
		}
		m.btw.lines = lines
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m replModel) handleAdversaryStreamMsg(msg tea.Msg) (replModel, tea.Cmd, bool) {
	if m.adversary.streamHandler == nil || !m.adversary.streamHandler.IsActive() {
		switch msg.(type) {
		case adversaryChunkMsg, adversaryDoneMsg, adversaryErrorMsg, adversaryToolStartMsg, adversaryToolEndMsg:
			return m, nil, true
		}
		return m, nil, false
	}

	switch msg := msg.(type) {
	case adversaryChunkMsg:
		m.adversary.streamHandler.HandleChunk(string(msg))
		return m, tea.Batch(m.afterStreamUpdate(), waitForAdversaryEvent(m.adversary.streamHandler.eventCh)), true
	case adversaryToolStartMsg:
		m.flushStreamRender()
		m.adversary.streamHandler.HandleToolStart(msg.toolCall)
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return m, waitForAdversaryEvent(m.adversary.streamHandler.eventCh), true
	case adversaryToolEndMsg:
		m.flushStreamRender()
		m.adversary.streamHandler.HandleToolEnd(msg.toolCall)
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return m, waitForAdversaryEvent(m.adversary.streamHandler.eventCh), true
	case adversaryDoneMsg:
		m.flushStreamRender()
		responseLines, _ := m.adversary.streamHandler.HandleDone()
		m.adversary.showSpinner = false
		m.adversary.lines = responseLines
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return m, nil, true
	case adversaryErrorMsg:
		m.flushStreamRender()
		pendingLines, errMsg := m.adversary.streamHandler.HandleError(msg.err)
		m.adversary.showSpinner = false
		lines := pendingLines
		if msg.err != nil && !errors.Is(msg.err, context.Canceled) {
			lines = append(lines, "  "+repltheme.ErrorStyle.Render(errMsg))
		}
		m.adversary.lines = lines
		m.updateViewportContent()
		m.scrollToBottomIfFollowing()
		return m, nil, true
	default:
		return m, nil, false
	}
}
