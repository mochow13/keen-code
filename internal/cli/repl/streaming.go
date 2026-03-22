package repl

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/user/keen-code/internal/llm"
	"github.com/user/keen-code/internal/tools"
)

type StreamHandler struct {
	isActive        bool
	currentResponse string
	eventCh         <-chan llm.StreamEvent
	loadingText     string
	lastWidth       int
	mdRenderer      *MarkdownRenderer
	segments        []streamSegment
}

type streamSegmentType string

const (
	segmentAssistant  streamSegmentType = "assistant"
	segmentReasoning  streamSegmentType = "reasoning"
	segmentToolStart  streamSegmentType = "tool_start"
	segmentToolEnd    streamSegmentType = "tool_end"
	segmentBash       streamSegmentType = "bash"
	segmentPermission streamSegmentType = "permission"
	segmentDiff       streamSegmentType = "diff"
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
	diffLines        []tools.EditDiffLine
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
	sh.lastWidth = 0
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

func (sh *StreamHandler) SetLoadingText(loadingText string) {
	sh.loadingText = loadingText
}

func (sh *StreamHandler) HasContent() bool {
	return len(sh.segments) > 0
}

func (sh *StreamHandler) HandleChunk(chunk string) {
	sh.currentResponse += chunk

	if n := len(sh.segments); n > 0 && sh.segments[n-1].kind == segmentAssistant {
		sh.segments[n-1].content += chunk
	} else {
		sh.segments = append(sh.segments, streamSegment{kind: segmentAssistant, content: chunk})
	}
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

func (sh *StreamHandler) HandlePermissionRequest(req *PermissionRequest) {
	sh.segments = append(sh.segments, streamSegment{
		kind:          segmentPermission,
		permissionReq: req,
	})
}

func (sh *StreamHandler) HandleDiff(lines []tools.EditDiffLine) {
	sh.segments = append(sh.segments, streamSegment{
		kind:      segmentDiff,
		diffLines: lines,
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

func (sh *StreamHandler) View(width int, showSpinner bool, spinnerView string) string {
	sh.lastWidth = width

	var view strings.Builder
	showInlineBashSpinner := showSpinner && sh.hasRunningBashSegment()

	for _, line := range sh.renderViewLines(width, showInlineBashSpinner, spinnerView) {
		view.WriteString("\n")
		view.WriteString(line)
	}

	if showSpinner && !showInlineBashSpinner {
		view.WriteString("\n  " + spinnerView + " " + sh.loadingText)
	}

	return view.String()
}

func (sh *StreamHandler) renderViewLines(width int, showInlineBashSpinner bool, spinnerView string) []string {
	lines := make([]string, 0)

	lastAssistantIdx := -1
	lastReasoningIdx := -1
	for i := range sh.segments {
		if sh.segments[i].kind == segmentAssistant {
			lastAssistantIdx = i
		}
		if sh.segments[i].kind == segmentReasoning {
			lastReasoningIdx = i
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
			bashLines := sh.renderBashSegment(seg, width, showInlineBashSpinner, spinnerView)
			lines = append(lines, bashLines...)
		case segmentAssistant:
			if seg.renderedLines == nil || i == lastAssistantIdx {
				seg.renderedLines = sh.renderAssistantViewLines(seg.content, width)
			}
			lines = append(lines, seg.renderedLines...)
		case segmentReasoning:
			if seg.renderedLines == nil || i == lastReasoningIdx {
				seg.renderedLines = sh.renderReasoningViewLines(seg.content, width)
			}
			lines = append(lines, seg.renderedLines...)
		case segmentPermission:
			if seg.permissionReq != nil {
				lines = append(lines, renderPermissionCard(seg, width)...)
			}
		case segmentDiff:
			lines = append(lines, renderDiffSegment(seg)...)
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
			bashLines := sh.renderBashSegment(seg, 0, false, "")
			lines = append(lines, bashLines...)
		case segmentAssistant:
			lines = append(lines, sh.renderAssistantTranscriptLines(seg.content)...)
		case segmentReasoning:
			lines = append(lines, sh.renderReasoningTranscriptLines(seg.content)...)
		case segmentPermission:
			if seg.permissionReq != nil {
				lines = append(lines, renderPermissionResolved(seg.permissionReq)...)
			}
		case segmentDiff:
			lines = append(lines, renderDiffSegment(seg)...)
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

func (sh *StreamHandler) renderReasoningViewLines(content string, width int) []string {
	if content == "" {
		return nil
	}

	responseLines := strings.Split(content, "\n")
	wrapWidth := width - 4
	if wrapWidth < 1 {
		wrapWidth = 1
	}
	wrapStyle := lipgloss.NewStyle().Width(wrapWidth)
	formatted := make([]string, 0, len(responseLines))
	for _, line := range responseLines {
		formatted = append(formatted, "  "+wrapStyle.Render(reasoningStyle.Render(line)))
	}
	return formatted
}

func (sh *StreamHandler) renderReasoningTranscriptLines(content string) []string {
	if content == "" {
		return nil
	}

	lines := strings.Split(content, "\n")
	wrapWidth := sh.lastWidth - 4
	if wrapWidth < 1 {
		wrapWidth = 120
	}
	wrapStyle := lipgloss.NewStyle().Width(wrapWidth)

	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, "  "+wrapStyle.Render(reasoningStyle.Render(line)))
	}
	return result
}

func formatResponseLines(response string) []string {
	lines := strings.Split(response, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = "  " + line
	}
	return result
}

func (sh *StreamHandler) renderBashSegment(seg *streamSegment, width int, showInlineSpinner bool, spinnerView string) []string {
	lines := make([]string, 0)

	lines = append(lines, "")
	lines = append(lines, bashCommandStyle.Render("$ "+seg.command))

	if seg.summary != "" {
		lines = append(lines, bashSummaryStyle.Render("› "+seg.summary))
	}

	if seg.toolCall == nil {
		runningLine := "Running command..."
		if showInlineSpinner && spinnerView != "" {
			runningLine = "\n" + spinnerView + " " + runningLine
		}
		lines = append(lines, bashRunningStyle.Render(runningLine))
		lines = append(lines, bashHintStyle.Render("\nPress Esc to interrupt"))
	}

	lines = append(lines, "")

	if seg.output != "" {
		outputLines := strings.Split(seg.output, "\n")
		total := len(outputLines)
		visible := outputLines
		if total > bashOutputMaxLines {
			visible = outputLines[:bashOutputMaxLines]
		}
		for _, line := range visible {
			if width > 0 {
				wrapStyle := lipgloss.NewStyle().Width(width - 4)
				lines = append(lines, "  "+bashOutputStyle.Render(wrapStyle.Render(line)))
			} else {
				lines = append(lines, "  "+bashOutputStyle.Render(line))
			}
		}
		if total > bashOutputMaxLines {
			accentStyle := lipgloss.NewStyle().Foreground(accentColor)
			lines = append(lines, "  "+accentStyle.Render(fmt.Sprintf("→ %d more lines", total-bashOutputMaxLines)))
		}
	}

	return lines
}

func (sh *StreamHandler) hasRunningBashSegment() bool {
	for i := len(sh.segments) - 1; i >= 0; i-- {
		if sh.segments[i].kind == segmentBash {
			return sh.segments[i].toolCall == nil
		}
	}
	return false
}

func renderDiffLine(dl tools.EditDiffLine) string {
	switch dl.Kind {
	case tools.DiffLineHunk:
		return "  " + diffHunkStyle.Render(dl.Content)
	case tools.DiffLineAdded:
		lineNum := fmt.Sprintf("%4d", dl.NewLineNum)
		return diffLineNumStyle.Render("     "+lineNum) + " " + diffAddStyle.Render("+ "+dl.Content)
	case tools.DiffLineRemoved:
		lineNum := fmt.Sprintf("%4d", dl.OldLineNum)
		return diffLineNumStyle.Render(lineNum+"     ") + " " + diffRemoveStyle.Render("- "+dl.Content)
	default:
		return diffLineNumStyle.Render(fmt.Sprintf("%4d %4d", dl.OldLineNum, dl.NewLineNum)) + " " + diffContextStyle.Render("  "+dl.Content)
	}
}

func renderDiffSegment(seg *streamSegment) []string {
	if len(seg.diffLines) == 0 {
		return nil
	}
	lines := make([]string, 0, len(seg.diffLines)+1)
	lines = append(lines, "")
	for _, dl := range seg.diffLines {
		lines = append(lines, renderDiffLine(dl))
	}
	return lines
}

const permissionPreviewMaxLines = 120
const bashOutputMaxLines = 30

func renderPermissionCard(seg *streamSegment, width int) []string {
	req := seg.permissionReq
	if req == nil {
		return nil
	}

	if req.Status != PermissionStatusPending {
		return renderPermissionResolved(req)
	}

	cardWidth := width - 4
	if cardWidth < 20 {
		cardWidth = 20
	}
	cardStyle := userPromptCardStyle.MaxWidth(cardWidth)
	contentWidth := cardWidth - cardStyle.GetHorizontalFrameSize()
	if contentWidth < 1 {
		contentWidth = 1
	}

	labelWidth := lipgloss.Width(infoLabelStyle.Render("Resolved:"))
	if labelWidth < 1 {
		labelWidth = 1
	}
	valueWidth := contentWidth - labelWidth - 1
	if valueWidth < 1 {
		valueWidth = 1
	}

	var sb strings.Builder

	if req.IsDangerous {
		sb.WriteString(warningTitleStyle.Render("⚠  Allow Dangerous Command?"))
	} else {
		sb.WriteString(userPromptStyle.Render("Permission Required"))
	}
	sb.WriteString("\n\n")

	sb.WriteString(formatPermissionKeyValue("Tool:", req.ToolName, labelWidth, valueWidth))
	if req.IsDangerous {
		sb.WriteString(formatPermissionKeyValue("Command:", req.Path, labelWidth, valueWidth))
	} else {
		sb.WriteString(formatPermissionKeyValue("Path:", req.Path, labelWidth, valueWidth))
		if req.ResolvedPath != "" {
			sb.WriteString(formatPermissionKeyValue("Resolved:", req.ResolvedPath, labelWidth, valueWidth))
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
			sb.WriteString(wrapTextWithStyle(l, previewStyle, contentWidth))
			sb.WriteString("\n")
		}
		if truncated {
			sb.WriteString(wrapTextWithStyle(fmt.Sprintf("... %d more preview lines omitted", total-permissionPreviewMaxLines), hintStyle, contentWidth))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")

	choices := permissionChoices(req.IsDangerous)
	for i, choice := range choices {
		if i == seg.permissionCursor {
			sb.WriteString(wrapTextWithStyle("> "+choice, userPromptSelectionStyle, contentWidth))
			sb.WriteString("\n")
		} else {
			sb.WriteString(wrapTextWithStyle("  "+choice, normalStyle, contentWidth))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(wrapTextWithStyle("[↑/↓ navigate  Enter confirm  Esc deny]", hintStyle, contentWidth))

	boxed := cardStyle.Render(sb.String())
	rawLines := strings.Split(strings.TrimRight(boxed, "\n"), "\n")
	result := make([]string, 0, len(rawLines)+1)
	result = append(result, "")
	for _, l := range rawLines {
		result = append(result, "  "+l)
	}
	return result
}

func wrapTextWithStyle(text string, style lipgloss.Style, width int) string {
	if width < 1 {
		width = 1
	}
	return lipgloss.NewStyle().Width(width).Render(style.Render(text))
}

func formatPermissionKeyValue(label, value string, labelWidth, valueWidth int) string {
	if labelWidth < 1 {
		labelWidth = 1
	}
	if valueWidth < 1 {
		valueWidth = 1
	}

	prefix := infoLabelStyle.Width(labelWidth).Render(label)
	continuation := strings.Repeat(" ", labelWidth+1)
	if value == "" {
		return prefix + " \n"
	}

	wrapped := wrapTextWithStyle(value, infoValueStyle, valueWidth)
	lines := strings.Split(strings.TrimRight(wrapped, "\n"), "\n")
	if len(lines) == 0 {
		return prefix + " \n"
	}

	var out strings.Builder
	out.WriteString(prefix + " " + lines[0] + "\n")
	for _, line := range lines[1:] {
		out.WriteString(continuation + line + "\n")
	}
	return out.String()
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
type llmReasoningChunkMsg string
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
type permissionReadyMsg struct {
	req *PermissionRequest
}
type diffReadyMsg struct {
	req diffEmitRequest
}
