package llm

import (
	"context"

	"github.com/user/keen-cli/internal/tools"
)

type LLMClient interface {
	StreamChat(ctx context.Context, messages []Message, toolRegistry *tools.Registry) (<-chan StreamEvent, error)
}
