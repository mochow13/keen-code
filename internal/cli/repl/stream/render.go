package stream

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/mochow13/keen-code/internal/agentcore"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	reploutput "github.com/mochow13/keen-code/internal/cli/repl/output"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
)

const (
	DefaultWidth             = 120
	ContentHorizontalPadding = 4

	bashOutputMaxLines             = 16
	bashOutputExpandedMaxLines     = 200
	bashOutputMaxLineChars         = 200
	bashOutputExpandedMaxLineChars = 2000
	diffLeftPadding                = 2
	diffRightPadding               = 2
	diffNumWidth                   = 4
	diffTabWidth                   = 4
)

func wrapAndIndent(styled string, wrapWidth int) []string {
	if wrapWidth < 1 {
		wrapWidth = 1
	}
	wrapped := lipgloss.NewStyle().Width(wrapWidth).Render(styled)
	parts := strings.Split(wrapped, "\n")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = "  " + p
	}
	return out
}

func wrapTextWithStyle(text string, style lipgloss.Style, width int) string {
	if width < 1 {
		width = 1
	}
	return lipgloss.NewStyle().Width(width).Render(style.Render(text))
}

func renderToolStatusLines(line string, width int) []string {
	if width <= 0 {
		width = DefaultWidth
	}
	line = strings.TrimPrefix(line, "  ")
	return wrapAndIndent(line, width-ContentHorizontalPadding)
}

func (sh *StreamHandler) renderViewLines(width int) []string {
	lines := make([]string, 0)
	var gaps []int
	sh.hasRunningTool = false

	lastAssistantIdx := -1
	lastReasoningIdx := -1
	for i := range sh.segments {
		if sh.segments[i].Kind == SegmentAssistant {
			lastAssistantIdx = i
		}
		if sh.segments[i].Kind == SegmentReasoning {
			lastReasoningIdx = i
		}
	}

	for i := 0; i < len(sh.segments); i++ {
		seg := &sh.segments[i]
		switch seg.Kind {
		case SegmentToolStart:
			if seg.ToolCall != nil && seg.ToolCall.Name == agentcore.ToolNameAskUser {
				continue
			}
			if endCalls, endIndex := consecutiveReadCalls(sh.segments, i); len(endCalls) > 1 {
				lines = append(lines, renderToolStatusLines(reploutput.FormatFoldedReads(seg.ToolCall, endCalls, sh.workingDir), width)...)
				i = endIndex
				continue
			}
			if seg.ToolCall != nil {
				if sh.shouldHideToolStart(i) || (i+1 < len(sh.segments) && sh.segments[i+1].Kind == SegmentToolEnd) {
					continue
				}
				lines = append(lines, renderToolStatusLines(sh.blinkRunning(reploutput.FormatToolStart(seg.ToolCall, sh.workingDir)), width)...)
			}
		case SegmentToolEnd:
			if seg.ToolCall != nil {
				if seg.ToolCall.Name == agentcore.ToolNameAskUser || isHiddenToolFailure(seg.ToolCall) {
					continue
				}
				if i > 0 && sh.segments[i-1].Kind == SegmentToolStart && sh.segments[i-1].ToolCall != nil {
					lines = append(lines, renderToolStatusLines(reploutput.FormatToolDone(sh.segments[i-1].ToolCall, seg.ToolCall, sh.workingDir), width)...)
				} else {
					lines = append(lines, renderToolStatusLines(reploutput.FormatToolEnd(seg.ToolCall), width)...)
				}
			}
		case SegmentBash:
			lines, gaps = appendSpacedBlock(lines, gaps, sh.renderBashSegment(seg, width, true))
		case SegmentSubagent:
			if isHiddenToolFailure(seg.EndToolCall) {
				continue
			}
			line := reploutput.FormatSubagentTool(seg.Agent, seg.ToolCall, seg.EndToolCall, sh.workingDir)
			if seg.EndToolCall == nil {
				line = sh.blinkRunning(line)
			}
			if line != "" {
				lines = append(lines, renderToolStatusLines(line, width)...)
			}
		case SegmentAssistant:
			if seg.renderedLines == nil || i == lastAssistantIdx || sh.lastWidth != width {
				seg.renderedLines = sh.renderAssistantViewLines(seg.Content, width)
			}
			lines = append(lines, seg.renderedLines...)
		case SegmentReasoning:
			if !sh.showThinking {
				continue
			}
			if seg.renderedLines == nil || i == lastReasoningIdx || sh.lastWidth != width {
				seg.renderedLines = sh.renderReasoningViewLines(seg.Content, width)
			}
			lines = append(lines, seg.renderedLines...)
		case SegmentPermission:
			if seg.PermissionReq != nil {
				lines = append(lines, renderPermissionCard(seg, width)...)
			}
		case SegmentDiff:
			lines = append(lines, renderDiffSegment(seg, width)...)
		case SegmentAskUser:
			if seg.AskUser != nil {
				if card := seg.AskUser.Render(width); card != "" {
					lines = append(lines, strings.Split(strings.Trim(card, "\n"), "\n")...)
				}
			}
		}
	}

	return insertGaps(lines, gaps)
}

