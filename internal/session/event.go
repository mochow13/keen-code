package session

import (
	"time"

	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/tools"
)

type EventKind string

const (
	KindSessionStarted    EventKind = "session_started"
	KindUserMessage       EventKind = "user_message"
	KindAssistantTurn     EventKind = "assistant_turn"
	KindCompactionApplied EventKind = "compaction_applied"
)

type Event struct {
	Seq  uint64    `json:"seq"`
	Kind EventKind `json:"kind"`

	SessionStarted    *SessionStartedPayload    `json:"session_started,omitempty"`
	UserMessage       *MessagePayload           `json:"user_message,omitempty"`
	AssistantTurn     *AssistantTurnPayload     `json:"assistant_turn,omitempty"`
	CompactionApplied *CompactionAppliedPayload `json:"compaction_applied,omitempty"`
}

type SessionStartedPayload struct {
	SessionID string    `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
	CWD       string    `json:"cwd"`
}

type MessagePayload struct {
	Content string `json:"content"`
}

type TranscriptItemKind string

const (
	TranscriptItemText      TranscriptItemKind = "text"
	TranscriptItemReasoning TranscriptItemKind = "reasoning"
	TranscriptItemToolStart TranscriptItemKind = "tool_start"
	TranscriptItemToolEnd   TranscriptItemKind = "tool_end"
	TranscriptItemBash      TranscriptItemKind = "bash"
	TranscriptItemDiff      TranscriptItemKind = "diff"
)

type AssistantTurnPayload struct {
	Transcript  []TranscriptItem `json:"transcript,omitempty"`
	Message     string           `json:"message,omitempty"`
	TurnMemory  *llm.TurnMemory  `json:"turn_memory,omitempty"`
	Interrupted bool             `json:"interrupted,omitempty"`
	Error       string           `json:"error,omitempty"`
}

type TranscriptItem struct {
	Kind      TranscriptItemKind `json:"kind"`
	Content   string             `json:"content,omitempty"`
	ToolStart *ToolStartPayload  `json:"tool_start,omitempty"`
	ToolEnd   *ToolEndPayload    `json:"tool_end,omitempty"`
	Bash      *BashPayload       `json:"bash,omitempty"`
	Diff      *DiffPayload       `json:"diff,omitempty"`
}

type ToolStartPayload struct {
	Name  string         `json:"name"`
	Input map[string]any `json:"input,omitempty"`
}

type ToolEndPayload struct {
	Name       string         `json:"name"`
	Input      map[string]any `json:"input,omitempty"`
	Output     any            `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	DurationNS int64          `json:"duration_ns,omitempty"`
}

type BashPayload struct {
	Command    string `json:"command"`
	Summary    string `json:"summary,omitempty"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
	DurationNS int64  `json:"duration_ns,omitempty"`
}

type DiffPayload struct {
	Lines []tools.EditDiffLine `json:"lines"`
}

type CompactionAppliedPayload struct {
	Status     string           `json:"status"`
	Transcript []TranscriptItem `json:"transcript,omitempty"`
	Messages   []llm.Message    `json:"messages"`
}

type Summary struct {
	ID              string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	LastUserMessage string
	Directory       string
	TranscriptPath  string
	LastSeq         uint64
}

func cloneMessages(messages []llm.Message) []llm.Message {
	return llm.CloneMessages(messages)
}
