package repl

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/user/keen-code/internal/llm"
)

func TestStreamHandler_HandleChunk(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	sh.HandleChunk("Hello")
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
	lines, errMsg := sh.HandleError(testErr)

	if len(lines) != 1 {
		t.Fatalf("expected 1 pending transcript line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "some content") {
		t.Errorf("expected pending line to include chunk content, got %q", lines[0])
	}

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

func TestStreamHandler_HandleDone_MixedSegmentsChronological(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	sh.HandleChunk("First chunk")
	sh.HandleToolStart(&llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "go.mod"}})
	sh.HandleChunk(" Second chunk")
	sh.HandleToolEnd(&llm.ToolCall{Name: "read_file", Duration: 5})

	lines, fullResponse := sh.HandleDone()

	if fullResponse != "First chunk Second chunk" {
		t.Fatalf("unexpected full response: %q", fullResponse)
	}

	if len(lines) != 4 {
		t.Fatalf("expected 4 transcript lines, got %d", len(lines))
	}

	if !strings.Contains(lines[0], "First chunk") {
		t.Fatalf("expected first line to be first assistant chunk, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "read_file") || !strings.Contains(lines[1], "⚙") {
		t.Fatalf("expected second line to be tool start, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "Second chunk") {
		t.Fatalf("expected third line to be second assistant chunk, got %q", lines[2])
	}
	if !strings.Contains(lines[3], "read_file") || !strings.Contains(lines[3], "✓") {
		t.Fatalf("expected fourth line to be tool end, got %q", lines[3])
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

func TestStreamHandler_View_WithRunningBashShowsInlineStatus(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Brewing...")
	sh.HandleBashStart("npm test", "running tests")

	view := sh.View(80, true, "⠋")

	if !strings.Contains(view, "Running command...") {
		t.Fatal("expected inline running message for bash")
	}
	if !strings.Contains(view, "Press Esc to interrupt") {
		t.Fatal("expected interrupt hint for running bash")
	}
	if strings.Contains(view, "Brewing...") {
		t.Fatal("expected bottom spinner line to be suppressed while bash is running")
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

func TestWaitForAsyncEvent_Chunk(t *testing.T) {
	eventCh := make(chan llm.StreamEvent, 1)
	eventCh <- llm.StreamEvent{
		Type:    llm.StreamEventTypeChunk,
		Content: "chunk data",
	}
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *PermissionRequest), make(chan diffEmitRequest))
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

func TestWaitForAsyncEvent_Done(t *testing.T) {
	eventCh := make(chan llm.StreamEvent, 1)
	eventCh <- llm.StreamEvent{
		Type: llm.StreamEventTypeDone,
	}
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *PermissionRequest), make(chan diffEmitRequest))
	msg := cmd()

	_, ok := msg.(llmDoneMsg)
	if !ok {
		t.Fatalf("expected llmDoneMsg, got %T", msg)
	}
}

func TestWaitForAsyncEvent_Error(t *testing.T) {
	testErr := errors.New("stream error")
	eventCh := make(chan llm.StreamEvent, 1)
	eventCh <- llm.StreamEvent{
		Type:  llm.StreamEventTypeError,
		Error: testErr,
	}
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *PermissionRequest), make(chan diffEmitRequest))
	msg := cmd()

	errMsg, ok := msg.(llmErrorMsg)
	if !ok {
		t.Fatalf("expected llmErrorMsg, got %T", msg)
	}
	if errMsg.err != testErr {
		t.Errorf("expected error '%v', got '%v'", testErr, errMsg.err)
	}
}

func TestWaitForAsyncEvent_ChannelClosed(t *testing.T) {
	eventCh := make(chan llm.StreamEvent)
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *PermissionRequest), make(chan diffEmitRequest))
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

func TestWaitForAsyncEvent_Permission(t *testing.T) {
	permissionCh := make(chan *PermissionRequest, 1)
	req := makeTestPermissionRequest(false)
	permissionCh <- req

	cmd := waitForAsyncEvent(make(chan llm.StreamEvent), permissionCh, make(chan diffEmitRequest))
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
	diffCh := make(chan diffEmitRequest, 1)
	req := diffEmitRequest{done: make(chan struct{})}
	diffCh <- req

	cmd := waitForAsyncEvent(make(chan llm.StreamEvent), make(chan *PermissionRequest), diffCh)
	msg := cmd()

	diffMsg, ok := msg.(diffReadyMsg)
	if !ok {
		t.Fatalf("expected diffReadyMsg, got %T", msg)
	}
	if diffMsg.req.done != req.done {
		t.Fatal("expected diff request payload to round-trip")
	}
}

