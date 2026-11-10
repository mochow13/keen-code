package compaction

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/tools"
)

func BuildRequest(history []core.Message, prompt string, automatic bool) ([]core.Message, error) {
	request := core.CloneMessages(history)
	if automatic {
		if _, ok := LatestUserMessage(history); !ok {
			return nil, fmt.Errorf("automatic compaction requires a user message")
		}
	}
	request = append(request, core.Message{Role: core.RoleUser, Content: prompt})
	return request, nil
}

func LatestUserMessage(messages []core.Message) (core.Message, bool) {
	for _, message := range slices.Backward(messages) {
		if message.Role == core.RoleUser {
			return message, true
		}
	}
	return core.Message{}, false
}

func AutomaticReplacement(summary string, history []core.Message) ([]core.Message, error) {
	latest, ok := LatestUserMessage(history)
	if !ok {
		return nil, fmt.Errorf("automatic compaction requires a user message")
	}
	if strings.TrimSpace(summary) == "" {
		return nil, fmt.Errorf("automatic compaction produced an empty summary")
	}
	content := "<compacted_context>\nThe prior conversation context was compacted automatically to fit the model's context window. The following summary preserves relevant goals, constraints, progress, discoveries, tool results, and pending work.\n\n" + summary + "\n</compacted_context>\n\n<last_user_message>\nThe following is the most recent user message. Treat it as the current task and its requirements as authoritative.\n\n" + latest.Content + "\n</last_user_message>"
	content = tools.AppendTodoContext(content, LatestTodos(history))
	replacement := make([]core.Message, 0, len(history)+1)
	for _, message := range core.CloneMessages(history) {
		if message.Role == core.RoleSystem {
			replacement = append(replacement, message)
		}
	}
	return append(replacement, core.Message{Role: core.RoleUser, Content: content}), nil
}

func LatestTodos(history []core.Message) tools.TodoList {
	for _, message := range slices.Backward(history) {
		if message.TurnMemory == nil {
			continue
		}
		for _, activity := range slices.Backward(message.TurnMemory.ToolActivity) {
			if activity.Tool != tools.TodoWriteToolName || activity.Status != "success" || activity.Input == nil {
				continue
			}
			if todos, err := tools.ParseTodoWriteInput(activity.Input); err == nil {
				return todos
			}
		}
	}
	return nil
}

func IsCancellation(err error) bool {
	return errors.Is(err, context.Canceled)
}
