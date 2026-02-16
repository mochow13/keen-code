package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/user/keen-cli/internal/cli/modelselection"
	"github.com/user/keen-cli/internal/llm"
)

func TestHandleLLMChunk(t *testing.T) {
	sh := NewStreamHandler()
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	m := replModel{
		streamHandler: sh,
		showSpinner:   true,
		width:         80,
	}

	newM, cmd := m.handleLLMChunk("hello")

	if newM.showSpinner {
		t.Error("expected showSpinner to be false after chunk")
	}
	if sh.GetResponse() != "hello" {
		t.Errorf("expected response 'hello', got '%s'", sh.GetResponse())
	}
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
}

func TestHandleLLMDone(t *testing.T) {
	sh := NewStreamHandler()
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")
	sh.HandleChunk("response line 1\nresponse line 2")

	m := replModel{
		streamHandler: sh,
		showSpinner:   true,
		width:         80,
		state: &replState{
			messages: []llm.Message{},
		},
		outputLines: []string{},
	}

	newM, cmd := m.handleLLMDone()

	if newM.showSpinner {
		t.Error("expected showSpinner to be false after done")
	}

	if len(m.state.messages) != 1 {
		t.Errorf("expected 1 message in history, got %d", len(m.state.messages))
	}
	if m.state.messages[0].Role != llm.RoleAssistant {
		t.Errorf("expected assistant role, got %s", m.state.messages[0].Role)
	}
	if m.state.messages[0].Content != "response line 1\nresponse line 2" {
		t.Errorf("unexpected message content: %s", m.state.messages[0].Content)
	}

	if len(newM.outputLines) != 3 {
		t.Errorf("expected 3 output lines (2 content + 1 empty), got %d", len(newM.outputLines))
	}

	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestHandleLLMError(t *testing.T) {
	sh := NewStreamHandler()
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	m := replModel{
		streamHandler: sh,
		showSpinner:   true,
		width:         80,
		outputLines:   []string{},
	}

	testErr := errors.New("stream failed")
	newM, cmd := m.handleLLMError(testErr)

	if newM.showSpinner {
		t.Error("expected showSpinner to be false after error")
	}

	if len(newM.outputLines) != 2 {
		t.Errorf("expected 2 output lines (1 error + 1 empty), got %d", len(newM.outputLines))
	}

	if !strings.Contains(newM.outputLines[0], "stream failed") {
		t.Errorf("expected error message in output, got: %s", newM.outputLines[0])
	}

	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestHandleKeyMsg_Enter(t *testing.T) {
	ta := textarea.New()
	ta.SetValue(helpCommand)
	m := replModel{
		textarea:      ta,
		width:         80,
		streamHandler: NewStreamHandler(),
		state: &replState{
			messages: []llm.Message{},
		},
		outputLines: []string{},
	}

	newM, cmd := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEnter})

	if !strings.Contains(strings.Join(newM.outputLines, ""), "Available Commands") {
		t.Error("expected help text in output after enter with /help")
	}
	if cmd != nil {
		t.Error("expected nil cmd for help command")
	}
}

func TestHandleKeyMsg_CtrlC(t *testing.T) {
	m := replModel{
		quitting: false,
	}

	newM, cmd := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyCtrlC})

	if !newM.quitting {
		t.Error("expected quitting to be true after ctrl+c")
	}

	if cmd == nil {
		t.Fatal("expected tea.Quit cmd after ctrl+c")
	}

	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestHandleKeyMsg_CtrlJ(t *testing.T) {
	ta := textarea.New()
	ta.SetValue("line 1")
	m := replModel{
		textarea: ta,
		width:    80,
	}

	newM, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyCtrlJ})

	if !strings.Contains(newM.textarea.Value(), "\n") {
		t.Error("expected newline in textarea after ctrl+j")
	}
}

func TestHandleKeyMsg_ModelSelectionMode(t *testing.T) {
	m := replModel{
		width:          80,
		modelSelection: &modelselection.Model{},
	}

	newM, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	if newM.modelSelection == nil {
		t.Error("expected modelSelection to remain set")
	}
}

func TestHandleKeyMsg_UnknownKey(t *testing.T) {
	ta := textarea.New()
	m := replModel{
		textarea: ta,
		width:    80,
	}

	_, cmd := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyF1})

	if cmd == nil {
		t.Log("cmd can be nil or non-nil depending on textarea behavior")
	}
}

func TestHandleLLMChunk_MultipleCalls(t *testing.T) {
	sh := NewStreamHandler()
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	m := replModel{
		streamHandler: sh,
		showSpinner:   true,
		width:         80,
	}

	m, _ = m.handleLLMChunk("Hello")
	m, _ = m.handleLLMChunk(" ")
	m, _ = m.handleLLMChunk("World")

	if sh.GetResponse() != "Hello World" {
		t.Errorf("expected 'Hello World', got '%s'", sh.GetResponse())
	}

	if m.showSpinner {
		t.Error("showSpinner should be false after first chunk")
	}
}

func TestHandleLLMDone_EmptyResponse(t *testing.T) {
	sh := NewStreamHandler()
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	m := replModel{
		streamHandler: sh,
		showSpinner:   true,
		width:         80,
		state: &replState{
			messages: []llm.Message{},
		},
		outputLines: []string{},
	}

	newM, _ := m.handleLLMDone()

	if len(m.state.messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(m.state.messages))
	}

	if len(newM.outputLines) != 2 {
		t.Errorf("expected 2 lines (1 empty response + 1 empty), got %d", len(newM.outputLines))
	}
}

func TestHandleLLMError_ResetsHandler(t *testing.T) {
	sh := NewStreamHandler()
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")
	sh.HandleChunk("partial content")

	m := replModel{
		streamHandler: sh,
		showSpinner:   true,
		width:         80,
		outputLines:   []string{},
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

func TestHandleKeyMsg_SpecialCharacters(t *testing.T) {
	m := replModel{
		width:          80,
		modelSelection: &modelselection.Model{},
	}

	newM, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("é")})

	_ = newM
}
