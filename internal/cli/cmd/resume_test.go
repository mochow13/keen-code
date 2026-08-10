package cmd

import (
	"testing"

	"github.com/mochow13/keen-code/internal/session"
)

func TestLoadResumeSession(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)

	store, err := session.NewStore(work)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	sess, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.Append(sess, session.Event{Kind: session.KindUserMessage, UserMessage: &session.MessagePayload{Content: "hello"}}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	loaded, err := loadResumeSession(work, sess.ID)
	if err != nil {
		t.Fatalf("loadResumeSession() error = %v", err)
	}
	if loaded.Session.ID != sess.ID {
		t.Fatalf("loaded session ID = %q, want %q", loaded.Session.ID, sess.ID)
	}

	_, err = loadResumeSession(work, "nonexistent")
	if err == nil {
		t.Fatal("loadResumeSession(nonexistent) error = nil, want error")
	}
}
