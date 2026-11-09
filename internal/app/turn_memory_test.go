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
