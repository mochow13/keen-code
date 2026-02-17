package cli

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/user/keen-cli/internal/llm"
)

func TestStreamHandler_HandleChunk(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	cmd := sh.HandleChunk("Hello")
	if sh.GetResponse() != "Hello" {
		t.Errorf("expected response 'Hello', got '%s'", sh.GetResponse())
	}
	if sh.HasContent() != true {
		t.Error("expected HasContent() to be true")
	}

	sh.HandleChunk(" World")
	if sh.GetResponse() != "Hello World" {
		t.Errorf("expected response 'Hello World', got '%s'", sh.GetResponse())
	}

	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
}

func TestStreamHandler_HandleDone(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")
	sh.HandleChunk("Line 1\nLine 2")

	lines, fullResponse := sh.HandleDone()

	if fullResponse != "Line 1\nLine 2" {
		t.Errorf("expected full response 'Line 1\\nLine 2', got '%s'", fullResponse)
	}

	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "  Line 1") {
		t.Errorf("expected first line to start with '  Line 1', got '%s'", lines[0])
	}

	if sh.IsActive() {
		t.Error("expected IsActive to be false after HandleDone")
	}
	if sh.HasContent() {
		t.Error("expected HasContent to be false after HandleDone")
	}
}

func TestStreamHandler_HandleError(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")
	sh.HandleChunk("some content")

	testErr := errors.New("stream failed")
	errMsg := sh.HandleError(testErr)

	if errMsg != "stream failed" {
		t.Errorf("expected error message 'stream failed', got '%s'", errMsg)
	}

	if sh.IsActive() {
		t.Error("expected IsActive to be false after HandleError")
	}
	if sh.HasContent() {
		t.Error("expected HasContent to be false after HandleError")
	}
}

func TestStreamHandler_View_WithSpinner(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Brewing...")

	view := sh.View(80, true, "⠋")

	if !strings.Contains(view, "⠋") {
		t.Error("expected view to contain spinner")
	}
	if !strings.Contains(view, "Brewing...") {
		t.Error("expected view to contain loading text")
	}
}

func TestStreamHandler_View_WithContent(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")
	sh.HandleChunk("Hello World")

	view := sh.View(80, false, "")

	if !strings.Contains(view, "Hello World") {
		t.Error("expected view to contain response content")
	}
}

func TestStreamHandler_View_NoSpinnerNoContent(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	view := sh.View(80, false, "⠋")

	if view != "" {
		t.Errorf("expected empty view when no spinner and no content, got '%s'", view)
	}
}

func TestWaitForNextEvent_Chunk(t *testing.T) {
	eventCh := make(chan llm.StreamEvent, 1)
	eventCh <- llm.StreamEvent{
		Type:    llm.StreamEventTypeChunk,
		Content: "chunk data",
	}
	close(eventCh)

	sh := NewStreamHandler(nil)
	sh.Start(eventCh, "Loading...")

	cmd := sh.WaitForEvent()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	chunkMsg, ok := msg.(llmChunkMsg)
	if !ok {
		t.Fatalf("expected llmChunkMsg, got %T", msg)
	}
	if string(chunkMsg) != "chunk data" {
		t.Errorf("expected chunk 'chunk data', got '%s'", string(chunkMsg))
	}
}

func TestWaitForNextEvent_Done(t *testing.T) {
	eventCh := make(chan llm.StreamEvent, 1)
	eventCh <- llm.StreamEvent{
		Type: llm.StreamEventTypeDone,
	}
	close(eventCh)

	sh := NewStreamHandler(nil)
	sh.Start(eventCh, "Loading...")

	cmd := sh.WaitForEvent()
	msg := cmd()

	_, ok := msg.(llmDoneMsg)
	if !ok {
		t.Fatalf("expected llmDoneMsg, got %T", msg)
	}
}

func TestWaitForNextEvent_Error(t *testing.T) {
	testErr := errors.New("stream error")
	eventCh := make(chan llm.StreamEvent, 1)
	eventCh <- llm.StreamEvent{
		Type:  llm.StreamEventTypeError,
		Error: testErr,
	}
	close(eventCh)

	sh := NewStreamHandler(nil)
	sh.Start(eventCh, "Loading...")

	cmd := sh.WaitForEvent()
	msg := cmd()

	errMsg, ok := msg.(llmErrorMsg)
	if !ok {
		t.Fatalf("expected llmErrorMsg, got %T", msg)
	}
	if errMsg.err != testErr {
		t.Errorf("expected error '%v', got '%v'", testErr, errMsg.err)
	}
}

func TestWaitForNextEvent_ChannelClosed(t *testing.T) {
	eventCh := make(chan llm.StreamEvent)
	close(eventCh)

	sh := NewStreamHandler(nil)
	sh.Start(eventCh, "Loading...")

	cmd := sh.WaitForEvent()
	msg := cmd()

	_, ok := msg.(llmDoneMsg)
	if !ok {
		t.Fatalf("expected llmDoneMsg when channel closed, got %T", msg)
	}
}

func TestFormatResponseLines(t *testing.T) {
	input := "Line 1\nLine 2\nLine 3"
	result := formatResponseLines(input)

	if len(result) != 3 {
		t.Errorf("expected 3 lines, got %d", len(result))
	}
	if result[0] != "  Line 1" {
		t.Errorf("expected '  Line 1', got '%s'", result[0])
	}
	if result[1] != "  Line 2" {
		t.Errorf("expected '  Line 2', got '%s'", result[1])
	}
}

func TestFormatResponseLines_Empty(t *testing.T) {
	result := formatResponseLines("")
	if len(result) != 1 {
		t.Errorf("expected 1 line for empty input, got %d", len(result))
	}
}

func TestStreamHandler_Start(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)

	sh.Start(eventCh, "Cooking...")

	if !sh.IsActive() {
		t.Error("expected IsActive to be true after Start")
	}
	if sh.GetLoadingText() != "Cooking..." {
		t.Errorf("expected loading text 'Cooking...', got '%s'", sh.GetLoadingText())
	}
	if sh.HasContent() {
		t.Error("expected HasContent to be false initially")
	}
}

func TestStreamHandler_Start_ResetsPreviousState(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)

	sh.Start(eventCh, "First")
	sh.HandleChunk("previous content")

	newEventCh := make(chan llm.StreamEvent)
	sh.Start(newEventCh, "Second")

	if sh.GetResponse() != "" {
		t.Error("expected response to be reset after new Start")
	}
	if sh.GetLoadingText() != "Second" {
		t.Error("expected loading text to be updated")
	}
}

func TestStreamHandler_WaitForEvent_ReturnsDoneOnClosedChannel(t *testing.T) {
	eventCh := make(chan llm.StreamEvent)
	close(eventCh)

	sh := NewStreamHandler(nil)
	sh.Start(eventCh, "Loading...")

	cmd := sh.WaitForEvent()
	if cmd == nil {
		t.Fatal("WaitForEvent should return a non-nil tea.Cmd")
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("cmd() should return a non-nil tea.Msg")
	}

	_, ok := msg.(llmDoneMsg)
	if !ok {
		t.Fatalf("expected llmDoneMsg when channel closed, got %T", msg)
	}
}

var _ tea.Msg = llmChunkMsg("")
var _ tea.Msg = llmDoneMsg{}
var _ tea.Msg = llmErrorMsg{}
