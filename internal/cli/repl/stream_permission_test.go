package repl

import (
	tea "charm.land/bubbletea/v2"
	"github.com/mochow13/keen-code/internal/agentcore"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	"testing"
)

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
