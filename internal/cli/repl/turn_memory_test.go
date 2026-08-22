package repl

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	replappstate "github.com/mochow13/keen-code/internal/cli/repl/appstate"
	reploutput "github.com/mochow13/keen-code/internal/cli/repl/output"
	"github.com/mochow13/keen-code/internal/llm"
)

func TestHandleLLMDone_AttachesTurnMemoryToAssistantMessage(t *testing.T) {
	workingDir := t.TempDir()
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")
	sh.HandleChunk("working")
	sh.HandleToolStart(&llm.ToolCall{Name: "edit_file", Input: map[string]any{"path": "nested/a.go"}})
	sh.HandleToolEnd(&llm.ToolCall{Name: "edit_file", Input: map[string]any{"path": "nested/a.go"}})
	sh.HandleChunk("done")

	m := replModel{
		stream:   streamState{handler: sh},
		loading:  loadingState{showSpinner: true},
		width:    80,
		appState: replappstate.New(nil, workingDir),
		output:   reploutput.NewOutputBuilder(80, ""),
	}
	m.startAssistantTurnMemory()
	sh.HandleToolEnd(&llm.ToolCall{
		Name:  "edit_file",
		Input: map[string]any{"path": filepath.Join(workingDir, "nested", "a.go"), "oldString": "old", "newString": "new"},
	})
	sh.HandleBashStart("go test ./...", "")
	sh.HandleBashEnd(&llm.ToolCall{
		Name:   "bash",
		Output: map[string]any{"exit_code": 1},
	})

	updated, _ := m.handleLLMDone()

	messages := updated.appState.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected one stored message, got %#v", messages)
	}
	if messages[0].TurnMemory == nil {
		t.Fatal("expected assistant turn memory")
	}
	activities := messages[0].TurnMemory.ToolActivity
	if len(activities) != 3 {
		t.Fatalf("unexpected tool activity %#v", activities)
	}
	if activities[1].Input["path"] != filepath.Join("nested", "a.go") || activities[1].Input["oldString"] != "old" || activities[1].Input["newString"] != "new" {
		t.Fatalf("unexpected edit activity %#v", activities[1])
	}
	if activities[2].Input["command"] != "go test ./..." || activities[2].Status != "success" || activities[2].ExitCode == nil || *activities[2].ExitCode != 1 {
		t.Fatalf("unexpected bash activity %#v", activities[2])
	}
}

func TestCollectHistoricalToolActivity_RetainsRawOutputsWhenEnabled(t *testing.T) {
	activities := collectHistoricalToolActivity([]streamSegment{
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "read_file", Output: map[string]any{"content": "package main"}}},
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "bash", Error: "command failed", Output: map[string]any{"exit_code": 1}}},
	}, "", true)

	if !activities[0].HasRawOutput || activities[0].RawOutput.(map[string]any)["content"] != "package main" {
		t.Fatalf("expected retained successful output, got %#v", activities[0])
	}
	if !activities[1].HasRawOutput || activities[1].RawOutput.(map[string]any)["error"] != "command failed" {
		t.Fatalf("expected retained error output, got %#v", activities[1])
	}
}

func TestCollectHistoricalToolActivity_OmitsRawOutputsByDefault(t *testing.T) {
	activities := collectHistoricalToolActivity([]streamSegment{{
		kind:     segmentToolEnd,
		toolCall: &llm.ToolCall{Name: "read_file", Output: map[string]any{"content": "package main"}},
	}}, "", false)

	if activities[0].HasRawOutput || activities[0].RawOutput != nil {
		t.Fatalf("expected output to be omitted, got %#v", activities[0])
	}
}

func TestCollectHistoricalToolActivity_RetainsAskUserResult(t *testing.T) {
	input := map[string]any{"questions": []any{map[string]any{"question": "Pick", "options": []any{"one", "two"}}}}
	output := map[string]any{"answers": []string{"two"}, "cancelled": false}
	activities := collectHistoricalToolActivity([]streamSegment{{
		kind:     segmentToolEnd,
		toolCall: &llm.ToolCall{Name: "ask_user", Input: input, Output: output},
	}}, "", false)
	if len(activities) != 1 || activities[0].Input == nil || activities[0].RetainedOutput == nil {
		t.Fatalf("expected retained ask_user input and output, got %#v", activities)
	}
	result, ok := activities[0].RetainedOutput.(map[string]any)
	if !ok || result["cancelled"] != false || result["tool"] != nil {
		t.Fatalf("unexpected retained ask_user result %#v", activities[0].RetainedOutput)
	}
}

