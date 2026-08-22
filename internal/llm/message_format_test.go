package llm

import "testing"

func TestFormatMessageForProvider_DoesNotAppendTurnMemory(t *testing.T) {
	message := Message{
		Role:    RoleAssistant,
		Content: "Updated the parser.",
		TurnMemory: &TurnMemory{ToolActivity: []HistoricalToolActivity{{
			Tool:   "write_file",
			Input:  map[string]any{"path": "a.go", "content": "content"},
			Status: "success",
		}}},
	}

	if got := FormatMessageForProvider(message); got != message.Content {
		t.Fatalf("expected assistant content only, got %q", got)
	}
}

func TestFormatMessageForProvider_LeavesUserMessageUntouched(t *testing.T) {
	message := Message{
		Role:    RoleUser,
		Content: "hello",
		TurnMemory: &TurnMemory{
			ToolActivity: []HistoricalToolActivity{{
				TextOffset: 2,
				Tool:       "read_file",
				Status:     "success",
			}},
		},
	}

	if got := FormatMessageForProvider(message); got != "hello" {
		t.Fatalf("expected user message to remain unchanged, got %q", got)
	}
}

func TestHistoricalMessageSteps_PreservesActivityOrder(t *testing.T) {
	message := Message{
		Role:    RoleAssistant,
		Content: "Let me inspect. Found it.",
		TurnMemory: &TurnMemory{
			ToolActivity: []HistoricalToolActivity{
				{TextOffset: 15, Tool: "read_file", Input: map[string]any{"path": "a.go"}, Status: "success"},
				{TextOffset: 15, Tool: "grep", Input: map[string]any{"path": "internal", "pattern": "TODO"}, Status: "error"},
			},
		},
	}

	steps := historicalMessageSteps(2, message)
	if len(steps) != 2 {
		t.Fatalf("expected two steps, got %#v", steps)
	}
	if steps[0].Text != "Let me inspect." || steps[1].Text != " Found it." {
		t.Fatalf("unexpected step text: %#v", steps)
	}
	if len(steps[0].Activities) != 2 {
		t.Fatalf("expected two grouped activities, got %#v", steps[0].Activities)
	}
	if steps[0].Activities[0].ID != "historical_2_0" || steps[0].Activities[0].Activity.Tool != "read_file" || steps[0].Activities[0].Activity.Input["path"] != "a.go" {
		t.Fatalf("unexpected first activity: %#v", steps[0].Activities[0])
	}
	if steps[0].Activities[1].ID != "historical_2_1" || steps[0].Activities[1].Activity.Tool != "grep" {
		t.Fatalf("unexpected second activity: %#v", steps[0].Activities[1])
	}
}

func TestHistoricalToolArguments_RetainsInput(t *testing.T) {
	activity := HistoricalToolActivity{Input: map[string]any{"path": "go.mod", "offset": 2}}
	if got := historicalToolArguments(activity); got != `{"offset":2,"path":"go.mod"}` {
		t.Fatalf("unexpected arguments %q", got)
	}
	if got := historicalToolArguments(HistoricalToolActivity{}); got != `{}` {
		t.Fatalf("expected empty arguments, got %q", got)
	}
}

func TestHistoricalToolResult_RetainsOnlyCompactOutcome(t *testing.T) {
	exitCode := 1
	tests := []struct {
		name     string
		activity HistoricalToolActivity
		want     string
	}{
		{name: "success", activity: HistoricalToolActivity{Status: "success"}, want: `{"status":"success"}`},
		{name: "error", activity: HistoricalToolActivity{Status: "error"}, want: `{"status":"error"}`},
		{name: "exit code", activity: HistoricalToolActivity{Status: "success", ExitCode: &exitCode}, want: `{"status":"success","exit_code":1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := historicalToolResult(tt.activity); got != tt.want {
				t.Fatalf("unexpected result: want %q, got %q", tt.want, got)
			}
		})
	}
}

func TestHistoricalToolResultRetainsAskUserOutput(t *testing.T) {
	activity := HistoricalToolActivity{
		Tool:           "ask_user",
		Status:         "success",
		RetainedOutput: map[string]any{"answers": []string{"PostgreSQL"}, "cancelled": false},
	}
	if got := historicalToolResult(activity); got != `{"answers":["PostgreSQL"],"cancelled":false}` {
		t.Fatalf("unexpected retained ask_user result %q", got)
	}
}

func TestCloneTurnMemory_ClonesHistoricalActivity(t *testing.T) {
	original := &TurnMemory{ToolActivity: []HistoricalToolActivity{{
		Tool:   "call_mcp_tool",
		Input:  map[string]any{"arguments": map[string]any{"query": "original"}, "values": []any{"first"}},
		Status: "success",
	}}}
	cloned := CloneTurnMemory(original)
	if cloned == nil || cloned.IsEmpty() {
		t.Fatalf("expected non-empty clone, got %#v", cloned)
	}
	cloned.ToolActivity[0].Tool = "grep"
	cloned.ToolActivity[0].Input["arguments"].(map[string]any)["query"] = "changed"
	cloned.ToolActivity[0].Input["values"].([]any)[0] = "changed"
	if original.ToolActivity[0].Tool != "call_mcp_tool" || original.ToolActivity[0].Input["arguments"].(map[string]any)["query"] != "original" || original.ToolActivity[0].Input["values"].([]any)[0] != "first" {
		t.Fatalf("expected independent clone, got %#v", original.ToolActivity)
	}
}

func TestFormatMessageForProvider_SkipsOffsetInsideUTF8Rune(t *testing.T) {
	message := Message{
		Role:    RoleAssistant,
		Content: "é",
		TurnMemory: &TurnMemory{ToolActivity: []HistoricalToolActivity{{
			TextOffset: 1,
			Tool:       "read_file",
			Status:     "success",
		}}},
	}

	if got := FormatMessageForProvider(message); got != "é" {
		t.Fatalf("expected invalid UTF-8 boundary to be skipped, got %q", got)
	}
}

func TestHistoricalMessageSteps_HandlesBoundaryAndInvalidOffsets(t *testing.T) {
	message := Message{
		Role:    RoleAssistant,
		Content: "done",
		TurnMemory: &TurnMemory{
			ToolActivity: []HistoricalToolActivity{
				{TextOffset: -1, Tool: "invalid", Status: "error"},
				{TextOffset: 0, Tool: "read_file", Status: "success"},
				{TextOffset: 4, Tool: "bash", Input: map[string]any{"command": "go test ./..."}, Status: "success"},
				{TextOffset: 5, Tool: "invalid", Status: "error"},
			},
		},
	}

	steps := historicalMessageSteps(0, message)
	if len(steps) != 3 {
		t.Fatalf("expected three steps, got %#v", steps)
	}
	if steps[0].Text != "" || steps[0].Activities[0].Activity.Tool != "read_file" {
		t.Fatalf("unexpected leading step: %#v", steps[0])
	}
	if steps[1].Text != "done" || steps[1].Activities[0].Activity.Tool != "bash" {
		t.Fatalf("unexpected trailing activity step: %#v", steps[1])
	}
	if steps[2].Text != "" || len(steps[2].Activities) != 0 {
		t.Fatalf("unexpected final step: %#v", steps[2])
	}
}
