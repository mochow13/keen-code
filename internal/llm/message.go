package llm

import "time"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role    Role
	Content string
}

type StreamEventType string

const (
	StreamEventTypeChunk     StreamEventType = "chunk"
	StreamEventTypeDone      StreamEventType = "done"
	StreamEventTypeError     StreamEventType = "error"
	StreamEventTypeToolStart StreamEventType = "tool_start"
	StreamEventTypeToolEnd   StreamEventType = "tool_end"
)

type StreamEvent struct {
	Type     StreamEventType
	Content  string
	Error    error
	ToolCall *ToolCall
}

type ToolCall struct {
	Name     string
	Input    map[string]any
	Output   any
	Error    string
	Duration time.Duration
}
