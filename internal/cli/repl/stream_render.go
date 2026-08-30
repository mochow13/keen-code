package repl

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	reploutput "github.com/mochow13/keen-code/internal/cli/repl/output"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/tools"
)

const bashOutputMaxLines = 16
const diffRightPadding = 2

// wrapAndIndent wraps an already-styled string to wrapWidth and prefixes every
// produced sub-line with two spaces so wrapped continuations stay aligned with
// the first line.
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

func renderToolStatusLines(line string, width int) []string {
	if width <= 0 {
		width = defaultWidth
	}
	line = strings.TrimPrefix(line, "  ")
	return wrapAndIndent(line, width-4)
}

func (sh *StreamHandler) renderViewLines(width int) []string {
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

	for i := 0; i < len(sh.segments); i++ {
		seg := &sh.segments[i]
		switch seg.kind {
		case segmentToolStart:
			if seg.toolCall != nil && seg.toolCall.Name == tools.AskUserToolName {
				continue
			}
			if endCalls, endIndex := consecutiveReadCalls(sh.segments, i); len(endCalls) > 1 {
				lines = append(lines, renderToolStatusLines(reploutput.FormatFoldedReads(seg.toolCall, endCalls, sh.workingDir), width)...)
				i = endIndex
				continue
			}
			if seg.toolCall != nil {
				if sh.shouldHideToolStart(i) || (i+1 < len(sh.segments) && sh.segments[i+1].kind == segmentToolEnd) {
					continue
				}
				lines = append(lines, renderToolStatusLines(reploutput.FormatToolStart(seg.toolCall, sh.workingDir), width)...)
			}
		case segmentToolEnd:
			if seg.toolCall != nil {
				if seg.toolCall.Name == tools.AskUserToolName || isHiddenToolFailure(seg.toolCall) {
					continue
				}
				if i > 0 && sh.segments[i-1].kind == segmentToolStart && sh.segments[i-1].toolCall != nil {
					lines = append(lines, renderToolStatusLines(reploutput.FormatToolDone(sh.segments[i-1].toolCall, seg.toolCall, sh.workingDir), width)...)
				} else {
					lines = append(lines, renderToolStatusLines(reploutput.FormatToolEnd(seg.toolCall), width)...)
				}
			}
		case segmentBash:
			lines = append(lines, sh.renderBashSegment(seg, width)...)
		case segmentSubagent:
			if isHiddenToolFailure(seg.endToolCall) {
				continue
			}
			line := reploutput.FormatSubagentTool(seg.agent, seg.toolCall, seg.endToolCall, sh.workingDir)
			if line != "" {
				lines = append(lines, renderToolStatusLines(line, width)...)
			}
		case segmentAssistant:
			if seg.renderedLines == nil || i == lastAssistantIdx {
				seg.renderedLines = sh.renderAssistantViewLines(seg.content, width)
			}
			lines = append(lines, seg.renderedLines...)
		case segmentReasoning:
			if !sh.showThinking {
				continue
			}
			if seg.renderedLines == nil || i == lastReasoningIdx {
				seg.renderedLines = sh.renderReasoningViewLines(seg.content, width)
			}
			lines = append(lines, seg.renderedLines...)
		case segmentPermission:
			if seg.permissionReq != nil {
				lines = append(lines, renderPermissionCard(seg, width)...)
			}
		case segmentDiff:
			lines = append(lines, renderDiffSegment(seg, width)...)
		case segmentAskUser:
			if seg.askUser != nil {
				lines = append(lines, strings.Split(strings.Trim(renderAskUserCard(*seg.askUser, width), "\n"), "\n")...)
			}
		}
	}

	return lines
}

