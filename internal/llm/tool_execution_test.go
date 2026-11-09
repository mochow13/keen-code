package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/llm/history"
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
	events := make(chan core.StreamEvent, 1)

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
	events := make(chan core.StreamEvent, 1)
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
	if event.Type != core.StreamEventTypeToolStart {
		t.Fatalf("expected tool start, got %q", event.Type)
	}
}

func TestExecuteValidatedTool_StripsReadFileMetadataForLLM(t *testing.T) {
	tool := &readFileMetadataTool{}
	registry := tools.NewRegistry()
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	_, output, err, started := executeValidatedTool(context.Background(), registry, tool.Name(), map[string]any{}, make(chan core.StreamEvent, 1))
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
	if got := history.ToolResult(activity); got == "" || got == history.SerializeJSON(tool.output) {
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

func TestDenyToolRegistry_RejectsExecutionAndSkipsValidation(t *testing.T) {
	tool := &validatingExecutionTool{}
	registry := tools.NewRegistry()
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	denied := denyToolRegistry(registry)
	deniedTool, ok := denied.Get(tool.Name())
	if !ok {
		t.Fatal("expected tool to remain registered")
	}
	if deniedTool.Name() != tool.Name() {
		t.Fatalf("expected tool name %q, got %q", tool.Name(), deniedTool.Name())
	}

	if err := tools.ValidateInput(context.Background(), deniedTool, map[string]any{}); err != nil {
		t.Fatalf("denied tools must not validate input, got %v", err)
	}

	if _, err := deniedTool.Execute(context.Background(), map[string]any{"value": "ok"}); err == nil || err.Error() != toolCallsDisabledMessage {
		t.Fatalf("expected rejection error, got %v", err)
	}
	if tool.executed {
		t.Fatal("denied tool must not execute the real tool")
	}
}

func TestDenyToolRegistry_NilRegistry(t *testing.T) {
	if got := denyToolRegistry(nil); got != nil {
		t.Fatalf("expected nil registry, got %#v", got)
	}
}

func TestDenyWriteToolRegistry_DeniesWritesButAllowsReads(t *testing.T) {
	write := &validatingExecutionTool{}
	registry := tools.NewRegistry()
	if err := registry.Register(writeToolStub{name: tools.WriteFileToolName}); err != nil {
		t.Fatalf("register write_file: %v", err)
	}
	if err := registry.Register(writeToolStub{name: tools.EditFileToolName}); err != nil {
		t.Fatalf("register edit_file: %v", err)
	}
	if err := registry.Register(write); err != nil {
		t.Fatalf("register validating: %v", err)
	}

	denied := denyWriteToolRegistry(registry)
	for _, name := range []string{tools.WriteFileToolName, tools.EditFileToolName, "validating"} {
		if _, ok := denied.Get(name); !ok {
			t.Fatalf("expected %s to remain registered", name)
		}
	}
	for _, name := range []string{tools.WriteFileToolName, tools.EditFileToolName} {
		got, _ := denied.Get(name)
		if _, err := got.Execute(context.Background(), nil); err == nil || err.Error() != writeToolsDisabledMessage {
			t.Fatalf("expected write rejection for %s, got %v", name, err)
		}
	}
	read, _ := denied.Get("validating")
	if _, err := read.Execute(context.Background(), nil); err != nil {
		t.Fatalf("expected read tool to execute, got %v", err)
	}
	if !write.executed {
		t.Fatal("expected non-write tool to execute the real tool")
	}
}

func TestDenyWriteToolRegistry_NilRegistry(t *testing.T) {
	if got := denyWriteToolRegistry(nil); got != nil {
		t.Fatalf("expected nil registry, got %#v", got)
	}
}

type writeToolStub struct {
	name     string
	executed bool
}

func (t writeToolStub) Name() string { return t.name }

func (t writeToolStub) Description() string { return "stub" }

func (t writeToolStub) InputSchema() map[string]any { return map[string]any{} }

func (t writeToolStub) Execute(context.Context, any) (any, error) {
	return map[string]any{"ok": true}, nil
}

func TestDenyToolRegistry_ExecutionEmitsRejectedToolEnd(t *testing.T) {
	tool := &validatingExecutionTool{}
	registry := tools.NewRegistry()
	if err := registry.Register(tool); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	events := make(chan core.StreamEvent, 2)

	execution := executeTool(context.Background(), denyToolRegistry(registry), tool.Name(), map[string]any{"value": "ok"}, events)

	if execution.Err == nil || execution.Err.Error() != toolCallsDisabledMessage {
		t.Fatalf("expected rejection error, got %v", execution.Err)
	}
	if execution.Activity.Status != "error" {
		t.Fatalf("expected error activity, got %#v", execution.Activity)
	}

	start := <-events
	if start.Type != core.StreamEventTypeToolStart {
		t.Fatalf("expected tool start, got %q", start.Type)
	}
	end := <-events
	if end.Type != core.StreamEventTypeToolEnd {
		t.Fatalf("expected tool end, got %q", end.Type)
	}
	if end.ToolCall.Error != toolCallsDisabledMessage || end.ToolCall.Output != nil {
		t.Fatalf("unexpected rejected tool end: %#v", end.ToolCall)
	}
}

func TestExecuteTool_CompletedEventRetainsBashInput(t *testing.T) {
	registry := tools.NewRegistry()
	tool := writeToolStub{name: tools.BashToolName}
	if err := registry.Register(tool); err != nil {
		t.Fatal(err)
	}
	events := make(chan core.StreamEvent, 2)
	input := map[string]any{"command": "go test ./..."}
	result := executeTool(context.Background(), registry, tool.Name(), input, events)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	start, end := <-events, <-events
	if start.Type != core.StreamEventTypeToolStart || end.Type != core.StreamEventTypeToolEnd {
		t.Fatalf("unexpected events: %q, %q", start.Type, end.Type)
	}
	if end.ToolCall == nil || end.ToolCall.Input["command"] != input["command"] {
		t.Fatalf("completed tool input: %#v", end.ToolCall)
	}
}
