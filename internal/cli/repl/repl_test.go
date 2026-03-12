package repl

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/user/keen-code/configs/providers"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/llm"
)

func newTestModel() replModel {
	ta := textarea.New()
	ta.Focus()
	ta.SetWidth(80)
	ta.SetHeight(maxHeight)
	ta.MaxHeight = 0
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	return replModel{
		textarea:            ta,
		viewport:            vp,
		ctx:                 &replContext{cfg: &config.ResolvedConfig{}},
		appState:            NewAppState(nil),
		output:              NewOutputBuilder(80),
		streamHandler:       NewStreamHandler(nil),
		permissionRequester: NewREPLPermissionRequester(),
		spinner:             spinner.New(),
		width:               80,
		height:              30,
	}
}

func TestUpdate_InlinePermission_AllowsToolStartEvent(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	req := &PermissionRequest{
		RequestID:    "1",
		ToolName:     "read_file",
		Path:         "../foo.txt",
		ResolvedPath: "/tmp/foo.txt",
		Status:       PermissionStatusPending,
		ResponseChan: make(chan bool, 1),
	}
	sh.HandlePermissionRequest(req)

	m := replModel{
		streamHandler: sh,
		showSpinner:   true,
		width:         80,
		output:        NewOutputBuilder(80),
	}

	toolCall := &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "../foo.txt"}}
	updatedModel, cmd := m.Update(llmToolStartMsg{toolCall: toolCall})

	updated, ok := updatedModel.(*replModel)
	if !ok {
		t.Fatalf("expected *replModel, got %T", updatedModel)
	}

	if updated.showSpinner {
		t.Error("expected showSpinner to be false after tool start while permission is pending")
	}

	if len(updated.output.GetLines()) != 0 {
		t.Errorf("expected no persisted output line for tool start, got %d", len(updated.output.GetLines()))
	}

	if cmd == nil {
		t.Error("expected non-nil cmd when handling tool start event")
	}
}

func TestHandleEnterKey_EmptyInput(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("")

	newM, cmd := m.handleEnterKey()

	if cmd != nil {
		t.Error("expected nil cmd for empty input")
	}
	if len(newM.output.GetLines()) != 0 {
		t.Error("expected no output for empty input")
	}
}

func TestHandleEnterKey_ActiveStream(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("some input")
	eventCh := make(chan llm.StreamEvent)
	m.streamHandler.Start(eventCh, "Loading...")

	newM, cmd := m.handleEnterKey()

	if cmd != nil {
		t.Error("expected nil cmd when stream is active")
	}
	if newM.textarea.Value() != "some input" {
		t.Error("expected textarea to remain unchanged when stream is active")
	}
}

