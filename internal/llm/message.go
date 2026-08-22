package llm

import "time"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role       Role
	Content    string
	TurnMemory *TurnMemory
}

type TurnMemory struct {
	ToolActivity []HistoricalToolActivity `json:"tool_activity,omitempty"`
}

type HistoricalToolActivity struct {
	TextOffset     int            `json:"text_offset"`
	Tool           string         `json:"tool"`
	Input          map[string]any `json:"input,omitempty"`
	Status         string         `json:"status"`
	ExitCode       *int           `json:"exit_code,omitempty"`
	HasRawOutput   bool           `json:"-"`
	RawOutput      any            `json:"-"`
	RetainedOutput any            `json:"retained_output,omitempty"`
}

func CloneMessage(message Message) Message {
	cloned := message
	cloned.TurnMemory = CloneTurnMemory(message.TurnMemory)
	return cloned
}

func CloneMessages(messages []Message) []Message {
	result := make([]Message, len(messages))
	for i, message := range messages {
		result[i] = CloneMessage(message)
	}
	return result
}

func CloneTurnMemory(memory *TurnMemory) *TurnMemory {
	if memory == nil {
		return nil
	}

	cloned := &TurnMemory{}
	if len(memory.ToolActivity) > 0 {
		cloned.ToolActivity = make([]HistoricalToolActivity, len(memory.ToolActivity))
		for i, activity := range memory.ToolActivity {
			cloned.ToolActivity[i] = activity
			cloned.ToolActivity[i].Input = cloneInputMap(activity.Input)
			cloned.ToolActivity[i].RawOutput = cloneInputValue(activity.RawOutput)
			cloned.ToolActivity[i].RetainedOutput = cloneInputValue(activity.RetainedOutput)
		}
	}
	return cloned
}

func cloneInputMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = cloneInputValue(value)
	}
	return cloned
}

func cloneInputValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneInputMap(value)
	case []any:
		cloned := make([]any, len(value))
		for i, item := range value {
			cloned[i] = cloneInputValue(item)
		}
		return cloned
	default:
		return value
	}
}

func (m *TurnMemory) IsEmpty() bool {
	return m == nil || len(m.ToolActivity) == 0
}

type StreamEventType string

const (
	StreamEventTypeChunk                   StreamEventType = "chunk"
	StreamEventTypeReasoningChunk          StreamEventType = "reasoning_chunk"
	StreamEventTypeDone                    StreamEventType = "done"
	StreamEventTypeError                   StreamEventType = "error"
	StreamEventTypeToolStart               StreamEventType = "tool_start"
	StreamEventTypeToolEnd                 StreamEventType = "tool_end"
	StreamEventTypeUsage                   StreamEventType = "usage"
	StreamEventTypeRetry                   StreamEventType = "retry"
	StreamEventTypeIncomplete              StreamEventType = "incomplete"
	StreamEventTypeAutoCompactionStarted   StreamEventType = "auto_compaction_started"
	StreamEventTypeAutoCompactionApplied   StreamEventType = "auto_compaction_applied"
	StreamEventTypeAutoCompactionCancelled StreamEventType = "auto_compaction_cancelled"
	StreamEventTypeAutoCompactionFailed    StreamEventType = "auto_compaction_failed"
)

type TokenUsage struct {
	InputTokens     int
	OutputTokens    int
	TotalTokens     int
	ReasoningTokens int
	CachedTokens    int
}

type AutoCompactionEvent struct {
	Cancel      func()
	Replacement []Message
	Usage       *TokenUsage
	Error       error
}

type StreamEvent struct {
	Type           StreamEventType
	Content        string
	Error          error
	ToolCall       *ToolCall
	Usage          *TokenUsage
	Attempt        int
	AutoCompaction *AutoCompactionEvent
}

type ToolCall struct {
	Name     string
	Input    map[string]any
	Output   any
	Error    string
	Duration time.Duration
}
