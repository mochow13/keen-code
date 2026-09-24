package llm

import (
	"context"
	"errors"
	"fmt"

	"log/slog"
	"time"

	"github.com/mochow13/keen-code/internal/llm/compress"
	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/tools"
)

func historicalToolActivity(name string, input map[string]any, output, llmOutput any, execErr error) core.HistoricalToolActivity {
	activity := core.HistoricalToolActivity{
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

type toolExecution struct {
	RawOutput any
	LLMOutput any
	Err       error
	Activity  core.HistoricalToolActivity
}

func executeTool(ctx context.Context, registry *tools.Registry, name string, input map[string]any, eventCh chan<- core.StreamEvent) toolExecution {
	start := time.Now()
	rawOutput, llmOutput, err, started := executeValidatedTool(ctx, registry, name, input, eventCh)
	duration := time.Since(start)
	toolCall := &core.ToolCall{Name: name, Input: input, Output: rawOutput, Duration: duration}
	if err != nil {
		toolCall.Error = err.Error()
		slog.Debug("Tool response", "tool", name, "error", err.Error(), "duration", duration)
		if started {
			sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeToolEnd, ToolCall: toolCall})
		}
	} else {
		slog.Debug("Tool response", "tool", name, "duration", duration)
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeToolEnd, ToolCall: toolCall})
	}
	return toolExecution{
		RawOutput: rawOutput,
		LLMOutput: llmOutput,
		Err:       err,
		Activity:  historicalToolActivity(name, input, rawOutput, llmOutput, err),
	}
}

const toolCallsDisabledMessage = "Tool calls are disabled during compaction; use the history."

const writeToolsDisabledMessage = "Write tools (write_file, edit_file) are disabled in plan mode; ask the user to switch with `/mode build` or `Shift+Tab`."

// denyToolRegistry preserves the tool definitions but rejects execution.
func denyToolRegistry(registry *tools.Registry) *tools.Registry {
	if registry == nil {
		return nil
	}
	denied := tools.NewRegistry()
	for _, tool := range registry.All() {
		_ = denied.Register(&deniedTool{Tool: tool})
	}
	return denied
}

type deniedTool struct {
	tools.Tool
}

func (d *deniedTool) ValidateInput(context.Context, any) error { return nil }

func (d *deniedTool) Execute(context.Context, any) (any, error) {
	return nil, errors.New(toolCallsDisabledMessage)
}

// denyWriteToolRegistry blocks write_file/edit_file but keeps definitions stable for caching.
func denyWriteToolRegistry(registry *tools.Registry) *tools.Registry {
	if registry == nil {
		return nil
	}
	denied := tools.NewRegistry()
	for _, tool := range registry.All() {
		if tool.Name() == tools.WriteFileToolName || tool.Name() == tools.EditFileToolName {
			_ = denied.Register(&deniedWriteTool{Tool: tool})
			continue
		}
		_ = denied.Register(tool)
	}
	return denied
}

type deniedWriteTool struct {
	tools.Tool
}

func (d *deniedWriteTool) ValidateInput(context.Context, any) error { return nil }

func (d *deniedWriteTool) Execute(context.Context, any) (any, error) {
	return nil, errors.New(writeToolsDisabledMessage)
}

func executeValidatedTool(
	ctx context.Context,
	registry *tools.Registry,
	name string,
	input map[string]any,
	eventCh chan<- core.StreamEvent,
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
	if !sendStreamEvent(ctx, eventCh, core.StreamEvent{
		Type: core.StreamEventTypeToolStart,
		ToolCall: &core.ToolCall{
			Name:  name,
			Input: input,
		},
	}) {
		return nil, nil, ctx.Err(), false
	}
	rawOutput, err = tool.Execute(ctx, input)
	if err != nil {
		return rawOutput, rawOutput, err, true
	}
	return rawOutput, compress.ForLLM(name, rawOutput), nil, true
}
