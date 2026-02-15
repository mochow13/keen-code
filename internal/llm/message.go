package llm

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
	StreamEventTypeChunk StreamEventType = "chunk"
	StreamEventTypeDone  StreamEventType = "done"
	StreamEventTypeError StreamEventType = "error"
)

type StreamEvent struct {
	Type    StreamEventType
	Content string
	Error   error
}