func TestCollectHistoricalToolActivity_RetainsWriteInputWithoutChangedPath(t *testing.T) {
	workingDir := t.TempDir()
	targetPath := filepath.Join(workingDir, "dir", "file.go")
	activities := collectHistoricalToolActivity([]streamSegment{{
		kind: segmentToolEnd,
		toolCall: &llm.ToolCall{
			Name:   "write_file",
			Input:  map[string]any{"path": targetPath, "content": "content"},
			Output: map[string]any{"file_changed": targetPath},
		},
	}}, workingDir, false)

	if len(activities) != 1 || activities[0].Input["path"] != filepath.Join("dir", "file.go") || activities[0].Input["content"] != "content" || activities[0].Status != "success" {
		t.Fatalf("expected retained write input and status-only outcome, got %#v", activities)
	}
}

func TestCollectHistoricalToolActivity_RelativizesRetainedPathInputs(t *testing.T) {
	workingDir := t.TempDir()
	outsidePath := filepath.Join(filepath.Dir(workingDir), "outside.go")
	tests := []struct {
		name     string
		tool     string
		path     string
		expected string
	}{
		{name: "read file", tool: "read_file", path: filepath.Join(workingDir, "dir", "file.go"), expected: filepath.Join("dir", "file.go")},
		{name: "grep", tool: "grep", path: filepath.Join(workingDir, "internal"), expected: "internal"},
		{name: "glob", tool: "glob", path: workingDir, expected: "."},
		{name: "write file", tool: "write_file", path: filepath.Join(workingDir, "dir", "file.go"), expected: filepath.Join("dir", "file.go")},
		{name: "edit file", tool: "edit_file", path: filepath.Join(workingDir, "dir", "file.go"), expected: filepath.Join("dir", "file.go")},
		{name: "outside working directory", tool: "read_file", path: outsidePath, expected: outsidePath},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := map[string]any{"path": test.path}
			activities := collectHistoricalToolActivity([]streamSegment{{
				kind:     segmentToolEnd,
				toolCall: &llm.ToolCall{Name: test.tool, Input: input},
			}}, workingDir, false)

			if len(activities) != 1 || activities[0].Input["path"] != test.expected {
				t.Fatalf("expected path %q, got %#v", test.expected, activities)
			}
			if input["path"] != test.path {
				t.Fatalf("expected original input to remain unchanged, got %#v", input)
			}
		})
	}
}

func TestCollectHistoricalToolActivity_RecordsOffsetsInputsAndStatus(t *testing.T) {
	workingDir := t.TempDir()
	readPath := filepath.Join(workingDir, "a.go")
	segments := []streamSegment{
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "glob", Input: map[string]any{"path": workingDir, "pattern": "**/*.go"}}},
		{kind: segmentAssistant, content: "Inspecting. "},
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": readPath}}},
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "edit_file", Error: "failed", Input: map[string]any{"path": readPath}}},
		{kind: segmentAssistant, content: "Done."},
		{kind: segmentBash, command: "go test ./...", toolCall: &llm.ToolCall{Name: "bash"}},
	}

	got := collectHistoricalToolActivity(segments, workingDir, false)
	if len(got) != 4 {
		t.Fatalf("expected four activities, got %#v", got)
	}
	if got[0].TextOffset != 0 || got[0].Input["path"] != "." || got[0].Input["pattern"] != "**/*.go" {
		t.Fatalf("unexpected glob activity %#v", got[0])
	}
	if got[1].TextOffset != len("Inspecting. ") || got[1].Input["path"] != "a.go" || got[1].Status != "success" {
		t.Fatalf("unexpected read activity %#v", got[1])
	}
	if got[2].TextOffset != got[1].TextOffset || got[2].Status != "error" || got[2].Input["path"] != "a.go" {
		t.Fatalf("unexpected edit activity %#v", got[2])
	}
	if got[3].TextOffset != len("Inspecting. Done.") || got[3].Input["command"] != "go test ./..." {
		t.Fatalf("unexpected bash activity %#v", got[3])
	}
}

func TestCollectHistoricalToolActivity_RetainsMCPInput(t *testing.T) {
	segments := []streamSegment{{
		kind: segmentToolEnd,
		toolCall: &llm.ToolCall{
			Name: "call_mcp_tool",
			Input: map[string]any{
				"server":    "context7",
				"tool":      "query-docs",
				"arguments": map[string]any{"query": "turn memory"},
			},
			Output: map[string]any{"content": "do not retain"},
		},
	}}

	got := collectHistoricalToolActivity(segments, "", false)
	if len(got) != 1 {
		t.Fatalf("expected one activity, got %#v", got)
	}
	arguments, ok := got[0].Input["arguments"].(map[string]any)
	if got[0].Tool != "call_mcp_tool" || got[0].Input["server"] != "context7" || got[0].Input["tool"] != "query-docs" || !ok || arguments["query"] != "turn memory" {
		t.Fatalf("unexpected MCP activity %#v", got[0])
	}
}

