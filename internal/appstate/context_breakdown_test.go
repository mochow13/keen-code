package appstate

import (
	"testing"

	"github.com/mochow13/keen-code/internal/llm/core"
)

func TestGetContextBreakdown_NoUsage(t *testing.T) {
	state := New(nil, t.TempDir())
	state.AppendMessage(core.Message{Role: core.RoleUser, Content: "hello world"})
	state.AppendMessage(core.Message{Role: core.RoleAssistant, Content: "hi there"})

	b := state.GetContextBreakdown()

	if b.SystemPromptTokens <= 0 {
		t.Error("expected system prompt tokens > 0")
	}
	if b.UserMessageTokens <= 0 {
		t.Error("expected user message tokens > 0")
	}
	if b.AssistantTokens <= 0 {
		t.Error("expected assistant tokens > 0")
	}
	if b.TotalEstimated != b.SystemPromptTokens+b.ToolDefTokens+b.UserMessageTokens+b.AssistantTokens+b.ToolResultTokens {
		t.Error("total should equal sum of categories when no usage reported")
	}
}

func TestGetContextBreakdown_IncludesToolDefinitions(t *testing.T) {
	state := New(nil, t.TempDir())
	if err := state.RegisterTool(dummyTool{name: "read_file"}); err != nil {
		t.Fatal(err)
	}
	if err := state.RegisterTool(dummyTool{name: "bash"}); err != nil {
		t.Fatal(err)
	}

	b := state.GetContextBreakdown()
	if b.ToolDefinitionCount != 2 {
		t.Errorf("expected 2 tool definitions, got %d", b.ToolDefinitionCount)
	}
	if b.ToolDefTokens <= 0 {
		t.Error("expected tool definition tokens > 0")
	}
}

func TestGetContextBreakdown_ToolActivityCountsAsToolResults(t *testing.T) {
	state := New(nil, t.TempDir())
	exitCode := 0
	state.AppendMessage(core.Message{
		Role:    core.RoleAssistant,
		Content: "let me check",
		TurnMemory: &core.TurnMemory{ToolActivity: []core.HistoricalToolActivity{
			{Tool: "bash", Input: map[string]any{"command": "ls -la"}, RawOutput: "total 0\ndrwxr-xr-x", ExitCode: &exitCode},
		}},
	})

	b := state.GetContextBreakdown()
	if b.ToolResultTokens <= 0 {
		t.Error("expected tool result tokens > 0")
	}
}

func TestGetContextBreakdown_ScalesToReportedUsage(t *testing.T) {
	state := New(nil, t.TempDir())
	state.AppendMessage(core.Message{Role: core.RoleUser, Content: "hello world, this is a longer message to estimate"})

	raw := state.GetContextBreakdown()
	reported := raw.TotalEstimated * 2

	state.SetLastUsage(&core.TokenUsage{InputTokens: reported, OutputTokens: 10})

	scaled := state.GetContextBreakdown()
	if scaled.TotalEstimated != reported {
		t.Errorf("expected total %d, got %d", reported, scaled.TotalEstimated)
	}
	sum := scaled.SystemPromptTokens + scaled.ToolDefTokens + scaled.UserMessageTokens + scaled.AssistantTokens + scaled.ToolResultTokens
	if diff := sum - reported; diff < -4 || diff > 4 {
		t.Errorf("scaled category sum %d diverges from reported total %d by %d", sum, reported, diff)
	}
	if scaled.SystemPromptTokens <= raw.SystemPromptTokens {
		t.Error("expected scaled system prompt tokens to increase when reported total doubles")
	}
}

func TestGetContextBreakdown_ZeroUsageDoesNotScale(t *testing.T) {
	state := New(nil, t.TempDir())
	state.AppendMessage(core.Message{Role: core.RoleUser, Content: "hi"})

	raw := state.GetContextBreakdown()
	state.SetLastUsage(&core.TokenUsage{InputTokens: 0})

	b := state.GetContextBreakdown()
	if b.TotalEstimated != raw.TotalEstimated {
		t.Errorf("expected raw total %d with zero usage, got %d", raw.TotalEstimated, b.TotalEstimated)
	}
}

