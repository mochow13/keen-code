package stream

import (
	"strings"

	"github.com/mochow13/keen-code/internal/agentcore"
	replmarkdown "github.com/mochow13/keen-code/internal/cli/repl/markdown"
)

type StreamHandler struct {
	isActive        bool
	currentResponse string
	rawResponse     string
	eventCh         <-chan agentcore.StreamEvent
	loadingText     string
	lastWidth       int
	workingDir      string
	mdRenderer      *replmarkdown.Renderer
	segments        []Segment
	showThinking    bool
}

// Option configures a StreamHandler without exposing its fields.
type Option func(*StreamHandler)

func WithWorkingDir(dir string) Option {
	return func(sh *StreamHandler) {
		sh.workingDir = dir
	}
}

func WithShowThinking(show bool) Option {
	return func(sh *StreamHandler) {
		sh.showThinking = show
	}
}

func WithWidth(width int) Option {
	return func(sh *StreamHandler) {
		sh.lastWidth = width
	}
}

func NewStreamHandler(mdRenderer *replmarkdown.Renderer, opts ...Option) *StreamHandler {
	sh := &StreamHandler{
		mdRenderer:   mdRenderer,
		segments:     make([]Segment, 0),
		showThinking: true,
	}
	for _, opt := range opts {
		opt(sh)
	}
	return sh
}

