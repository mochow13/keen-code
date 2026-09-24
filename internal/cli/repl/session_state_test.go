package repl

import (
	"testing"

	"github.com/mochow13/keen-code/internal/session"
)

func TestReplSessionState_SetSession(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)

	state := newReplSessionState(work)
	if state == nil {
		t.Fatal("newReplSessionState() returned nil")
	}

	if state.currentID() != "" {
		t.Fatalf("expected empty current ID, got %q", state.currentID())
	}

	sess := &session.Session{ID: "test-session-id"}
	state.setSession(sess)

	if state.currentID() != sess.ID {
		t.Fatalf("currentID() = %q, want %q", state.currentID(), sess.ID)
	}
}
