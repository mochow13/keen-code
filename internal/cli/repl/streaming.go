package repl

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/user/keen-code/internal/llm"
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
	segmentAssistant  streamSegmentType = "assistant"
	segmentToolStart  streamSegmentType = "tool_start"
	segmentToolEnd    streamSegmentType = "tool_end"
	segmentBash       streamSegmentType = "bash"
	segmentPermission streamSegmentType = "permission"
)

type streamSegment struct {
	kind             streamSegmentType
	content          string
	toolCall         *llm.ToolCall
	command          string
	summary          string
	output           string
	renderedLines    []string
	permissionReq    *PermissionRequest
	permissionCursor int
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

func (sh *StreamHandler) HandleBashStart(command, summary string) tea.Cmd {
	sh.segments = append(sh.segments, streamSegment{
		kind:    segmentBash,
		command: command,
		summary: summary,
	})
	return sh.waitForNextEvent()
}

func (sh *StreamHandler) HandleBashOutput(chunk string) tea.Cmd {
	n := len(sh.segments)
	if n > 0 && sh.segments[n-1].kind == segmentBash {
		sh.segments[n-1].output += chunk
	}
	return sh.waitForNextEvent()
}

func (sh *StreamHandler) HandleBashEnd(toolCall *llm.ToolCall) tea.Cmd {
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
	return sh.waitForNextEvent()
}

func (sh *StreamHandler) HandlePermissionRequest(req *PermissionRequest) {
	sh.segments = append(sh.segments, streamSegment{
		kind:          segmentPermission,
		permissionReq: req,
	})
}

func (sh *StreamHandler) HasPendingPermission() bool {
	n := len(sh.segments)
	if n == 0 {
		return false
	}
	seg := &sh.segments[n-1]
	return seg.kind == segmentPermission &&
		seg.permissionReq != nil &&
		seg.permissionReq.Status == PermissionStatusPending
}

func (sh *StreamHandler) MovePendingCursor(delta int) {
	n := len(sh.segments)
	if n == 0 {
		return
	}
	seg := &sh.segments[n-1]
	if seg.kind != segmentPermission || seg.permissionReq == nil {
		return
	}
	choices := permissionChoices(seg.permissionReq.IsDangerous)
	newCursor := seg.permissionCursor + delta
	if newCursor < 0 {
		newCursor = 0
	}
	if newCursor >= len(choices) {
		newCursor = len(choices) - 1
	}
	seg.permissionCursor = newCursor
}

func (sh *StreamHandler) GetPendingChoice() PermissionChoice {
	n := len(sh.segments)
	if n == 0 {
		return PermissionChoiceDeny
	}
	seg := &sh.segments[n-1]
	if seg.kind != segmentPermission || seg.permissionReq == nil {
		return PermissionChoiceDeny
	}
	return permissionChoiceAt(seg.permissionCursor, seg.permissionReq.IsDangerous)
}

func (sh *StreamHandler) GetPendingPermissionRequest() *PermissionRequest {
	n := len(sh.segments)
	if n == 0 {
		return nil
	}
	seg := &sh.segments[n-1]
	if seg.kind != segmentPermission || seg.permissionReq == nil {
		return nil
	}
	return seg.permissionReq
}

func (sh *StreamHandler) ResolvePendingPermission(status PermissionStatus) {
	n := len(sh.segments)
	if n == 0 {
		return
	}
	seg := &sh.segments[n-1]
	if seg.kind != segmentPermission || seg.permissionReq == nil {
		return
	}
	seg.permissionReq.Status = status
	seg.renderedLines = nil
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
	sh.eventCh = nil
	sh.loadingText = ""
	sh.segments = make([]streamSegment, 0)
}

func (sh *StreamHandler) WaitForEvent() tea.Cmd {
	return sh.waitForNextEvent()
}

func (sh *StreamHandler) waitForNextEvent() tea.Cmd {
	eventCh := sh.eventCh
	return func() tea.Msg {
		event, ok := <-eventCh
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

	lastAssistantIdx := -1
	for i := range sh.segments {
		if sh.segments[i].kind == segmentAssistant {
			lastAssistantIdx = i
		}
	}

	for i := range sh.segments {
		seg := &sh.segments[i]
		switch seg.kind {
		case segmentToolStart:
			if seg.toolCall != nil {
				lines = append(lines, formatToolStart(seg.toolCall))
			}
		case segmentToolEnd:
			if seg.toolCall != nil {
				lines = append(lines, formatToolEnd(seg.toolCall))
			}
		case segmentBash:
			bashLines := sh.renderBashSegment(seg, width)
			lines = append(lines, bashLines...)
		case segmentAssistant:
			if seg.renderedLines == nil || i == lastAssistantIdx {
				seg.renderedLines = sh.renderAssistantViewLines(seg.content, width)
			}
			lines = append(lines, seg.renderedLines...)
		case segmentPermission:
			if seg.permissionReq != nil {
				lines = append(lines, renderPermissionCard(seg, width)...)
			}
		}
	}

	return lines
}

func (sh *StreamHandler) renderTranscriptLines() []string {
	lines := make([]string, 0)

	for i := range sh.segments {
		seg := &sh.segments[i]
		switch seg.kind {
		case segmentToolStart:
			if seg.toolCall != nil {
				lines = append(lines, formatToolStart(seg.toolCall))
			}
		case segmentToolEnd:
			if seg.toolCall != nil {
				lines = append(lines, formatToolEnd(seg.toolCall))
			}
		case segmentBash:
			bashLines := sh.renderBashSegment(seg, 0)
			lines = append(lines, bashLines...)
		case segmentAssistant:
			lines = append(lines, sh.renderAssistantTranscriptLines(seg.content)...)
		case segmentPermission:
			if seg.permissionReq != nil {
				lines = append(lines, renderPermissionResolved(seg.permissionReq)...)
			}
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

func (sh *StreamHandler) renderBashSegment(seg *streamSegment, width int) []string {
	lines := make([]string, 0)

	lines = append(lines, "")
	lines = append(lines, bashCommandStyle.Render("$ "+seg.command))

	if seg.summary != "" {
		lines = append(lines, bashSummaryStyle.Render("› "+seg.summary))
	}

	lines = append(lines, "")

	if seg.output != "" {
		outputLines := strings.SplitSeq(seg.output, "\n")
		for line := range outputLines {
			if width > 0 {
				wrapStyle := lipgloss.NewStyle().Width(width - 4)
				lines = append(lines, "  "+bashOutputStyle.Render(wrapStyle.Render(line)))
			} else {
				lines = append(lines, "  "+bashOutputStyle.Render(line))
			}
		}
	}

	return lines
}

const permissionPreviewMaxLines = 120

func renderPermissionCard(seg *streamSegment, width int) []string {
	req := seg.permissionReq
	if req == nil {
		return nil
	}

	if req.Status != PermissionStatusPending {
		return renderPermissionResolved(req)
	}

	var sb strings.Builder

	if req.IsDangerous {
		sb.WriteString(warningTitleStyle.Render("⚠  Allow Dangerous Command?"))
	} else {
		sb.WriteString(permissionTitleStyle.Render("Permission Required"))
	}
	sb.WriteString("\n\n")

	sb.WriteString(infoLabelStyle.Render("Tool:") + " " + infoValueStyle.Render(req.ToolName) + "\n")
	sb.WriteString(infoLabelStyle.Render("Operation:") + " " + infoValueStyle.Render(req.Operation) + "\n")
	if req.IsDangerous {
		sb.WriteString(infoLabelStyle.Render("Command:") + " " + infoValueStyle.Render(req.Path) + "\n")
	} else {
		sb.WriteString(infoLabelStyle.Render("Path:") + " " + infoValueStyle.Render(req.Path) + "\n")
		if req.ResolvedPath != "" {
			sb.WriteString(infoLabelStyle.Render("Resolved:") + " " + infoValueStyle.Render(req.ResolvedPath) + "\n")
		}
	}

	if req.Preview != "" {
		previewStyle := lipgloss.NewStyle().Foreground(mutedColor)
		previewLines := strings.Split(req.Preview, "\n")
		total := len(previewLines)
		truncated := total > permissionPreviewMaxLines
		if truncated {
			previewLines = previewLines[:permissionPreviewMaxLines]
		}
		sb.WriteString("\n")
		for _, l := range previewLines {
			sb.WriteString(previewStyle.Render(l) + "\n")
		}
		if truncated {
			sb.WriteString(hintStyle.Render(fmt.Sprintf("... %d more preview lines omitted", total-permissionPreviewMaxLines)) + "\n")
		}
	}

	sb.WriteString("\n")

	choices := permissionChoices(req.IsDangerous)
	for i, choice := range choices {
		if i == seg.permissionCursor {
			sb.WriteString("> " + permissionSelectionStyle.Render(choice) + "\n")
		} else {
			sb.WriteString("  " + normalStyle.Render(choice) + "\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(hintStyle.Render("[↑/↓ navigate  Enter confirm  Esc deny]"))

	boxed := permissionCardStyle.Render(sb.String())
	rawLines := strings.Split(strings.TrimRight(boxed, "\n"), "\n")
	result := make([]string, 0, len(rawLines)+1)
	result = append(result, "")
	for _, l := range rawLines {
		result = append(result, "  "+l)
	}
	return result
}

func renderPermissionResolved(req *PermissionRequest) []string {
	var line string
	switch req.Status {
	case PermissionStatusAllowed:
		line = "  " + highlightStyle.Render("✓ Permission granted for "+req.ToolName)
	case PermissionStatusAllowedSession:
		line = "  " + highlightStyle.Render("✓ Permission granted for "+req.ToolName+" (this session)")
	case PermissionStatusDenied:
		line = "  " + lipgloss.NewStyle().Foreground(mutedColor).Render("✗ Permission denied for "+req.ToolName)
	case PermissionStatusAutoAllowedSession:
		line = "  " + highlightStyle.Render("✓ Auto-approved for "+req.ToolName+" (session)")
	default:
		line = "  " + lipgloss.NewStyle().Foreground(mutedColor).Render("✗ Permission cancelled for "+req.ToolName)
	}
	return []string{line}
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
