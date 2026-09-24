package subagents

import (
	"context"
	"fmt"
	"github.com/mochow13/keen-code/internal/llm/core"
	"maps"
	"strings"
)

type ToolActivity struct {
	RunID  string
	CallID string
	Agent  string
	Event  core.StreamEvent
}

func collectResult(ctx context.Context, events <-chan core.StreamEvent, agent, runID string, activity chan<- ToolActivity) (string, error) {
	var sb strings.Builder
	var callCounter int
	pending := make([]string, 0)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return strings.TrimSpace(sb.String()), ctx.Err()
			}
			switch event.Type {
			case core.StreamEventTypeChunk:
				sb.WriteString(event.Content)
			case core.StreamEventTypeToolStart:
				callCounter++
				callID := fmt.Sprintf("tool-%d", callCounter)
				pending = append(pending, callID)
				forwardActivity(ctx, activity, sanitizeActivity(agent, runID, callID, event))
			case core.StreamEventTypeToolEnd:
				callID := ""
				if len(pending) > 0 {
					callID = pending[0]
					pending = pending[1:]
				} else {
					callCounter++
					callID = fmt.Sprintf("tool-%d", callCounter)
				}
				forwardActivity(ctx, activity, sanitizeActivity(agent, runID, callID, event))
			case core.StreamEventTypeDone:
				return strings.TrimSpace(sb.String()), nil
			case core.StreamEventTypeError, core.StreamEventTypeIncomplete:
				if event.Error != nil {
					return strings.TrimSpace(sb.String()), event.Error
				}
				return strings.TrimSpace(sb.String()), fmt.Errorf("subagent stream incomplete")
			}
		case <-ctx.Done():
			return strings.TrimSpace(sb.String()), ctx.Err()
		}
	}
}

func sanitizeActivity(agent, runID, callID string, event core.StreamEvent) ToolActivity {
	var call *core.ToolCall
	if event.ToolCall != nil {
		cloned := *event.ToolCall
		cloned.Output = nil
		if event.Type == core.StreamEventTypeToolEnd {
			cloned.Input = nil
		} else if cloned.Input != nil {
			cloned.Input = cloneInput(cloned.Input)
		}
		call = &cloned
	}
	return ToolActivity{
		RunID:  runID,
		CallID: callID,
		Agent:  agent,
		Event:  core.StreamEvent{Type: event.Type, ToolCall: call},
	}
}

func cloneInput(input map[string]any) map[string]any {
	cloned := make(map[string]any, len(input))
	maps.Copy(cloned, input)
	return cloned
}

func forwardActivity(ctx context.Context, sink chan<- ToolActivity, activity ToolActivity) {
	if sink == nil {
		return
	}
	select {
	case sink <- activity:
	case <-ctx.Done():
	}
}
