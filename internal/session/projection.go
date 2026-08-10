package session

import "github.com/mochow13/keen-code/internal/llm"

func BuildConversation(events []Event) []llm.Message {
	var messages []llm.Message

	for _, event := range events {
		switch event.Kind {
		case KindUserMessage:
			if event.UserMessage != nil {
				messages = append(messages, llm.Message{
					Role:    llm.RoleUser,
					Content: event.UserMessage.Content,
				})
			}
		case KindAssistantTurn:
			if event.AssistantTurn != nil && (event.AssistantTurn.Message != "" || !event.AssistantTurn.TurnMemory.IsEmpty()) {
				messages = append(messages, llm.Message{
					Role:       llm.RoleAssistant,
					Content:    event.AssistantTurn.Message,
					TurnMemory: llm.CloneTurnMemory(event.AssistantTurn.TurnMemory),
				})
			}
		case KindCompactionApplied:
			if event.CompactionApplied != nil {
				messages = cloneMessages(event.CompactionApplied.Messages)
			}
		}
	}

	return messages
}
