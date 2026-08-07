package appstate

import (
	"testing"

	"github.com/user/keen-code/internal/llm"
)

func TestGetContextBreakdown_NoUsage(t *testing.T) {
	state := New(nil, t.TempDir())
	state.AppendMessage(llm.Message{Role: llm.RoleUser, Content: "hello world"})
	state.AppendMessage(llm.Message{Role: llm.RoleAssistant, Content: "hi there"})

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
	state.AppendMessage(llm.Message{
		Role:    llm.RoleAssistant,
		Content: "let me check",
		TurnMemory: &llm.TurnMemory{ToolActivity: []llm.HistoricalToolActivity{
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
	state.AppendMessage(llm.Message{Role: llm.RoleUser, Content: "hello world, this is a longer message to estimate"})

	raw := state.GetContextBreakdown()
	reported := raw.TotalEstimated * 2

	state.SetLastUsage(&llm.TokenUsage{InputTokens: reported, OutputTokens: 10})

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
	state.AppendMessage(llm.Message{Role: llm.RoleUser, Content: "hi"})

	raw := state.GetContextBreakdown()
	state.SetLastUsage(&llm.TokenUsage{InputTokens: 0})

	b := state.GetContextBreakdown()
	if b.TotalEstimated != raw.TotalEstimated {
		t.Errorf("expected raw total %d with zero usage, got %d", raw.TotalEstimated, b.TotalEstimated)
	}
}
