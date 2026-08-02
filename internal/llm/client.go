package llm

import (
	"context"
	"strings"

	"github.com/user/keen-code/internal/tools"
)

type LLMClient interface {
	StreamChat(ctx context.Context, messages []Message, toolRegistry *tools.Registry, opts ...StreamOptions) (<-chan StreamEvent, error)
	Reset()
}

type StreamOptions struct {
	SessionID             string
	OneShot               bool
	DisableAutoCompaction bool
}

func streamOptions(opts []StreamOptions) StreamOptions {
	if len(opts) == 0 {
		return StreamOptions{}
	}
	return opts[0]
}

func opencodeSessionID(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return strings.ReplaceAll(sessionID, "-", "")
}

func promptCacheKey(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return strings.ReplaceAll(sessionID, "-", "")
}
