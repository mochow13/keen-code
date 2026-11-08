package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/app"
	"github.com/mochow13/keen-code/internal/session"
)

type testAgent struct {
	events    chan agentcore.StreamEvent
	submitErr error
	ctx       context.Context
	sessionID string
	input     string
	messages  []agentcore.Message
}

func (a *testAgent) FormatUserMessage(content string) string {
	return "formatted: " + content
}

func (a *testAgent) Submit(ctx context.Context, sessionID, input string) (<-chan agentcore.StreamEvent, error) {
	a.ctx = ctx
	a.sessionID = sessionID
	a.input = input
	if a.submitErr != nil {
		return nil, a.submitErr
	}
	if a.events == nil {
		a.events = make(chan agentcore.StreamEvent)
	}
	return a.events, nil
}

func (a *testAgent) ReplaceMessages(messages []agentcore.Message) {
	a.messages = messages
}

func newPersistentSession(t *testing.T, agent *testAgent) *app.Session {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	sessions, err := app.NewSession(agent, t.TempDir())
	if err != nil {
		t.Fatalf("NewSession() error: %v", err)
	}
	return sessions
}

func TestSessionStartTurnPersistsUserAndOwnsCancellation(t *testing.T) {
	agent := &testAgent{}
	sessions := newPersistentSession(t, agent)

	turn, err := sessions.StartTurn(context.Background(), "hello")
	if err != nil {
		t.Fatalf("StartTurn() error: %v", err)
	}
	if agent.input != "hello" {
		t.Fatalf("Submit() input = %q, want hello", agent.input)
	}
	if agent.sessionID == "" || agent.sessionID != sessions.CurrentID() {
		t.Fatalf("Submit() session ID = %q, current = %q", agent.sessionID, sessions.CurrentID())
	}

	turn.Close()
	turn.Close()
	select {
	case <-agent.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("turn context was not cancelled")
	}

	summaries, err := sessions.List()
	if err != nil || len(summaries) != 1 {
		t.Fatalf("List() = %v, %v; want one session", summaries, err)
	}
	loaded, err := sessions.Load(summaries[0])
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(loaded.Events) != 2 {
		t.Fatalf("persisted %d events, want session start and user message", len(loaded.Events))
	}
	user := loaded.Events[1].UserMessage
	if user == nil || user.Content != "formatted: hello" {
		t.Fatalf("persisted user message = %#v", user)
	}
}

func TestSessionAllowsOnlyOneActiveTurn(t *testing.T) {
	agent := &testAgent{}
	sessions := newPersistentSession(t, agent)

	first, err := sessions.StartTurn(context.Background(), "first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.StartTurn(context.Background(), "second"); !errors.Is(err, app.ErrTurnActive) {
		t.Fatalf("second StartTurn() error = %v, want ErrTurnActive", err)
	}
	first.Close()
	second, err := sessions.StartTurn(context.Background(), "second")
	if err != nil {
		t.Fatalf("StartTurn() after Close error: %v", err)
	}
	second.Close()
}

func TestSessionPersistsCompactionAndResumeRestoresConversation(t *testing.T) {
	agent := &testAgent{}
	sessions := newPersistentSession(t, agent)
	turn, err := sessions.StartTurn(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	turn.Close()

	transcript := []session.TranscriptItem{{Kind: session.TranscriptItemText, Content: "working"}}
	memory := &agentcore.TurnMemory{ToolActivity: []agentcore.HistoricalToolActivity{{Tool: "read_file", Status: "success"}}}
	if err := sessions.AppendAutoCompaction(transcript, agentcore.Message{
		Role:       agentcore.RoleAssistant,
		Content:    "working",
		TurnMemory: memory,
	}, []agentcore.Message{
		{Role: agentcore.RoleUser, Content: "summary"},
	}); err != nil {
		t.Fatalf("AppendAutoCompaction() error: %v", err)
	}

	summaries, err := sessions.List()
	if err != nil || len(summaries) != 1 {
		t.Fatalf("List() = %v, %v; want one session", summaries, err)
	}
	loaded, err := sessions.Load(summaries[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 4 {
		t.Fatalf("persisted %d events, want 4", len(loaded.Events))
	}
	checkpoint := loaded.Events[2].AssistantTurn
	if checkpoint == nil || checkpoint.Message != "working" || checkpoint.TurnMemory == nil || len(checkpoint.Transcript) != 1 {
		t.Fatalf("checkpoint = %#v", checkpoint)
	}
	compaction := loaded.Events[3].CompactionApplied
	if compaction == nil || len(compaction.Messages) != 1 || compaction.Messages[0].Content != "summary" {
		t.Fatalf("compaction = %#v", compaction)
	}
	if len(agent.messages) != 1 || agent.messages[0].Content != "summary" {
		t.Fatalf("restored messages = %#v", agent.messages)
	}
}
