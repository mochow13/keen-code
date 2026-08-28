package repl

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	replappstate "github.com/mochow13/keen-code/internal/cli/repl/appstate"
	replcommands "github.com/mochow13/keen-code/internal/cli/repl/commands"
	replfilesearch "github.com/mochow13/keen-code/internal/cli/repl/filesearch"
	reploutput "github.com/mochow13/keen-code/internal/cli/repl/output"
	replwidgets "github.com/mochow13/keen-code/internal/cli/repl/widgets"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/providers"
	"github.com/mochow13/keen-code/internal/session"
	"github.com/mochow13/keen-code/internal/subagents"
	"github.com/mochow13/keen-code/internal/tools"
)

func TestHandleLLMChunk(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	m := replModel{
		stream:  streamState{handler: sh},
		loading: loadingState{showSpinner: true},
		width:   80,
	}

	newM, cmd := m.handleLLMChunk("hello")

	if !newM.loading.showSpinner {
		t.Error("expected showSpinner to remain true after chunk")
	}
	if sh.GetResponse() != "hello" {
		t.Errorf("expected response 'hello', got '%s'", sh.GetResponse())
	}
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
}

func TestContextStatus_UpdatesOnUsageEvent(t *testing.T) {
	m := newTestModel()
	m.ctx = &replContext{
		workingDir: "",
		cfg: &config.ResolvedConfig{
			Provider: "openai",
			Model:    "gpt-5.4",
		},
		registry: &providers.Registry{
			Providers: []providers.Provider{
				{
					ID: "openai",
					Models: []providers.Model{
						{ID: "gpt-5.4", ContextWindow: 2000},
					},
				},
			},
		},
	}
	m.appState = replappstate.New(nil, t.TempDir())
	m.refreshContextStatus()
	if m.contextStatus.Percent != 0 {
		t.Fatalf("expected 0%% initially, got %.2f", m.contextStatus.Percent)
	}

	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")
	m.loading.showSpinner = true

	updatedAfterChunk, _ := m.handleLLMChunk("hello")
	if updatedAfterChunk.contextStatus.Percent != 0 {
		t.Fatalf("expected context percent to remain 0 during chunk, got %.2f", updatedAfterChunk.contextStatus.Percent)
	}

	updatedAfterUsage, _ := updatedAfterChunk.handleLLMUsage(&llm.TokenUsage{InputTokens: 1000})
	if updatedAfterUsage.contextStatus.Percent != 50.0 {
		t.Fatalf("expected 50%% after usage event, got %.2f", updatedAfterUsage.contextStatus.Percent)
	}
}

func TestHandleLLMDoneDrainsSubagentActivity(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(chan llm.StreamEvent), "Loading...")
	activity := make(chan subagents.ToolActivity, 2)
	activity <- subagents.ToolActivity{RunID: "run-1", CallID: "tool-1", Agent: "worker", Event: llm.StreamEvent{
		Type:     llm.StreamEventTypeToolStart,
		ToolCall: &llm.ToolCall{Name: "bash", Input: map[string]any{"command": "go test ./..."}},
	}}
	activity <- subagents.ToolActivity{RunID: "run-1", CallID: "tool-1", Agent: "worker", Event: llm.StreamEvent{
		Type:     llm.StreamEventTypeToolEnd,
		ToolCall: &llm.ToolCall{Name: "bash"},
	}}
	m := replModel{
		stream:           streamState{handler: sh},
		loading:          loadingState{showSpinner: true},
		width:            80,
		appState:         replappstate.New(nil, t.TempDir()),
		output:           reploutput.NewOutputBuilder(80, ""),
		subagentActivity: activity,
	}

	updated, _ := m.handleLLMDone()
	output := strings.Join(updated.output.GetLines(), "\n")
	if !strings.Contains(output, "[worker]") || !strings.Contains(output, "go test ./...") {
		t.Fatalf("expected drained subagent activity in completed output, got %q", output)
	}
	if len(activity) != 0 {
		t.Fatalf("expected activity channel to be drained, got %d pending events", len(activity))
	}
}

