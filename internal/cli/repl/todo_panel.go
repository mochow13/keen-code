package repl

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/app"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	"github.com/mochow13/keen-code/internal/tools"
)

const (
	keyCtrlT = "ctrl+t"

	todoPanelMaxRows           = 5
	todoPanelCompactBelowLines = 20
)

type todoPanelState struct {
	collapsed bool
	dismissed bool
}

func (m *replModel) todos() app.Todos {
	return m.sessions.Todos()
}

func (m *replModel) todoPanelVisible() bool {
	return len(m.todos()) > 0 && !m.todoPanel.dismissed
}

func (m *replModel) todoPanelCompact() bool {
	return m.todoPanel.collapsed || (m.height > 0 && m.height < todoPanelCompactBelowLines)
}

func (m *replModel) todoPanelLines() []string {
	if !m.todoPanelVisible() {
		return nil
	}
	return renderTodoPanel(m.todos(), m.width, m.todoPanelCompact())
}

func (m *replModel) todoPanelHeight() int {
	lines := m.todoPanelLines()
	if len(lines) == 0 {
		return 0
	}
	return 1 + len(lines)
}

func (m *replModel) renderTodoPanelView() string {
	lines := m.todoPanelLines()
	if len(lines) == 0 {
		return ""
	}
	return "\n" + strings.Join(lines, "\n") + "\n"
}

func (m *replModel) observeTodos(toolCall *agentcore.ToolCall) bool {
	if !m.sessions.ObserveToolCall(toolCall) {
		return false
	}
	m.todoPanel.dismissed = false
	return true
}

func (m *replModel) dismissCompletedTodos() {
	if m.todos().AllDone() {
		m.todoPanel.dismissed = true
	}
}

func (m *replModel) resetTodoPanelForSession() {
	m.todoPanel.dismissed = m.todos().AllDone()
}

func (m *replModel) toggleTodoPanel() {
	m.todoPanel.collapsed = !m.todoPanel.collapsed
	m.adjustTextareaHeight()
	m.updateViewportContent()
	m.scrollToBottomIfFollowing()
}

func (m *replModel) turnLoadingText() string {
	if active, ok := m.todos().Active(); ok {
		return active.ActiveForm + "…"
	}
	return nextLoadingText()
}

func renderTodoPanel(todos app.Todos, width int, compact bool) []string {
	if len(todos) == 0 {
		return nil
	}
	maxWidth := max(width, 1)
	fit := func(line string) string { return ansi.Truncate(line, maxWidth, "…") }
	done, total := todos.Counts()
	if todos.AllDone() {
		return []string{fit("  " + repltheme.TodoDoneMarkStyle.Render("✓") + " " + repltheme.TodoDoneStyle.Render("All "+pluralTasks(total)+" done"))}
	}
	header := "  " + repltheme.TodoHeaderStyle.Render("Tasks") + " " + repltheme.TodoCountStyle.Render(fmt.Sprintf("%d/%d", done, total))
	if compact {
		focus := todos[todoAnchor(todos)]
		return []string{fit(header + repltheme.TodoCountStyle.Render(" · ") + renderTodoItem(focus))}
	}
	lines := []string{fit(header)}
	start, end := todoWindow(todos, todoPanelMaxRows)
	if start > 0 {
		lines = append(lines, fit("  "+renderTodoEarlier(todos[:start])))
	}
	for _, todo := range todos[start:end] {
		lines = append(lines, fit("  "+renderTodoItem(todo)))
	}
	if rest := len(todos) - end; rest > 0 {
		lines = append(lines, fit("  "+repltheme.TodoDoneStyle.Render(fmt.Sprintf("□ +%d more", rest))))
	}
	return lines
}

func renderTodoItem(todo tools.TodoItem) string {
	switch todo.Status {
	case tools.TodoCompleted:
		return repltheme.TodoDoneMarkStyle.Render("✓") + " " + repltheme.TodoDoneStyle.Render(todo.Content)
	case tools.TodoInProgress:
		return repltheme.TodoActiveStyle.Render("■ " + todo.Content)
	default:
		return repltheme.TodoPendingStyle.Render("□ " + todo.Content)
	}
}

func renderTodoEarlier(earlier app.Todos) string {
	done, total := earlier.Counts()
	if done == total {
		return repltheme.TodoDoneMarkStyle.Render("✓") + " " + repltheme.TodoDoneStyle.Render(fmt.Sprintf("%d completed", done))
	}
	return repltheme.TodoDoneStyle.Render(fmt.Sprintf("… %d earlier", total))
}

// todoAnchor is the task the panel centres on: the active task, else the
// first unfinished one.
func todoAnchor(todos app.Todos) int {
	first := -1
	for i, todo := range todos {
		if todo.Status == tools.TodoInProgress {
			return i
		}
		if first < 0 && todo.Status != tools.TodoCompleted {
			first = i
		}
	}
	return max(first, 0)
}

// todoWindow returns the slice of tasks to list individually within maxRows
// rows; tasks before it fold into a summary row and tasks after it into a
// "+N more" row. Fold rows always cover at least two tasks.
func todoWindow(todos app.Todos, maxRows int) (start, end int) {
	n := len(todos)
	if n <= maxRows {
		return 0, n
	}
	anchor := todoAnchor(todos)
	switch {
	case n-anchor <= maxRows-1:
		return n - (maxRows - 1), n
	case anchor <= 1:
		return 0, maxRows - 1
	default:
		return anchor, anchor + maxRows - 2
	}
}

func pluralTasks(n int) string {
	if n == 1 {
		return "1 task"
	}
	return fmt.Sprintf("%d tasks", n)
}
