package tools

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type delegateCall struct {
	agent         string
	instanceIndex int
	instanceCount int
	task          string
}

type mockSubagentRunner struct {
	mu sync.Mutex

	result any
	err    error
	calls  []delegateCall

	started chan struct{}
	release chan struct{}
}

func (m *mockSubagentRunner) RunDelegate(ctx context.Context, agent string, instanceIndex, instanceCount int, task string) (any, error) {
	m.mu.Lock()
	m.calls = append(m.calls, delegateCall{agent: agent, instanceIndex: instanceIndex, instanceCount: instanceCount, task: task})
	m.mu.Unlock()
	if m.started != nil {
		m.started <- struct{}{}
	}
	if m.release != nil {
		select {
		case <-m.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return m.result, m.err
}

func (m *mockSubagentRunner) recordedCalls() []delegateCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]delegateCall(nil), m.calls...)
}

func TestDelegateTool_Metadata(t *testing.T) {
	tool := NewDelegateTool(&mockSubagentRunner{}, []string{"explorer", "implementer"})

	if tool.Name() != "delegate_task" {
		t.Fatalf("Name() = %q, want %q", tool.Name(), "delegate_task")
	}
	if !strings.Contains(tool.Description(), "up to 10") || !strings.Contains(tool.Description(), "parallel") {
		t.Fatalf("Description() = %q, want parallel limit", tool.Description())
	}
}

func TestDelegateTool_InputSchema(t *testing.T) {
	tool := NewDelegateTool(&mockSubagentRunner{}, []string{"explorer", "implementer"})
	schema := tool.InputSchema()

	if schema["type"] != "object" {
		t.Fatalf("schema type = %v, want object", schema["type"])
	}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("required type = %T, want []string", schema["required"])
	}
	if !reflect.DeepEqual(required, []string{"tasks"}) {
		t.Fatalf("required = %v, want [tasks]", required)
	}

	properties := schema["properties"].(map[string]any)
	tasks := properties["tasks"].(map[string]any)
	if tasks["minItems"] != 1 || tasks["maxItems"] != maxDelegateTasks {
		t.Fatalf("task bounds = %v..%v, want 1..%d", tasks["minItems"], tasks["maxItems"], maxDelegateTasks)
	}
	items := tasks["items"].(map[string]any)
	if !reflect.DeepEqual(items["required"], []string{"agent", "task"}) {
		t.Fatalf("item required = %v, want [agent task]", items["required"])
	}
	itemProperties := items["properties"].(map[string]any)
	for _, name := range []string{"agent", "task"} {
		if _, ok := itemProperties[name]; !ok {
			t.Fatalf("item properties missing %q", name)
		}
	}
	if _, ok := itemProperties["timeout_seconds"]; ok {
		t.Fatal("item properties should not include timeout_seconds")
	}
	agent := itemProperties["agent"].(map[string]any)
	if !reflect.DeepEqual(agent["enum"], []string{"explorer", "implementer"}) {
		t.Fatalf("agent enum = %#v, want [explorer implementer]", agent["enum"])
	}
}

func TestDelegateTool_ExecutePassesTasksToRunner(t *testing.T) {
	runner := &mockSubagentRunner{result: map[string]any{"status": "completed"}}
	tool := NewDelegateTool(runner, []string{"explorer", "implementer"})

	result, err := tool.Execute(context.Background(), delegateTasks(
		map[string]any{"agent": "explorer", "task": "Inspect internal/tools."},
	))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	calls := runner.recordedCalls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	wantCall := delegateCall{agent: "explorer", instanceIndex: 1, instanceCount: 1, task: "Inspect internal/tools."}
	if calls[0] != wantCall {
		t.Fatalf("call = %+v, want %+v", calls[0], wantCall)
	}
	output := result.(map[string]any)
	results := output["results"].([]delegateResult)
	if len(results) != 1 || !reflect.DeepEqual(results[0].Result, runner.result) {
		t.Fatalf("results = %#v, want runner result", results)
	}
}

func TestDelegateTool_AssignsPerAgentInstancePositions(t *testing.T) {
	runner := &mockSubagentRunner{}
	tool := NewDelegateTool(runner, []string{"explorer", "implementer"})
	_, err := tool.Execute(context.Background(), delegateTasks(
		map[string]any{"agent": "explorer", "task": "inspect"},
		map[string]any{"agent": "implementer", "task": "one"},
		map[string]any{"agent": "implementer", "task": "two"},
	))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	calls := runner.recordedCalls()
	if len(calls) != 3 {
		t.Fatalf("calls = %d, want 3", len(calls))
	}
	byTask := make(map[string]delegateCall, len(calls))
	for _, call := range calls {
		byTask[call.task] = call
	}
	if got := byTask["inspect"]; got.instanceIndex != 1 || got.instanceCount != 1 {
		t.Fatalf("explorer position = (%d, %d), want (1, 1)", got.instanceIndex, got.instanceCount)
	}
	for task, wantIndex := range map[string]int{"one": 1, "two": 2} {
		got := byTask[task]
		if got.instanceIndex != wantIndex || got.instanceCount != 2 {
			t.Fatalf("implementer %s position = (%d, %d), want (%d, 2)", task, got.instanceIndex, got.instanceCount, wantIndex)
		}
	}
}

