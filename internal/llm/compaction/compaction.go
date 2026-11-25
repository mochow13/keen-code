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

const (
	compactedTag   = "<compacted_context>\n"
	compactedOpen  = compactedTag + "The prior conversation context was compacted to fit the model's context window. The following summary preserves relevant goals, constraints, progress, discoveries, tool results, and pending work.\n\n"
	compactedClose = "\n</compacted_context>"
	lastUserOpen   = "\n\n<last_user_message>\nThe following is the most recent user message. Treat it as the current task and its requirements as authoritative.\n\n"
	lastUserClose  = "\n</last_user_message>"
)

func wrapSummary(summary string) string {
	return compactedOpen + strings.TrimSpace(summary) + compactedClose
}

// ManualReplacement builds the user message that replaces the history after a
// user-requested compaction. It runs between turns, so unlike
// AutomaticReplacement it keeps no user message as the current task.
func ManualReplacement(summary string, todos tools.TodoList) (core.Message, error) {
	if strings.TrimSpace(summary) == "" {
		return core.Message{}, fmt.Errorf("compaction returned empty summary")
	}
	return core.Message{Role: core.RoleUser, Content: tools.AppendTodoContext(wrapSummary(summary), todos)}, nil
}

func AutomaticReplacement(summary string, history []core.Message) ([]core.Message, error) {
	latest, ok := LatestUserMessage(history)
	if !ok {
		return nil, fmt.Errorf("automatic compaction requires a user message")
	}
	if strings.TrimSpace(summary) == "" {
		return nil, fmt.Errorf("automatic compaction produced an empty summary")
	}
	userMessage, priorTodoContext := unwrapReplacement(latest.Content)
	content := wrapSummary(summary) + lastUserOpen + userMessage + lastUserClose
	if todos, found := latestTodos(history); found {
		content = tools.AppendTodoContext(content, todos)
	} else {
		content += priorTodoContext
	}
	replacement := make([]core.Message, 0, len(history)+1)
	for _, message := range core.CloneMessages(history) {
		if message.Role == core.RoleSystem {
			replacement = append(replacement, message)
		}
	}
	return append(replacement, core.Message{Role: core.RoleUser, Content: content}), nil
}

// unwrapReplacement recovers the original user message, and any todo context
// that followed it, from a message produced by an earlier AutomaticReplacement.
// Other content is returned unchanged.
func unwrapReplacement(content string) (userMessage, todoContext string) {
	const marker = compactedClose + lastUserOpen
	userMessage = content
	for first := true; strings.HasPrefix(userMessage, compactedTag); first = false {
		start := strings.Index(userMessage, marker)
		end := strings.LastIndex(userMessage, lastUserClose)
		if start < 0 || end < start+len(marker) {
			break
		}
		if first {
			todoContext = userMessage[end+len(lastUserClose):]
		}
		userMessage = userMessage[start+len(marker) : end]
	}
	return userMessage, todoContext
}

func LatestTodos(history []core.Message) tools.TodoList {
	todos, _ := latestTodos(history)
	return todos
}

func latestTodos(history []core.Message) (tools.TodoList, bool) {
	for _, message := range slices.Backward(history) {
		if message.TurnMemory == nil {
			continue
		}
		for _, activity := range slices.Backward(message.TurnMemory.ToolActivity) {
			if activity.Tool != tools.TodoWriteToolName || activity.Status != "success" || activity.Input == nil {
				continue
			}
			if todos, err := tools.ParseTodoWriteInput(activity.Input); err == nil {
				return todos, true
			}
		}
	}
	return nil, false
}

func IsCancellation(err error) bool {
	return errors.Is(err, context.Canceled)
}