var _ tea.Msg = llmChunkMsg("")
var _ tea.Msg = llmDoneMsg{}
var _ tea.Msg = llmErrorMsg{}
var _ tea.Msg = permissionReadyMsg{}
var _ tea.Msg = diffReadyMsg{}

func makeTestPermissionRequest(isDangerous bool) *PermissionRequest {
	return &PermissionRequest{
		RequestID:    "test-1",
		ToolName:     "read_file",
		Path:         "../secret.txt",
		ResolvedPath: "/home/user/secret.txt",
		IsDangerous:  isDangerous,
		Status:       PermissionStatusPending,
		ResponseChan: make(chan bool, 1),
	}
}

func TestStreamHandler_HandlePermissionRequest_AddsSegment(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	if len(sh.segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(sh.segments))
	}
	if sh.segments[0].kind != segmentPermission {
		t.Errorf("expected segmentPermission, got %q", sh.segments[0].kind)
	}
	if sh.segments[0].permissionReq != req {
		t.Error("expected permission request to be stored in segment")
	}
}

func TestStreamHandler_HasPendingPermission_True(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	if !sh.HasPendingPermission() {
		t.Error("expected HasPendingPermission to be true")
	}
}

func TestStreamHandler_HasPendingPermission_FalseWhenResolved(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(PermissionStatusAllowed)

	if sh.HasPendingPermission() {
		t.Error("expected HasPendingPermission to be false after resolution")
	}
}

func TestStreamHandler_MovePendingCursor(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	sh.MovePendingCursor(1)
	if sh.segments[0].permissionCursor != 1 {
		t.Errorf("expected cursor at 1, got %d", sh.segments[0].permissionCursor)
	}

	sh.MovePendingCursor(100)
	if sh.segments[0].permissionCursor != 2 {
		t.Errorf("expected cursor clamped at 2, got %d", sh.segments[0].permissionCursor)
	}

	sh.MovePendingCursor(-100)
	if sh.segments[0].permissionCursor != 0 {
		t.Errorf("expected cursor clamped at 0, got %d", sh.segments[0].permissionCursor)
	}
}

func TestStreamHandler_GetPendingChoice_NonDangerous(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	if sh.GetPendingChoice() != PermissionChoiceAllow {
		t.Error("expected initial choice to be Allow")
	}

	sh.MovePendingCursor(1)
	if sh.GetPendingChoice() != PermissionChoiceAllowSession {
		t.Error("expected choice at cursor 1 to be AllowSession")
	}

	sh.MovePendingCursor(1)
	if sh.GetPendingChoice() != PermissionChoiceDeny {
		t.Error("expected choice at cursor 2 to be Deny")
	}
}

func TestStreamHandler_GetPendingChoice_Dangerous(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(true)
	sh.HandlePermissionRequest(req)

	sh.MovePendingCursor(1)
	if sh.GetPendingChoice() != PermissionChoiceDeny {
		t.Error("expected cursor 1 to be Deny for dangerous (no AllowSession)")
	}
}

func TestStreamHandler_ResolvePendingPermission(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(PermissionStatusAllowedSession)

	if sh.segments[0].permissionReq.Status != PermissionStatusAllowedSession {
		t.Errorf("expected status AllowedSession, got %q", sh.segments[0].permissionReq.Status)
	}
}

func TestRenderPermissionCard_Pending(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	view := sh.View(80, false, "")

	if !strings.Contains(view, "Permission Required") {
		t.Error("expected 'Permission Required' in pending card")
	}
	if !strings.Contains(view, "read_file") {
		t.Error("expected tool name in card")
	}
	if !strings.Contains(view, "Allow for this session") {
		t.Error("expected 'Allow for this session' choice in card")
	}
	if !strings.Contains(view, "↑/↓") {
		t.Error("expected keyboard hint in card")
	}
}

