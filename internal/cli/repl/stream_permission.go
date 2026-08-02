package repl

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	replpermissions "github.com/user/keen-code/internal/cli/repl/permissions"
	repltheme "github.com/user/keen-code/internal/cli/repl/theme"
)

const permissionPreviewMaxLines = 120

func (sh *StreamHandler) HandlePermissionRequest(req *replpermissions.Request) {
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
		seg.permissionReq.Status == replpermissions.StatusPending
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
	choices := replpermissions.Choices(seg.permissionReq.IsDangerous)
	newCursor := seg.permissionCursor + delta
	if newCursor < 0 {
		newCursor = 0
	}
	if newCursor >= len(choices) {
		newCursor = len(choices) - 1
	}
	seg.permissionCursor = newCursor
}

func (sh *StreamHandler) GetPendingChoice() replpermissions.Choice {
	n := len(sh.segments)
	if n == 0 {
		return replpermissions.ChoiceDeny
	}
	seg := &sh.segments[n-1]
	if seg.kind != segmentPermission || seg.permissionReq == nil {
		return replpermissions.ChoiceDeny
	}
	return replpermissions.ChoiceAt(seg.permissionCursor, seg.permissionReq.IsDangerous)
}

func (sh *StreamHandler) GetPendingPermissionRequest() *replpermissions.Request {
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

func (sh *StreamHandler) ResolvePendingPermission(status replpermissions.Status) {
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

func renderPermissionCard(seg *streamSegment, width int) []string {
	req := seg.permissionReq
	if req == nil {
		return nil
	}

	if req.Status != replpermissions.StatusPending {
		return renderPermissionResolved(req)
	}

	cardWidth := width - 4
	if cardWidth < 20 {
		cardWidth = 20
	}
	cardStyle := repltheme.UserPromptCardStyle.MaxWidth(cardWidth)
	contentWidth := cardWidth - cardStyle.GetHorizontalFrameSize()
	if contentWidth < 1 {
		contentWidth = 1
	}

	labelWidth := lipgloss.Width(repltheme.InfoLabelStyle.Render("Resolved:"))
	if labelWidth < 1 {
		labelWidth = 1
	}
	valueWidth := contentWidth - labelWidth - 1
	if valueWidth < 1 {
		valueWidth = 1
	}

	var sb strings.Builder

	if req.IsDangerous {
		sb.WriteString(repltheme.WarningTitleStyle.Render("⚠  Allow Dangerous Command?"))
	} else {
		sb.WriteString(repltheme.UserPromptStyle.Render("Permission Required"))
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
		previewStyle := lipgloss.NewStyle().Foreground(repltheme.MutedColor)
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
			sb.WriteString(wrapTextWithStyle(fmt.Sprintf("... %d more preview lines omitted", total-permissionPreviewMaxLines), repltheme.HintStyle, contentWidth))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")

	choices := replpermissions.Choices(req.IsDangerous)
	for i, choice := range choices {
		if i == seg.permissionCursor {
			sb.WriteString(wrapTextWithStyle("▶ "+choice, repltheme.UserPromptSelectionStyle, contentWidth))
			sb.WriteString("\n")
		} else {
			sb.WriteString(wrapTextWithStyle("  "+choice, repltheme.NormalStyle, contentWidth))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(wrapTextWithStyle("[↑/↓ navigate  Enter confirm  Esc deny]", repltheme.HintStyle, contentWidth))

	boxed := cardStyle.Render(sb.String())
	rawLines := strings.Split(strings.TrimRight(boxed, "\n"), "\n")
	result := make([]string, 0, len(rawLines)+1)
	result = append(result, "")
	for _, l := range rawLines {
		result = append(result, "  "+l)
	}
	return result
}

func formatPermissionKeyValue(label, value string, labelWidth, valueWidth int) string {
	labelWidth = max(1, labelWidth)
	valueWidth = max(1, valueWidth)

	prefix := repltheme.InfoLabelStyle.Width(labelWidth).Render(label)
	continuation := strings.Repeat(" ", labelWidth+1)
	if value == "" {
		return prefix + " \n"
	}

	wrapped := wrapTextWithStyle(value, repltheme.InfoValueStyle, valueWidth)
	lines := strings.Split(strings.TrimRight(wrapped, "\n"), "\n")
	if len(lines) == 0 {
		return prefix + " \n"
	}

	var out strings.Builder
	out.WriteString(prefix)
	out.WriteString(" ")
	out.WriteString(lines[0])
	out.WriteString("\n")
	for _, line := range lines[1:] {
		out.WriteString(continuation)
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String()
}

func renderPermissionResolved(req *replpermissions.Request) []string {
	var line string
	switch req.Status {
	case replpermissions.StatusAllowed:
		line = "  " + repltheme.HighlightStyle.Render("✓ Permission granted for "+req.ToolName)
	case replpermissions.StatusAllowedSession:
		line = "  " + repltheme.HighlightStyle.Render("✓ Permission granted for "+req.ToolName+" (this session)")
	case replpermissions.StatusDenied:
		line = "  " + lipgloss.NewStyle().Foreground(repltheme.MutedColor).Render("✗ Permission denied for "+req.ToolName)
	case replpermissions.StatusRedirected:
		line = "  " + lipgloss.NewStyle().Foreground(repltheme.MutedColor).Render("↩ Redirected for "+req.ToolName)
	case replpermissions.StatusAutoAllowedSession:
		line = "  " + repltheme.HighlightStyle.Render("✓ Auto-approved for "+req.ToolName+" (session)")
	default:
		line = "  " + lipgloss.NewStyle().Foreground(repltheme.MutedColor).Render("✗ Permission cancelled for "+req.ToolName)
	}
	return []string{line}
}
