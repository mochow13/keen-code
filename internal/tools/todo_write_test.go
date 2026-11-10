package tools

import (
	"context"
	"strings"
	"testing"
)

func todoInput(items ...map[string]any) map[string]any {
	todos := make([]any, len(items))
	for i, item := range items {
		todos[i] = item
	}
	return map[string]any{"todos": todos}
}

func todo(content, status string) map[string]any {
	return map[string]any{"content": content, "activeForm": content + "ing", "status": status}
}

func TestTodoWriteValidInput(t *testing.T) {
	tool := NewTodoWriteTool()
	input := todoInput(todo(" Explore ", "completed"), todo("Implement", "in_progress"), todo("Test", "pending"))
	if err := tool.ValidateInput(context.Background(), input); err != nil {
		t.Fatalf("ValidateInput: %v", err)
	}
	todos, err := ParseTodoWriteInput(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 3 || todos[0].Content != "Explore" || todos[1].Status != TodoInProgress || todos[1].ActiveForm != "Implementing" {
		t.Fatalf("unexpected todos: %#v", todos)
	}
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if text, _ := out.(string); !strings.Contains(text, "1/3 completed") {
		t.Fatalf("unexpected output: %v", out)
	}
}

func TestTodoWriteEmptyListClears(t *testing.T) {
	out, err := NewTodoWriteTool().Execute(context.Background(), todoInput())
	if err != nil || out != "Task list cleared." {
		t.Fatalf("got %v, %v", out, err)
	}
}

func TestTodoWriteInvalidInput(t *testing.T) {
	tooMany := make([]map[string]any, maxTodoItems+1)
	for i := range tooMany {
		tooMany[i] = todo("Task", "pending")
	}
	tests := map[string]struct {
		input any
		want  string
	}{
		"not a map":       {input: "x", want: "expected map"},
		"missing todos":   {input: map[string]any{}, want: `missing required "todos"`},
		"todos not array": {input: map[string]any{"todos": "x"}, want: "must be an array"},
		"item not object": {input: map[string]any{"todos": []any{"x"}}, want: "todos[0] must be an object"},
		"empty content":   {input: todoInput(todo("  ", "pending")), want: "content must be a non-empty"},
		"missing active":  {input: todoInput(map[string]any{"content": "A", "status": "pending"}), want: "activeForm must be a non-empty"},
		"bad status":      {input: todoInput(todo("A", "done")), want: "status must be one of"},
		"two in progress": {input: todoInput(todo("A", "in_progress"), todo("B", "in_progress")), want: "only one task may be in_progress"},
		"too many":        {input: todoInput(tooMany...), want: "at most 30"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ParseTodoWriteInput(tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestAppendTodoContext(t *testing.T) {
	if got := AppendTodoContext("summary", nil); got != "summary" {
		t.Fatalf("empty list should not change content: %q", got)
	}
	if got := AppendTodoContext("summary", TodoList{{Content: "Task A", Status: TodoCompleted}}); got != "summary" {
		t.Fatalf("finished list should not change content: %q", got)
	}
	got := AppendTodoContext("summary", TodoList{
		{Content: "Task A", Status: TodoCompleted},
		{Content: "Task B", Status: TodoInProgress},
		{Content: "Task C", Status: TodoPending},
	})
	for _, want := range []string{"summary\n\n<current_todos>", "- [completed] Task A", "- [in_progress] Task B", "- [pending] Task C", "</current_todos>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}
