package repl

import (
	"github.com/mochow13/keen-code/internal/agentcore"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
)

type streamSegmentType string

const (
	segmentAssistant  streamSegmentType = "assistant"
	segmentReasoning  streamSegmentType = "reasoning"
	segmentToolStart  streamSegmentType = "tool_start"
	segmentToolEnd    streamSegmentType = "tool_end"
	segmentBash       streamSegmentType = "bash"
	segmentPermission streamSegmentType = "permission"
	segmentDiff       streamSegmentType = "diff"
	segmentSubagent   streamSegmentType = "subagent_tool"
	segmentAskUser    streamSegmentType = "ask_user"
)

type streamSegment struct {
	kind             streamSegmentType
	content          string
	toolCall         *agentcore.ToolCall
	command          string
	summary          string
	output           string
	renderedLines    []string
	permissionReq    *replpermissions.Request
	permissionCursor int
	diffLines        []agentcore.EditDiffLine
	agent            string
	activityKey      string
	endToolCall      *agentcore.ToolCall
	askUser          *askUserState
}
