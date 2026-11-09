package app

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mochow13/keen-code/internal/agentcore"
)

func recordActivities(workingDir string, retain bool, calls ...*agentcore.ToolCall) []agentcore.HistoricalToolActivity {
	turn := &Turn{}
	turn.ConfigureMemory(workingDir, retain)
	for _, call := range calls {
		turn.RecordToolCall(call, "")
	}
	return turn.Memory().ToolActivity
}

func TestTurnMemoryOutputPolicies(t *testing.T) {
	output := map[string]any{"content": "1:abc|package main", "bytes_read": 18, "lines_read": 1, "total_lines": 1, "truncated": false}
	activities := recordActivities("", true,
		&agentcore.ToolCall{Name: "read_file", Output: output},
		&agentcore.ToolCall{Name: "bash", Error: "command failed", Output: map[string]any{"exit_code": 1}},
	)
	read := activities[0]
	if !read.HasRawOutput || read.RawOutput.(map[string]any)["bytes_read"] != 18 {
		t.Fatalf("unexpected raw output: %#v", read)
	}
	retained := read.RetainedOutput.(map[string]any)
	if _, ok := retained["bytes_read"]; ok {
		t.Fatalf("retained bytes_read: %#v", retained)
	}
	if _, ok := retained["lines_read"]; ok {
		t.Fatalf("retained lines_read: %#v", retained)
	}
	if retained["total_lines"] != 1 || retained["truncated"] != false {
		t.Fatalf("lost pagination: %#v", retained)
	}
	if !activities[1].HasRawOutput || activities[1].RawOutput.(map[string]any)["error"] != "command failed" {
		t.Fatalf("unexpected error output: %#v", activities[1])
	}
	omitted := recordActivities("", false, &agentcore.ToolCall{Name: "read_file", Output: output})[0]
	if omitted.HasRawOutput || omitted.RawOutput != nil {
		t.Fatalf("expected omitted output: %#v", omitted)
	}
	input := map[string]any{"questions": []any{map[string]any{"question": "Pick", "options": []any{"one", "two"}}}}
	answer := map[string]any{"answers": []string{"two"}, "cancelled": false}
	ask := recordActivities("", false, &agentcore.ToolCall{Name: "ask_user", Input: input, Output: answer})[0]
	result, ok := ask.RetainedOutput.(map[string]any)
	if ask.Input == nil || !ok || result["cancelled"] != false || result["tool"] != nil {
		t.Fatalf("unexpected ask_user activity: %#v", ask)
	}
}

func TestTurnMemoryPaths(t *testing.T) {
	workingDir := t.TempDir()
	outside := filepath.Join(filepath.Dir(workingDir), "outside.go")
	for _, tc := range []struct{ tool, path, want string }{
		{"read_file", filepath.Join(workingDir, "dir", "file.go"), filepath.Join("dir", "file.go")},
		{"grep", filepath.Join(workingDir, "internal"), "internal"},
		{"glob", workingDir, "."},
		{"write_file", filepath.Join(workingDir, "dir", "file.go"), filepath.Join("dir", "file.go")},
		{"edit_file", filepath.Join(workingDir, "dir", "file.go"), filepath.Join("dir", "file.go")},
		{"read_file", outside, outside},
	} {
		t.Run(tc.tool+tc.want, func(t *testing.T) {
			input := map[string]any{"path": tc.path, "content": "content"}
			activity := recordActivities(workingDir, false, &agentcore.ToolCall{Name: tc.tool, Input: input, Output: map[string]any{"file_changed": tc.path}})[0]
			if activity.Input["path"] != tc.want || input["path"] != tc.path || activity.Status != "success" {
				t.Fatalf("unexpected activity or input: %#v %#v", activity, input)
			}
			if tc.tool == "write_file" && activity.Input["content"] != "content" {
				t.Fatalf("lost content: %#v", activity)
			}
		})
	}
}