func TestCollectHistoricalToolActivity_DoesNotInferRetainedOutcomesFromArguments(t *testing.T) {
	activities := collectHistoricalToolActivity([]streamSegment{
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "write_file", Input: map[string]any{"path": "a.go", "content": "content"}, Output: map[string]any{"path": "a.go"}}},
		{kind: segmentBash, command: "go test ./...", toolCall: &llm.ToolCall{Name: "bash", Input: map[string]any{"command": "go test ./..."}, Output: map[string]any{"exit_code": 1}}},
	}, "", false)

	if activities[0].Input["path"] != "a.go" || activities[0].Input["content"] != "content" || activities[0].Status != "success" {
		t.Fatalf("expected write input and status only, got %#v", activities[0])
	}
	if activities[1].Input["command"] != "go test ./..." || activities[1].Status != "success" || activities[1].ExitCode == nil || *activities[1].ExitCode != 1 {
		t.Fatalf("expected successful tool status, command input, and exit code output, got %#v", activities[1])
	}
}

func TestCollectHistoricalToolActivity_BashToolErrorSetsErrorStatus(t *testing.T) {
	activities := collectHistoricalToolActivity([]streamSegment{{
		kind:    segmentBash,
		command: "go test ./...",
		toolCall: &llm.ToolCall{
			Name:   "bash",
			Error:  "tool execution failed",
			Output: map[string]any{"exit_code": 1},
		},
	}}, "", false)

	if len(activities) != 1 || activities[0].Status != "error" || activities[0].ExitCode == nil || *activities[0].ExitCode != 1 {
		t.Fatalf("expected tool error status and command exit code, got %#v", activities)
	}
}

func TestCollectHistoricalToolActivity_StripsOversizedMCPArguments(t *testing.T) {
	oversized := string(make([]byte, maxHistoricalToolInputFieldBytes+1))
	segments := []streamSegment{{
		kind: segmentToolEnd,
		toolCall: &llm.ToolCall{
			Name: "call_mcp_tool",
			Input: map[string]any{
				"server": "context7",
				"tool":   "query-docs",
				"arguments": map[string]any{
					"query": oversized,
					"limit": 10,
				},
			},
		},
	}}

	got := collectHistoricalToolActivity(segments, "", false)
	if _, ok := got[0].Input["arguments"]; ok {
		t.Fatalf("expected oversized MCP arguments to be stripped, got %#v", got[0])
	}
	if got[0].Input["server"] != "context7" || got[0].Input["tool"] != "query-docs" {
		t.Fatalf("expected bounded wrapper arguments to remain, got %#v", got[0])
	}
}

func TestCollectHistoricalToolActivity_RetainsWriteAndEditInputs(t *testing.T) {
	segments := []streamSegment{
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "a.go"}}},
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "write_file", Input: map[string]any{"path": "a.go", "content": "content"}}},
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "edit_file", Input: map[string]any{"path": "a.go", "oldString": "old", "newString": "new", "shouldReplaceAll": true}}},
	}

	got := collectHistoricalToolActivity(segments, "", false)
	if got[0].Input["path"] != "a.go" || got[1].Input["path"] != "a.go" || got[1].Input["content"] != "content" {
		t.Fatalf("unexpected retained read/write inputs %#v", got)
	}
	if got[2].Input["path"] != "a.go" || got[2].Input["oldString"] != "old" || got[2].Input["newString"] != "new" || got[2].Input["shouldReplaceAll"] != true {
		t.Fatalf("unexpected retained edit input %#v", got[2])
	}
}

func TestCollectHistoricalToolActivity_TruncatesOversizedWriteAndEditStrings(t *testing.T) {
	oversizedASCII := strings.Repeat("a", maxHistoricalToolInputFieldBytes+1)
	oversizedUTF8 := strings.Repeat("é", maxHistoricalToolInputFieldBytes/2+1)
	segments := []streamSegment{
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "write_file", Input: map[string]any{"path": "a.go", "content": oversizedASCII}}},
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "edit_file", Input: map[string]any{"path": "a.go", "oldString": oversizedASCII, "newString": oversizedUTF8}}},
	}

	got := collectHistoricalToolActivity(segments, "", false)
	writeContent := got[0].Input["content"].(string)
	oldString := got[1].Input["oldString"].(string)
	newString := got[1].Input["newString"].(string)
	if len(writeContent) != maxHistoricalToolInputFieldBytes || len(oldString) != maxHistoricalToolInputFieldBytes {
		t.Fatalf("expected ASCII fields truncated to %d bytes, got %d and %d", maxHistoricalToolInputFieldBytes, len(writeContent), len(oldString))
	}
	if len(newString) > maxHistoricalToolInputFieldBytes || !utf8.ValidString(newString) || newString == "" {
		t.Fatalf("expected valid bounded UTF-8 without marker, got %q", newString)
	}
	if strings.Contains(writeContent, "truncated") || strings.Contains(newString, "truncated") {
		t.Fatalf("did not expect truncation marker, got %#v", got)
	}
}