func TestDelegateTool_ExecuteRunsTasksInParallel(t *testing.T) {
	const taskCount = 3
	runner := &mockSubagentRunner{
		result:  "ok",
		started: make(chan struct{}, taskCount),
		release: make(chan struct{}),
	}
	tool := NewDelegateTool(runner, []string{"explorer", "implementer"})
	done := make(chan error, 1)

	go func() {
		_, err := tool.Execute(context.Background(), delegateTasks(
			map[string]any{"agent": "explorer", "task": "one"},
			map[string]any{"agent": "explorer", "task": "two"},
			map[string]any{"agent": "explorer", "task": "three"},
		))
		done <- err
	}()

	for i := 0; i < taskCount; i++ {
		select {
		case <-runner.started:
		case <-time.After(time.Second):
			t.Fatal("tasks did not start in parallel")
		}
	}
	close(runner.release)
	if err := <-done; err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestDelegateTool_ExecuteReturnsPerTaskErrors(t *testing.T) {
	wantErr := errors.New("subagent failed")
	runner := &mockSubagentRunner{
		result: map[string]any{"status": "error"},
		err:    wantErr,
	}
	tool := NewDelegateTool(runner, []string{"explorer", "implementer"})

	result, err := tool.Execute(context.Background(), delegateTasks(
		map[string]any{"agent": "explorer", "task": "Inspect docs."},
	))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := result.(map[string]any)
	results := output["results"].([]delegateResult)
	if len(results) != 1 || results[0].Error != wantErr.Error() {
		t.Fatalf("results = %#v, want per-task error", results)
	}
	if !reflect.DeepEqual(results[0].Result, runner.result) {
		t.Fatalf("result = %#v, want %#v", results[0].Result, runner.result)
	}
}

func TestDelegateTool_ValidateInputRejectsInvalidInput(t *testing.T) {
	tooMany := make([]any, maxDelegateTasks+1)
	for i := range tooMany {
		tooMany[i] = map[string]any{"agent": "explorer", "task": "Inspect docs."}
	}
	tests := []struct {
		name    string
		input   any
		wantErr string
	}{
		{name: "missing tasks", input: map[string]any{}, wantErr: `missing required "tasks" parameter`},
		{name: "empty tasks", input: delegateTasks(), wantErr: "at least one task"},
		{name: "too many tasks", input: map[string]any{"tasks": tooMany}, wantErr: "at most 10 tasks"},
		{name: "missing agent", input: delegateTasks(map[string]any{"task": "Inspect docs."}), wantErr: "tasks[0].agent"},
		{name: "missing task", input: delegateTasks(map[string]any{"agent": "explorer"}), wantErr: "tasks[0].task"},
		{name: "unknown agent", input: delegateTasks(map[string]any{"agent": "reviewer", "task": "Review changes."}), wantErr: "must be one of: explorer, implementer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &mockSubagentRunner{}
			tool := NewDelegateTool(runner, []string{"explorer", "implementer"})

			err := tool.ValidateInput(context.Background(), tt.input)
			if err == nil {
				t.Fatal("ValidateInput() expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateInput() error = %v, want containing %q", err, tt.wantErr)
			}
			if len(runner.recordedCalls()) != 0 {
				t.Fatal("runner should not be called for invalid input")
			}
		})
	}
}

func TestDelegateTool_ExecuteRejectsUnknownAgent(t *testing.T) {
	runner := &mockSubagentRunner{}
	tool := NewDelegateTool(runner, []string{"explorer"})

	_, err := tool.Execute(context.Background(), delegateTasks(
		map[string]any{"agent": "reviewer", "task": "Review changes."},
	))
	if err == nil || !strings.Contains(err.Error(), "must be one of: explorer") {
		t.Fatalf("Execute() error = %v, want unknown-agent validation error", err)
	}
	if len(runner.recordedCalls()) != 0 {
		t.Fatal("runner should not be called for an unknown agent")
	}
}

func TestDelegateTool_ExecuteRejectsMissingRunner(t *testing.T) {
	tool := NewDelegateTool(nil, []string{"explorer"})

	_, err := tool.Execute(context.Background(), delegateTasks(
		map[string]any{"agent": "explorer", "task": "Inspect docs."},
	))
	if err == nil {
		t.Fatal("Execute() expected error")
	}
	if !strings.Contains(err.Error(), "subagent runner not configured") {
		t.Fatalf("Execute() error = %v, want runner configuration error", err)
	}
}

func delegateTasks(tasks ...map[string]any) map[string]any {
	items := make([]any, len(tasks))
	for i, task := range tasks {
		items[i] = task
	}
	return map[string]any{"tasks": items}
}
