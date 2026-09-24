package agentcore

import (
	"time"

	"github.com/mochow13/keen-code/internal/tools"
)

type Mode string

const (
	ModePlan  Mode = "plan"
	ModeBuild Mode = "build"
	ModeYolo  Mode = "yolo"
)

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

func (m *TurnMemory) IsEmpty() bool {
	return m == nil || len(m.ToolActivity) == 0
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

type ToolCall struct {
	Name     string
	Input    map[string]any
	Output   any
	Error    string
	Duration time.Duration
}

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

type StreamEvent struct {
	Type           StreamEventType
	Content        string
	Error          error
	ToolCall       *ToolCall
	Usage          *TokenUsage
	Attempt        int
	AutoCompaction *AutoCompactionEvent
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

type ToolActivity struct {
	RunID  string
	CallID string
	Agent  string
	Event  StreamEvent
}

type SkillStatus bool

const (
	SkillStatusDisabled SkillStatus = false
	SkillStatusEnabled  SkillStatus = true
)

type Skill struct {
	Name        string
	Description string
	Location    string
}

type SkillsDiscovery struct {
	Skills   []Skill
	Warnings []string
}

type SkillsConfig struct {
	IsEnabled map[string]bool `json:"is_enabled"`
}

func (c SkillsConfig) Enabled(name string) bool {
	if c.IsEnabled == nil {
		return true
	}
	enabled, ok := c.IsEnabled[name]
	if !ok {
		return true
	}
	return enabled
}

type SubagentProfile struct {
	Name        string
	Description string
	Hidden      bool
}

type SubagentsDiscovery struct {
	Profiles []SubagentProfile
	Warnings []string
}

const (
	ToolNameAskUser   = tools.AskUserToolName
	ToolNameBash      = tools.BashToolName
	ToolNameCallMCP   = tools.CallMCPToolName
	ToolNameDelegate  = tools.DelegateToolName
	ToolNameEditFile  = tools.EditFileToolName
	ToolNameGlob      = tools.GlobToolName
	ToolNameGrep      = tools.GrepToolName
	ToolNameReadFile  = tools.ReadFileToolName
	ToolNameWebFetch  = tools.WebFetchToolName
	ToolNameWriteFile = tools.WriteFileToolName
)

type AskUserQuestion struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

type AskUserRequest struct {
	Questions []AskUserQuestion `json:"questions"`
}

type AskUserResult struct {
	Answers   []string `json:"answers"`
	Cancelled bool     `json:"cancelled"`
}

type EditDiffLineKind int

const (
	EditDiffLineContext EditDiffLineKind = iota
	EditDiffLineAdded
	EditDiffLineRemoved
	EditDiffLineHunk
)

type EditDiffLine struct {
	Kind       EditDiffLineKind
	OldLineNum int
	NewLineNum int
	Content    string
}