func TestHandleLLMDone(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")
	sh.HandleChunk("response line 1\nresponse line 2")

	m := replModel{
		stream:   streamState{handler: sh},
		loading:  loadingState{showSpinner: true},
		width:    80,
		appState: replappstate.New(nil, t.TempDir()),
		output:   reploutput.NewOutputBuilder(80, ""),
	}

	newM, cmd := m.handleLLMDone()

	if newM.loading.showSpinner {
		t.Error("expected showSpinner to be false after done")
	}

	if len(m.appState.GetMessages()) != 1 {
		t.Errorf("expected 1 message in history, got %d", len(m.appState.GetMessages()))
	}
	if m.appState.GetMessages()[0].Role != llm.RoleAssistant {
		t.Errorf("expected assistant role, got %s", m.appState.GetMessages()[0].Role)
	}
	if m.appState.GetMessages()[0].Content != "response line 1\nresponse line 2" {
		t.Errorf("unexpected message content: %s", m.appState.GetMessages()[0].Content)
	}

	if len(newM.output.GetLines()) != 3 {
		t.Errorf("expected 3 output lines (2 content + 1 empty), got %d", len(newM.output.GetLines()))
	}

	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestHandleLLMError(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	m := replModel{
		stream:   streamState{handler: sh},
		loading:  loadingState{showSpinner: true},
		width:    80,
		appState: replappstate.New(nil, t.TempDir()),
		output:   reploutput.NewOutputBuilder(80, ""),
	}

	testErr := errors.New("stream failed")
	newM, cmd := m.handleLLMError(testErr)

	if newM.loading.showSpinner {
		t.Error("expected showSpinner to be false after error")
	}

	if len(newM.output.GetLines()) != 2 {
		t.Errorf("expected 2 output lines (1 error + 1 empty), got %d", len(newM.output.GetLines()))
	}

	if !strings.Contains(newM.output.GetLines()[0], "stream failed") {
		t.Errorf("expected error message in output, got: %s", newM.output.GetLines()[0])
	}

	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestHandleKeyMsg_Enter(t *testing.T) {
	ta := textarea.New()
	ta.SetValue(replcommands.Help)
	m := replModel{
		textarea: ta,
		width:    80,
		stream:   streamState{handler: NewStreamHandler(nil)},
		ctx:      &replContext{},
		output:   reploutput.NewOutputBuilder(80, ""),
	}

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !strings.Contains(newM.output.Join(), "Available Commands") {
		t.Error("expected help text in output after enter with /help")
	}
	if cmd != nil {
		t.Error("expected nil cmd for help command")
	}
}

func TestHandleKeyMsg_CtrlC_EmptyInputQuits(t *testing.T) {
	m := replModel{
		quitting: false,
	}

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	if !newM.quitting {
		t.Error("expected quitting to be true after ctrl+c with empty input")
	}

	if cmd == nil {
		t.Fatal("expected tea.Quit cmd after ctrl+c with empty input")
	}

	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestHandleKeyMsg_CtrlC_WithInputClearsAndDoesNotQuit(t *testing.T) {
	ta := textarea.New()
	ta.SetValue("draft text")

	m := replModel{
		textarea: ta,
		quitting: false,
	}

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	if newM.textarea.Value() != "" {
		t.Errorf("expected textarea to be cleared, got %q", newM.textarea.Value())
	}

	if newM.quitting {
		t.Error("expected quitting to remain false when ctrl+c clears input")
	}

	if cmd != nil {
		t.Error("expected nil cmd when ctrl+c clears input")
	}
}

func TestHandleKeyMsg_CtrlC_WithActiveStreamInterrupts(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")
	m.stream.handler.HandleChunk("partial response")
	m.loading.showSpinner = true

	canceled := false
	m.stream.cancel = func() {
		canceled = true
	}

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	if !canceled {
		t.Error("expected stream cancel function to be called on ctrl+c")
	}
	if newM.stream.cancel != nil {
		t.Error("expected stream cancel function to be cleared after ctrl+c")
	}
	if newM.stream.handler.IsActive() {
		t.Error("expected stream handler to be inactive after ctrl+c interruption")
	}
	if newM.loading.showSpinner {
		t.Error("expected spinner to be hidden after ctrl+c interruption")
	}
	if !strings.Contains(newM.output.Join(), "partial response") {
		t.Error("expected streamed partial content to be preserved on ctrl+c interruption")
	}
	if !strings.Contains(newM.output.Join(), "Interrupted") {
		t.Error("expected interrupted message in output")
	}
	if cmd != nil {
		t.Error("expected nil cmd for ctrl+c interruption")
	}
}

func TestHandleKeyMsg_CtrlC_WithAskUserCancelsQuestionnaireFirst(t *testing.T) {
	m := newTestModel()
	m.stream.handler.Start(make(chan llm.StreamEvent), "Loading...")
	m.askUser = testAskUserState()
	m.stream.handler.SetAskUser(&m.askUser)
	streamCanceled := false
	m.stream.cancel = func() { streamCanceled = true }

	updated, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	if streamCanceled {
		t.Fatal("ctrl+c should cancel the questionnaire before interrupting the stream")
	}
	if updated.askUser.active() || !updated.stream.handler.IsActive() {
		t.Fatal("ctrl+c should resolve the questionnaire and leave the stream active")
	}
	if cmd != nil {
		t.Fatal("questionnaire cancellation returned an unexpected command")
	}
}

func TestHandleKeyMsg_CtrlC_WithActiveStreamDrainsNextQueuedInput(t *testing.T) {
	m := newTestModel()
	m.ctx.cfg = &config.ResolvedConfig{APIKey: "key", Model: "model"}
	client := &mockLLMClient{}
	m.appState = replappstate.New(client, "")
	m.stream.handler.Start(make(chan llm.StreamEvent), "Loading...")
	m.loading.showSpinner = true
	m.stream.cancel = func() {}
	m.queuedInputs = []string{"next prompt", "later prompt"}

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	if !newM.stream.handler.IsActive() {
		t.Fatal("expected next queued input to start a stream")
	}
	if len(newM.queuedInputs) != 1 || newM.queuedInputs[0] != "later prompt" {
		t.Fatalf("expected only later prompt to remain queued, got %v", newM.queuedInputs)
	}
	if messages := newM.appState.GetMessages(); len(messages) != 1 || messages[0].Content != "next prompt" {
		t.Fatalf("expected next prompt to be submitted, got %#v", messages)
	}
	if cmd == nil {
		t.Fatal("expected command for the next stream")
	}
}

func TestHandleKeyMsg_CtrlC_SecondCtrlCQuitsAfterInterrupt(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")
	m.loading.showSpinner = true
	m.stream.cancel = func() {}

	interrupted, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if interrupted.stream.handler.IsActive() {
		t.Fatal("expected stream to be interrupted before second ctrl+c")
	}

	second, cmd := interrupted.handleKeyMsg(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !second.quitting {
		t.Error("expected second ctrl+c to quit after interruption")
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd on second ctrl+c after interruption")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", cmd())
	}
}

func TestHandleKeyMsg_Esc_WithActiveStreamInterrupts(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")
	m.stream.handler.HandleChunk("partial response")
	m.loading.showSpinner = true

	canceled := false
	m.stream.cancel = func() {
		canceled = true
	}

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})

	if !canceled {
		t.Error("expected stream cancel function to be called on esc")
	}
	if newM.stream.cancel != nil {
		t.Error("expected stream cancel function to be cleared after esc")
	}
	if newM.stream.handler.IsActive() {
		t.Error("expected stream handler to be inactive after esc interruption")
	}
	if newM.loading.showSpinner {
		t.Error("expected spinner to be hidden after esc interruption")
	}
	if !strings.Contains(newM.output.Join(), "partial response") {
		t.Error("expected streamed partial content to be preserved on interruption")
	}
	if !strings.Contains(newM.output.Join(), "Interrupted") {
		t.Error("expected interrupted message in output")
	}
	if cmd != nil {
		t.Error("expected nil cmd for esc interruption")
	}
}

func TestHandleKeyMsg_Esc_WithActiveStreamDrainsNextQueuedInput(t *testing.T) {
	m := newTestModel()
	m.ctx.cfg = &config.ResolvedConfig{APIKey: "key", Model: "model"}
	client := &mockLLMClient{}
	m.appState = replappstate.New(client, "")
	m.stream.handler.Start(make(chan llm.StreamEvent), "Loading...")
	m.loading.showSpinner = true
	m.stream.cancel = func() {}
	m.queuedInputs = []string{"next prompt", "later prompt"}

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})

	if !newM.stream.handler.IsActive() {
		t.Fatal("expected next queued input to start a stream")
	}
	if len(newM.queuedInputs) != 1 || newM.queuedInputs[0] != "later prompt" {
		t.Fatalf("expected only later prompt to remain queued, got %v", newM.queuedInputs)
	}
	if messages := newM.appState.GetMessages(); len(messages) != 1 || messages[0].Content != "next prompt" {
		t.Fatalf("expected next prompt to be submitted, got %#v", messages)
	}
	if cmd == nil {
		t.Fatal("expected command for the next stream")
	}
}

func TestHandleLLMStreamMsg_IgnoresPreviousStreamAfterQueuedInputStarts(t *testing.T) {
	m := newTestModel()
	oldEventCh := make(chan llm.StreamEvent)
	newEventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(newEventCh, "Loading...")
	m.stream.handler.HandleChunk("new response")
	m.loading.showSpinner = true

	newM, cmd, handled := m.handleLLMStreamMsg(mainStreamMsg{
		eventCh: oldEventCh,
		event:   llm.StreamEvent{Type: llm.StreamEventTypeError, Error: context.Canceled},
	})

	if !handled {
		t.Fatal("expected stale stream message to be handled")
	}
	if cmd != nil {
		t.Fatal("expected no command for stale stream message")
	}
	if !newM.stream.handler.IsActive() {
		t.Fatal("expected current stream to remain active")
	}
	if got := newM.stream.handler.GetResponse(); got != "new response" {
		t.Fatalf("expected current response to remain intact, got %q", got)
	}
}

