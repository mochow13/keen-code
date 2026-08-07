package llm

import "testing"

func TestEstimateContextBreakdown_Empty(t *testing.T) {
	b := EstimateContextBreakdown("", nil, nil)
	if b.TotalEstimated != 0 {
		t.Errorf("expected 0 total, got %d", b.TotalEstimated)
	}
}

func TestEstimateContextBreakdown_Categories(t *testing.T) {
	systemPrompt := "system prompt text"
	toolDefs := []ContextToolDef{
		{Name: "read_file", Description: "reads a file", InputSchema: map[string]any{"type": "object"}},
		{Name: "bash", Description: "runs commands", InputSchema: map[string]any{"type": "object"}},
	}
	messages := []ContextMessage{
		{Role: RoleUser, Content: "hello there"},
		{Role: RoleAssistant, Content: "hi!", ToolActivity: []HistoricalToolActivity{
			{Tool: "read_file", Input: map[string]any{"path": "/tmp/x"}, RawOutput: "file contents here"},
		}},
		{Role: RoleUser, Content: "thanks"},
	}

	b := EstimateContextBreakdown(systemPrompt, toolDefs, messages)

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
	messages := []ContextMessage{
		{Role: RoleAssistant, ToolActivity: []HistoricalToolActivity{
			{Tool: "bash", Input: map[string]any{"command": "ls"}},
		}},
	}
	b := EstimateContextBreakdown("", nil, messages)
	if b.ToolResultTokens <= 0 {
		t.Error("expected tool input to count toward tool result tokens")
	}
}

func TestEstimateContextBreakdown_NonStringRawOutput(t *testing.T) {
	messages := []ContextMessage{
		{Role: RoleAssistant, ToolActivity: []HistoricalToolActivity{
			{Tool: "read_file", RawOutput: map[string]any{"lines": 42}},
		}},
	}
	b := EstimateContextBreakdown("", nil, messages)
	if b.ToolResultTokens <= 0 {
		t.Error("expected non-string raw output to be estimated via JSON")
	}
}
