package llm

type ContextBreakdown struct {
	SystemPromptTokens  int
	ToolDefinitionCount int
	ToolDefTokens       int
	UserMessageTokens   int
	AssistantTokens     int
	ToolResultTokens    int
	TotalEstimated      int
}

type ContextToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type ContextMessage struct {
	Role         Role
	Content      string
	ToolActivity []HistoricalToolActivity
}

func EstimateContextBreakdown(systemPrompt string, toolDefs []ContextToolDef, messages []ContextMessage) ContextBreakdown {
	b := ContextBreakdown{
		SystemPromptTokens:  estimateContextTokenCount(systemPrompt),
		ToolDefinitionCount: len(toolDefs),
	}
	for _, def := range toolDefs {
		b.ToolDefTokens += estimateContextTokenCount(def.Name) + estimateContextTokenCount(def.Description) + estimateJSONTokenCount(def.InputSchema)
	}
	for _, msg := range messages {
		switch msg.Role {
		case RoleUser:
			b.UserMessageTokens += estimateContextTokenCount(msg.Content)
		case RoleAssistant:
			b.AssistantTokens += estimateContextTokenCount(msg.Content)
			for _, activity := range msg.ToolActivity {
				if activity.Input != nil {
					b.ToolResultTokens += estimateJSONTokenCount(activity.Input)
				}
				if output, ok := activity.RawOutput.(string); ok {
					b.ToolResultTokens += estimateContextTokenCount(output)
				} else if activity.RawOutput != nil {
					b.ToolResultTokens += estimateJSONTokenCount(activity.RawOutput)
				}
				if activity.RetainedOutput != nil {
					b.ToolResultTokens += estimateJSONTokenCount(activity.RetainedOutput)
				}
			}
		}
	}
	b.TotalEstimated = b.SystemPromptTokens + b.ToolDefTokens + b.UserMessageTokens + b.AssistantTokens + b.ToolResultTokens
	return b
}