func TestHandleKeyMsg_Esc_WhenIdleNoOp(t *testing.T) {
	m := newTestModel()

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})

	if newM.quitting {
		t.Error("expected esc to not quit when no active stream")
	}
	if len(newM.output.GetLines()) != 0 {
		t.Error("expected no output when esc pressed without active stream")
	}
	if cmd != nil {
		t.Error("expected nil cmd when esc pressed without active stream")
	}
}

func TestHandleKeyMsg_Esc_WithInputClearsAndDoesNotQuit(t *testing.T) {
	ta := textarea.New()
	ta.SetValue("draft text")

	m := replModel{
		textarea: ta,
		quitting: false,
	}

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})

	if newM.textarea.Value() != "" {
		t.Errorf("expected textarea to be cleared, got %q", newM.textarea.Value())
	}

	if newM.quitting {
		t.Error("expected quitting to remain false when esc clears input")
	}

	if cmd != nil {
		t.Error("expected nil cmd when esc clears input")
	}
}

func TestHandleKeyMsg_Esc_CancelsCompactionBeforeSuggestions(t *testing.T) {
	m := newTestModel()
	m.compaction.active = true
	m.suggestion.Refresh("/c")
	cancelled := false
	m.compaction.cancel = func() {
		cancelled = true
	}

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})

	if !cancelled {
		t.Fatal("expected esc to cancel compaction")
	}
	if newM.compaction.cancel != nil {
		t.Fatal("expected cancel func to be cleared after esc")
	}
	if !newM.suggestion.Visible() {
		t.Fatal("expected suggestions to remain untouched while compaction consumes input")
	}
	if cmd != nil {
		t.Fatal("expected no follow-up cmd")
	}
}

func TestHandleKeyMsg_AllowsTypingDuringCompaction(t *testing.T) {
	m := newTestModel()
	m.compaction.active = true
	m.textarea.SetValue("draft")

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'x', Text: "x"})

	if newM.textarea.Value() != "draftx" {
		t.Fatalf("expected typing to reach the textarea during compaction, got %q", newM.textarea.Value())
	}
}

func TestHandleKeyMsg_IgnoresCtrlCDuringCompaction(t *testing.T) {
	m := newTestModel()
	m.compaction.active = true
	m.compaction.mode = compactionManual
	cancelled := false
	m.compaction.cancel = func() {
		cancelled = true
	}

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	if cancelled {
		t.Fatal("expected ctrl+c not to cancel compaction")
	}
	if newM.quitting {
		t.Fatal("expected ctrl+c not to quit during compaction")
	}
	if cmd != nil {
		t.Fatal("expected no cmd while compacting")
	}
}

func TestHandleKeyMsg_TabTogglesInputFocus(t *testing.T) {
	m := newTestModel()

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyTab})
	if newM.textarea.Focused() {
		t.Fatal("expected tab to blur focused input")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd when blurring input")
	}

	newM, cmd = newM.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyTab})
	if !newM.textarea.Focused() {
		t.Fatal("expected tab to focus blurred input")
	}
	if cmd == nil {
		t.Fatal("expected focus command when focusing input")
	}
}

func TestHandleKeyMsg_ShiftTabTogglesMode(t *testing.T) {
	m := newTestModel()

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Text: "shift+tab"})
	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	if newM.currentMode() != llm.ModePlan {
		t.Fatalf("expected plan mode, got %q", newM.currentMode())
	}
	if newM.appState.Mode() != llm.ModePlan {
		t.Fatalf("expected app state plan mode, got %q", newM.appState.Mode())
	}

	newM, _ = newM.handleKeyMsg(tea.KeyPressMsg{Text: "shift+tab"})
	if newM.currentMode() != llm.ModeBuild {
		t.Fatalf("expected build mode, got %q", newM.currentMode())
	}
}

func TestHandleKeyMsg_InputFocusUpDownDoesNotScrollViewportWhenHistoryExhausted(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("draft")
	offset := scrollViewportAwayFromBottom(t, &m)

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	if got := newM.viewport.YOffset(); got != offset {
		t.Fatalf("expected input-focused up key not to scroll viewport, got offset %d want %d", got, offset)
	}
	if newM.textarea.Value() != "draft" {
		t.Fatalf("expected exhausted history to leave input unchanged, got %q", newM.textarea.Value())
	}

	newM, cmd = m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	if got := newM.viewport.YOffset(); got != offset {
		t.Fatalf("expected input-focused down key not to scroll viewport, got offset %d want %d", got, offset)
	}
}

func TestHandleKeyMsg_ViewportFocusUpDownScrolls(t *testing.T) {
	m := newTestModel()
	m.blurInput()
	m.viewport.SetHeight(6)
	for range 40 {
		m.output.AddLine("existing output")
	}
	m.updateViewportContent()
	m.viewport.GotoBottom()
	bottomOffset := m.viewport.YOffset()

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	if newM.viewport.YOffset() >= bottomOffset {
		t.Fatalf("expected viewport-focused up key to scroll up from %d, got %d", bottomOffset, newM.viewport.YOffset())
	}
	upOffset := newM.viewport.YOffset()

	newM, cmd = newM.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd != nil {
		t.Fatal("expected nil cmd")
	}
	if newM.viewport.YOffset() <= upOffset {
		t.Fatalf("expected viewport-focused down key to scroll down from %d, got %d", upOffset, newM.viewport.YOffset())
	}
}

func TestHandleKeyMsg_ViewportFocusTypingFocusesInputAndTypes(t *testing.T) {
	m := newTestModel()
	m.blurInput()

	newM, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !newM.textarea.Focused() {
		t.Fatal("expected typing to focus input")
	}
	if newM.textarea.Value() != "x" {
		t.Fatalf("expected typed character in input, got %q", newM.textarea.Value())
	}
	if cmd == nil {
		t.Fatal("expected focus/update command")
	}
}

func TestHandleKeyMsg_CtrlJ(t *testing.T) {
	ta := textarea.New()
	ta.Focus()
	ta.KeyMap.InsertNewline.SetKeys("ctrl+j")
	ta.KeyMap.InsertNewline.SetEnabled(true)
	ta.SetValue("line 1")
	ta.CursorEnd()
	m := replModel{
		textarea: ta,
		width:    80,
	}

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})

	if !strings.Contains(newM.textarea.Value(), "\n") {
		t.Error("expected newline in textarea after ctrl+j")
	}
}

func TestHandleKeyMsg_ModelSelectionMode(t *testing.T) {
	m := replModel{
		width:          80,
		modelSelection: &replwidgets.Model{},
	}

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'a', Text: "a"})

	if newM.modelSelection == nil {
		t.Error("expected modelSelection to remain set")
	}
}

func TestUpdateNormalMode_ModelSelectionPasteGoesToAPIKeyInput(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("existing prompt")
	m.modelSelection = &replwidgets.Model{
		Step: replwidgets.StepAPIKey,
	}

	newM, _ := m.updateNormalMode(tea.PasteMsg{Content: "sk-test-123"})

	if newM.modelSelection.APIKeyInput != "sk-test-123" {
		t.Fatalf("expected pasted API key to go to model selection, got %q", newM.modelSelection.APIKeyInput)
	}
	if newM.textarea.Value() != "existing prompt" {
		t.Fatalf("expected textarea to remain unchanged, got %q", newM.textarea.Value())
	}
}