func (sh *StreamHandler) renderTranscriptLines() []string {
	return sh.renderTranscriptAtWidth(0)
}

func (sh *StreamHandler) renderTranscriptAtWidth(width int) []string {
	lines := make([]string, 0)
	var gaps []int

	for i := 0; i < len(sh.segments); i++ {
		seg := &sh.segments[i]
		switch seg.Kind {
		case SegmentToolStart:
			if seg.ToolCall != nil && seg.ToolCall.Name == agentcore.ToolNameAskUser {
				continue
			}
			if endCalls, endIndex := consecutiveReadCalls(sh.segments, i); len(endCalls) > 1 {
				lines = append(lines, renderToolStatusLines(reploutput.FormatFoldedReads(seg.ToolCall, endCalls, sh.workingDir), sh.lastWidth)...)
				i = endIndex
				continue
			}
			if seg.ToolCall != nil {
				if sh.shouldHideToolStart(i) || (i+1 < len(sh.segments) && sh.segments[i+1].Kind == SegmentToolEnd) {
					continue
				}
				lines = append(lines, renderToolStatusLines(reploutput.FormatToolStart(seg.ToolCall, sh.workingDir), sh.lastWidth)...)
			}
		case SegmentToolEnd:
			if seg.ToolCall != nil {
				if seg.ToolCall.Name == agentcore.ToolNameAskUser || isHiddenToolFailure(seg.ToolCall) {
					continue
				}
				if i > 0 && sh.segments[i-1].Kind == SegmentToolStart && sh.segments[i-1].ToolCall != nil {
					lines = append(lines, renderToolStatusLines(reploutput.FormatToolDone(sh.segments[i-1].ToolCall, seg.ToolCall, sh.workingDir), sh.lastWidth)...)
				} else {
					lines = append(lines, renderToolStatusLines(reploutput.FormatToolEnd(seg.ToolCall), sh.lastWidth)...)
				}
			}
		case SegmentBash:
			lines, gaps = appendSpacedBlock(lines, gaps, sh.renderBashSegment(seg, width, false))
		case SegmentSubagent:
			if isHiddenToolFailure(seg.EndToolCall) {
				continue
			}
			line := reploutput.FormatSubagentTool(seg.Agent, seg.ToolCall, seg.EndToolCall, sh.workingDir)
			if line != "" {
				lines = append(lines, renderToolStatusLines(line, sh.lastWidth)...)
			}
		case SegmentAssistant:
			if width > 0 {
				lines = append(lines, sh.renderAssistantViewLines(seg.Content, width)...)
			} else {
				lines = append(lines, sh.renderAssistantTranscriptLines(seg.Content)...)
			}
		case SegmentReasoning:
			if !sh.showThinking {
				continue
			}
			if width > 0 {
				lines = append(lines, sh.renderReasoningViewLines(seg.Content, width)...)
			} else {
				lines = append(lines, sh.renderReasoningTranscriptLines(seg.Content)...)
			}
		case SegmentPermission:
			if seg.PermissionReq != nil {
				lines = append(lines, wrapPermissionResolved(seg.PermissionReq, width)...)
			}
		case SegmentDiff:
			lines = append(lines, renderDiffSegment(seg, sh.lastWidth)...)
		case SegmentAskUser:
			if seg.AskUser != nil {
				width := sh.lastWidth
				if width <= 0 {
					width = DefaultWidth
				}
				if card := seg.AskUser.Render(width); card != "" {
					lines = append(lines, strings.Split(strings.Trim(card, "\n"), "\n")...)
				}
			}
		}
	}

	return insertGaps(lines, gaps)
}

