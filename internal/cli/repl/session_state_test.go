package repl

import (
	"testing"

	"github.com/mochow13/keen-code/internal/session"
)

func TestSessionResumeSelectsCurrentSession(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)

	state := newTestSession(work)
	if state == nil {
		t.Fatal("newTestSession() returned nil")
	}
	if state.CurrentID() != "" {
		t.Fatalf("expected empty current ID, got %q", state.CurrentID())
	}

	sess := &session.Session{ID: "test-session-id"}
	state.Resume(&session.LoadedSession{Session: sess})
	if state.CurrentID() != sess.ID {
		t.Fatalf("CurrentID() = %q, want %q", state.CurrentID(), sess.ID)
	}
}