func TestRenderPermissionCard_Dangerous(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(true)
	sh.HandlePermissionRequest(req)

	view := sh.View(80, false, "")

	if !strings.Contains(view, "Allow Dangerous Command") {
		t.Error("expected dangerous warning in card")
	}
	if strings.Contains(view, "Allow for this session") {
		t.Error("expected no 'Allow for this session' for dangerous operations")
	}
}

func TestRenderPermissionCard_Resolved_Allowed(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(PermissionStatusAllowed)

	view := sh.View(80, false, "")

	if !strings.Contains(view, "✓") {
		t.Error("expected checkmark in resolved allowed card")
	}
	if strings.Contains(view, "Permission Required") {
		t.Error("expected no card title in resolved state")
	}
}

func TestRenderPermissionCard_Resolved_Denied(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(PermissionStatusDenied)

	view := sh.View(80, false, "")

	if !strings.Contains(view, "✗") {
		t.Error("expected X mark in resolved denied card")
	}
}

func TestRenderPermissionCard_PreviewTruncation(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	var previewLines []string
	for i := range permissionPreviewMaxLines + 10 {
		previewLines = append(previewLines, strings.Repeat("x", i%40))
	}
	req.Preview = strings.Join(previewLines, "\n")
	sh.HandlePermissionRequest(req)

	view := sh.View(80, false, "")

	if !strings.Contains(view, "more preview lines omitted") {
		t.Error("expected truncation message in card with long preview")
	}
}

func TestPermissionTranscript_ResolvedBeforeDone(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	sh.HandleChunk("before permission")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(PermissionStatusAllowedSession)

	sh.HandleChunk(" after permission")

	lines, _ := sh.HandleDone()

	foundBefore, foundStatus, foundAfter := false, false, false
	for _, l := range lines {
		if strings.Contains(l, "before permission") {
			foundBefore = true
		}
		if strings.Contains(l, "✓") && strings.Contains(l, "this session") {
			foundStatus = true
		}
		if strings.Contains(l, "after permission") {
			foundAfter = true
		}
	}

	if !foundBefore {
		t.Error("expected 'before permission' in transcript")
	}
	if !foundStatus {
		t.Error("expected resolved permission status line in transcript")
	}
	if !foundAfter {
		t.Error("expected 'after permission' in transcript")
	}
}

func TestHandleKeyMsg_PermissionEnter_ResolvesAllowed(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.streamHandler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.streamHandler.HandlePermissionRequest(req)

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})

	if newM.streamHandler.HasPendingPermission() {
		t.Error("expected permission to be resolved after Enter")
	}
	if req.Status != PermissionStatusAllowed {
		t.Errorf("expected status Allowed, got %q", req.Status)
	}
}

func TestHandleKeyMsg_PermissionEsc_Denies(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.streamHandler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.streamHandler.HandlePermissionRequest(req)

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})

	if newM.streamHandler.HasPendingPermission() {
		t.Error("expected permission to be resolved after Esc")
	}
	if req.Status != PermissionStatusDenied {
		t.Errorf("expected status Denied, got %q", req.Status)
	}
}

func TestHandleKeyMsg_PermissionEnter_AllowSession(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.streamHandler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.streamHandler.HandlePermissionRequest(req)

	m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})

	if req.Status != PermissionStatusAllowedSession {
		t.Errorf("expected status AllowedSession, got %q", req.Status)
	}
	if !newM.permissionRequester.sessionAllowedTools["read_file"] {
		t.Error("expected read_file to be session-allowed after AllowSession choice")
	}
}

func TestHandleKeyMsg_NonPermissionKey_PassesToTextarea(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.streamHandler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.streamHandler.HandlePermissionRequest(req)

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'a', Text: "a"})

	if !newM.streamHandler.HasPendingPermission() {
		t.Error("expected permission to still be pending after non-permission key")
	}
}

func TestHandleKeyMsg_Enter_WhenPermissionPending_DoesNotSubmit(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("some user input")
	eventCh := make(chan llm.StreamEvent)
	m.streamHandler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.streamHandler.HandlePermissionRequest(req)

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})

	if newM.textarea.Value() != "some user input" {
		t.Error("expected textarea to keep its value when Enter resolves permission")
	}
	if newM.streamHandler.HasPendingPermission() {
		t.Error("expected permission to be resolved")
	}
}