func consecutiveReadCalls(segments []Segment, startIndex int) ([]*agentcore.ToolCall, int) {
	if startIndex >= len(segments) || segments[startIndex].Kind != SegmentToolStart {
		return nil, startIndex
	}
	startCall := segments[startIndex].ToolCall
	if startCall == nil || startCall.Name != agentcore.ToolNameReadFile {
		return nil, startIndex
	}
	path, _ := startCall.Input["path"].(string)

	var endCalls []*agentcore.ToolCall
	endIndex := startIndex
	for i := startIndex; i+1 < len(segments); i += 2 {
		start := segments[i]
		end := segments[i+1]
		if start.Kind != SegmentToolStart || start.ToolCall == nil || start.ToolCall.Name != agentcore.ToolNameReadFile ||
			end.Kind != SegmentToolEnd || end.ToolCall == nil || end.ToolCall.Name != agentcore.ToolNameReadFile || end.ToolCall.Error != "" {
			break
		}
		readPath, _ := start.ToolCall.Input["path"].(string)
		if readPath != path {
			break
		}
		endCalls = append(endCalls, end.ToolCall)
		endIndex = i + 1
	}
	return endCalls, endIndex
}

func (sh *StreamHandler) shouldHideToolStart(index int) bool {
	return index+1 < len(sh.segments) && sh.segments[index+1].Kind == SegmentToolEnd && isHiddenToolFailure(sh.segments[index+1].ToolCall)
}

