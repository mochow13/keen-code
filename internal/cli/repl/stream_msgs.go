package repl

import (
	"time"

	"github.com/mochow13/keen-code/internal/agentcore"
	replaskuser "github.com/mochow13/keen-code/internal/cli/repl/askuser"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
	repltooling "github.com/mochow13/keen-code/internal/cli/repl/tooling"
	keenmcp "github.com/mochow13/keen-code/internal/mcp"
)

type llmChunkMsg string
type llmReasoningChunkMsg string
type llmDoneMsg struct{}
type llmIncompleteMsg struct {
	err error
}
type llmErrorMsg struct {
	err error
}
type llmRetryMsg struct {
	err     error
	attempt int
}
type llmToolStartMsg struct {
	toolCall *agentcore.ToolCall
}
type llmToolEndMsg struct {
	toolCall *agentcore.ToolCall
}
type llmUsageMsg struct {
	usage *agentcore.TokenUsage
}
type llmAutoCompactionStartedMsg struct {
	event *agentcore.AutoCompactionEvent
}
type llmAutoCompactionAppliedMsg struct {
	event *agentcore.AutoCompactionEvent
}
type llmAutoCompactionCancelledMsg struct {
	event *agentcore.AutoCompactionEvent
}
type llmAutoCompactionFailedMsg struct {
	event *agentcore.AutoCompactionEvent
}
type mainStreamMsg struct {
	eventCh <-chan agentcore.StreamEvent
	event   agentcore.StreamEvent
	closed  bool
}
type subagentActivityMsg struct {
	activity agentcore.ToolActivity
}
type askUserReadyMsg struct {
	req *replaskuser.Request
}

type permissionReadyMsg struct {
	req *replpermissions.Request
}
type diffReadyMsg struct {
	req repltooling.DiffRequest
}
type compactionDoneMsg struct{}
type compactionErrMsg struct {
	err error
}
type cleanupDoneMsg struct {
	err error
}
type updateCheckMsg struct {
	latest string
}
type mcpStartupStatusMsg struct {
	Statuses []keenmcp.ServerStatus
	Err      error
}
type mcpConnectDoneMsg struct {
	Server string
	Status keenmcp.ServerStatus
	Err    error
}

type copyNotificationExpiredMsg struct {
	expiresAt int64
}

type bangOutputMsg struct {
	stream string
	line   string
}
type bangDoneMsg struct {
	err      error
	exitCode int
	timedOut bool
	canceled bool
	duration time.Duration
}

type btwChunkMsg string
type btwDoneMsg struct{}
type btwErrorMsg struct {
	err error
}

// streamRenderMsg flushes a batched stream render. It is produced by a short
// tea.Tick so rapid chunks are coalesced into a single viewport rebuild instead
// of one rebuild per token.
type streamRenderMsg struct{}

type adversaryChunkMsg string
type adversaryDoneMsg struct{}
type adversaryErrorMsg struct {
	err error
}
type adversaryToolStartMsg struct {
	toolCall *agentcore.ToolCall
}
type adversaryToolEndMsg struct {
	toolCall *agentcore.ToolCall
}
