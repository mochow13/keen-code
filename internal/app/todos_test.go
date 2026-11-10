package app_test

import (
	"testing"

	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/app"
	"github.com/mochow13/keen-code/internal/session"
	"github.com/mochow13/keen-code/internal/tools"
)

func todoWriteInput(statuses ...string) map[string]any {
	items := make([]any, len(statuses))
	for i, status := range statuses {
		items[i] = map[string]any{"content": "Task " + string(rune('A'+i)), "activeForm": "Doing " + string(rune('A'+i)), "status": status}
	}
	return map[string]any{"todos": items}
}

func todoEnd(input map[string]any, errText string) session.TranscriptItem {
	return session.TranscriptItem{Kind: session.TranscriptItemToolEnd, ToolEnd: &session.ToolEndPayload{Name: tools.TodoWriteToolName, Input: input, Error: errText}}
}

func TestSessionObserveToolCall(t *testing.T) {
	s := app.NewSessionWithoutStore(&testAgent{})
	if s.ObserveToolCall(&agentcore.ToolCall{Name: agentcore.ToolNameReadFile}) {
		t.Fatal("non-todo tool must not change todos")
	}
	if s.ObserveToolCall(&agentcore.ToolCall{Name: agentcore.ToolNameTodoWrite, Input: todoWriteInput("pending"), Error: "boom"}) {
		t.Fatal("failed todo_write must not change todos")
	}
	if !s.ObserveToolCall(&agentcore.ToolCall{Name: agentcore.ToolNameTodoWrite, Input: todoWriteInput("completed", "in_progress", "pending")}) {
		t.Fatal("successful todo_write must change todos")
	}
	todos := s.Todos()
	done, total := todos.Counts()
	active, ok := todos.Active()
	if done != 1 || total != 3 || !ok || active.Content != "Task B" || todos.AllDone() {
		t.Fatalf("unexpected todos: %#v", todos)
	}
	s.Reset()
	if len(s.Todos()) != 0 {
		t.Fatal("Reset must clear todos")
	}
}

func TestTodosFromEventsUsesLatestSuccessfulWriteAcrossCompaction(t *testing.T) {
	events := []session.Event{
		{Kind: session.KindAssistantTurn, AssistantTurn: &session.AssistantTurnPayload{Transcript: []session.TranscriptItem{
			todoEnd(todoWriteInput("pending", "pending"), ""),
			todoEnd(todoWriteInput("in_progress", "pending"), ""),
		}}},
		{Kind: session.KindCompactionApplied, CompactionApplied: &session.CompactionAppliedPayload{}},
		{Kind: session.KindAssistantTurn, AssistantTurn: &session.AssistantTurnPayload{Transcript: []session.TranscriptItem{
			todoEnd(todoWriteInput("completed", "completed"), "failed"),
		}}},
	}
	todos := app.TodosFromEvents(events)
	if len(todos) != 2 || todos[0].Status != tools.TodoInProgress {
		t.Fatalf("unexpected todos: %#v", todos)
	}
}
