package llm

import (
	"strconv"
	"unicode/utf8"
)

type historicalMessageStep struct {
	Text       string
	Activities []historicalToolInvocation
}

type historicalToolInvocation struct {
	ID       string
	Activity HistoricalToolActivity
}

func FormatMessageForProvider(message Message) string {
	return message.Content
}

func historicalMessageSteps(messageIndex int, message Message) []historicalMessageStep {
	if message.Role != RoleAssistant || message.TurnMemory == nil || len(message.TurnMemory.ToolActivity) == 0 {
		return []historicalMessageStep{{Text: FormatMessageForProvider(message)}}
	}

	steps := make([]historicalMessageStep, 0, len(message.TurnMemory.ToolActivity)+1)
	cursor := 0
	activityIndex := 0
	for _, activity := range message.TurnMemory.ToolActivity {
		if activity.TextOffset < cursor || activity.TextOffset > len(message.Content) || activity.Tool == "" {
			continue
		}
		if activity.TextOffset > 0 && activity.TextOffset < len(message.Content) && !utf8.RuneStart(message.Content[activity.TextOffset]) {
			continue
		}

		invocation := historicalToolInvocation{
			ID:       "historical_" + strconv.Itoa(messageIndex) + "_" + strconv.Itoa(activityIndex),
			Activity: activity,
		}
		activityIndex++

		if len(steps) > 0 && activity.TextOffset == cursor && len(steps[len(steps)-1].Activities) > 0 {
			steps[len(steps)-1].Activities = append(steps[len(steps)-1].Activities, invocation)
			continue
		}

		steps = append(steps, historicalMessageStep{
			Text:       message.Content[cursor:activity.TextOffset],
			Activities: []historicalToolInvocation{invocation},
		})
		cursor = activity.TextOffset
	}

	if len(steps) == 0 {
		return []historicalMessageStep{{Text: FormatMessageForProvider(message)}}
	}

	finalMessage := message
	finalMessage.Content = message.Content[cursor:]
	steps = append(steps, historicalMessageStep{Text: FormatMessageForProvider(finalMessage)})
	return steps
}

func historicalToolInput(activity HistoricalToolActivity) map[string]any {
	if activity.Input == nil {
		return map[string]any{}
	}
	return activity.Input
}

func historicalToolArguments(activity HistoricalToolActivity) string {
	return serializeJSON(historicalToolInput(activity))
}

func historicalToolResult(activity HistoricalToolActivity) string {
	if activity.HasRawOutput {
		return serializeJSON(activity.RawOutput)
	}

	status := activity.Status
	if status != "success" {
		status = "error"
	}
	result := struct {
		Status   string `json:"status"`
		ExitCode *int   `json:"exit_code,omitempty"`
	}{
		Status:   status,
		ExitCode: activity.ExitCode,
	}
	return serializeJSON(result)
}