func TestHandleLLMChunk_MultipleCalls(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	m := replModel{
		stream:  streamState{handler: sh},
		loading: loadingState{showSpinner: true},
		width:   80,
	}

	m, _ = m.handleLLMChunk("Hello")
	m, _ = m.handleLLMChunk(" ")
	m, _ = m.handleLLMChunk("World")

	if sh.GetResponse() != "Hello World" {
		t.Errorf("expected 'Hello World', got '%s'", sh.GetResponse())
	}

	if !m.loading.showSpinner {
		t.Error("loading.showSpinner should remain true during streaming")
	}
}

func TestHandleLLMDone_EmptyResponse(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	m := replModel{
		stream:   streamState{handler: sh},
		loading:  loadingState{showSpinner: true},
		width:    80,
		appState: replappstate.New(nil, t.TempDir()),
		output:   reploutput.NewOutputBuilder(80, ""),
	}

	newM, _ := m.handleLLMDone()

	if len(m.appState.GetMessages()) != 1 {
		t.Errorf("expected 1 message, got %d", len(m.appState.GetMessages()))
	}

	if len(newM.output.GetLines()) != 1 {
		t.Errorf("expected 1 line (trailing empty spacer), got %d", len(newM.output.GetLines()))
	}
}

func TestHandleLLMError_ResetsHandler(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")
	sh.HandleChunk("partial content")

	m := replModel{
		stream:   streamState{handler: sh},
		loading:  loadingState{showSpinner: true},
		width:    80,
		appState: replappstate.New(nil, t.TempDir()),
		output:   reploutput.NewOutputBuilder(80, ""),
	}

	newM, _ := m.handleLLMError(errors.New("fail"))

	if sh.IsActive() {
		t.Error("handler should not be active after error")
	}
	if sh.HasContent() {
		t.Error("handler should not have content after error")
	}

	_ = newM
}

func TestHandleLLMError_MaterializesMessageAndTurnMemory(t *testing.T) {
	workingDir := t.TempDir()
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")
	sh.HandleChunk("partial response")

	m := replModel{
		stream:   streamState{handler: sh},
		loading:  loadingState{showSpinner: true},
		width:    80,
		appState: replappstate.New(nil, workingDir),
		output:   reploutput.NewOutputBuilder(80, ""),
	}
	m.startAssistantTurnMemory()
	sh.HandleToolEnd(&llm.ToolCall{
		Name:  "write_file",
		Input: map[string]any{"path": workingDir + "/foo.go", "content": "package foo"},
	})

	updated, _ := m.handleLLMError(errors.New("rate limit"))

	messages := updated.appState.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 materialized assistant message, got %d", len(messages))
	}
	msg := messages[0]
	if msg.Role != llm.RoleAssistant {
		t.Errorf("expected assistant role, got %s", msg.Role)
	}
	if msg.Content != "partial response" {
		t.Errorf("expected partial response content, got %q", msg.Content)
	}
	if msg.TurnMemory == nil {
		t.Fatal("expected TurnMemory to be preserved, got nil")
	}
	if len(msg.TurnMemory.ToolActivity) != 1 || msg.TurnMemory.ToolActivity[0].Input["path"] != "foo.go" || msg.TurnMemory.ToolActivity[0].Input["content"] != "package foo" {
		t.Fatalf("expected write input activity, got %#v", msg.TurnMemory.ToolActivity)
	}
}

