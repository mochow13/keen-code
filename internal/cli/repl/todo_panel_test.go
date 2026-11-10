package repl

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/app"
	"github.com/mochow13/keen-code/internal/tools"
)

func makeTodos(statuses ...tools.TodoStatus) app.Todos {
	todos := make(app.Todos, len(statuses))
	for i, status := range statuses {
		todos[i] = tools.TodoItem{Content: fmt.Sprintf("Task %d", i+1), ActiveForm: fmt.Sprintf("Doing task %d", i+1), Status: status}
	}
	return todos
}

func todoWriteCall(todos app.Todos) *agentcore.ToolCall {
	items := make([]any, len(todos))
	for i, todo := range todos {
		items[i] = map[string]any{"content": todo.Content, "activeForm": todo.ActiveForm, "status": string(todo.Status)}
	}
	return &agentcore.ToolCall{Name: agentcore.ToolNameTodoWrite, Input: map[string]any{"todos": items}}
}

func plainLines(lines []string) string {
	return ansi.Strip(strings.Join(lines, "\n"))
}

const (
	tp = tools.TodoPending
	ta = tools.TodoInProgress
	tc = tools.TodoCompleted
)

func TestTodoWindow(t *testing.T) {
	tests := []struct {
		name       string
		todos      app.Todos
		start, end int
	}{
		{"fits", makeTodos(tc, ta, tp, tp, tp), 0, 5},
		{"fresh list", makeTodos(tp, tp, tp, tp, tp, tp, tp, tp), 0, 4},
		{"second task active", makeTodos(tc, ta, tp, tp, tp, tp, tp, tp), 0, 4},
		{"middle", makeTodos(tc, tc, tc, ta, tp, tp, tp, tp, tp), 3, 6},
		{"near end", makeTodos(tc, tc, tc, tc, tc, tc, ta, tp), 4, 8},
		{"no active uses first unfinished", makeTodos(tc, tc, tc, tp, tp, tp, tp, tp, tp), 3, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := todoWindow(tt.todos, todoPanelMaxRows)
			if start != tt.start || end != tt.end {
				t.Fatalf("got [%d,%d), want [%d,%d)", start, end, tt.start, tt.end)
			}
			rows := end - start
			if start > 0 {
				rows++
			}
			if end < len(tt.todos) {
				rows++
			}
			if rows > todoPanelMaxRows {
				t.Fatalf("window uses %d rows, max %d", rows, todoPanelMaxRows)
			}
		})
	}
}

func TestRenderTodoPanelExpanded(t *testing.T) {
	got := plainLines(renderTodoPanel(makeTodos(tc, tc, tc, ta, tp, tp, tp, tp, tp), 80, false))
	want := strings.Join([]string{
		"  Tasks 3/9",
		"  ✓ 3 completed",
		"  ■ Task 4",
		"  □ Task 5",
		"  □ Task 6",
		"  □ +3 more",
	}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderTodoPanelCompactAndDone(t *testing.T) {
	if got := plainLines(renderTodoPanel(makeTodos(tc, ta, tp), 80, true)); got != "  Tasks 1/3 · ■ Task 2" {
		t.Fatalf("compact: %q", got)
	}
	if got := plainLines(renderTodoPanel(makeTodos(tc, tp, tp), 80, true)); got != "  Tasks 1/3 · □ Task 2" {
		t.Fatalf("compact without active task: %q", got)
	}
	if got := plainLines(renderTodoPanel(makeTodos(tc, tc), 80, false)); got != "  ✓ All 2 tasks done" {
		t.Fatalf("done: %q", got)
	}
}

func TestRenderTodoPanelTruncatesToWidth(t *testing.T) {
	todos := makeTodos(ta)
	todos[0].Content = strings.Repeat("long ", 30)
	for _, line := range renderTodoPanel(todos, 30, false) {
		if w := ansi.StringWidth(line); w > 30 {
			t.Fatalf("line width %d exceeds 30: %q", w, ansi.Strip(line))
		}
	}
}

func TestTodoPanelLifecycle(t *testing.T) {
	m := newTestModel()
	m.sessions = app.NewSessionWithoutStore(m.agentCore)
	m.adjustTextareaHeight()
	baseViewport := m.viewport.Height()
	if m.todoPanelHeight() != 0 || m.renderTodoPanelView() != "" {
		t.Fatal("panel should be hidden without todos")
	}

	m.handleToolEnd(todoWriteCall(makeTodos(tc, ta, tp)))
	if got := m.todoPanelHeight(); got != 5 {
		t.Fatalf("expanded panel height = %d, want 5", got)
	}
	if m.viewport.Height() != baseViewport-5 {
		t.Fatalf("viewport height = %d, want %d", m.viewport.Height(), baseViewport-5)
	}
	if m.inputAreaTop() != m.viewport.Height()+m.todoPanelHeight() {
		t.Fatalf("inputAreaTop must account for the panel")
	}
	if got := m.turnLoadingText(); got != "Doing task 2…" {
		t.Fatalf("loading text = %q", got)
	}

	m.toggleTodoPanel()
	if got := m.todoPanelHeight(); got != 2 {
		t.Fatalf("collapsed panel height = %d, want 2", got)
	}
	m.toggleTodoPanel()

	m.handleToolEnd(todoWriteCall(makeTodos(tc, tc, tc)))
	if got := plainLines(m.todoPanelLines()); got != "  ✓ All 3 tasks done" {
		t.Fatalf("done panel: %q", got)
	}
	m.dismissCompletedTodos()
	if m.todoPanelVisible() {
		t.Fatal("completed list should hide once the user moves on")
	}

	m.handleToolEnd(todoWriteCall(makeTodos(ta)))
	if !m.todoPanelVisible() {
		t.Fatal("a new list should show the panel again")
	}
}

func TestTodoPanelCompactsOnShortTerminal(t *testing.T) {
	m := newTestModel()
	m.sessions = app.NewSessionWithoutStore(m.agentCore)
	m.height = todoPanelCompactBelowLines - 1
	m.sessions.ObserveToolCall(todoWriteCall(makeTodos(tc, ta, tp)))
	if got := len(m.todoPanelLines()); got != 1 {
		t.Fatalf("short terminal should use one line, got %d", got)
	}
}
