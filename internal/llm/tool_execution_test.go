package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/mochow13/keen-code/internal/tools"
)

type validatingExecutionTool struct {
	executed bool
}

func (t *validatingExecutionTool) Name() string { return "validating" }

func (t *validatingExecutionTool) Description() string { return "validates input" }

func (t *validatingExecutionTool) InputSchema() map[string]any { return map[string]any{} }

func (t *validatingExecutionTool) ValidateInput(_ context.Context, input any) error {
	params, ok := input.(map[string]any)
	if !ok || params["value"] == nil {
		return errors.New("invalid input: missing required 'value' parameter")
	}
	return nil
}

func (t *validatingExecutionTool) Execute(_ context.Context, _ any) (any, error) {
	t.executed = true
	return map[string]any{"ok": true}, nil
}

func TestExecuteValidatedTool_HidesInvalidCalls(t *testing.T) {
	tool := &validatingExecutionTool{}
	registry := tools.NewRegistry()
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	events := make(chan StreamEvent, 1)

	_, _, err, started := executeValidatedTool(context.Background(), registry, tool.Name(), map[string]any{}, events)

	if err == nil {
		t.Fatal("expected validation error")
	}
	if started {
		t.Fatal("invalid tool call should not start")
	}
	if tool.executed {
		t.Fatal("invalid tool call should not execute")
	}
	if len(events) != 0 {
		t.Fatal("invalid tool call should not emit UI events")
	}
}

func TestExecuteValidatedTool_EmitsStartAfterValidation(t *testing.T) {
	tool := &validatingExecutionTool{}
	registry := tools.NewRegistry()
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	events := make(chan StreamEvent, 1)
	input := map[string]any{"value": "ok"}

	_, output, err, started := executeValidatedTool(context.Background(), registry, tool.Name(), input, events)

	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	if !started || !tool.executed {
		t.Fatal("valid tool call should start and execute")
	}
	if output == nil {
		t.Fatal("expected tool output")
	}
	event := <-events
	if event.Type != StreamEventTypeToolStart {
		t.Fatalf("expected tool start, got %q", event.Type)
	}
}

func TestExecuteValidatedTool_StripsReadFileMetadataForLLM(t *testing.T) {
	tool := &readFileMetadataTool{}
	registry := tools.NewRegistry()
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	_, output, err, started := executeValidatedTool(context.Background(), registry, tool.Name(), map[string]any{}, make(chan StreamEvent, 1))
	if err != nil || !started {
		t.Fatalf("execute tool: err = %v, started = %v", err, started)
	}
	result := output.(map[string]any)
	if _, exists := result["bytes_read"]; exists {
		t.Fatal("LLM result must not include bytes_read")
	}
	if _, exists := result["lines_read"]; exists {
		t.Fatal("LLM result must not include lines_read")
	}
	if result["total_lines"] != 1 || result["truncated"] != false {
		t.Fatalf("LLM result lost pagination metadata: %#v", result)
	}
	if tool.output["bytes_read"] != 12 || tool.output["lines_read"] != 1 {
		t.Fatalf("tool output was modified: %#v", tool.output)
	}
	activity := historicalToolActivity(tool.Name(), nil, tool.output, output, nil)
	if got := historicalToolResult(activity); got == "" || got == serializeJSON(tool.output) {
		t.Fatalf("history must retain the LLM-only output: %s", got)
	}
}

type readFileMetadataTool struct {
	output map[string]any
}

func (t *readFileMetadataTool) Name() string { return tools.ReadFileToolName }

func (t *readFileMetadataTool) Description() string { return "returns read file metadata" }

func (t *readFileMetadataTool) InputSchema() map[string]any { return map[string]any{} }

func (t *readFileMetadataTool) Execute(_ context.Context, _ any) (any, error) {
	t.output = map[string]any{
		"content":     "1:abc|content",
		"bytes_read":  12,
		"lines_read":  1,
		"total_lines": 1,
		"truncated":   false,
	}
	return t.output, nil
}