func TestHandleLLMError_ContextCanceled_DoesNotAddErrorLine(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")
	sh.HandleChunk("partial content")

	m := replModel{
		stream:   streamState{handler: sh, cancel: func() {}},
		loading:  loadingState{showSpinner: true},
		width:    80,
		appState: replappstate.New(nil, t.TempDir()),
		output:   reploutput.NewOutputBuilder(80, ""),
	}

	newM, cmd := m.handleLLMError(context.Canceled)

	if len(newM.output.GetLines()) != 1 {
		t.Fatalf("expected only pending transcript line, got %d", len(newM.output.GetLines()))
	}
	if strings.Contains(newM.output.Join(), "context canceled") {
		t.Error("expected cancellation to not render an error line")
	}
	if newM.stream.cancel != nil {
		t.Error("expected stream cancel function to be cleared")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestHandleToolStart(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	m := replModel{
		stream:  streamState{handler: sh},
		loading: loadingState{showSpinner: true},
		width:   80,
		output:  reploutput.NewOutputBuilder(80, ""),
	}

	toolCall := &llm.ToolCall{
		Name:  "test_tool",
		Input: map[string]any{"arg1": "value1"},
	}

	newM, cmd := m.handleToolStart(toolCall)

	if !newM.loading.showSpinner {
		t.Error("expected showSpinner to remain true after tool start")
	}

	if len(newM.output.GetLines()) != 0 {
		t.Errorf("expected no persisted output lines for tool start, got %d", len(newM.output.GetLines()))
	}

	if cmd == nil {
		t.Error("expected non-nil cmd from handleToolStart")
	}

	if len(sh.segments) != 1 {
		t.Errorf("expected 1 stream segment in handler, got %d", len(sh.segments))
	}

	if sh.segments[0].kind != segmentToolStart {
		t.Errorf("expected first segment kind %q, got %q", segmentToolStart, sh.segments[0].kind)
	}
}

func TestHandleAskUserToolActivityIsRecordedWithoutGenericStatus(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(chan llm.StreamEvent), "Loading...")
	m := replModel{stream: streamState{handler: sh}, width: 80, output: reploutput.NewOutputBuilder(80, "")}
	call := &llm.ToolCall{Name: tools.AskUserToolName, Input: map[string]any{"questions": []any{map[string]any{"question": "Pick", "options": []any{"one", "two"}}}}}

	m.handleToolStart(call)
	m.handleToolEnd(&llm.ToolCall{Name: tools.AskUserToolName, Input: call.Input, Output: map[string]any{"tool": tools.AskUserToolName, "answers": []string{"one"}}})
	if len(sh.segments) != 2 {
		t.Fatalf("expected ask_user start and end segments, got %#v", sh.segments)
	}
	if got := sh.renderTranscriptLines(); len(got) != 0 {
		t.Fatalf("expected no generic ask_user status, got %q", got)
	}
}

func TestHandleToolStart_BashKeepsSpinnerActive(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	m := replModel{
		stream:  streamState{handler: sh},
		loading: loadingState{showSpinner: true},
		width:   80,
		output:  reploutput.NewOutputBuilder(80, ""),
	}

	toolCall := &llm.ToolCall{
		Name:  "bash",
		Input: map[string]any{"command": "npm test", "summary": "running tests"},
	}

	newM, cmd := m.handleToolStart(toolCall)

	if !newM.loading.showSpinner {
		t.Error("expected showSpinner to remain true for running bash")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd from handleToolStart")
	}
	if len(sh.segments) != 1 || sh.segments[0].kind != segmentBash {
		t.Fatalf("expected a bash segment to be added")
	}
}

func TestHandleToolEnd(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	m := replModel{
		stream: streamState{handler: sh},
		width:  80,
		output: reploutput.NewOutputBuilder(80, ""),
	}

	toolCall := &llm.ToolCall{
		Name:     "test_tool",
		Input:    map[string]any{"arg1": "value1"},
		Output:   "tool result",
		Duration: 1500000000,
	}

	newM, cmd := m.handleToolEnd(toolCall)

	if len(newM.output.GetLines()) != 0 {
		t.Errorf("expected no persisted output lines for tool end, got %d", len(newM.output.GetLines()))
	}

	if cmd == nil {
		t.Error("expected non-nil cmd from handleToolEnd")
	}

	if len(sh.segments) != 1 {
		t.Errorf("expected 1 stream segment in handler, got %d", len(sh.segments))
	}

	if sh.segments[0].kind != segmentToolEnd {
		t.Errorf("expected first segment kind %q, got %q", segmentToolEnd, sh.segments[0].kind)
	}
}

func TestHandleToolEnd_WithError(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	m := replModel{
		stream: streamState{handler: sh},
		width:  80,
		output: reploutput.NewOutputBuilder(80, ""),
	}

	toolCall := &llm.ToolCall{
		Name:  "test_tool",
		Input: map[string]any{"arg1": "value1"},
		Error: "connection failed",
	}

	newM, cmd := m.handleToolEnd(toolCall)

	if len(newM.output.GetLines()) != 0 {
		t.Errorf("expected no persisted output lines for tool end, got %d", len(newM.output.GetLines()))
	}

	if cmd == nil {
		t.Error("expected non-nil cmd from handleToolEnd")
	}

	if len(sh.segments) != 1 {
		t.Errorf("expected 1 stream segment in handler, got %d", len(sh.segments))
	}

	if sh.segments[0].kind != segmentToolEnd {
		t.Errorf("expected first segment kind %q, got %q", segmentToolEnd, sh.segments[0].kind)
	}

	if sh.segments[0].toolCall == nil || sh.segments[0].toolCall.Error != "connection failed" {
		t.Errorf("expected tool end segment with error details")
	}
}

func TestHandleLLMStreamMsg_ToolEnd_ReturnsSpinnerTick(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	m := newTestModel()
	m.stream.handler = sh
	m.loading.showSpinner = true

	toolCall := &llm.ToolCall{
		Name:   "test_tool",
		Input:  map[string]any{"arg1": "value1"},
		Output: "tool result",
	}

	updated, cmd, handled := m.handleLLMStreamMsg(llmToolEndMsg{toolCall: toolCall})

	if !handled {
		t.Error("expected tool end msg to be handled")
	}

	if !updated.loading.showSpinner {
		t.Error("expected showSpinner to remain true after tool end")
	}

	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
}

func TestHandleBtwStreamMsg_Chunk(t *testing.T) {
	btwSh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	btwSh.Start(eventCh, "Loading...")

	m := newTestModel()
	m.btw.streamHandler = btwSh

	updated, cmd, handled := m.handleBtwStreamMsg(btwChunkMsg("hello"))

	if !handled {
		t.Fatal("expected btw chunk msg to be handled")
	}
	if btwSh.GetResponse() != "hello" {
		t.Fatalf("expected btw response 'hello', got %q", btwSh.GetResponse())
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for btw chunk")
	}
	_ = updated
}

func TestHandleBtwStreamMsg_Done(t *testing.T) {
	btwSh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	btwSh.Start(eventCh, "Loading...")
	btwSh.HandleChunk("answer text")

	m := newTestModel()
	m.btw.streamHandler = btwSh
	m.btw.showSpinner = true
	m.btw.question = "what?"

	updated, cmd, handled := m.handleBtwStreamMsg(btwDoneMsg{})

	if !handled {
		t.Fatal("expected btw done msg to be handled")
	}
	if updated.btw.showSpinner {
		t.Fatal("expected btw spinner to stop after done")
	}
	if updated.btw.lines == nil {
		t.Fatal("expected btw lines to be set after done")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd after btw done")
	}
}

func TestHandleBtwStreamMsg_Error(t *testing.T) {
	btwSh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	btwSh.Start(eventCh, "Loading...")

	m := newTestModel()
	m.btw.streamHandler = btwSh
	m.btw.showSpinner = true
	m.btw.question = "question"

	updated, cmd, handled := m.handleBtwStreamMsg(btwErrorMsg{err: errors.New("oops")})

	if !handled {
		t.Fatal("expected btw error msg to be handled")
	}
	if updated.btw.showSpinner {
		t.Fatal("expected btw spinner to stop after error")
	}
	if updated.btw.lines == nil {
		t.Fatal("expected btw lines to be set after error")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd after btw error")
	}
}

func TestHandleBtwStreamMsg_InactiveHandlerSwallowsMessages(t *testing.T) {
	btwSh := NewStreamHandler(nil)

	m := newTestModel()
	m.btw.streamHandler = btwSh

	_, _, handled := m.handleBtwStreamMsg(btwChunkMsg("orphan"))

	if !handled {
		t.Fatal("expected stale btw chunk to be swallowed")
	}
}

func newTestModelWithFileSearcher(t *testing.T) (replModel, string) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.txt"), []byte("foo"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.go"), []byte("bar"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}

	m := newTestModel()
	m.fileSearcher = replfilesearch.NewFileSearcher(dir, nil)
	return m, dir
}

func TestHandleKeyMsg_SlashCommandStillShowsFileSuggestions(t *testing.T) {
	m, _ := newTestModelWithFileSearcher(t)
	m.textarea.SetValue("/cmd @fo")
	m.textarea.SetCursorColumn(len("/cmd @fo"))

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'o', Text: "o"})

	if newM.textarea.Value() != "/cmd @foo" {
		t.Fatalf("expected textarea value '/cmd @foo', got %q", newM.textarea.Value())
	}
	if !newM.suggestion.Visible() {
		t.Fatal("expected file suggestions to be visible for slash command with @ token")
	}
	if !newM.suggestion.IsFileMode() {
		t.Fatal("expected suggestions to be in file mode")
	}
	if !strings.Contains(newM.suggestion.View(80), "foo.txt") {
		t.Fatalf("expected foo.txt in suggestions, got %q", newM.suggestion.View(80))
	}
}

func TestHandleKeyMsg_SlashCommandWithoutAtShowsCommandSuggestions(t *testing.T) {
	m, _ := newTestModelWithFileSearcher(t)
	m.textarea.SetValue("/cl")
	m.textarea.SetCursorColumn(len("/cl"))

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'e', Text: "e"})

	if newM.textarea.Value() != "/cle" {
		t.Fatalf("expected textarea value '/cle', got %q", newM.textarea.Value())
	}
	if !newM.suggestion.Visible() {
		t.Fatal("expected command suggestions to be visible")
	}
	if newM.suggestion.IsFileMode() {
		t.Fatal("expected suggestions to be in command mode")
	}
	if !strings.Contains(newM.suggestion.View(80), "/clear") {
		t.Fatalf("expected /clear in suggestions, got %q", newM.suggestion.View(80))
	}
}

