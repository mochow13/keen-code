package llm

import (
	"context"

	"github.com/mochow13/keen-code/internal/llm/core"
)

// sendStreamEvent stops waiting when the consumer cancels the stream.
func sendStreamEvent(ctx context.Context, events chan<- core.StreamEvent, event core.StreamEvent) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case events <- event:
		return true
	}
}
