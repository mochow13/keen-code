package repl

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/keen-cli/internal/llm"
)

type StreamHandler struct {
	isActive        bool
	currentResponse string
	eventCh         <-chan llm.StreamEvent
	loadingText     string
	mdRenderer      *MarkdownRenderer
	segments        []streamSegment
}

type streamSegmentType string

const (
	segmentAssistant streamSegmentType = "assistant"
	segmentToolStart streamSegmentType = "tool_start"
	segmentToolEnd   streamSegmentType = "tool_end"
)

type streamSegment struct {
	kind     streamSegmentType
	content  string
	toolCall *llm.ToolCall
}

func NewStreamHandler(mdRenderer *MarkdownRenderer) *StreamHandler {
	return &StreamHandler{
		mdRenderer: mdRenderer,
		segments:   make([]streamSegment, 0),
	}
}

func (sh *StreamHandler) Start(eventCh <-chan llm.StreamEvent, loadingText string) {
	sh.isActive = true
	sh.currentResponse = ""
	sh.eventCh = eventCh
	sh.loadingText = loadingText
	sh.segments = make([]streamSegment, 0)
}

func (sh *StreamHandler) IsActive() bool {
	return sh.isActive
}

func (sh *StreamHandler) GetResponse() string {
	return sh.currentResponse
}

func (sh *StreamHandler) GetLoadingText() string {
	return sh.loadingText
}

func (sh *StreamHandler) HasContent() bool {
	return len(sh.segments) > 0
}

func (sh *StreamHandler) HandleChunk(chunk string) tea.Cmd {
	sh.currentResponse += chunk

	if n := len(sh.segments); n > 0 && sh.segments[n-1].kind == segmentAssistant {
		sh.segments[n-1].content += chunk
	} else {
		sh.segments = append(sh.segments, streamSegment{kind: segmentAssistant, content: chunk})
	}

	return sh.waitForNextEvent()
}

func (sh *StreamHandler) HandleToolStart(toolCall *llm.ToolCall) tea.Cmd {
	sh.segments = append(sh.segments, streamSegment{kind: segmentToolStart, toolCall: toolCall})
	return sh.waitForNextEvent()
}

func (sh *StreamHandler) HandleToolEnd(toolCall *llm.ToolCall) tea.Cmd {
	sh.segments = append(sh.segments, streamSegment{kind: segmentToolEnd, toolCall: toolCall})
	return sh.waitForNextEvent()
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
	return lines, err.Error()
}

func (sh *StreamHandler) resetState() {
	sh.isActive = false
	sh.currentResponse = ""
	sh.eventCh = nil
	sh.loadingText = ""
	sh.segments = make([]streamSegment, 0)
}

func (sh *StreamHandler) WaitForEvent() tea.Cmd {
	return sh.waitForNextEvent()
}

func (sh *StreamHandler) waitForNextEvent() tea.Cmd {
	return func() tea.Msg {
		event, ok := <-sh.eventCh
		if !ok {
			return llmDoneMsg{}
		}

		switch event.Type {
		case llm.StreamEventTypeChunk:
			return llmChunkMsg(event.Content)
		case llm.StreamEventTypeDone:
			return llmDoneMsg{}
		case llm.StreamEventTypeError:
			return llmErrorMsg{err: event.Error}
		case llm.StreamEventTypeToolStart:
			return llmToolStartMsg{toolCall: event.ToolCall}
		case llm.StreamEventTypeToolEnd:
			return llmToolEndMsg{toolCall: event.ToolCall}
		default:
			return llmDoneMsg{}
		}
	}
}

func (sh *StreamHandler) View(width int, showSpinner bool, spinnerView string) string {
	var view strings.Builder

	for _, line := range sh.renderViewLines(width) {
		view.WriteString("\n")
		view.WriteString(line)
	}

	if showSpinner {
		view.WriteString("\n  " + spinnerView + " " + sh.loadingText)
	}

	return view.String()
}

func (sh *StreamHandler) renderViewLines(width int) []string {
	lines := make([]string, 0)

	for _, segment := range sh.segments {
		switch segment.kind {
		case segmentToolStart:
			if segment.toolCall != nil {
				lines = append(lines, formatToolStart(segment.toolCall))
			}
		case segmentToolEnd:
			if segment.toolCall != nil {
				lines = append(lines, formatToolEnd(segment.toolCall))
			}
		case segmentAssistant:
			lines = append(lines, sh.renderAssistantViewLines(segment.content, width)...)
		}
	}

	return lines
}

func (sh *StreamHandler) renderTranscriptLines() []string {
	lines := make([]string, 0)

	for _, segment := range sh.segments {
		switch segment.kind {
		case segmentToolStart:
			if segment.toolCall != nil {
				lines = append(lines, formatToolStart(segment.toolCall))
			}
		case segmentToolEnd:
			if segment.toolCall != nil {
				lines = append(lines, formatToolEnd(segment.toolCall))
			}
		case segmentAssistant:
			lines = append(lines, sh.renderAssistantTranscriptLines(segment.content)...)
		}
	}

	return lines
}

func (sh *StreamHandler) renderAssistantViewLines(content string, width int) []string {
	if content == "" {
		return nil
	}

	if sh.mdRenderer != nil {
		rendered := sh.mdRenderer.Render(content)
		if rendered == "" {
			return nil
		}
		rawLines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
		formatted := make([]string, 0, len(rawLines))
		for _, line := range rawLines {
			formatted = append(formatted, "  "+line)
		}
		return formatted
	}

	responseLines := strings.Split(content, "\n")
	wrapWidth := width - 4
	if wrapWidth < 1 {
		wrapWidth = 1
	}
	wrapStyle := lipgloss.NewStyle().Width(wrapWidth)
	formatted := make([]string, 0, len(responseLines))
	for _, line := range responseLines {
		formatted = append(formatted, "  "+wrapStyle.Render(assistantStyle.Render(line)))
	}
	return formatted
}

func (sh *StreamHandler) renderAssistantTranscriptLines(content string) []string {
	if content == "" {
		return nil
	}

	if sh.mdRenderer != nil {
		rendered := sh.mdRenderer.Render(content)
		if rendered == "" {
			return nil
		}
		rawLines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
		formatted := make([]string, 0, len(rawLines))
		for _, line := range rawLines {
			formatted = append(formatted, "  "+line)
		}
		return formatted
	}

	return formatResponseLines(content)
}

func formatResponseLines(response string) []string {
	lines := strings.Split(response, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = "  " + line
	}
	return result
}

type llmChunkMsg string
type llmDoneMsg struct{}
type llmErrorMsg struct {
	err error
}
type llmToolStartMsg struct {
	toolCall *llm.ToolCall
}
type llmToolEndMsg struct {
	toolCall *llm.ToolCall
}