func TestAutoCompactionEscCancelsOnlyChild(t *testing.T) {
	m := newTestModel()
	m.compaction.active = true
	m.compaction.mode = compactionAutomatic
	childCancelled := false
	parentCancelled := false
	m.compaction.cancel = func() { childCancelled = true }
	m.stream.cancel = func() { parentCancelled = true }

	updated, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})

	if !childCancelled {
		t.Fatal("Esc did not cancel automatic compaction child")
	}
	if parentCancelled {
		t.Fatal("Esc cancelled the parent stream")
	}
	if updated.compaction.cancel != nil {
		t.Fatal("child cancellation callback was not cleared")
	}
}

func TestAutoCompactionAppliedStripsSystemMessagesFromAppState(t *testing.T) {
	m := newTestModel()
	m.stream.handler.Start(make(chan llm.StreamEvent), "Working...")
	m.compaction.active = true
	m.compaction.mode = compactionAutomatic

	updated, _ := m.handleAutoCompactionApplied(&llm.AutoCompactionEvent{Replacement: []llm.Message{
		{Role: llm.RoleSystem, Content: "provider system prompt"},
		{Role: llm.RoleUser, Content: "checkpoint"},
	}})

	messages := updated.appState.GetMessages()
	if len(messages) != 1 || messages[0].Role != llm.RoleUser || messages[0].Content != "checkpoint" {
		t.Fatalf("AppState messages = %#v, want system-free replacement", messages)
	}
}

func TestAutoCompactionAppliedReplacesHistoryWithoutAppendingCheckpoint(t *testing.T) {
	m := newTestModel()
	m.stream.handler.Start(make(chan llm.StreamEvent), "Working...")
	m.stream.handler.HandleChunk("visible work")
	m.appState.AddMessage(llm.RoleUser, "original task")
	m.compaction.active = true
	m.compaction.mode = compactionAutomatic
	replacement := []llm.Message{{Role: llm.RoleUser, Content: "checkpoint"}}

	updated, _ := m.handleAutoCompactionApplied(&llm.AutoCompactionEvent{Replacement: replacement})

	messages := updated.appState.GetMessages()
	if len(messages) != 1 || messages[0].Content != "checkpoint" {
		t.Fatalf("history = %#v, want replacement only", messages)
	}
	if updated.stream.handler.GetResponse() != "" {
		t.Fatal("checkpoint content remained in active handler")
	}
	if updated.compaction.active || updated.compaction.mode != compactionNone {
		t.Fatal("automatic compaction state was not cleared")
	}
	if updated.loading.text == "Compacting..." {
		t.Fatal("loader was not restored")
	}
	if updated.notification.text != "Context compacted automatically." {
		t.Fatalf("notification = %q", updated.notification.text)
	}
}

func TestHandleLLMReasoningChunk(t *testing.T) {
	m := newTestModel()
	m.stream.handler.Start(make(chan llm.StreamEvent), "Working...")

	updated, cmd := m.handleLLMReasoningChunk("considering options")

	if !updated.stream.handler.HasContent() {
		t.Fatal("reasoning chunk did not activate stream content")
	}
	if cmd == nil {
		t.Fatal("reasoning chunk did not schedule the next event")
	}
}

func TestHandleLLMIncompletePreservesPartialResponse(t *testing.T) {
	m := newTestModel()
	m.stream.handler.Start(make(chan llm.StreamEvent), "Working...")
	m.stream.handler.HandleChunk("partial answer")
	m.loading.showSpinner = true

	updated, cmd := m.handleLLMIncomplete(errors.New("response truncated"))

	if updated.loading.showSpinner {
		t.Fatal("incomplete response left spinner active")
	}
	if got := updated.output.Join(); !strings.Contains(got, "partial answer") || !strings.Contains(got, "response truncated") {
		t.Fatalf("incomplete output = %q", got)
	}
	if messages := updated.appState.GetMessages(); len(messages) != 0 {
		t.Fatalf("incomplete response unexpectedly changed conversation: %#v", messages)
	}
	if cmd != nil {
		t.Fatal("incomplete response without queued input returned a command")
	}
}

func TestHandleLLMIncompleteCancellationOmitsError(t *testing.T) {
	m := newTestModel()
	m.stream.handler.Start(make(chan llm.StreamEvent), "Working...")
	m.stream.handler.HandleChunk("partial answer")

	updated, _ := m.handleLLMIncomplete(context.Canceled)

	if got := updated.output.Join(); strings.Contains(got, "context canceled") {
		t.Fatalf("cancellation rendered as an error: %q", got)
	}
}

func TestHandleLLMIncompleteClearsAskUser(t *testing.T) {
	m := newTestModel()
	m.stream.handler.Start(make(chan llm.StreamEvent), "Working...")
	m.askUser = testAskUserState()
	m.stream.handler.SetAskUser(&m.askUser)

	updated, _ := m.handleLLMIncomplete(errors.New("response truncated"))
	if updated.askUser.active() {
		t.Fatal("incomplete response left questionnaire active")
	}
}

func TestHandleLLMErrorClearsAskUser(t *testing.T) {
	m := newTestModel()
	m.stream.handler.Start(make(chan llm.StreamEvent), "Working...")
	m.askUser = testAskUserState()
	m.stream.handler.SetAskUser(&m.askUser)

	updated, _ := m.handleLLMError(errors.New("stream failed"))
	if updated.askUser.active() {
		t.Fatal("stream error left questionnaire active")
	}
}

func TestHandleLLMRetryRewindsCurrentAttempt(t *testing.T) {
	m := newTestModel()
	m.stream.handler.Start(make(chan llm.StreamEvent), "Working...")
	m.stream.handler.HandleChunk("discarded attempt")

	updated, cmd := m.handleLLMRetry(errors.New("temporary failure"), 3)

	if got := updated.stream.handler.GetResponse(); got != "" {
		t.Fatalf("retry retained response %q", got)
	}
	if updated.loading.text != "Retrying (attempt 3)..." {
		t.Fatalf("loading text = %q", updated.loading.text)
	}
	if cmd == nil {
		t.Fatal("retry did not schedule the next event")
	}
}

func TestAutomaticCompactionLifecycle(t *testing.T) {
	m := newTestModel()
	m.stream.handler.Start(make(chan llm.StreamEvent), "Working...")
	cancelled := false

	started, cmd := m.handleAutoCompactionStarted(&llm.AutoCompactionEvent{Cancel: func() { cancelled = true }})
	if !started.compaction.active || started.compaction.mode != compactionAutomatic || started.loading.text != "Compacting..." {
		t.Fatalf("unexpected started state: %#v", started.compaction)
	}
	if cmd == nil {
		t.Fatal("start did not schedule the next stream event")
	}
	started.compaction.cancel()
	if !cancelled {
		t.Fatal("automatic compaction cancel callback was not retained")
	}

	stopped, cmd := started.handleAutoCompactionStopped()
	if stopped.compaction.active || stopped.compaction.mode != compactionNone || stopped.compaction.cancel != nil {
		t.Fatalf("unexpected stopped state: %#v", stopped.compaction)
	}
	if stopped.loading.text == "Compacting..." {
		t.Fatal("stopped compaction retained compaction loading text")
	}
	if cmd == nil {
		t.Fatal("stop did not resume waiting for stream events")
	}
}

