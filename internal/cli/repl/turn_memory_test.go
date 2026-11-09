package repl

import (
	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/app"
	replstream "github.com/mochow13/keen-code/internal/cli/repl/stream"
	"path/filepath"
	"testing"

	reploutput "github.com/mochow13/keen-code/internal/cli/repl/output"
)

func TestHandleLLMDone_AttachesTurnMemoryToAssistantMessage(t *testing.T) {
	workingDir := t.TempDir()
	sh := replstream.NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	m := replModel{
		stream:    streamState{handler: sh},
		loading:   loadingState{showSpinner: true},
		width:     80,
		agentCore: newAgentCore(nil, workingDir),
		output:    reploutput.NewOutputBuilder(80, ""),
	}
	m.turn = &app.Turn{}
	m.turn.ConfigureMemory(workingDir, false)
	sh.HandleChunk("working")
	m.handleToolEnd(&agentcore.ToolCall{
		Name:  "edit_file",
		Input: map[string]any{"path": filepath.Join(workingDir, "nested", "a.go"), "oldString": "old", "newString": "new"},
	})
	sh.HandleChunk("done")
	m.handleToolStart(&agentcore.ToolCall{Name: "bash", Input: map[string]any{"command": "go test ./..."}})
	m.handleToolEnd(&agentcore.ToolCall{
		Name:   "bash",
		Input:  map[string]any{"command": "go test ./..."},
		Output: map[string]any{"exit_code": 1},
	})

	updated, _ := m.handleLLMDone()

	messages := updated.agentCore.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected one stored message, got %#v", messages)
	}
	if messages[0].TurnMemory == nil {
		t.Fatal("expected assistant turn memory")
	}
	activities := messages[0].TurnMemory.ToolActivity
	if len(activities) != 2 {
		t.Fatalf("unexpected tool activity %#v", activities)
	}
	if activities[0].Input["path"] != filepath.Join("nested", "a.go") || activities[0].Input["oldString"] != "old" || activities[0].Input["newString"] != "new" {
		t.Fatalf("unexpected edit activity %#v", activities[0])
	}
	if activities[1].Input["command"] != "go test ./..." || activities[1].Status != "success" || activities[1].ExitCode == nil || *activities[1].ExitCode != 1 {
		t.Fatalf("unexpected bash activity %#v", activities[1])
	}
}

func TestTurnMemoryOffsetAfterRetry(t *testing.T) {
	handler := replstream.NewStreamHandler(nil)
	handler.Start(make(chan agentcore.StreamEvent), "")
	turn := &app.Turn{}
	handler.HandleChunk("discarded text")
	handler.RewindForRetry()
	handler.HandleChunk("hé")
	turn.RecordToolCall(&agentcore.ToolCall{Name: "read_file"}, handler.GetResponse())
	handler.HandleToolEnd(&agentcore.ToolCall{Name: "read_file"})
	handler.HandleChunk(" end")
	memory := turn.Memory()
	if got, want := memory.ToolActivity[0].TextOffset, len("hé"); got != want {
		t.Fatalf("offset = %d, want %d", got, want)
	}
	if got := handler.GetResponse(); got != "hé end" {
		t.Fatalf("response = %q", got)
	}
}
