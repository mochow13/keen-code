package llm

import (
	"context"
	"fmt"

	"github.com/mochow13/keen-code/internal/llm/compress"
	"github.com/mochow13/keen-code/internal/tools"
)

func historicalToolActivity(name string, input map[string]any, output, llmOutput any, execErr error) HistoricalToolActivity {
	activity := HistoricalToolActivity{
		Tool:         name,
		Input:        input,
		HasRawOutput: true,
	}
	if execErr != nil {
		activity.Status = "error"
		activity.RawOutput = map[string]any{"error": execErr.Error()}
		return activity
	}

	activity.Status = "success"
	activity.RawOutput = output
	activity.RetainedOutput = llmOutput
	return activity
}

func executeValidatedTool(
	ctx context.Context,
	registry *tools.Registry,
	name string,
	input map[string]any,
	eventCh chan<- StreamEvent,
) (rawOutput, llmOutput any, err error, started bool) {
	if registry == nil {
		return nil, nil, fmt.Errorf("tool registry not available"), false
	}
	tool, exists := registry.Get(name)
	if !exists {
		return nil, nil, fmt.Errorf("tool %q not found", name), false
	}
	if err := tools.ValidateInput(ctx, tool, input); err != nil {
		return nil, nil, err, false
	}
	eventCh <- StreamEvent{
		Type: StreamEventTypeToolStart,
		ToolCall: &ToolCall{
			Name:  name,
			Input: input,
		},
	}
	rawOutput, err = tool.Execute(ctx, input)
	if err != nil {
		return rawOutput, rawOutput, err, true
	}
	return rawOutput, compress.ForLLM(name, rawOutput), nil, true
}