func TestAutoCompactionAppliedWithoutReplacementStops(t *testing.T) {
	m := newTestModel()
	m.stream.handler.Start(make(chan llm.StreamEvent), "Working...")
	m.compaction = compactionState{active: true, mode: compactionAutomatic}

	updated, cmd := m.handleAutoCompactionApplied(nil)

	if updated.compaction.active || updated.compaction.mode != compactionNone {
		t.Fatal("empty compaction replacement did not stop compaction")
	}
	if cmd == nil {
		t.Fatal("empty replacement did not resume stream waiting")
	}
}

func TestHandleCompactionErrorReportsFailureAndFlushesStream(t *testing.T) {
	m := newTestModel()
	m.compaction = compactionState{active: true, mode: compactionManual, cancel: func() {}}
	m.loading.showSpinner = true
	m.stream.handler.Start(make(chan llm.StreamEvent), "Compacting...")
	m.stream.handler.HandleChunk("unfinished summary")

	updated, _ := m.handleCompactionError(errors.New("service unavailable"))

	if updated.compaction.active || updated.loading.showSpinner || updated.compaction.cancel != nil {
		t.Fatal("failed compaction did not reset state")
	}
	got := updated.output.Join()
	if !strings.Contains(got, "unfinished summary") || !strings.Contains(got, "Compaction failed: service unavailable") {
		t.Fatalf("compaction failure output = %q", got)
	}
}

func TestSanitizeDelegateToolCall(t *testing.T) {
	original := &llm.ToolCall{Name: "delegate_task", Output: map[string]any{
		"completed":          2,
		"failed":             1,
		"completed_by_agent": map[string]any{"reviewer": 2},
		"failed_by_agent":    map[string]any{"tester": 1},
		"results":            []any{"large detail"},
	}}

	sanitized := sanitizeDelegateToolCall(original)
	output, ok := sanitized.Output.(map[string]any)
	if !ok {
		t.Fatalf("sanitized output type = %T", sanitized.Output)
	}
	if len(output) != 4 || output["completed"] != 2 {
		t.Fatalf("sanitized output = %#v", output)
	}
	if _, exists := output["results"]; exists {
		t.Fatal("sanitized output retained verbose results")
	}
	if _, exists := original.Output.(map[string]any)["results"]; !exists {
		t.Fatal("sanitization mutated original tool call")
	}

	plain := &llm.ToolCall{Name: "read_file", Output: "content"}
	if sanitizeDelegateToolCall(plain) != plain || sanitizeDelegateToolCall(nil) != nil {
		t.Fatal("non-delegate tool call was cloned")
	}
	delegateWithText := &llm.ToolCall{Name: "delegate_task", Output: "summary"}
	if got := sanitizeDelegateToolCall(delegateWithText); got == delegateWithText || got.Output != "summary" {
		t.Fatal("delegate call with text output was not safely cloned")
	}
}

func TestExtractAtToken(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		cursor int
		token  string
		start  int
		found  bool
	}{
		{name: "start", input: "@main.go", cursor: 8, token: "main.go", found: true},
		{name: "after space", input: "read @internal", cursor: 14, token: "internal", start: 5, found: true},
		{name: "invalid cursor", input: "@file", cursor: 7},
		{name: "missing marker", input: "file", cursor: 4},
		{name: "embedded marker", input: "email@example", cursor: 13},
		{name: "empty token", input: "read @", cursor: 6},
		{name: "space in token", input: "@one two", cursor: 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, start, found := extractAtToken(tt.input, tt.cursor)
			if token != tt.token || start != tt.start || found != tt.found {
				t.Fatalf("extractAtToken() = (%q, %d, %v), want (%q, %d, %v)", token, start, found, tt.token, tt.start, tt.found)
			}
		})
	}
}

func TestHandleFileModeSelectionReplacesToken(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("inspect @mai later")
	m.textarea.SetCursorColumn(len("inspect @mai"))
	m.suggestion.RefreshFiles([]string{"main.go"})

	updated, cmd := m.handleFileModeSelection()

	if got := updated.textarea.Value(); got != "inspect @main.go  later" {
		t.Fatalf("textarea = %q", got)
	}
	if updated.suggestion.Visible() {
		t.Fatal("file suggestions remained visible")
	}
	if cmd != nil {
		t.Fatal("file selection returned a command")
	}
}

func TestHandleSuggestionNavigationAndSelection(t *testing.T) {
	m := newTestModel()
	m.suggestion.RefreshFiles([]string{"first.go", "second.go"})

	handled, moved, _ := m.handleSuggestionKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	if !handled || moved.suggestion.Current() == nil || moved.suggestion.Current().Name != "second.go" {
		t.Fatal("down key did not move file suggestion")
	}
	handled, moved, _ = moved.handleSuggestionKeyMsg(tea.KeyPressMsg{Code: tea.KeyUp})
	if !handled || moved.suggestion.Current() == nil || moved.suggestion.Current().Name != "first.go" {
		t.Fatal("up key did not move file suggestion")
	}
	handled, selected, _ := moved.handleSuggestionKeyMsg(tea.KeyPressMsg{Code: tea.KeyTab})
	if !handled || selected.textarea.Value() != "" {
		t.Fatal("file selection without an @ token changed input")
	}
	if selected.suggestion.Visible() {
		t.Fatal("tab selection did not hide suggestions")
	}
}

func TestHandleKeyMsgViewportNavigation(t *testing.T) {
	m := newTestModel()
	m.viewport.SetHeight(4)
	for range 30 {
		m.output.AddLine("line")
	}
	m.updateViewportContent()
	m.viewport.GotoBottom()

	pagedUp, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if pagedUp.viewport.AtBottom() || !pagedUp.userScrolled {
		t.Fatal("page up did not leave follow mode")
	}
	upOffset := pagedUp.viewport.YOffset()
	pagedDown, _ := pagedUp.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if pagedDown.viewport.YOffset() <= upOffset {
		t.Fatal("page down did not advance viewport")
	}
	home, _ := pagedDown.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyHome})
	if home.viewport.YOffset() != 0 || !home.userScrolled {
		t.Fatal("home did not move to top")
	}
	end, _ := home.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnd})
	if !end.viewport.AtBottom() || end.userScrolled {
		t.Fatal("end did not restore follow mode")
	}
}

func TestHandleKeyMsgClearsQueuedInputs(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: 'c', Mod: tea.ModCtrl}, {Code: tea.KeyEsc}} {
		m := newTestModel()
		m.queuedInputs = []string{"first", "second"}
		updated, cmd := m.handleKeyMsg(key)
		if len(updated.queuedInputs) != 0 {
			t.Fatalf("%s left queued inputs: %v", key.String(), updated.queuedInputs)
		}
		if cmd == nil {
			t.Fatalf("%s did not return queue-cleared notification", key.String())
		}
	}
}

