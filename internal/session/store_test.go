package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/user/keen-code/internal/llm"
)

func TestSanitizeWorkingDir(t *testing.T) {
	got := sanitizeWorkingDir("/Users/me/src/keen-code")
	if got != "Users-me-src-keen-code" {
		t.Fatalf("unexpected slug: %q", got)
	}
}

func TestBuildConversation_ReplacesOnCompaction(t *testing.T) {
	events := []Event{
		{
			Kind:        KindUserMessage,
			UserMessage: &MessagePayload{Content: "first"},
		},
		{
			Kind: KindAssistantTurn,
			AssistantTurn: &AssistantTurnPayload{
				Message: "reply",
			},
		},
		{
			Kind: KindCompactionApplied,
			CompactionApplied: &CompactionAppliedPayload{
				Status: "Context compacted.",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "summary"},
				},
			},
		},
		{
			Kind:        KindUserMessage,
			UserMessage: &MessagePayload{Content: "after"},
		},
	}

	got := BuildConversation(events)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].Content != "summary" || got[1].Content != "after" {
		t.Fatalf("unexpected conversation: %#v", got)
	}
}

func TestStoreCreateAppendListLoad(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	store, err := NewStore(filepath.Join(tmp, "project"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	session, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := store.Append(session, Event{
		Kind:        KindUserMessage,
		UserMessage: &MessagePayload{Content: "hello world"},
	}); err != nil {
		t.Fatalf("Append(user) error = %v", err)
	}

	if err := store.Append(session, Event{
		Kind: KindAssistantTurn,
		AssistantTurn: &AssistantTurnPayload{
			Message: "hi",
		},
	}); err != nil {
		t.Fatalf("Append(assistant) error = %v", err)
	}

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].LastUserMessage != "hello world" {
		t.Fatalf("unexpected last user message preview: %q", summaries[0].LastUserMessage)
	}

	loaded, err := store.Load(summaries[0])
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(loaded.Events))
	}
	if loaded.Session.nextSeq != 4 {
		t.Fatalf("expected next sequence 4, got %d", loaded.Session.nextSeq)
	}
}

func TestStoreAppendBatch_AtomicallyAppendsSequentialEvents(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	store, err := NewStore(filepath.Join(tmp, "project"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	session, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err = store.AppendBatch(session, []Event{
		{Kind: KindAssistantTurn, AssistantTurn: &AssistantTurnPayload{Message: "checkpoint"}},
		{Kind: KindCompactionApplied, CompactionApplied: &CompactionAppliedPayload{Messages: []llm.Message{{Role: llm.RoleUser, Content: "summary"}}}},
	})
	if err != nil {
		t.Fatalf("AppendBatch() error = %v", err)
	}

	events, err := loadEvents(session.TranscriptPath)
	if err != nil {
		t.Fatalf("loadEvents() error = %v", err)
	}
	if len(events) != 3 || events[1].Seq != 2 || events[2].Seq != 3 {
		t.Fatalf("events = %#v, want sequential batch events", events)
	}
	if session.nextSeq != 4 {
		t.Fatalf("next sequence = %d, want 4", session.nextSeq)
	}
}

func TestStoreAppendBatch_FailureLeavesNoPartialEvents(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	store, err := NewStore(filepath.Join(tmp, "project"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	session, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := os.RemoveAll(session.Directory); err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}

	err = store.AppendBatch(session, []Event{
		{Kind: KindAssistantTurn, AssistantTurn: &AssistantTurnPayload{Message: "checkpoint"}},
		{Kind: KindCompactionApplied, CompactionApplied: &CompactionAppliedPayload{}},
	})
	if err == nil {
		t.Fatal("AppendBatch() succeeded after session directory was removed")
	}
	if session.nextSeq != 2 {
		t.Fatalf("next sequence = %d, want unchanged value 2", session.nextSeq)
	}
	if _, err := os.Stat(session.TranscriptPath); !os.IsNotExist(err) {
		t.Fatalf("transcript stat error = %v, want not exist", err)
	}
}

func TestStoreList_UsesLastUserMessagePreview(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	store, err := NewStore(filepath.Join(tmp, "project"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	session, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := store.Append(session, Event{
		Kind:        KindUserMessage,
		UserMessage: &MessagePayload{Content: "first message"},
	}); err != nil {
		t.Fatalf("Append(first user) error = %v", err)
	}

	if err := store.Append(session, Event{
		Kind: KindAssistantTurn,
		AssistantTurn: &AssistantTurnPayload{
			Message: "reply",
		},
	}); err != nil {
		t.Fatalf("Append(assistant) error = %v", err)
	}

	if err := store.Append(session, Event{
		Kind:        KindUserMessage,
		UserMessage: &MessagePayload{Content: "second message"},
	}); err != nil {
		t.Fatalf("Append(second user) error = %v", err)
	}

	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].LastUserMessage != "second message" {
		t.Fatalf("expected last user message preview, got %q", summaries[0].LastUserMessage)
	}
}

func TestLoadEvents_SkipsMalformedLine(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, transcriptFileName)

	content := `{"seq":1,"kind":"user_message","user_message":{"content":"ok"}}` + "\n" +
		`{not-json}` + "\n" +
		`{"seq":2,"kind":"assistant_turn","assistant_turn":{"message":"still ok"}}`

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	events, err := loadEvents(path)
	if err != nil {
		t.Fatalf("loadEvents() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 valid events, got %d", len(events))
	}
}

func TestSummarize_FallsBackToUpdatedAtAndDirectoryName(t *testing.T) {
	updatedAt := time.Date(2026, 4, 16, 13, 0, 0, 0, time.UTC)
	summary := summarize(
		"session-id",
		"/tmp/session",
		"/tmp/session/transcript_events.jsonl",
		updatedAt,
		nil,
	)

	if summary.ID != "session-id" {
		t.Fatalf("expected fallback summary ID, got %q", summary.ID)
	}
	if !summary.CreatedAt.Equal(updatedAt) {
		t.Fatalf("expected created_at %v, got %v", updatedAt, summary.CreatedAt)
	}
	if !summary.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("expected updated_at %v, got %v", updatedAt, summary.UpdatedAt)
	}
}
