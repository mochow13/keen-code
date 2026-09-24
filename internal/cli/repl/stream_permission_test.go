package repl

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mochow13/keen-code/internal/cli/repl/agentcore"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	"strings"
	"testing"
)

func TestStreamHandler_RewindForRetry_PreservesResolvedPermissionAndDiff(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	sh.HandleChunk("I'll edit this. ")
	sh.HandleDiff([]agentcore.EditDiffLine{{Kind: agentcore.EditDiffLineAdded, Content: "new line", NewLineNum: 1}})
	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(replpermissions.StatusAllowed)
	sh.HandleReasoningChunk("checking result")
	sh.HandleChunk("The edit completed")

	sh.RewindForRetry()

	if len(sh.segments) != 3 {
		t.Fatalf("expected 3 surviving segments after rewind, got %d", len(sh.segments))
	}
	if sh.segments[0].kind != segmentAssistant || sh.segments[1].kind != segmentDiff || sh.segments[2].kind != segmentPermission {
		t.Fatalf("expected assistant/diff/permission segments to remain, got %q/%q/%q", sh.segments[0].kind, sh.segments[1].kind, sh.segments[2].kind)
	}
	if sh.segments[2].permissionReq.Status != replpermissions.StatusAllowed {
		t.Fatalf("expected resolved permission to remain allowed, got %q", sh.segments[2].permissionReq.Status)
	}
	if got := sh.GetResponse(); got != "I'll edit this. " {
		t.Fatalf("expected rebuilt response %q, got %q", "I'll edit this. ", got)
	}
}

func makeTestPermissionRequest(isDangerous bool) *replpermissions.Request {
	return &replpermissions.Request{
		RequestID:    "test-1",
		ToolName:     "read_file",
		Path:         "../secret.txt",
		ResolvedPath: "/home/user/secret.txt",
		IsDangerous:  isDangerous,
		Status:       replpermissions.StatusPending,
		ResponseChan: make(chan bool, 1),
	}
}

func TestStreamHandler_HandlePermissionRequest_AddsSegment(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

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
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	if !sh.HasPendingPermission() {
		t.Error("expected HasPendingPermission to be true")
	}
}

func TestStreamHandler_HasPendingPermission_FalseWhenResolved(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(replpermissions.StatusAllowed)

	if sh.HasPendingPermission() {
		t.Error("expected HasPendingPermission to be false after resolution")
	}
}

func TestStreamHandler_MovePendingCursor(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	sh.MovePendingCursor(1)
	if sh.segments[0].permissionCursor != 1 {
		t.Errorf("expected cursor at 1, got %d", sh.segments[0].permissionCursor)
	}

	sh.MovePendingCursor(100)
	if sh.segments[0].permissionCursor != 3 {
		t.Errorf("expected cursor clamped at 3, got %d", sh.segments[0].permissionCursor)
	}

	sh.MovePendingCursor(-100)
	if sh.segments[0].permissionCursor != 0 {
		t.Errorf("expected cursor clamped at 0, got %d", sh.segments[0].permissionCursor)
	}
}

func TestStreamHandler_GetPendingChoice_NonDangerous(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	if sh.GetPendingChoice() != replpermissions.ChoiceAllow {
		t.Error("expected initial choice to be Allow")
	}

	sh.MovePendingCursor(1)
	if sh.GetPendingChoice() != replpermissions.ChoiceAllowSession {
		t.Error("expected choice at cursor 1 to be AllowSession")
	}

	sh.MovePendingCursor(1)
	if sh.GetPendingChoice() != replpermissions.ChoiceDeny {
		t.Error("expected choice at cursor 2 to be Deny")
	}
}

func TestStreamHandler_GetPendingChoice_Dangerous(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(true)
	sh.HandlePermissionRequest(req)

	sh.MovePendingCursor(1)
	if sh.GetPendingChoice() != replpermissions.ChoiceDeny {
		t.Error("expected cursor 1 to be Deny for dangerous (no AllowSession)")
	}
}

func TestStreamHandler_ResolvePendingPermission(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(replpermissions.StatusAllowedSession)

	if sh.segments[0].permissionReq.Status != replpermissions.StatusAllowedSession {
		t.Errorf("expected status AllowedSession, got %q", sh.segments[0].permissionReq.Status)
	}
}

func TestRenderPermissionCard_Pending(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	view := sh.View(80)

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
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(true)
	sh.HandlePermissionRequest(req)

	view := sh.View(80)

	if !strings.Contains(view, "Allow Dangerous Command") {
		t.Error("expected dangerous warning in card")
	}
	if strings.Contains(view, "Allow for this session") {
		t.Error("expected no 'Allow for this session' for dangerous operations")
	}
}