func TestTurnMemoryOffsetsInputsAndStatus(t *testing.T) {
	workingDir := t.TempDir()
	path := filepath.Join(workingDir, "a.go")
	turn := &Turn{}
	turn.ConfigureMemory(workingDir, false)
	turn.RecordToolCall(&agentcore.ToolCall{Name: "glob", Input: map[string]any{"path": workingDir, "pattern": "**/*.go"}}, "")
	turn.RecordToolCall(&agentcore.ToolCall{Name: "read_file", Input: map[string]any{"path": path}}, "Inspecting. ")
	turn.RecordToolCall(&agentcore.ToolCall{Name: "edit_file", Error: "failed", Input: map[string]any{"path": path}}, "Inspecting. ")
	turn.RecordToolCall(&agentcore.ToolCall{Name: "bash", Input: map[string]any{"command": "go test ./..."}}, "Inspecting. Done.")
	got := turn.Memory().ToolActivity
	if len(got) != 4 {
		t.Fatalf("expected four activities: %#v", got)
	}
	if got[0].TextOffset != 0 || got[0].Input["path"] != "." || got[0].Input["pattern"] != "**/*.go" {
		t.Fatalf("glob: %#v", got[0])
	}
	if got[1].TextOffset != len("Inspecting. ") || got[1].Input["path"] != "a.go" || got[1].Status != "success" {
		t.Fatalf("read: %#v", got[1])
	}
	if got[2].TextOffset != got[1].TextOffset || got[2].Status != "error" || got[2].Input["path"] != "a.go" {
		t.Fatalf("edit: %#v", got[2])
	}
	if got[3].TextOffset != len("Inspecting. Done.") || got[3].Input["command"] != "go test ./..." {
		t.Fatalf("bash: %#v", got[3])
	}
}

func TestTurnMemoryRetainsAndBoundsInputs(t *testing.T) {
	oversized := strings.Repeat("a", maxHistoricalToolInputFieldBytes+1)
	utf8Text := strings.Repeat("é", maxHistoricalToolInputFieldBytes/2+1)
	activities := recordActivities("", false,
		&agentcore.ToolCall{Name: "read_file", Input: map[string]any{"path": "a.go"}},
		&agentcore.ToolCall{Name: "write_file", Input: map[string]any{"path": "a.go", "content": oversized}},
		&agentcore.ToolCall{Name: "edit_file", Input: map[string]any{"path": "a.go", "oldString": oversized, "newString": utf8Text, "shouldReplaceAll": true}},
		&agentcore.ToolCall{Name: "call_mcp_tool", Input: map[string]any{"server": "context7", "tool": "query-docs", "arguments": map[string]any{"query": "turn memory"}}},
		&agentcore.ToolCall{Name: "call_mcp_tool", Input: map[string]any{"server": "context7", "tool": "query-docs", "arguments": map[string]any{"query": oversized}}},
	)
	if activities[0].Input["path"] != "a.go" || activities[1].Input["path"] != "a.go" || activities[2].Input["shouldReplaceAll"] != true {
		t.Fatalf("lost input: %#v", activities)
	}
	write := activities[1].Input["content"].(string)
	old := activities[2].Input["oldString"].(string)
	newText := activities[2].Input["newString"].(string)
	if len(write) != maxHistoricalToolInputFieldBytes || len(old) != maxHistoricalToolInputFieldBytes || len(newText) > maxHistoricalToolInputFieldBytes || !utf8.ValidString(newText) || newText == "" || strings.Contains(write, "truncated") || strings.Contains(newText, "truncated") {
		t.Fatalf("incorrect truncation")
	}
	args, ok := activities[3].Input["arguments"].(map[string]any)
	if !ok || args["query"] != "turn memory" || activities[3].Input["server"] != "context7" || activities[3].Input["tool"] != "query-docs" {
		t.Fatalf("unexpected MCP input: %#v", activities[3])
	}
	if _, ok := activities[4].Input["arguments"]; ok {
		t.Fatalf("oversized MCP arguments retained: %#v", activities[4])
	}
	if activities[4].Input["server"] != "context7" || activities[4].Input["tool"] != "query-docs" {
		t.Fatalf("lost MCP wrapper: %#v", activities[4])
	}
}

func TestTurnMemoryBashOutcome(t *testing.T) {
	activities := recordActivities("", false,
		&agentcore.ToolCall{Name: "write_file", Input: map[string]any{"path": "a.go", "content": "content"}, Output: map[string]any{"path": "a.go"}},
		&agentcore.ToolCall{Name: "bash", Input: map[string]any{"command": "go test ./..."}, Output: map[string]any{"exit_code": 1}},
		&agentcore.ToolCall{Name: "bash", Error: "tool execution failed", Output: map[string]any{"exit_code": 1}},
	)
	if activities[0].Input["path"] != "a.go" || activities[0].Input["content"] != "content" || activities[0].Status != "success" {
		t.Fatalf("write: %#v", activities[0])
	}
	if activities[1].Input["command"] != "go test ./..." || activities[1].Status != "success" || activities[1].ExitCode == nil || *activities[1].ExitCode != 1 {
		t.Fatalf("bash: %#v", activities[1])
	}
	if activities[2].Status != "error" || activities[2].ExitCode == nil || *activities[2].ExitCode != 1 {
		t.Fatalf("bash error: %#v", activities[2])
	}
}
