package llm

import (
	"context"
)

type LLMClient interface {
	StreamChat(ctx context.Context, messages []Message) (<-chan StreamEvent, error)
}
