package stream

import (
	"fmt"
	"strings"

	"github.com/mochow13/keen-code/internal/agentcore"

	"charm.land/lipgloss/v2"
	reploutput "github.com/mochow13/keen-code/internal/cli/repl/output"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
)

const (
	DefaultWidth             = 120
	ContentHorizontalPadding = 4

	bashOutputMaxLines = 16
	diffLeftPadding    = 2
	diffRightPadding   = 2
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
				lines = append(lines, renderToolStatusLines(reploutput.FormatToolStart(seg.ToolCall, sh.workingDir), width)...)
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
			lines = append(lines, sh.renderBashSegment(seg, width)...)
		case SegmentSubagent:
			if isHiddenToolFailure(seg.EndToolCall) {
				continue
			}
			line := reploutput.FormatSubagentTool(seg.Agent, seg.ToolCall, seg.EndToolCall, sh.workingDir)
			if line != "" {
				lines = append(lines, renderToolStatusLines(line, width)...)
			}
		case SegmentAssistant:
			if seg.renderedLines == nil || i == lastAssistantIdx {
				seg.renderedLines = sh.renderAssistantViewLines(seg.Content, width)
			}
			lines = append(lines, seg.renderedLines...)
		case SegmentReasoning:
			if !sh.showThinking {
				continue
			}
			if seg.renderedLines == nil || i == lastReasoningIdx {
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

	return lines
}

func (sh *StreamHandler) renderTranscriptLines() []string {
	lines := make([]string, 0)

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
			lines = append(lines, sh.renderBashSegment(seg, 0)...)
		case SegmentSubagent:
			if isHiddenToolFailure(seg.EndToolCall) {
				continue
			}
			line := reploutput.FormatSubagentTool(seg.Agent, seg.ToolCall, seg.EndToolCall, sh.workingDir)
			if line != "" {
				lines = append(lines, renderToolStatusLines(line, sh.lastWidth)...)
			}
		case SegmentAssistant:
			lines = append(lines, sh.renderAssistantTranscriptLines(seg.Content)...)
		case SegmentReasoning:
			if !sh.showThinking {
				continue
			}
			lines = append(lines, sh.renderReasoningTranscriptLines(seg.Content)...)
		case SegmentPermission:
			if seg.PermissionReq != nil {
				lines = append(lines, renderPermissionResolved(seg.PermissionReq)...)
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

	return lines
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

func (sh *StreamHandler) renderBashSegment(seg *Segment, width int) []string {
	ruleWidth := DefaultWidth
	if width > 0 {
		ruleWidth = width
	}
	if ruleWidth < 1 {
		ruleWidth = 1
	}
	rule := repltheme.RuleStyle.Render(strings.Repeat("─", ruleWidth))

	lines := make([]string, 0)

	lines = append(lines, "")
	lines = append(lines, rule)
	if width > 0 {
		lines = append(lines, wrapAndIndent(repltheme.BashCommandStyle.Render("$ "+seg.Command), width-ContentHorizontalPadding)...)
	} else {
		lines = append(lines, repltheme.BashCommandStyle.Render("  $ "+seg.Command))
	}

	if seg.Summary != "" {
		lines = append(lines, repltheme.BashSummaryStyle.Render("  › "+seg.Summary))
	}

	lines = append(lines, "")

	if seg.Output != "" {
		outputLines := strings.Split(seg.Output, "\n")
		total := len(outputLines)
		visible := outputLines
		if total > bashOutputMaxLines {
			visible = outputLines[:bashOutputMaxLines]
		}
		for _, line := range visible {
			if width > 0 {
				lines = append(lines, wrapAndIndent(repltheme.BashOutputStyle.Render(line), width-ContentHorizontalPadding)...)
			} else {
				lines = append(lines, "  "+repltheme.BashOutputStyle.Render(line))
			}
		}
		if total > bashOutputMaxLines {
			accentStyle := lipgloss.NewStyle().Foreground(repltheme.AccentColor)
			lines = append(lines, "  "+accentStyle.Render(fmt.Sprintf("→ %d more lines", total-bashOutputMaxLines)))
		}
	}

	lines = append(lines, rule)

	return lines
}

func renderWrappedDiffLine(prefix string, content string, contentStyle lipgloss.Style, width int) []string {
	renderedPrefix := prefix
	if width <= 0 {
		return []string{renderedPrefix + contentStyle.Render(content)}
	}

	contentWidth := width - lipgloss.Width(renderedPrefix) - diffRightPadding
	if contentWidth < 1 {
		contentWidth = 1
	}

	wrapped := lipgloss.NewStyle().Width(contentWidth).Render(contentStyle.Render(content))
	wrappedLines := strings.Split(strings.TrimRight(wrapped, "\n"), "\n")
	if len(wrappedLines) == 0 {
		return []string{renderedPrefix}
	}

	lines := make([]string, 0, len(wrappedLines))
	lines = append(lines, renderedPrefix+wrappedLines[0])

	continuationPrefix := strings.Repeat(" ", lipgloss.Width(renderedPrefix))
	for _, line := range wrappedLines[1:] {
		lines = append(lines, continuationPrefix+line)
	}

	return lines
}

func renderDiffLines(dl agentcore.EditDiffLine, width int) []string {
	switch dl.Kind {
	case agentcore.EditDiffLineHunk:
		return renderWrappedDiffLine("  ", dl.Content, repltheme.DiffHunkStyle, width)
	case agentcore.EditDiffLineAdded:
		lineNum := fmt.Sprintf("%4d", dl.NewLineNum)
		prefix := repltheme.DiffLineNumStyle.Render("     "+lineNum) + " " + repltheme.DiffAddStyle.Render("+ ")
		return renderWrappedDiffLine(prefix, dl.Content, repltheme.DiffAddStyle, width)
	case agentcore.EditDiffLineRemoved:
		lineNum := fmt.Sprintf("%4d", dl.OldLineNum)
		prefix := repltheme.DiffLineNumStyle.Render(lineNum+"     ") + " " + repltheme.DiffRemoveStyle.Render("- ")
		return renderWrappedDiffLine(prefix, dl.Content, repltheme.DiffRemoveStyle, width)
	default:
		prefix := repltheme.DiffLineNumStyle.Render(fmt.Sprintf("%4d %4d", dl.OldLineNum, dl.NewLineNum)) + " " + repltheme.DiffContextStyle.Render("  ")
		return renderWrappedDiffLine(prefix, dl.Content, repltheme.DiffContextStyle, width)
	}
}

func renderDiffSegment(seg *Segment, width int) []string {
	if len(seg.DiffLines) == 0 {
		return nil
	}

	rendered := make([]string, 0, len(seg.DiffLines))
	for _, dl := range seg.DiffLines {
		rendered = append(rendered, renderDiffLines(dl, width)...)
	}

	ruleWidth := DefaultWidth - diffLeftPadding - diffRightPadding
	if width > 0 {
		ruleWidth = width - diffLeftPadding - diffRightPadding
	}
	if ruleWidth < 1 {
		ruleWidth = 1
	}

	rule := strings.Repeat(" ", diffLeftPadding) + repltheme.RuleStyle.Render(strings.Repeat("─", ruleWidth))
	lines := make([]string, 0, len(rendered)+3)
	lines = append(lines, "")
	lines = append(lines, rule)
	lines = append(lines, rendered...)
	lines = append(lines, rule)
	return lines
}
