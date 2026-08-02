package llm

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

func BuildCompactionRequest(history []Message, extraPrompt string, automatic bool) ([]Message, error) {
	request := make([]Message, 0, len(history)+2)
	prompt := BuildCompactionPrompt(extraPrompt)
	if automatic {
		prompt = BuildAutoCompactionPrompt()
	}
	request = append(request, Message{Role: RoleSystem, Content: prompt})
	for _, message := range CloneMessages(history) {
		if message.Role != RoleSystem {
			request = append(request, message)
		}
	}
	if automatic {
		if _, ok := latestUserMessage(history); !ok {
			return nil, fmt.Errorf("automatic compaction requires a user message")
		}
	}
	return request, nil
}

func latestUserMessage(messages []Message) (Message, bool) {
	for _, message := range slices.Backward(messages) {
		if message.Role == RoleUser {
			return message, true
		}
	}
	return Message{}, false
}

func automaticCompactionReplacement(summary string, history []Message) ([]Message, error) {
	latest, ok := latestUserMessage(history)
	if !ok {
		return nil, fmt.Errorf("automatic compaction requires a user message")
	}
	if strings.TrimSpace(summary) == "" {
		return nil, fmt.Errorf("automatic compaction produced an empty summary")
	}
	content := "<compacted_context>\nThe prior conversation context was compacted automatically to fit the model's context window. The following summary preserves relevant goals, constraints, progress, discoveries, tool results, and pending work.\n\n" + summary + "\n</compacted_context>\n\n<last_user_message>\nThe following is the most recent user message. Treat it as the current task and its requirements as authoritative.\n\n" + latest.Content + "\n</last_user_message>"
	replacement := make([]Message, 0, len(history)+1)
	for _, message := range CloneMessages(history) {
		if message.Role == RoleSystem {
			replacement = append(replacement, message)
		}
	}
	return append(replacement, Message{Role: RoleUser, Content: content}), nil
}

func AutoCompact(ctx context.Context, client LLMClient, history []Message, sessionID string) ([]Message, *TokenUsage, error) {
	request, err := BuildCompactionRequest(history, "", true)
	if err != nil {
		return nil, nil, err
	}
	events, err := client.StreamChat(ctx, request, nil, StreamOptions{
		SessionID:             sessionID,
		OneShot:               true,
		DisableAutoCompaction: true,
	})
	if err != nil {
		return nil, nil, err
	}

	var summary strings.Builder
	var usage *TokenUsage

	for event := range events {
		switch event.Type {
		case StreamEventTypeChunk:
			summary.WriteString(event.Content)
		case StreamEventTypeUsage:
			usage = event.Usage
		case StreamEventTypeError:
			if event.Error != nil {
				return nil, usage, event.Error
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, usage, err
	}
	replacement, err := automaticCompactionReplacement(summary.String(), history)
	if err != nil {
		return nil, usage, err
	}
	return replacement, usage, nil
}

func isAutoCompactionCancellation(err error) bool {
	return errors.Is(err, context.Canceled)
}