// IsHiddenToolFailure reports whether a tool failure is hidden from the
// transcript view (e.g. expected read/edit misses). Headless progress uses
// it to skip noise on the console.
func IsHiddenToolFailure(toolCall *agentcore.ToolCall) bool {
	return isHiddenToolFailure(toolCall)
}
func isHiddenToolFailure(toolCall *agentcore.ToolCall) bool {
	if toolCall == nil {
		return false
	}
	if toolCall.Name == agentcore.ToolNameReadFile {
		return strings.HasPrefix(toolCall.Error, "not found: file ")
	}
	if toolCall.Name != agentcore.ToolNameEditFile {
		return false
	}
	return strings.Contains(toolCall.Error, "line hash mismatch") ||
		strings.Contains(toolCall.Error, "anchor ") && strings.Contains(toolCall.Error, "does not exist in the current file snapshot") ||
		strings.Contains(toolCall.Error, "only insert_head is valid for an empty file") ||
		strings.HasPrefix(toolCall.Error, "ops ") && (strings.Contains(toolCall.Error, "overlapping ranges") || strings.Contains(toolCall.Error, " conflict:")) ||
		strings.HasPrefix(toolCall.Error, "not found: file ") ||
		strings.HasPrefix(toolCall.Error, "not a file: ") && strings.HasSuffix(toolCall.Error, " is a directory") ||
		strings.HasPrefix(toolCall.Error, "path resolution failed:")
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
	wrapWidth := width - ContentHorizontalPadding
	formatted := make([]string, 0, len(responseLines))
	for _, line := range responseLines {
		formatted = append(formatted, wrapAndIndent(repltheme.AssistantStyle.Render(line), wrapWidth)...)
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
	wrapWidth := width - ContentHorizontalPadding
	formatted := make([]string, 0, len(responseLines))
	for _, line := range responseLines {
		formatted = append(formatted, wrapAndIndent(repltheme.ReasoningStyle.Render(line), wrapWidth)...)
	}
	return formatted
}

func (sh *StreamHandler) renderReasoningTranscriptLines(content string) []string {
	if content == "" {
		return nil
	}

	lines := strings.Split(content, "\n")
	wrapWidth := sh.lastWidth - ContentHorizontalPadding
	if wrapWidth < 1 {
		wrapWidth = DefaultWidth
	}

	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, wrapAndIndent(repltheme.ReasoningStyle.Render(line), wrapWidth)...)
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

func (sh *StreamHandler) RenderSegments(segments []Segment, width int) []string {
	width = max(width, 1)
	if sh.mdRenderer != nil {
		_ = sh.mdRenderer.UpdateWidth(width)
	}
	handler := &StreamHandler{segments: segments, mdRenderer: sh.mdRenderer, workingDir: sh.workingDir, showThinking: sh.showThinking, display: sh.display, lastWidth: width}
	return handler.renderTranscriptAtWidth(width)
}

func (sh *StreamHandler) AppendTranscript(output *reploutput.OutputBuilder, segments []Segment) {
	snapshot := CloneSegments(segments)
	handler := &StreamHandler{mdRenderer: sh.mdRenderer, workingDir: sh.workingDir, showThinking: sh.showThinking, display: sh.display}
	output.AddResponsive(func(width int) []string { return handler.RenderSegments(snapshot, width) })
}

func appendSpacedBlock(lines []string, gaps []int, block []string) ([]string, []int) {
	if len(block) > 1 {
		gaps = append(gaps, len(lines), len(lines)+len(block))
	}
	return append(lines, block...), gaps
}

func insertGaps(lines []string, gaps []int) []string {
	for i := len(gaps) - 1; i >= 0; i-- {
		at := gaps[i]
		if at <= 0 || at >= len(lines) || isBlankLine(lines[at-1]) || isBlankLine(lines[at]) {
			continue
		}
		lines = slices.Insert(lines, at, "")
	}
	return lines
}

func isBlankLine(line string) bool {
	return strings.TrimSpace(ansi.Strip(line)) == ""
}

func (sh *StreamHandler) blinkRunning(line string) string {
	sh.hasRunningTool = true
	if sh.markerHidden {
		return reploutput.HideRunningMarker(line)
	}
	return line
}

func (sh *StreamHandler) renderBashSegment(seg *Segment, width int, live bool) []string {
	finished := seg.ToolCall != nil
	failed := reploutput.BashFailed(seg.ToolCall)
	stdout, stderr := splitOutputLines(seg.Output), splitOutputLines(seg.Stderr)
	collapsed := finished && !failed && len(stdout)+len(stderr) == 0

	lines := sh.renderBashHeader(seg, width, live && !finished, collapsed)
	if collapsed {
		return lines
	}
	body := append([]string{repltheme.BashCommandStyle.Render("$ " + seg.Command)}, sh.bashOutputLines(seg, stdout, stderr, failed)...)
	return append(lines, withGutter(body, bashGutter(finished, failed), width)...)
}

func (sh *StreamHandler) renderBashHeader(seg *Segment, width int, running, collapsed bool) []string {
	header := reploutput.FormatBashStatus(seg.Summary, seg.Command, seg.ToolCall, collapsed)
	if running {
		header = sh.blinkRunning(header)
	}
	if width <= 0 {
		return []string{header}
	}
	return renderToolStatusLines(header, width)
}

func (sh *StreamHandler) bashOutputLines(seg *Segment, stdout, stderr []string, failed bool) []string {
	type stream struct {
		lines []string
		style lipgloss.Style
	}
	streams := [2]stream{{stdout, repltheme.BashOutputStyle}, {stderr, repltheme.ToolErrorStyle}}
	if failed {
		streams[0], streams[1] = streams[1], streams[0]
	}
	expanded := sh.display != nil && sh.display.BashExpanded
	limit, lineChars, hint := bashOutputMaxLines, bashOutputMaxLineChars, " · ctrl+o to expand"
	if expanded {
		limit, lineChars, hint = bashOutputExpandedMaxLines, bashOutputExpandedMaxLineChars, ""
	}

	meta := repltheme.ToolMetaStyle.Render
	var body []string
	first, second := splitLineBudget(len(streams[0].lines), len(streams[1].lines), limit)
	for i, budget := range [2]int{first, second} {
		s := streams[i]
		if hidden := len(s.lines) - budget; hidden > 0 {
			body = append(body, meta(fmt.Sprintf("… %d earlier lines", hidden)+hint))
			hint = ""
		}
		for _, line := range s.lines[len(s.lines)-budget:] {
			body = append(body, truncateOutputLine(line, lineChars, s.style))
		}
	}

	total := len(stdout) + len(stderr)
	switch {
	case failed && total == 0:
		body = append(body, meta("(no output)"))
	case expanded && total > bashOutputMaxLines:
		body = append(body, meta("ctrl+o to collapse"))
	}
	for _, saved := range [2]struct{ stream, file string }{{"stdout", seg.StdoutFile}, {"stderr", seg.StderrFile}} {
		if saved.file != "" {
			body = append(body, meta("full "+saved.stream+": "+displayHomePath(saved.file)))
		}
	}
	return body
}

func truncateOutputLine(line string, maxChars int, style lipgloss.Style) string {
	width := ansi.StringWidth(line)
	if width <= maxChars {
		return style.Render(line)
	}
	return style.Render(ansi.Truncate(line, maxChars, "")) + repltheme.ToolMetaStyle.Render(fmt.Sprintf(" … +%d chars", width-maxChars))
}

func bashGutter(finished, failed bool) string {
	style := repltheme.ToolMetaStyle
	switch {
	case failed:
		style = repltheme.ToolErrorStyle
	case finished:
		style = repltheme.RuleStyle
	}
	return "  " + style.Render("│") + " "
}

func withGutter(body []string, gutter string, width int) []string {
	lines := make([]string, 0, len(body))
	wrap := lipgloss.NewStyle().Width(max(1, width-ContentHorizontalPadding-2))
	for _, styled := range body {
		if width <= 0 {
			lines = append(lines, gutter+styled)
			continue
		}
		for _, part := range strings.Split(wrap.Render(styled), "\n") {
			lines = append(lines, gutter+part)
		}
	}
	return lines
}

func splitOutputLines(output string) []string {
	output = strings.TrimRight(output, "\n")
	if strings.TrimSpace(output) == "" {
		return nil
	}
	return strings.Split(output, "\n")
}

func splitLineBudget(first, second, budget int) (int, int) {
	if first+second <= budget {
		return first, second
	}
	half := budget / 2
	switch {
	case second <= half:
		return budget - second, second
	case first <= budget-half:
		return first, budget - first
	default:
		return budget - half, half
	}
}

func displayHomePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rest, ok := strings.CutPrefix(path, home+string(os.PathSeparator)); ok {
		return "~" + string(os.PathSeparator) + rest
	}
	return path
}

