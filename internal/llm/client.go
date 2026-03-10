package llm

import (
	"context"

	"github.com/user/keen-code/internal/tools"
)

type LLMClient interface {
	StreamChat(ctx context.Context, messages []Message, toolRegistry *tools.Registry) (<-chan StreamEvent, error)
}