func TestRenderPermissionCard_Resolved_Allowed(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(replpermissions.StatusAllowed)

	view := sh.View(80)

	if !strings.Contains(view, "✓") {
		t.Error("expected checkmark in resolved allowed card")
	}
	if strings.Contains(view, "Permission Required") {
		t.Error("expected no card title in resolved state")
	}
}

func TestRenderPermissionCard_Resolved_Denied(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(replpermissions.StatusDenied)

	view := sh.View(80)

	if !strings.Contains(view, "✗") {
		t.Error("expected X mark in resolved denied card")
	}
}

func TestRenderPermissionCard_PreviewTruncation(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	var previewLines []string
	for i := range permissionPreviewMaxLines + 10 {
		previewLines = append(previewLines, strings.Repeat("x", i%40))
	}
	req.Preview = strings.Join(previewLines, "\n")
	sh.HandlePermissionRequest(req)

	view := sh.View(80)

	if !strings.Contains(view, "more preview lines omitted") {
		t.Error("expected truncation message in card with long preview")
	}
}

func TestRenderPermissionCard_LongPathWrapsWithinWidth(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	req.Path = "/very/long/path/" + strings.Repeat("nested-directory/", 12) + "file.go"
	req.ResolvedPath = "/Users/example/" + strings.Repeat("really-long-segment/", 10) + "file.go"
	sh.HandlePermissionRequest(req)

	width := 50
	view := sh.View(width)

	for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("line exceeds viewport width (%d > %d): %q", w, width, line)
		}
	}

	if !strings.Contains(view, "Path:") {
		t.Error("expected Path field to be present")
	}
	if !strings.Contains(view, "Resolved:") {
		t.Error("expected Resolved field to be present")
	}
}

func TestRenderPermissionCard_LongDangerousCommandWrapsWithinWidth(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(true)
	req.Path = "rm -rf " + strings.Repeat("/tmp/very-long-segment-name/", 12)
	sh.HandlePermissionRequest(req)

	width := 48
	view := sh.View(width)

	for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("line exceeds viewport width (%d > %d): %q", w, width, line)
		}
	}

	if !strings.Contains(view, "Allow Dangerous Command") {
		t.Error("expected dangerous command title to be present")
	}
}

func TestPermissionTranscript_ResolvedBeforeDone(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan agentcore.StreamEvent)
	sh.Start(eventCh, "Loading...")

	sh.HandleChunk("before permission")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(replpermissions.StatusAllowedSession)

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
	eventCh := make(chan agentcore.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.stream.handler.HandlePermissionRequest(req)

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})

	if newM.stream.handler.HasPendingPermission() {
		t.Error("expected permission to be resolved after Enter")
	}
	if req.Status != replpermissions.StatusAllowed {
		t.Errorf("expected status Allowed, got %q", req.Status)
	}
}

func TestHandleKeyMsg_PermissionEsc_Denies(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan agentcore.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.stream.handler.HandlePermissionRequest(req)

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})

	if newM.stream.handler.HasPendingPermission() {
		t.Error("expected permission to be resolved after Esc")
	}
	if req.Status != replpermissions.StatusDenied {
		t.Errorf("expected status Denied, got %q", req.Status)
	}
}

func TestHandleKeyMsg_PermissionEnter_AllowSession(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan agentcore.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.stream.handler.HandlePermissionRequest(req)

	m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})

	if req.Status != replpermissions.StatusAllowedSession {
		t.Errorf("expected status AllowedSession, got %q", req.Status)
	}
	if !newM.permissionRequester.IsSessionAllowed("read_file") {
		t.Error("expected read_file to be session-allowed after AllowSession choice")
	}
}

func TestHandleKeyMsg_NonPermissionKey_PassesToTextarea(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan agentcore.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.stream.handler.HandlePermissionRequest(req)

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'a', Text: "a"})

	if !newM.stream.handler.HasPendingPermission() {
		t.Error("expected permission to still be pending after non-permission key")
	}
}

func TestHandleKeyMsg_Enter_WhenPermissionPending_DoesNotSubmit(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("some user input")
	eventCh := make(chan agentcore.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.stream.handler.HandlePermissionRequest(req)

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})

	if newM.textarea.Value() != "some user input" {
		t.Error("expected textarea to keep its value when Enter resolves permission")
	}
	if newM.stream.handler.HasPendingPermission() {
		t.Error("expected permission to be resolved")
	}
}
