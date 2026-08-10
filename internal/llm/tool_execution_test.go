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

	_, err, started := executeValidatedTool(context.Background(), registry, tool.Name(), map[string]any{}, events)

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

	output, err, started := executeValidatedTool(context.Background(), registry, tool.Name(), input, events)

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
