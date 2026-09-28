package stream

import (
	"strings"

	"github.com/mochow13/keen-code/internal/agentcore"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
)

type SegmentType string

const (
	SegmentAssistant  SegmentType = "assistant"
	SegmentReasoning  SegmentType = "reasoning"
	SegmentToolStart  SegmentType = "tool_start"
	SegmentToolEnd    SegmentType = "tool_end"
	SegmentBash       SegmentType = "bash"
	SegmentPermission SegmentType = "permission"
	SegmentDiff       SegmentType = "diff"
	SegmentSubagent   SegmentType = "subagent_tool"
	SegmentAskUser    SegmentType = "ask_user"
)

// AskUserCard is an immutable snapshot of the ask-user REPL state for one
// stream segment. The parent REPL owns rendering; the stream package only
// stores the snapshot and calls Render lazily.
type AskUserCard struct {
	Active bool
	Render func(width int) string
}

func (c *AskUserCard) Clone() *AskUserCard {
	if c == nil {
		return nil
	}
	cp := *c
	return &cp
}

func (c *AskUserCard) active() bool {
	return c != nil && c.Active
}

type Segment struct {
	Kind          SegmentType
	Content       string
	ToolCall      *agentcore.ToolCall
	Command       string
	Summary       string
	Output        string
	PermissionReq *replpermissions.Request
	DiffLines     []agentcore.EditDiffLine
	Agent         string
	ActivityKey   string
	EndToolCall   *agentcore.ToolCall
	AskUser       *AskUserCard

	renderedLines    []string
	permissionCursor int
}

func CloneSegments(segments []Segment) []Segment {
	result := make([]Segment, len(segments))
	for i, seg := range segments {
		result[i] = seg
		if seg.ToolCall != nil {
			toolCall := *seg.ToolCall
			toolCall.Input = cloneInput(seg.ToolCall.Input)
			result[i].ToolCall = &toolCall
		}
		if seg.EndToolCall != nil {
			endCall := *seg.EndToolCall
			endCall.Input = cloneInput(seg.EndToolCall.Input)
			result[i].EndToolCall = &endCall
		}
		if len(seg.DiffLines) > 0 {
			diffLines := make([]agentcore.EditDiffLine, len(seg.DiffLines))
			copy(diffLines, seg.DiffLines)
			result[i].DiffLines = diffLines
		}
		if seg.AskUser != nil {
			result[i].AskUser = seg.AskUser.Clone()
		}
		if len(seg.renderedLines) > 0 {
			rendered := make([]string, len(seg.renderedLines))
			copy(rendered, seg.renderedLines)
			result[i].renderedLines = rendered
		}
	}
	return result
}

func cloneInput(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func FinalAssistantRun(segments []Segment) string {
	start := len(segments)
	for start > 0 && segments[start-1].Kind == SegmentAssistant {
		start--
	}
	var content strings.Builder
	for _, segment := range segments[start:] {
		content.WriteString(segment.Content)
	}
	return content.String()
}

func HasNonTextActivity(segments []Segment) bool {
	for _, segment := range segments {
		if segment.Kind != SegmentAssistant && segment.Kind != SegmentReasoning {
			return true
		}
	}
	return false
}
