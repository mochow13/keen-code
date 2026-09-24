package core

import "encoding/json"

const DefaultContextWindowTokenCount = 200000

func EstimateContextTokenCount(text string) int {
	if text == "" {
		return 0
	}
	return max(1, (len(text)+2)/3)
}

func EstimateJSONTokenCount(v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return EstimateContextTokenCount(string(b))
}

func ContextInputBudget(contextWindowTokenCount int) int {
	window := contextWindowTokenCount
	if window <= 0 {
		window = DefaultContextWindowTokenCount
	}
	return max(1, window-max(4096, window/20))
}

func ShouldAutoCompact(estimatedInputTokenCount, effectiveBudget int) bool {
	if effectiveBudget <= 0 {
		return estimatedInputTokenCount > 0
	}
	return estimatedInputTokenCount >= effectiveBudget-(effectiveBudget/10)
}

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
		SystemPromptTokens:  EstimateContextTokenCount(systemPrompt),
		ToolDefinitionCount: len(toolDefs),
	}
	for _, def := range toolDefs {
		b.ToolDefTokens += EstimateContextTokenCount(def.Name) + EstimateContextTokenCount(def.Description) + EstimateJSONTokenCount(def.InputSchema)
	}
	for _, msg := range messages {
		switch msg.Role {
		case RoleUser:
			b.UserMessageTokens += EstimateContextTokenCount(msg.Content)
		case RoleAssistant:
			b.AssistantTokens += EstimateContextTokenCount(msg.Content)
			for _, activity := range msg.ToolActivity {
				if activity.Input != nil {
					b.ToolResultTokens += EstimateJSONTokenCount(activity.Input)
				}
				output := activity.RawOutput
				if activity.RetainedOutput != nil {
					output = activity.RetainedOutput
				}
				if stringOutput, ok := output.(string); ok {
					b.ToolResultTokens += EstimateContextTokenCount(stringOutput)
				} else if output != nil {
					b.ToolResultTokens += EstimateJSONTokenCount(output)
				}
			}
		}
	}
	b.TotalEstimated = b.SystemPromptTokens + b.ToolDefTokens + b.UserMessageTokens + b.AssistantTokens + b.ToolResultTokens
	return b
}
