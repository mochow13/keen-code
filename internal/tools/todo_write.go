package tools

import (
	"context"
	"fmt"
	"strings"
)

const maxTodoItems = 30

type TodoStatus string

const (
	TodoPending    TodoStatus = "pending"
	TodoInProgress TodoStatus = "in_progress"
	TodoCompleted  TodoStatus = "completed"
)

type TodoItem struct {
	Content    string     `json:"content"`
	ActiveForm string     `json:"activeForm"`
	Status     TodoStatus `json:"status"`
}

type TodoList []TodoItem

func (l TodoList) Counts() (done, total int) {
	for _, todo := range l {
		if todo.Status == TodoCompleted {
			done++
		}
	}
	return done, len(l)
}

func (l TodoList) Active() (TodoItem, bool) {
	for _, todo := range l {
		if todo.Status == TodoInProgress {
			return todo, true
		}
	}
	return TodoItem{}, false
}

func (l TodoList) AllDone() bool {
	done, total := l.Counts()
	return total > 0 && done == total
}

type TodoWriteTool struct{}

func NewTodoWriteTool() *TodoWriteTool { return &TodoWriteTool{} }

func (t *TodoWriteTool) Name() string { return TodoWriteToolName }

func (t *TodoWriteTool) Description() string {
	return "Create or update the task list for the current work. Always send the complete list; it replaces the previous one. " +
		"Use it for work with three or more distinct steps or when the user gives several tasks; skip it for trivial requests. " +
		"Keep exactly one task in_progress while working, mark a task completed immediately after finishing it, and send an empty list to clear it."
}

func (t *TodoWriteTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"todos": map[string]any{
				"type": "array", "maxItems": maxTodoItems,
				"description": "The complete, ordered task list.",
				"items": map[string]any{"type": "object", "properties": map[string]any{
					"content":    map[string]any{"type": "string", "description": "Imperative task description, e.g. \"Run the tests\""},
					"activeForm": map[string]any{"type": "string", "description": "Present-continuous form shown while in progress, e.g. \"Running the tests\""},
					"status":     map[string]any{"type": "string", "enum": []string{string(TodoPending), string(TodoInProgress), string(TodoCompleted)}},
				}, "required": []string{"content", "activeForm", "status"}, "additionalProperties": false},
			},
		},
		"required": []string{"todos"}, "additionalProperties": false,
	}
}

func (t *TodoWriteTool) ValidateInput(_ context.Context, input any) error {
	_, err := ParseTodoWriteInput(input)
	return err
}

func (t *TodoWriteTool) Execute(_ context.Context, input any) (any, error) {
	todos, err := ParseTodoWriteInput(input)
	if err != nil {
		return nil, err
	}
	if len(todos) == 0 {
		return "Task list cleared.", nil
	}
	done, total := todos.Counts()
	return fmt.Sprintf("Task list updated (%d/%d completed). Keep it current: one task in_progress at a time, mark tasks completed as soon as they are done.", done, total), nil
}

func AppendTodoContext(content string, todos TodoList) string {
	if done, total := todos.Counts(); done == total {
		return content
	}
	var b strings.Builder
	b.WriteString(content)
	b.WriteString("\n\n<current_todos>\nYour task list (from todo_write) at the time of compaction. Continue from it and keep it updated with todo_write.\n")
	for _, todo := range todos {
		fmt.Fprintf(&b, "- [%s] %s\n", todo.Status, todo.Content)
	}
	b.WriteString("</current_todos>")
	return b.String()
}

func ParseTodoWriteInput(input any) (TodoList, error) {
	params, ok := input.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid input: expected map[string]any, got %T", input)
	}
	rawTodos, ok := params["todos"].([]any)
	if !ok {
		if _, exists := params["todos"]; !exists {
			return nil, missingRequiredParameter(TodoWriteToolName, "todos", `{"todos":[{"content":"Run the tests","activeForm":"Running the tests","status":"in_progress"}]}`, "Send the complete list, or an empty list to clear it")
		}
		return nil, fmt.Errorf("invalid input: todos must be an array")
	}
	if len(rawTodos) > maxTodoItems {
		return nil, fmt.Errorf("invalid input: todos must contain at most %d entries", maxTodoItems)
	}
	todos := make(TodoList, len(rawTodos))
	inProgress := 0
	for i, raw := range rawTodos {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid input: todos[%d] must be an object", i)
		}
		content, _ := item["content"].(string)
		content = strings.TrimSpace(content)
		if content == "" {
			return nil, fmt.Errorf("invalid input: todos[%d].content must be a non-empty string", i)
		}
		activeForm, _ := item["activeForm"].(string)
		activeForm = strings.TrimSpace(activeForm)
		if activeForm == "" {
			return nil, fmt.Errorf("invalid input: todos[%d].activeForm must be a non-empty string", i)
		}
		status, _ := item["status"].(string)
		switch TodoStatus(status) {
		case TodoPending, TodoCompleted:
		case TodoInProgress:
			inProgress++
		default:
			return nil, fmt.Errorf("invalid input: todos[%d].status must be one of pending, in_progress, completed", i)
		}
		todos[i] = TodoItem{Content: content, ActiveForm: activeForm, Status: TodoStatus(status)}
	}
	if inProgress > 1 {
		return nil, fmt.Errorf("invalid input: only one task may be in_progress at a time, got %d", inProgress)
	}
	return todos, nil
}