func TestEstimateContextBreakdown_Empty(t *testing.T) {
	b := core.EstimateContextBreakdown("", nil, nil)
	if b.TotalEstimated != 0 {
		t.Errorf("expected 0 total, got %d", b.TotalEstimated)
	}
}

func TestEstimateContextBreakdown_Categories(t *testing.T) {
	systemPrompt := "system prompt text"
	toolDefs := []core.ContextToolDef{
		{Name: "read_file", Description: "reads a file", InputSchema: map[string]any{"type": "object"}},
		{Name: "bash", Description: "runs commands", InputSchema: map[string]any{"type": "object"}},
	}
	messages := []core.ContextMessage{
		{Role: core.RoleUser, Content: "hello there"},
		{Role: core.RoleAssistant, Content: "hi!", ToolActivity: []core.HistoricalToolActivity{
			{Tool: "read_file", Input: map[string]any{"path": "/tmp/x"}, RawOutput: "file contents here"},
		}},
		{Role: core.RoleUser, Content: "thanks"},
	}

	b := core.EstimateContextBreakdown(systemPrompt, toolDefs, messages)

	if b.SystemPromptTokens <= 0 {
		t.Error("expected system prompt tokens > 0")
	}
	if b.ToolDefinitionCount != 2 {
		t.Errorf("expected 2 tool definitions, got %d", b.ToolDefinitionCount)
	}
	if b.ToolDefTokens <= 0 {
		t.Error("expected tool definition tokens > 0")
	}
	if b.UserMessageTokens <= 0 {
		t.Error("expected user message tokens > 0")
	}
	if b.AssistantTokens <= 0 {
		t.Error("expected assistant tokens > 0")
	}
	if b.ToolResultTokens <= 0 {
		t.Error("expected tool result tokens > 0")
	}

	sum := b.SystemPromptTokens + b.ToolDefTokens + b.UserMessageTokens + b.AssistantTokens + b.ToolResultTokens
	if sum != b.TotalEstimated {
		t.Errorf("category sum %d != total %d", sum, b.TotalEstimated)
	}
}

func TestEstimateContextBreakdown_ToolResultWithoutRawOutput(t *testing.T) {
	messages := []core.ContextMessage{
		{Role: core.RoleAssistant, ToolActivity: []core.HistoricalToolActivity{
			{Tool: "bash", Input: map[string]any{"command": "ls"}},
		}},
	}
	b := core.EstimateContextBreakdown("", nil, messages)
	if b.ToolResultTokens <= 0 {
		t.Error("expected tool input to count toward tool result tokens")
	}
}

func TestEstimateContextBreakdown_NonStringRawOutput(t *testing.T) {
	messages := []core.ContextMessage{
		{Role: core.RoleAssistant, ToolActivity: []core.HistoricalToolActivity{
			{Tool: "read_file", RawOutput: map[string]any{"lines": 42}},
		}},
	}
	b := core.EstimateContextBreakdown("", nil, messages)
	if b.ToolResultTokens <= 0 {
		t.Error("expected non-string raw output to be estimated via JSON")
	}
}

func TestEstimateContextBreakdown_PrefersRetainedOutput(t *testing.T) {
	retainedOutput := map[string]any{"content": "compact"}
	messages := []core.ContextMessage{
		{Role: core.RoleAssistant, ToolActivity: []core.HistoricalToolActivity{
			{
				Tool:           "read_file",
				RawOutput:      map[string]any{"content": "a substantially longer raw output"},
				RetainedOutput: retainedOutput,
			},
		}},
	}

	b := core.EstimateContextBreakdown("", nil, messages)
	want := core.EstimateJSONTokenCount(retainedOutput)
	if b.ToolResultTokens != want {
		t.Errorf("ToolResultTokens = %d, want %d", b.ToolResultTokens, want)
	}
}