func TestHandleSessionPickerNavigationAndCancel(t *testing.T) {
	m := newTestModel()
	m.sessionPicker = replwidgets.NewSessionPicker([]session.Summary{
		{ID: "first", LastUserMessage: "first prompt"},
		{ID: "second", LastUserMessage: "second prompt"},
	})

	updated, _ := m.handleSessionPickerKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	if current := updated.sessionPicker.Current(); current == nil || current.ID != "second" {
		t.Fatalf("down selected %#v", current)
	}
	updated, _ = updated.handleSessionPickerKeyMsg(tea.KeyPressMsg{Code: tea.KeyUp})
	if current := updated.sessionPicker.Current(); current == nil || current.ID != "first" {
		t.Fatalf("up selected %#v", current)
	}
	updated, _ = updated.handleSessionPickerKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})
	if updated.sessionPicker != nil {
		t.Fatal("escape did not close session picker")
	}
}

func TestHandleSessionPickerIgnoresNonKeyAndEmptySelection(t *testing.T) {
	m := newTestModel()
	m.sessionPicker = replwidgets.NewSessionPicker(nil)
	updated, cmd := m.handleSessionPickerKeyMsg(tea.WindowSizeMsg{})
	if updated.sessionPicker == nil || cmd != nil {
		t.Fatal("non-key message changed session picker")
	}
	updated, cmd = updated.handleSessionPickerKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.sessionPicker == nil || cmd != nil {
		t.Fatal("enter on empty picker changed state")
	}
}

func TestHandleLLMStreamMsgRoutesMainEvents(t *testing.T) {
	tests := []struct {
		name  string
		event llm.StreamEvent
		check func(*testing.T, replModel)
	}{
		{name: "chunk", event: llm.StreamEvent{Type: llm.StreamEventTypeChunk, Content: "answer"}, check: func(t *testing.T, m replModel) {
			if m.stream.handler.GetResponse() != "answer" {
				t.Fatal("chunk was not routed")
			}
		}},
		{name: "reasoning", event: llm.StreamEvent{Type: llm.StreamEventTypeReasoningChunk, Content: "thought"}, check: func(t *testing.T, m replModel) {
			if !m.stream.handler.HasContent() {
				t.Fatal("reasoning was not routed")
			}
		}},
		{name: "usage", event: llm.StreamEvent{Type: llm.StreamEventTypeUsage, Usage: &llm.TokenUsage{InputTokens: 7}}, check: func(t *testing.T, m replModel) {
			if m.appState.GetLastUsage() == nil || m.appState.GetLastUsage().InputTokens != 7 {
				t.Fatal("usage was not routed")
			}
		}},
		{name: "retry", event: llm.StreamEvent{Type: llm.StreamEventTypeRetry, Attempt: 2}, check: func(t *testing.T, m replModel) {
			if m.loading.text != "Retrying (attempt 2)..." {
				t.Fatalf("loading text = %q", m.loading.text)
			}
		}},
		{name: "tool start", event: llm.StreamEvent{Type: llm.StreamEventTypeToolStart, ToolCall: &llm.ToolCall{Name: "read_file"}}, check: func(t *testing.T, m replModel) {
			if len(m.stream.handler.segments) != 1 {
				t.Fatal("tool start was not routed")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			events := make(chan llm.StreamEvent)
			m.stream.handler.Start(events, "Working...")
			updated, _, handled := m.handleLLMStreamMsg(mainStreamMsg{eventCh: events, event: tt.event})
			if !handled {
				t.Fatal("main stream event was not handled")
			}
			tt.check(t, updated)
		})
	}
}

func TestHandleLLMStreamMsgInactiveStreamSwallowsLateEvents(t *testing.T) {
	messages := []tea.Msg{
		llmChunkMsg("late"), llmReasoningChunkMsg("late"), llmDoneMsg{},
		llmIncompleteMsg{err: errors.New("late")}, llmErrorMsg{err: errors.New("late")},
		llmRetryMsg{attempt: 2}, llmToolStartMsg{}, llmToolEndMsg{}, llmUsageMsg{},
		llmAutoCompactionStartedMsg{}, llmAutoCompactionAppliedMsg{},
		llmAutoCompactionCancelledMsg{}, llmAutoCompactionFailedMsg{},
	}
	for _, msg := range messages {
		m := newTestModel()
		_, cmd, handled := m.handleLLMStreamMsg(msg)
		if !handled || cmd != nil {
			t.Fatalf("late %T = handled %v, cmd %v", msg, handled, cmd)
		}
	}
}

func TestHandleAdversaryStreamLifecycle(t *testing.T) {
	m := newTestModel()
	events := make(chan llm.StreamEvent)
	m.adversary.streamHandler = NewStreamHandler(nil)
	m.adversary.streamHandler.Start(events, "Reviewing...")
	m.adversary.showSpinner = true

	updated, cmd, handled := m.handleAdversaryStreamMsg(adversaryChunkMsg("review text"))
	if !handled || cmd == nil || updated.adversary.streamHandler.GetResponse() != "review text" {
		t.Fatal("adversary chunk was not handled")
	}
	updated, cmd, handled = updated.handleAdversaryStreamMsg(adversaryToolStartMsg{toolCall: &llm.ToolCall{Name: "read_file"}})
	if !handled || cmd == nil || len(updated.adversary.streamHandler.segments) < 2 {
		t.Fatal("adversary tool start was not handled")
	}
	updated, cmd, handled = updated.handleAdversaryStreamMsg(adversaryToolEndMsg{toolCall: &llm.ToolCall{Name: "read_file", Output: "ok"}})
	if !handled || cmd == nil {
		t.Fatal("adversary tool end was not handled")
	}
	updated, cmd, handled = updated.handleAdversaryStreamMsg(adversaryDoneMsg{})
	if !handled || cmd != nil || updated.adversary.showSpinner || len(updated.adversary.lines) == 0 {
		t.Fatal("adversary completion did not finalize output")
	}
}

func TestHandleAdversaryStreamErrorAndStaleMessages(t *testing.T) {
	m := newTestModel()
	m.adversary.streamHandler = NewStreamHandler(nil)
	m.adversary.streamHandler.Start(make(chan llm.StreamEvent), "Reviewing...")
	m.adversary.showSpinner = true

	updated, cmd, handled := m.handleAdversaryStreamMsg(adversaryErrorMsg{err: errors.New("review failed")})
	if !handled || cmd != nil || updated.adversary.showSpinner || !strings.Contains(strings.Join(updated.adversary.lines, "\n"), "review failed") {
		t.Fatal("adversary error did not finalize with error output")
	}
	for _, msg := range []tea.Msg{adversaryChunkMsg("late"), adversaryDoneMsg{}, adversaryErrorMsg{}, adversaryToolStartMsg{}, adversaryToolEndMsg{}} {
		_, cmd, handled = updated.handleAdversaryStreamMsg(msg)
		if !handled || cmd != nil {
			t.Fatalf("stale %T was not swallowed", msg)
		}
	}
	if _, _, handled := updated.handleAdversaryStreamMsg(tea.WindowSizeMsg{}); handled {
		t.Fatal("unrelated message was handled by inactive adversary stream")
	}
}
