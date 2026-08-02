package repl

import (
	"strings"

	replmarkdown "github.com/user/keen-code/internal/cli/repl/markdown"
	"github.com/user/keen-code/internal/llm"
	"github.com/user/keen-code/internal/subagents"
	"github.com/user/keen-code/internal/tools"
)

type StreamHandler struct {
	isActive        bool
	currentResponse string
	rawResponse     string
	eventCh         <-chan llm.StreamEvent
	loadingText     string
	lastWidth       int
	workingDir      string
	mdRenderer      *replmarkdown.Renderer
	segments        []streamSegment
	showThinking    bool
}

func NewStreamHandler(mdRenderer *replmarkdown.Renderer) *StreamHandler {
	return &StreamHandler{
		mdRenderer:   mdRenderer,
		segments:     make([]streamSegment, 0),
		showThinking: true,
	}
}

func (sh *StreamHandler) Start(eventCh <-chan llm.StreamEvent, loadingText string) {
	sh.isActive = true
	sh.currentResponse = ""
	sh.rawResponse = ""
	sh.eventCh = eventCh
	sh.loadingText = loadingText
	sh.lastWidth = 0
	sh.segments = make([]streamSegment, 0)
}

func (sh *StreamHandler) IsActive() bool {
	return sh.isActive
}

func (sh *StreamHandler) GetResponse() string {
	return sh.currentResponse
}

func (sh *StreamHandler) GetRawResponse() string {
	return sh.rawResponse
}

func (sh *StreamHandler) GetLoadingText() string {
	return sh.loadingText
}

func (sh *StreamHandler) SetLoadingText(loadingText string) {
	sh.loadingText = loadingText
}

func (sh *StreamHandler) HasContent() bool {
	return len(sh.segments) > 0
}

func (sh *StreamHandler) HandleChunk(chunk string) {
	sh.rawResponse += chunk
	sh.appendAssistantVisible(chunk)
}

func (sh *StreamHandler) HandleReasoningChunk(chunk string) {
	if n := len(sh.segments); n > 0 && sh.segments[n-1].kind == segmentReasoning {
		sh.segments[n-1].content += chunk
		return
	}
	sh.segments = append(sh.segments, streamSegment{kind: segmentReasoning, content: chunk})
}

func (sh *StreamHandler) HandleToolStart(toolCall *llm.ToolCall) {
	sh.segments = append(sh.segments, streamSegment{kind: segmentToolStart, toolCall: toolCall})
}

func (sh *StreamHandler) HandleToolEnd(toolCall *llm.ToolCall) {
	sh.segments = append(sh.segments, streamSegment{kind: segmentToolEnd, toolCall: toolCall})
}

func (sh *StreamHandler) HandleSubagentActivity(activity subagents.ToolActivity) {
	key := activity.RunID + ":" + activity.CallID
	switch activity.Event.Type {
	case llm.StreamEventTypeToolStart:
		sh.segments = append(sh.segments, streamSegment{
			kind:        segmentSubagent,
			agent:       activity.Agent,
			activityKey: key,
			toolCall:    activity.Event.ToolCall,
		})
	case llm.StreamEventTypeToolEnd:
		for i := len(sh.segments) - 1; i >= 0; i-- {
			if sh.segments[i].kind == segmentSubagent && sh.segments[i].activityKey == key {
				sh.segments[i].endToolCall = activity.Event.ToolCall
				return
			}
		}
		sh.segments = append(sh.segments, streamSegment{
			kind:        segmentSubagent,
			agent:       activity.Agent,
			activityKey: key,
			endToolCall: activity.Event.ToolCall,
		})
	}
}

func (sh *StreamHandler) HandleBashStart(command, summary string) {
	sh.segments = append(sh.segments, streamSegment{
		kind:    segmentBash,
		command: command,
		summary: summary,
	})
}

func (sh *StreamHandler) HandleBashEnd(toolCall *llm.ToolCall) {
	n := len(sh.segments)
	if n > 0 && sh.segments[n-1].kind == segmentBash {
		if result, ok := toolCall.Output.(map[string]any); ok {
			if stdout, ok := result["stdout"].(string); ok {
				sh.segments[n-1].output = stdout
			}
			if stderr, ok := result["stderr"].(string); ok && stderr != "" {
				if sh.segments[n-1].output != "" {
					sh.segments[n-1].output += "\n"
				}
				sh.segments[n-1].output += stderr
			}
		}
		sh.segments[n-1].toolCall = toolCall
	}
}

func (sh *StreamHandler) HandleDiff(lines []tools.EditDiffLine) {
	sh.segments = append(sh.segments, streamSegment{
		kind:      segmentDiff,
		diffLines: lines,
	})
}

func (sh *StreamHandler) HandleDone() ([]string, string) {
	response := sh.currentResponse
	lines := sh.renderTranscriptLines()
	sh.resetState()
	return lines, response
}

func (sh *StreamHandler) HandleError(err error) ([]string, string) {
	lines := sh.renderTranscriptLines()
	sh.resetState()
	if err == nil {
		return lines, ""
	}
	return lines, err.Error()
}

func (sh *StreamHandler) HandleInterrupt() []string {
	lines := sh.renderTranscriptLines()
	sh.resetState()
	return lines
}

func (sh *StreamHandler) Interrupt() {
	sh.resetState()
}

func (sh *StreamHandler) resetState() {
	sh.isActive = false
	sh.currentResponse = ""
	sh.rawResponse = ""
	sh.eventCh = nil
	sh.loadingText = ""
	sh.segments = make([]streamSegment, 0)
}

func (sh *StreamHandler) appendAssistantVisible(chunk string) {
	if chunk == "" {
		return
	}

	sh.currentResponse += chunk

	if n := len(sh.segments); n > 0 && sh.segments[n-1].kind == segmentAssistant {
		sh.segments[n-1].content += chunk
		return
	}

	sh.segments = append(sh.segments, streamSegment{kind: segmentAssistant, content: chunk})
}

// Checkpoint returns the current visible content and starts a fresh segment
// collection without disconnecting the active stream.
func (sh *StreamHandler) Checkpoint() ([]string, string, []streamSegment) {
	segments := cloneStreamSegments(sh.segments)
	lines := sh.renderTranscriptLines()
	response := sh.currentResponse
	sh.ResetContent()
	return lines, response, segments
}

func (sh *StreamHandler) ResetContent() {
	sh.currentResponse = ""
	sh.rawResponse = ""
	sh.segments = make([]streamSegment, 0)
}

// RewindForRetry discards only the in-flight assistant/reasoning chunks from a
// failed stream attempt so the upcoming retry does not duplicate them. Segments
// from completed prior tool-loop iterations (tool calls, bash output, diffs,
// permissions) are preserved. The accumulated response strings are rebuilt
// from the surviving assistant segments so they match what the user still sees.
func (sh *StreamHandler) RewindForRetry() {
	for len(sh.segments) > 0 {
		last := sh.segments[len(sh.segments)-1]
		if last.kind == segmentAssistant || last.kind == segmentReasoning {
			sh.segments = sh.segments[:len(sh.segments)-1]
			continue
		}
		break
	}

	var rebuilt strings.Builder
	for _, seg := range sh.segments {
		if seg.kind == segmentAssistant {
			rebuilt.WriteString(seg.content)
		}
	}
	sh.currentResponse = rebuilt.String()
	sh.rawResponse = sh.currentResponse
}

func (sh *StreamHandler) View(width int) string {
	sh.lastWidth = width

	var view strings.Builder

	for _, line := range sh.renderViewLines(width) {
		view.WriteString("\n")
		view.WriteString(line)
	}

	return view.String()
}