func sanitizeCommandOutput(output string) string {
	if output == "" {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if strings.Contains(line, "\r") {
			line = lastNonEmptyCarriageSegment(line)
		}
		lines[i] = strings.Map(func(r rune) rune {
			if r == '\t' || r >= 0x20 && r != 0x7f {
				return r
			}
			return -1
		}, ansi.Strip(line))
	}
	return strings.Join(lines, "\n")
}

func lastNonEmptyCarriageSegment(line string) string {
	parts := strings.Split(line, "\r")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return ""
}

func renderDiffSegment(seg *Segment, width int) []string {
	if !hasDiffChanges(seg.DiffLines) {
		return nil
	}
	if width <= 0 {
		width = DefaultWidth
	}
	rowWidth := max(width-diffLeftPadding-diffRightPadding, 1)
	padding := strings.Repeat(" ", diffLeftPadding)
	rule := padding + repltheme.RuleStyle.Render(strings.Repeat("─", rowWidth))

	lines := make([]string, 0, len(seg.DiffLines)+3)
	lines = append(lines, "", rule)
	rendered := false
	for _, dl := range seg.DiffLines {
		if dl.Kind == agentcore.EditDiffLineHunk {
			if rendered {
				lines = append(lines, padding+repltheme.DiffLineNumStyle.Render(fmt.Sprintf("%*s", diffNumWidth, "⋮")))
			}
			continue
		}
		lines = append(lines, renderDiffRow(dl, rowWidth, padding)...)
		rendered = true
	}
	return append(lines, rule)
}

func hasDiffChanges(lines []agentcore.EditDiffLine) bool {
	for _, dl := range lines {
		if dl.Kind == agentcore.EditDiffLineAdded || dl.Kind == agentcore.EditDiffLineRemoved {
			return true
		}
	}
	return false
}

func renderDiffRow(dl agentcore.EditDiffLine, rowWidth int, padding string) []string {
	var gutter, sign string
	signStyle := repltheme.DiffContextStyle
	textStyle := lipgloss.NewStyle()
	tinted := true
	switch dl.Kind {
	case agentcore.EditDiffLineAdded:
		gutter = fmt.Sprintf("%*s %*d", diffNumWidth, "", diffNumWidth, dl.NewLineNum)
		sign, signStyle, textStyle = "+", repltheme.DiffAddStyle, repltheme.DiffAddLineStyle
	case agentcore.EditDiffLineRemoved:
		gutter = fmt.Sprintf("%*d %*s", diffNumWidth, dl.OldLineNum, diffNumWidth, "")
		sign, signStyle, textStyle = "-", repltheme.DiffRemoveStyle, repltheme.DiffRemoveLineStyle
	default:
		gutter = fmt.Sprintf("%*d %*d", diffNumWidth, dl.OldLineNum, diffNumWidth, dl.NewLineNum)
		sign, textStyle, tinted = " ", repltheme.DiffContextStyle, false
	}

	bg := textStyle.GetBackground()
	numStyle := repltheme.DiffLineNumStyle.Background(bg)
	signStyle = signStyle.Background(bg)
	contentWidth := max(rowWidth-lipgloss.Width(gutter)-3, 1)
	content := strings.ReplaceAll(dl.Content, "\t", strings.Repeat(" ", diffTabWidth))
	chunks := strings.Split(ansi.Wrap(content, contentWidth, ""), "\n")

	rows := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		if i == 1 {
			gutter, sign = strings.Repeat(" ", lipgloss.Width(gutter)), " "
		}
		row := padding + numStyle.Render(gutter+" ") + signStyle.Render(sign+" ") + textStyle.Render(chunk)
		if tinted {
			if fill := contentWidth - ansi.StringWidth(chunk); fill > 0 {
				row += textStyle.Render(strings.Repeat(" ", fill))
			}
		}
		rows = append(rows, row)
	}
	return rows
}