func TestHandleEnterKey_ExitCommand(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue(exitCommand)

	newM, cmd := m.handleEnterKey()

	if !newM.quitting {
		t.Error("expected quitting to be true")
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestHandleEnterKey_HelpCommand(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue(helpCommand)

	newM, _ := m.handleEnterKey()

	if !strings.Contains(newM.output.Join(), "Available Commands") {
		t.Error("expected help text in output")
	}
	if newM.textarea.Value() != "" {
		t.Error("expected textarea to be reset after help command")
	}
}

func TestHandleEnterKey_ModelCommand(t *testing.T) {
	m := newTestModel()
	m.ctx.registry = &providers.Registry{Providers: []providers.Provider{}}
	m.ctx.globalCfg = &config.GlobalConfig{}
	m.ctx.loader = config.NewLoader()
	m.textarea.SetValue(modelCommand)

	newM, _ := m.handleEnterKey()

	if newM.modelSelection == nil {
		t.Error("expected model selection to be started")
	}
	if newM.textarea.Value() != "" {
		t.Error("expected textarea to be reset")
	}
}

func TestHandleEnterKey_ClientNotReady(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("hello there")

	newM, _ := m.handleEnterKey()

	found := false
	for _, line := range newM.output.GetLines() {
		if strings.Contains(line, "LLM client not initialized") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error about LLM client not initialized")
	}
	if newM.textarea.Value() != "" {
		t.Error("expected textarea to be reset")
	}
}

func TestAdjustTextareaHeight(t *testing.T) {
	m := newTestModel()
	m.textarea.SetHeight(1)
	m.adjustTextareaHeight()

	if m.textarea.Height() != maxHeight {
		t.Errorf("expected textarea height %d, got %d", maxHeight, m.textarea.Height())
	}
	expectedVPHeight := m.height - m.textarea.Height() - 4
	if m.viewport.Height() != expectedVPHeight {
		t.Errorf("expected viewport height %d, got %d", expectedVPHeight, m.viewport.Height())
	}
}

func TestAdjustTextareaHeight_ZeroHeight(t *testing.T) {
	m := newTestModel()
	m.height = 0
	m.adjustTextareaHeight()

	if m.textarea.Height() != maxHeight {
		t.Errorf("expected textarea height %d, got %d", maxHeight, m.textarea.Height())
	}
}

func TestIsAtTopOfInput(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("line1")

	if !m.isAtTopOfInput() {
		t.Error("expected isAtTopOfInput to be true for single line")
	}
}

func TestIsAtBottomOfInput(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("line1")

	if !m.isAtBottomOfInput() {
		t.Error("expected isAtBottomOfInput to be true for single line")
	}
}

func TestUpdateNormalMode_WindowResize(t *testing.T) {
	m := newTestModel()

	resizeMsg := tea.WindowSizeMsg{Width: 100, Height: 40}
	newM, cmd := m.updateNormalMode(resizeMsg)

	if newM.width != 100 {
		t.Errorf("expected width 100, got %d", newM.width)
	}
	if newM.height != 40 {
		t.Errorf("expected height 40, got %d", newM.height)
	}
	if cmd != nil {
		t.Error("expected nil cmd for window resize")
	}
}

func TestUpdate_RoutesToNormalMode(t *testing.T) {
	m := newTestModel()

	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	updated := result.(*replModel)

	if updated.width != 100 {
		t.Errorf("expected width 100, got %d", updated.width)
	}
}

func TestUpdate_RoutesToPermissionHandling(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.streamHandler.Start(eventCh, "Loading...")

	req := &PermissionRequest{
		RequestID:    "1",
		ToolName:     "read_file",
		Path:         "foo.txt",
		ResolvedPath: "/resolved/foo.txt",
		Status:       PermissionStatusPending,
		ResponseChan: make(chan bool, 1),
	}
	m.streamHandler.HandlePermissionRequest(req)

	if !m.streamHandler.HasPendingPermission() {
		t.Fatal("expected pending permission")
	}

	// Pressing 'j' should move the cursor down
	result, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	updated := result.(*replModel)

	if !updated.streamHandler.HasPendingPermission() {
		t.Error("expected pending permission to remain after 'j' key")
	}
}

func TestHandleLLMStreamMsg_UnknownMsg(t *testing.T) {
	m := newTestModel()
	_, _, handled := m.handleLLMStreamMsg(tea.WindowSizeMsg{})

	if handled {
		t.Error("expected unknown msg to not be handled")
	}
}

func TestHandleLLMStreamMsg_RoutesChunk(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.streamHandler.Start(eventCh, "Loading...")
	m.showSpinner = true

	newM, _, handled := m.handleLLMStreamMsg(llmChunkMsg("hello"))

	if !handled {
		t.Error("expected chunk msg to be handled")
	}
	if newM.showSpinner {
		t.Error("expected showSpinner false after chunk")
	}
}

func TestGetHelpText(t *testing.T) {
	text := getHelpText()

	if !strings.Contains(text, "/help") {
		t.Error("expected /help in help text")
	}
	if !strings.Contains(text, "/model") {
		t.Error("expected /model in help text")
	}
	if !strings.Contains(text, "/exit") {
		t.Error("expected /exit in help text")
	}
}
