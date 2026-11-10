package app

import (
	"testing"

	"github.com/mochow13/keen-code/internal/agentcore"
)

func TestTurnMemoryOffsetsAndReset(t *testing.T) {
	turn := &Turn{}
	turn.ConfigureMemory(t.TempDir(), false)
	turn.RecordToolCall(&agentcore.ToolCall{Name: "read_file"}, "é")
	turn.RecordToolCall(&agentcore.ToolCall{Name: "glob"}, "é")
	memory := turn.Memory()
	if len(memory.ToolActivity) != 2 || memory.ToolActivity[0].TextOffset != 2 || memory.ToolActivity[1].TextOffset != 2 {
		t.Fatalf("unexpected activities: %#v", memory.ToolActivity)
	}
	memory.ToolActivity[0].Tool = "modified"
	if turn.Memory().ToolActivity[0].Tool != "read_file" {
		t.Fatal("Memory returned mutable internal state")
	}
	turn.ResetMemory()
	if turn.Memory() != nil {
		t.Fatal("ResetMemory retained activities")
	}
	turn.RecordToolCall(&agentcore.ToolCall{Name: "glob"}, "x")
	if got := turn.Memory().ToolActivity[0].TextOffset; got != 1 {
		t.Fatalf("offset after reset = %d", got)
	}
}

func TestTurnMemoryBashCommandFromCompletedCall(t *testing.T) {
	turn := &Turn{}
	call := &agentcore.ToolCall{Name: "bash", Input: map[string]any{"command": "go test ./..."}, Output: map[string]any{"exit_code": 1}}
	turn.RecordToolCall(call, "before")
	activity := turn.Memory().ToolActivity[0]
	if activity.Input["command"] != "go test ./..." || activity.ExitCode == nil || *activity.ExitCode != 1 {
		t.Fatalf("unexpected activity: %#v", activity)
	}
	if call.Input["command"] != "go test ./..." {
		t.Fatalf("mutated tool call: %#v", call)
	}
}

func TestTurnMemoryKeepsOnlyLatestTodoWriteInput(t *testing.T) {
	turn := &Turn{}
	turn.ConfigureMemory(t.TempDir(), false)
	first := map[string]any{"todos": []any{map[string]any{"content": "A", "activeForm": "Doing A", "status": "in_progress"}}}
	second := map[string]any{"todos": []any{map[string]any{"content": "A", "activeForm": "Doing A", "status": "completed"}}}
	turn.RecordToolCall(&agentcore.ToolCall{Name: agentcore.ToolNameTodoWrite, Input: first}, "")
	turn.RecordToolCall(&agentcore.ToolCall{Name: agentcore.ToolNameReadFile}, "")
	turn.RecordToolCall(&agentcore.ToolCall{Name: agentcore.ToolNameTodoWrite, Input: second}, "")
	activity := turn.Memory().ToolActivity
	if len(activity) != 3 || activity[0].Input != nil || activity[2].Input == nil {
		t.Fatalf("expected only the latest todo_write input to be retained: %#v", activity)
	}
}

func TestTurnMemoryFailedTodoWriteKeepsLastValidInput(t *testing.T) {
	turn := &Turn{}
	turn.ConfigureMemory(t.TempDir(), false)
	valid := map[string]any{"todos": []any{map[string]any{"content": "A", "activeForm": "Doing A", "status": "in_progress"}}}
	rejected := map[string]any{"todos": []any{}}
	turn.RecordToolCall(&agentcore.ToolCall{Name: agentcore.ToolNameTodoWrite, Input: valid}, "")
	turn.RecordToolCall(&agentcore.ToolCall{Name: agentcore.ToolNameTodoWrite, Input: rejected, Error: "invalid input"}, "")
	activity := turn.Memory().ToolActivity
	if len(activity) != 2 || activity[0].Input == nil || activity[1].Input != nil {
		t.Fatalf("failed todo_write must not displace the last valid list: %#v", activity)
	}
}
