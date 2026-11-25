package compaction

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/tools"
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

func TestAutomaticReplacementDoesNotNestOnRepeatedCompaction(t *testing.T) {
	history := []core.Message{
		{Role: core.RoleSystem, Content: "system"},
		{Role: core.RoleUser, Content: "original task"},
		{Role: core.RoleAssistant, TurnMemory: &core.TurnMemory{ToolActivity: []core.HistoricalToolActivity{todoActivity("success", "completed", "in_progress")}}},
	}
	for _, summary := range []string{"summary one", "summary two", "summary three"} {
		replacement, err := AutomaticReplacement(summary, history)
		if err != nil {
			t.Fatal(err)
		}
		history = append(replacement, core.Message{Role: core.RoleAssistant, Content: "more work"})
	}

	want, err := AutomaticReplacement("summary three", []core.Message{
		{Role: core.RoleSystem, Content: "system"},
		{Role: core.RoleUser, Content: "original task"},
		{Role: core.RoleAssistant, TurnMemory: &core.TurnMemory{ToolActivity: []core.HistoricalToolActivity{todoActivity("success", "completed", "in_progress")}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := history[1].Content; got != want[1].Content {
		t.Fatalf("repeated compaction changed the replacement shape:\n%s", got)
	}
}

func TestAutomaticReplacementNewTodosReplaceCarriedTodos(t *testing.T) {
	first, err := AutomaticReplacement("summary one", []core.Message{
		{Role: core.RoleUser, Content: "original task"},
		{Role: core.RoleAssistant, TurnMemory: &core.TurnMemory{ToolActivity: []core.HistoricalToolActivity{todoActivity("success", "in_progress")}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	history := append(first, core.Message{Role: core.RoleAssistant, TurnMemory: &core.TurnMemory{ToolActivity: []core.HistoricalToolActivity{todoActivity("success", "completed")}}})
	second, err := AutomaticReplacement("summary two", history)
	if err != nil {
		t.Fatal(err)
	}
	content := second[len(second)-1].Content
	if strings.Contains(content, "<current_todos>") || !strings.HasSuffix(content, "original task\n</last_user_message>") {
		t.Fatalf("stale todos carried forward: %q", content)
	}
}

func TestAutomaticReplacementUnwrapsAlreadyNestedHistory(t *testing.T) {
	legacyOpen := compactedTag + "The prior conversation context was compacted automatically to fit the model's context window.\n\n"
	nested := compactedOpen + "two" + compactedClose + lastUserOpen + legacyOpen + "one" + compactedClose + lastUserOpen + "original task" + lastUserClose + lastUserClose
	replacement, err := AutomaticReplacement("three", []core.Message{{Role: core.RoleUser, Content: nested}})
	if err != nil {
		t.Fatal(err)
	}
	if want := compactedOpen + "three" + compactedClose + lastUserOpen + "original task" + lastUserClose; replacement[0].Content != want {
		t.Fatalf("unexpected replacement: %q", replacement[0].Content)
	}
}

func TestManualReplacementSharesAutomaticFraming(t *testing.T) {
	manual, err := ManualReplacement("  summary  ", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := compactedOpen + "summary" + compactedClose; manual.Role != core.RoleUser || manual.Content != want {
		t.Fatalf("unexpected manual replacement: %#v", manual)
	}
	automatic, err := AutomaticReplacement("summary", []core.Message{{Role: core.RoleUser, Content: "task"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(automatic[0].Content, manual.Content+lastUserOpen) {
		t.Fatalf("automatic replacement does not start with the manual framing: %q", automatic[0].Content)
	}
}

func TestManualReplacementAppendsOpenTodosAfterSummary(t *testing.T) {
	todos, err := tools.ParseTodoWriteInput(todoActivity("success", "completed", "in_progress").Input)
	if err != nil {
		t.Fatal(err)
	}
	manual, err := ManualReplacement("summary", todos)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(manual.Content, compactedOpen+"summary"+compactedClose+"\n\n<current_todos>") || !strings.Contains(manual.Content, "- [in_progress] Task B") {
		t.Fatalf("unexpected manual replacement: %q", manual.Content)
	}
}

func TestManualReplacementRejectsEmptySummary(t *testing.T) {
	if _, err := ManualReplacement(" \n\t ", nil); err == nil || err.Error() != "compaction returned empty summary" {
		t.Fatalf("expected empty-summary error, got %v", err)
	}
}