func (sh *StreamHandler) renderTranscriptLines() []string {
	lines := make([]string, 0)

	for i := 0; i < len(sh.segments); i++ {
		seg := &sh.segments[i]
		switch seg.kind {
		case segmentToolStart:
			if seg.toolCall != nil && seg.toolCall.Name == tools.AskUserToolName {
				continue
			}
			if endCalls, endIndex := consecutiveReadCalls(sh.segments, i); len(endCalls) > 1 {
				lines = append(lines, renderToolStatusLines(reploutput.FormatFoldedReads(seg.toolCall, endCalls, sh.workingDir), sh.lastWidth)...)
				i = endIndex
				continue
			}
			if seg.toolCall != nil {
				if sh.shouldHideToolStart(i) || (i+1 < len(sh.segments) && sh.segments[i+1].kind == segmentToolEnd) {
					continue
				}
				lines = append(lines, renderToolStatusLines(reploutput.FormatToolStart(seg.toolCall, sh.workingDir), sh.lastWidth)...)
			}
		case segmentToolEnd:
			if seg.toolCall != nil {
				if seg.toolCall.Name == tools.AskUserToolName || isHiddenToolFailure(seg.toolCall) {
					continue
				}
				if i > 0 && sh.segments[i-1].kind == segmentToolStart && sh.segments[i-1].toolCall != nil {
					lines = append(lines, renderToolStatusLines(reploutput.FormatToolDone(sh.segments[i-1].toolCall, seg.toolCall, sh.workingDir), sh.lastWidth)...)
				} else {
					lines = append(lines, renderToolStatusLines(reploutput.FormatToolEnd(seg.toolCall), sh.lastWidth)...)
				}
			}
		case segmentBash:
			lines = append(lines, sh.renderBashSegment(seg, 0)...)
		case segmentSubagent:
			if isHiddenToolFailure(seg.endToolCall) {
				continue
			}
			line := reploutput.FormatSubagentTool(seg.agent, seg.toolCall, seg.endToolCall, sh.workingDir)
			if line != "" {
				lines = append(lines, renderToolStatusLines(line, sh.lastWidth)...)
			}
		case segmentAssistant:
			lines = append(lines, sh.renderAssistantTranscriptLines(seg.content)...)
		case segmentReasoning:
			if !sh.showThinking {
				continue
			}
			lines = append(lines, sh.renderReasoningTranscriptLines(seg.content)...)
		case segmentPermission:
			if seg.permissionReq != nil {
				lines = append(lines, renderPermissionResolved(seg.permissionReq)...)
			}
		case segmentDiff:
			lines = append(lines, renderDiffSegment(seg, sh.lastWidth)...)
		case segmentAskUser:
			if seg.askUser != nil {
				width := sh.lastWidth
				if width <= 0 {
					width = defaultWidth
				}
				lines = append(lines, strings.Split(strings.Trim(renderAskUserCard(*seg.askUser, width), "\n"), "\n")...)
			}
		}
	}

	return lines
}

func consecutiveReadCalls(segments []streamSegment, startIndex int) ([]*llm.ToolCall, int) {
	if startIndex >= len(segments) || segments[startIndex].kind != segmentToolStart {
		return nil, startIndex
	}
	startCall := segments[startIndex].toolCall
	if startCall == nil || startCall.Name != "read_file" {
		return nil, startIndex
	}
	path, _ := startCall.Input["path"].(string)

	var endCalls []*llm.ToolCall
	endIndex := startIndex
	for i := startIndex; i+1 < len(segments); i += 2 {
		start := segments[i]
		end := segments[i+1]
		if start.kind != segmentToolStart || start.toolCall == nil || start.toolCall.Name != "read_file" ||
			end.kind != segmentToolEnd || end.toolCall == nil || end.toolCall.Name != "read_file" || end.toolCall.Error != "" {
			break
		}
		readPath, _ := start.toolCall.Input["path"].(string)
		if readPath != path {
			break
		}
		endCalls = append(endCalls, end.toolCall)
		endIndex = i + 1
	}
	return endCalls, endIndex
}

func (sh *StreamHandler) shouldHideToolStart(index int) bool {
	return index+1 < len(sh.segments) && sh.segments[index+1].kind == segmentToolEnd && isHiddenToolFailure(sh.segments[index+1].toolCall)
}

