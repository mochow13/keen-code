package compaction

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/llm/core"
)

func TestBuildRequestPreservesHistoryAndAppendsPrompt(t *testing.T) {
	history := []core.Message{
		{Role: core.RoleSystem, Content: "system history"},
		{Role: core.RoleUser, Content: "task"},
	}

	request, err := BuildRequest(history, "compaction prompt", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(request) != 3 || request[0].Role != core.RoleSystem || request[0].Content != "system history" || request[1].Content != "task" || request[2].Role != core.RoleUser || request[2].Content != "compaction prompt" {
		t.Fatalf("unexpected request: %#v", request)
	}
	request[0].Content = "changed"
	if history[0].Content != "system history" {
		t.Fatal("request aliases history")
	}
}

func TestBuildRequestRequiresUserMessageForAutomaticCompaction(t *testing.T) {
	_, err := BuildRequest([]core.Message{{Role: core.RoleAssistant, Content: "response"}}, "prompt", true)
	if err == nil || !strings.Contains(err.Error(), "requires a user message") {
		t.Fatalf("expected missing-user-message error, got %v", err)
	}
}

func TestAutomaticReplacement(t *testing.T) {
	history := []core.Message{
		{Role: core.RoleSystem, Content: "system"},
		{Role: core.RoleUser, Content: "first"},
		{Role: core.RoleAssistant, Content: "answer"},
		{Role: core.RoleUser, Content: "current task"},
	}

	replacement, err := AutomaticReplacement("summary", history)
	if err != nil {
		t.Fatal(err)
	}
	if len(replacement) != 2 || replacement[0].Content != "system" || !strings.Contains(replacement[1].Content, "summary") || !strings.Contains(replacement[1].Content, "current task") {
		t.Fatalf("unexpected replacement: %#v", replacement)
	}
}

func TestAutomaticReplacementRejectsEmptySummary(t *testing.T) {
	_, err := AutomaticReplacement("  ", []core.Message{{Role: core.RoleUser, Content: "task"}})
	if err == nil || !strings.Contains(err.Error(), "empty summary") {
		t.Fatalf("expected empty-summary error, got %v", err)
	}
}

func TestIsCancellation(t *testing.T) {
	if !IsCancellation(context.Canceled) || !IsCancellation(errors.Join(errors.New("wrapped"), context.Canceled)) {
		t.Fatal("expected context cancellation to be classified")
	}
	if IsCancellation(errors.New("other")) || IsCancellation(nil) {
		t.Fatal("unexpected cancellation classification")
	}
}

func todoActivity(status string, todoStatuses ...string) core.HistoricalToolActivity {
	items := make([]any, len(todoStatuses))
	for i, s := range todoStatuses {
		items[i] = map[string]any{"content": "Task " + string(rune('A'+i)), "activeForm": "Doing", "status": s}
	}
	return core.HistoricalToolActivity{Tool: "todo_write", Status: status, Input: map[string]any{"todos": items}}
}

func TestAutomaticReplacementCarriesLatestTodos(t *testing.T) {
	history := []core.Message{
		{Role: core.RoleUser, Content: "do the work"},
		{Role: core.RoleAssistant, TurnMemory: &core.TurnMemory{ToolActivity: []core.HistoricalToolActivity{
			todoActivity("success", "pending", "pending"),
			todoActivity("success", "completed", "in_progress"),
			todoActivity("error", "completed", "completed"),
		}}},
	}
	replacement, err := AutomaticReplacement("summary", history)
	if err != nil {
		t.Fatal(err)
	}
	content := replacement[len(replacement)-1].Content
	for _, want := range []string{"<current_todos>", "- [completed] Task A", "- [in_progress] Task B"} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing %q in %q", want, content)
		}
	}
}

func TestAutomaticReplacementOmitsFinishedTodos(t *testing.T) {
	history := []core.Message{
		{Role: core.RoleUser, Content: "do the work"},
		{Role: core.RoleAssistant, TurnMemory: &core.TurnMemory{ToolActivity: []core.HistoricalToolActivity{todoActivity("success", "completed")}}},
	}
	replacement, err := AutomaticReplacement("summary", history)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(replacement[len(replacement)-1].Content, "<current_todos>") {
		t.Fatal("finished task list should not be injected")
	}
}
