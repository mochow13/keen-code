package repl

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"github.com/mochow13/keen-code/internal/agentcore"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	repltooling "github.com/mochow13/keen-code/internal/cli/repl/tooling"
	"testing"
)

func TestWaitForAsyncEvent_Chunk(t *testing.T) {
	eventCh := make(chan agentcore.StreamEvent, 1)
	eventCh <- agentcore.StreamEvent{
		Type:    agentcore.StreamEventTypeChunk,
		Content: "chunk data",
	}
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *replpermissions.Request), make(chan repltooling.DiffRequest), nil, nil)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	streamMsg, ok := msg.(mainStreamMsg)
	if !ok {
		t.Fatalf("expected mainStreamMsg, got %T", msg)
	}
	if streamMsg.eventCh != eventCh {
		t.Fatal("expected source event channel to round-trip")
	}
	if streamMsg.closed {
		t.Fatal("expected open stream event")
	}
	if streamMsg.event.Type != agentcore.StreamEventTypeChunk || streamMsg.event.Content != "chunk data" {
		t.Fatalf("unexpected stream event: %#v", streamMsg.event)
	}
}

func TestWaitForAsyncEvent_Done(t *testing.T) {
	eventCh := make(chan agentcore.StreamEvent, 1)
	eventCh <- agentcore.StreamEvent{
		Type: agentcore.StreamEventTypeDone,
	}
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *replpermissions.Request), make(chan repltooling.DiffRequest), nil, nil)
	msg := cmd()

	streamMsg, ok := msg.(mainStreamMsg)
	if !ok {
		t.Fatalf("expected mainStreamMsg, got %T", msg)
	}
	if streamMsg.closed || streamMsg.event.Type != agentcore.StreamEventTypeDone {
		t.Fatalf("expected done event, got %#v", streamMsg)
	}
}

func TestWaitForAsyncEvent_ReasoningChunk(t *testing.T) {
	eventCh := make(chan agentcore.StreamEvent, 1)
	eventCh <- agentcore.StreamEvent{
		Type:    agentcore.StreamEventTypeReasoningChunk,
		Content: "thinking",
	}
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *replpermissions.Request), make(chan repltooling.DiffRequest), nil, nil)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	streamMsg, ok := msg.(mainStreamMsg)
	if !ok {
		t.Fatalf("expected mainStreamMsg, got %T", msg)
	}
	if streamMsg.closed || streamMsg.event.Type != agentcore.StreamEventTypeReasoningChunk || streamMsg.event.Content != "thinking" {
		t.Fatalf("unexpected stream event: %#v", streamMsg)
	}
}

func TestWaitForAsyncEvent_Error(t *testing.T) {
	testErr := errors.New("stream error")
	eventCh := make(chan agentcore.StreamEvent, 1)
	eventCh <- agentcore.StreamEvent{
		Type:  agentcore.StreamEventTypeError,
		Error: testErr,
	}
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *replpermissions.Request), make(chan repltooling.DiffRequest), nil, nil)
	msg := cmd()

	streamMsg, ok := msg.(mainStreamMsg)
	if !ok {
		t.Fatalf("expected mainStreamMsg, got %T", msg)
	}
	if streamMsg.closed || streamMsg.event.Type != agentcore.StreamEventTypeError || streamMsg.event.Error != testErr {
		t.Fatalf("unexpected stream event: %#v", streamMsg)
	}
}

func TestWaitForAsyncEvent_ChannelClosed(t *testing.T) {
	eventCh := make(chan agentcore.StreamEvent)
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *replpermissions.Request), make(chan repltooling.DiffRequest), nil, nil)
	msg := cmd()

	streamMsg, ok := msg.(mainStreamMsg)
	if !ok {
		t.Fatalf("expected mainStreamMsg, got %T", msg)
	}
	if !streamMsg.closed {
		t.Fatalf("expected closed stream message, got %#v", streamMsg)
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

func TestWaitForAsyncEvent_Permission(t *testing.T) {
	permissionCh := make(chan *replpermissions.Request, 1)
	req := makeTestPermissionRequest(false)
	permissionCh <- req

	cmd := waitForAsyncEvent(make(chan agentcore.StreamEvent), permissionCh, make(chan repltooling.DiffRequest), nil, nil)
	msg := cmd()

	permissionMsg, ok := msg.(permissionReadyMsg)
	if !ok {
		t.Fatalf("expected permissionReadyMsg, got %T", msg)
	}
	if permissionMsg.req != req {
		t.Fatal("expected permission request payload to round-trip")
	}
}

func TestWaitForAsyncEvent_Diff(t *testing.T) {
	diffCh := make(chan repltooling.DiffRequest, 1)
	req := repltooling.DiffRequest{Done: make(chan struct{})}
	diffCh <- req

	cmd := waitForAsyncEvent(make(chan agentcore.StreamEvent), make(chan *replpermissions.Request), diffCh, nil, nil)
	msg := cmd()

	diffMsg, ok := msg.(diffReadyMsg)
	if !ok {
		t.Fatalf("expected diffReadyMsg, got %T", msg)
	}
	if diffMsg.req.Done != req.Done {
		t.Fatal("expected diff request payload to round-trip")
	}
}

var _ tea.Msg = llmChunkMsg("")
var _ tea.Msg = llmReasoningChunkMsg("")
var _ tea.Msg = llmDoneMsg{}
var _ tea.Msg = llmErrorMsg{}
var _ tea.Msg = mainStreamMsg{}
var _ tea.Msg = permissionReadyMsg{}
var _ tea.Msg = diffReadyMsg{}
