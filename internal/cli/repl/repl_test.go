package repl

import (
	"testing"

	"github.com/user/keen-cli/internal/llm"
)

func TestUpdate_PermissionSelector_AllowsToolStartEvent(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	m := replModel{
		permissionSelector: NewPermissionSelector("read_file", "../foo.txt", "/tmp/foo.txt", "read"),
		streamHandler:      sh,
		showSpinner:        true,
		width:              80,
		output:             NewOutputBuilder(80),
	}

	toolCall := &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "../foo.txt"}}
	updatedModel, cmd := m.Update(llmToolStartMsg{toolCall: toolCall})

	updated, ok := updatedModel.(*replModel)
	if !ok {
		t.Fatalf("expected *replModel, got %T", updatedModel)
	}

	if updated.showSpinner {
		t.Error("expected showSpinner to be false after tool start while permission selector is active")
	}

	if len(updated.output.GetLines()) != 0 {
		t.Errorf("expected no persisted output line for tool start, got %d", len(updated.output.GetLines()))
	}

	if cmd == nil {
		t.Error("expected non-nil cmd when handling tool start event")
	}
}