func isHiddenToolFailure(toolCall *llm.ToolCall) bool {
	if toolCall == nil {
		return false
	}
	if toolCall.Name == "read_file" {
		return strings.HasPrefix(toolCall.Error, "not found: file ")
	}
	if toolCall.Name != "edit_file" {
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
	wrapWidth := width - 4
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
	wrapWidth := width - 4
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
	wrapWidth := sh.lastWidth - 4
	if wrapWidth < 1 {
		wrapWidth = 120
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

func (sh *StreamHandler) renderBashSegment(seg *streamSegment, width int) []string {
	ruleWidth := defaultWidth
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
		lines = append(lines, wrapAndIndent(repltheme.BashCommandStyle.Render("$ "+seg.command), width-4)...)
	} else {
		lines = append(lines, repltheme.BashCommandStyle.Render("  $ "+seg.command))
	}

	if seg.summary != "" {
		lines = append(lines, repltheme.BashSummaryStyle.Render("  › "+seg.summary))
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
				lines = append(lines, wrapAndIndent(repltheme.BashOutputStyle.Render(line), width-4)...)
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

func renderDiffLines(dl tools.EditDiffLine, width int) []string {
	switch dl.Kind {
	case tools.DiffLineHunk:
		return renderWrappedDiffLine("  ", dl.Content, repltheme.DiffHunkStyle, width)
	case tools.DiffLineAdded:
		lineNum := fmt.Sprintf("%4d", dl.NewLineNum)
		prefix := repltheme.DiffLineNumStyle.Render("     "+lineNum) + " " + repltheme.DiffAddStyle.Render("+ ")
		return renderWrappedDiffLine(prefix, dl.Content, repltheme.DiffAddStyle, width)
	case tools.DiffLineRemoved:
		lineNum := fmt.Sprintf("%4d", dl.OldLineNum)
		prefix := repltheme.DiffLineNumStyle.Render(lineNum+"     ") + " " + repltheme.DiffRemoveStyle.Render("- ")
		return renderWrappedDiffLine(prefix, dl.Content, repltheme.DiffRemoveStyle, width)
	default:
		prefix := repltheme.DiffLineNumStyle.Render(fmt.Sprintf("%4d %4d", dl.OldLineNum, dl.NewLineNum)) + " " + repltheme.DiffContextStyle.Render("  ")
		return renderWrappedDiffLine(prefix, dl.Content, repltheme.DiffContextStyle, width)
	}
}

func renderDiffSegment(seg *streamSegment, width int) []string {
	if len(seg.diffLines) == 0 {
		return nil
	}

	rendered := make([]string, 0, len(seg.diffLines))
	for _, dl := range seg.diffLines {
		rendered = append(rendered, renderDiffLines(dl, width)...)
	}

	ruleWidth := defaultWidth - 2 - diffRightPadding
	if width > 0 {
		ruleWidth = width - 2 - diffRightPadding
	}
	if ruleWidth < 1 {
		ruleWidth = 1
	}

	rule := "  " + repltheme.RuleStyle.Render(strings.Repeat("─", ruleWidth))
	lines := make([]string, 0, len(rendered)+3)
	lines = append(lines, "")
	lines = append(lines, rule)
	lines = append(lines, rendered...)
	lines = append(lines, rule)
	return lines
}

func renderBtwQuestionHeader(question string) string {
	chip := repltheme.BtwChipStyle.Render("btw")
	return chip + " " + repltheme.BtwLabelStyle.Render(question)
}

func renderBtwLeftBorder(line string) string {
	border := repltheme.BtwBorderStyle.Render("▌")
	return border + " " + line
}

func (m *replModel) renderBtwInline(width int) string {
	contentWidth := width - 4
	if contentWidth < 1 {
		contentWidth = 1
	}

	var view strings.Builder
	view.WriteString("\n\n")

	header := renderBtwQuestionHeader(m.btw.question)
	view.WriteString(renderBtwLeftBorder(header))
	view.WriteString("\n")

	streamView := strings.TrimLeft(m.btw.streamHandler.View(contentWidth), "\n")
	for _, line := range strings.Split(streamView, "\n") {
		view.WriteString(renderBtwLeftBorder(line))
		view.WriteString("\n")
	}

	if m.btw.showSpinner {
		view.WriteString(renderBtwLeftBorder(m.btw.spinner.View()))
		view.WriteString("\n")
	}

	return view.String()
}

func (m *replModel) renderBtwInlineFinished(width int) string {
	var view strings.Builder
	view.WriteString("\n")

	header := renderBtwQuestionHeader(m.btw.question)
	view.WriteString(renderBtwLeftBorder(header))
	view.WriteString("\n")

	for _, line := range m.btw.lines {
		view.WriteString(renderBtwLeftBorder(line))
		view.WriteString("\n")
	}

	return view.String()
}

func renderAdversaryHeader(focus string) string {
	chip := repltheme.AdversaryChipStyle.Render("adversary")
	if focus == "" {
		return chip
	}
	return chip + " " + repltheme.AdversaryLabelStyle.Render(focus)
}

func renderAdversaryLeftBorder(line string) string {
	border := repltheme.AdversaryBorderStyle.Render("▌")
	return border + " " + line
}

func (m *replModel) renderAdversaryInline(width int) string {
	contentWidth := max(width-4, 1)

	var view strings.Builder
	view.WriteString("\n\n")

	view.WriteString(renderAdversaryLeftBorder(renderAdversaryHeader(m.adversary.focus)))
	view.WriteString("\n")

	streamView := strings.TrimLeft(m.adversary.streamHandler.View(contentWidth), "\n")
	for _, line := range strings.Split(streamView, "\n") {
		view.WriteString(renderAdversaryLeftBorder(line))
		view.WriteString("\n")
	}

	if m.adversary.showSpinner {
		view.WriteString(renderAdversaryLeftBorder(m.adversary.spinner.View()))
		view.WriteString("\n")
	}

	return view.String()
}

func (m *replModel) renderAdversaryInlineFinished(width int) string {
	var view strings.Builder
	view.WriteString("\n")

	view.WriteString(renderAdversaryLeftBorder(renderAdversaryHeader(m.adversary.focus)))
	view.WriteString("\n")

	for _, line := range m.adversary.lines {
		view.WriteString(renderAdversaryLeftBorder(line))
		view.WriteString("\n")
	}

	return view.String()
}