func (sh *StreamHandler) Start(eventCh <-chan agentcore.StreamEvent, loadingText string) {
	sh.isActive = true
	sh.currentResponse = ""
	sh.rawResponse = ""
	sh.eventCh = eventCh
	sh.loadingText = loadingText
	sh.lastWidth = 0
	sh.segments = make([]Segment, 0)
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

func (sh *StreamHandler) EventChannel() <-chan agentcore.StreamEvent {
	if sh == nil {
		return nil
	}
	return sh.eventCh
}

func (sh *StreamHandler) WorkingDir() string {
	if sh == nil {
		return ""
	}
	return sh.workingDir
}

func (sh *StreamHandler) SetWorkingDir(dir string) {
	if sh == nil {
		return
	}
	sh.workingDir = dir
}

func (sh *StreamHandler) ShowThinking() bool {
	if sh == nil {
		return false
	}
	return sh.showThinking
}

func (sh *StreamHandler) SetShowThinking(show bool) {
	if sh == nil {
		return
	}
	sh.showThinking = show
}

func (sh *StreamHandler) LastWidth() int {
	if sh == nil {
		return 0
	}
	return sh.lastWidth
}

func (sh *StreamHandler) SetLastWidth(width int) {
	if sh == nil {
		return
	}
	sh.lastWidth = width
}

func (sh *StreamHandler) Snapshot() []Segment {
	if sh == nil {
		return nil
	}
	return CloneSegments(sh.segments)
}

func (sh *StreamHandler) HasContent() bool {
	return len(sh.segments) > 0
}

func (sh *StreamHandler) HandleChunk(chunk string) {
	sh.rawResponse += chunk
	sh.appendAssistantVisible(chunk)
}

func (sh *StreamHandler) HandleReasoningChunk(chunk string) {
	if n := len(sh.segments); n > 0 && sh.segments[n-1].Kind == SegmentReasoning {
		sh.segments[n-1].Content += chunk
		return
	}
	sh.segments = append(sh.segments, Segment{Kind: SegmentReasoning, Content: chunk})
}

func (sh *StreamHandler) HandleToolStart(toolCall *agentcore.ToolCall) {
	sh.segments = append(sh.segments, Segment{Kind: SegmentToolStart, ToolCall: toolCall})
}

func (sh *StreamHandler) HandleToolEnd(toolCall *agentcore.ToolCall) {
	sh.segments = append(sh.segments, Segment{Kind: SegmentToolEnd, ToolCall: toolCall})
}

func (sh *StreamHandler) SetAskUser(card *AskUserCard) {
	for i := len(sh.segments) - 1; i >= 0; i-- {
		segment := &sh.segments[i]
		if segment.Kind != SegmentAskUser || !segment.AskUser.active() {
			continue
		}
		if card == nil {
			sh.segments = append(sh.segments[:i], sh.segments[i+1:]...)
		} else {
			segment.AskUser = card.Clone()
		}
		return
	}
	if card != nil {
		sh.segments = append(sh.segments, Segment{Kind: SegmentAskUser, AskUser: card.Clone()})
	}
}

func (sh *StreamHandler) HandleSubagentActivity(activity agentcore.ToolActivity) {
	key := activity.RunID + ":" + activity.CallID
	switch activity.Event.Type {
	case agentcore.StreamEventTypeToolStart:
		sh.segments = append(sh.segments, Segment{
			Kind:        SegmentSubagent,
			Agent:       activity.Agent,
			ActivityKey: key,
			ToolCall:    activity.Event.ToolCall,
		})
	case agentcore.StreamEventTypeToolEnd:
		for i := len(sh.segments) - 1; i >= 0; i-- {
			if sh.segments[i].Kind == SegmentSubagent && sh.segments[i].ActivityKey == key {
				sh.segments[i].EndToolCall = activity.Event.ToolCall
				return
			}
		}
		sh.segments = append(sh.segments, Segment{
			Kind:        SegmentSubagent,
			Agent:       activity.Agent,
			ActivityKey: key,
			EndToolCall: activity.Event.ToolCall,
		})
	}
}

func (sh *StreamHandler) HandleBashStart(command, summary string) {
	sh.segments = append(sh.segments, Segment{
		Kind:    SegmentBash,
		Command: command,
		Summary: summary,
	})
}

func (sh *StreamHandler) HandleBashEnd(toolCall *agentcore.ToolCall) {
	n := len(sh.segments)
	if n > 0 && sh.segments[n-1].Kind == SegmentBash {
		if result, ok := toolCall.Output.(map[string]any); ok {
			if stdout, ok := result["stdout"].(string); ok {
				sh.segments[n-1].Output = stdout
			}
			if stderr, ok := result["stderr"].(string); ok && stderr != "" {
				if sh.segments[n-1].Output != "" {
					sh.segments[n-1].Output += "\n"
				}
				sh.segments[n-1].Output += stderr
			}
		}
		sh.segments[n-1].ToolCall = toolCall
	}
}

func (sh *StreamHandler) HandleDiff(lines []agentcore.EditDiffLine) {
	sh.segments = append(sh.segments, Segment{
		Kind:      SegmentDiff,
		DiffLines: lines,
	})
}

// TranscriptLines renders the current stream transcript without finalizing or
// resetting the handler.
func (sh *StreamHandler) TranscriptLines() []string {
	if sh == nil {
		return nil
	}
	return sh.renderTranscriptLines()
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
	sh.segments = make([]Segment, 0)
}

func (sh *StreamHandler) appendAssistantVisible(chunk string) {
	if chunk == "" {
		return
	}

	sh.currentResponse += chunk

	if n := len(sh.segments); n > 0 && sh.segments[n-1].Kind == SegmentAssistant {
		sh.segments[n-1].Content += chunk
		return
	}

	sh.segments = append(sh.segments, Segment{Kind: SegmentAssistant, Content: chunk})
}

// Checkpoint returns the current visible content and starts a fresh segment
// collection without disconnecting the active stream.
func (sh *StreamHandler) Checkpoint() ([]string, string, []Segment) {
	segments := CloneSegments(sh.segments)
	lines := sh.renderTranscriptLines()
	response := sh.currentResponse
	sh.ResetContent()
	return lines, response, segments
}

func (sh *StreamHandler) ResetContent() {
	sh.currentResponse = ""
	sh.rawResponse = ""
	sh.segments = make([]Segment, 0)
}

// RewindForRetry discards only the in-flight assistant/reasoning chunks from a
// failed stream attempt so the upcoming retry does not duplicate them. Segments
// from completed prior tool-loop iterations (tool calls, bash output, diffs,
// permissions) are preserved. The accumulated response strings are rebuilt
// from the surviving assistant segments so they match what the user still sees.
func (sh *StreamHandler) RewindForRetry() {
	for len(sh.segments) > 0 {
		last := sh.segments[len(sh.segments)-1]
		if last.Kind == SegmentAssistant || last.Kind == SegmentReasoning {
			sh.segments = sh.segments[:len(sh.segments)-1]
			continue
		}
		break
	}

	var rebuilt strings.Builder
	for _, seg := range sh.segments {
		if seg.Kind == SegmentAssistant {
			rebuilt.WriteString(seg.Content)
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
