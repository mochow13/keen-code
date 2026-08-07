package repl

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	repltheme "github.com/user/keen-code/internal/cli/repl/theme"
	"github.com/user/keen-code/internal/llm"
)

const (
	compactionSuggestThreshold = 70.0
)

type contextStatus struct {
	CurrentTokens     int
	ContextWindow     int
	Percent           float64
	KnownWindow       bool
	KnownTokens       bool
	TotalInputTokens  int
	TotalOutputTokens int
}

func (s *contextStatus) AddUsage(usage *llm.TokenUsage) {
	if usage == nil {
		return
	}
	s.TotalInputTokens += usage.InputTokens
	s.TotalOutputTokens += usage.OutputTokens
}

func (s *contextStatus) ResetTotals() {
	s.TotalInputTokens = 0
	s.TotalOutputTokens = 0
}

func (s contextStatus) ShouldSuggestCompaction() bool {
	return s.KnownWindow && s.KnownTokens && s.Percent >= compactionSuggestThreshold
}

func usagePercent(currentTokens, contextWindow int) float64 {
	if currentTokens <= 0 || contextWindow <= 0 {
		return 0
	}
	percent := (float64(currentTokens) * 100.0) / float64(contextWindow)
	if percent > 100 {
		return 100
	}
	if percent < 0 {
		return 0
	}
	return percent
}

func (m replModel) computeContextStatus() contextStatus {
	var providerID, modelID string
	if m.ctx != nil && m.ctx.cfg != nil {
		providerID = m.ctx.cfg.Provider
		modelID = m.ctx.cfg.Model
	}

	var contextWindow int
	var knownWindow bool
	if m.ctx != nil && m.ctx.registry != nil && providerID != "" && modelID != "" {
		contextWindow, knownWindow = m.ctx.registry.GetModelContextWindow(providerID, modelID)
	}

	var currentTokens int
	var knownTokens bool
	if m.appState != nil {
		if usage := m.appState.GetLastUsage(); usage != nil {
			currentTokens = usage.InputTokens
			knownTokens = true
		}
	}

	status := contextStatus{
		CurrentTokens:     currentTokens,
		ContextWindow:     contextWindow,
		KnownWindow:       knownWindow,
		KnownTokens:       knownTokens,
		TotalInputTokens:  m.contextStatus.TotalInputTokens,
		TotalOutputTokens: m.contextStatus.TotalOutputTokens,
	}
	if knownWindow && knownTokens {
		status.Percent = usagePercent(currentTokens, contextWindow)
	}
	return status
}

func (m *replModel) refreshContextStatus() {
	if m == nil {
		return
	}
	m.contextStatus = m.computeContextStatus()
}

func formatPercent(percent float64) string {
	p := strconv.FormatFloat(percent, 'f', 2, 64)
	p = strings.TrimRight(p, "0")
	p = strings.TrimRight(p, ".")
	return p + "%"
}

func contextPercentStyle(percent float64) lipgloss.Style {
	if percent >= 95 {
		return repltheme.ContextStatusPercentCriticalStyle
	}
	if percent >= 80 {
		return repltheme.ContextStatusPercentWarnStyle
	}
	return repltheme.ContextStatusPercentStyle
}

func formatCompactTokens(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	if n < 1_000_000 {
		v := float64(n) / 1000.0
		if v >= 999.95 {
			return formatCompactFloat(v/1000.0) + "M"
		}
		return formatCompactFloat(v) + "k"
	}
	return formatCompactFloat(float64(n)/1_000_000.0) + "M"
}

func formatCompactFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', 1, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

func renderContextStatus(status contextStatus) string {
	if !status.KnownWindow || status.ContextWindow <= 0 {
		return repltheme.ContextStatusUnknownStyle.Render("N/A")
	}
	if !status.KnownTokens {
		return repltheme.MetaLabelStyle.Render("context:") + " " + repltheme.ContextStatusPercentStyle.Render("0.0%") + repltheme.MetaLabelStyle.Render(" • ") + repltheme.MetaLabelStyle.Render("0 ↑ / 0 ↓")
	}

	percent := contextPercentStyle(status.Percent).Render(formatPercent(status.Percent))
	result := repltheme.MetaLabelStyle.Render("context:") + " " + percent

	if status.TotalInputTokens > 0 || status.TotalOutputTokens > 0 {
		tokensText := formatCompactTokens(status.TotalInputTokens) + " ↑ / " + formatCompactTokens(status.TotalOutputTokens) + " ↓"
		result += repltheme.MetaLabelStyle.Render(" • ") + repltheme.MetaLabelStyle.Render(tokensText)
	}

	return result
}

func (m *replModel) handleContextCommand() {
	breakdown := m.appState.GetContextBreakdown()
	status := m.computeContextStatus()

	m.output.AddStyledLine("  "+repltheme.TitleStyle.Render("Context Usage"), lipgloss.NewStyle())
	m.output.AddEmptyLine()

	window := status.ContextWindow
	knownWindow := status.KnownWindow && window > 0

	percentOf := func(tokens int) string {
		if !knownWindow {
			return "-"
		}
		return formatPercent(usagePercent(tokens, window))
	}

	rows := [][]string{
		{"System prompt", formatCompactTokens(breakdown.SystemPromptTokens), percentOf(breakdown.SystemPromptTokens)},
		{"Tool definitions (" + strconv.Itoa(breakdown.ToolDefinitionCount) + ")", formatCompactTokens(breakdown.ToolDefTokens), percentOf(breakdown.ToolDefTokens)},
		{"User messages", formatCompactTokens(breakdown.UserMessageTokens), percentOf(breakdown.UserMessageTokens)},
		{"Assistant messages", formatCompactTokens(breakdown.AssistantTokens), percentOf(breakdown.AssistantTokens)},
		{"Tool results", formatCompactTokens(breakdown.ToolResultTokens), percentOf(breakdown.ToolResultTokens)},
		{"Total", formatCompactTokens(breakdown.TotalEstimated), percentOf(breakdown.TotalEstimated)},
	}
	if knownWindow {
		free := max(0, window-breakdown.TotalEstimated)
		rows = append(rows, []string{"Free", formatCompactTokens(free), formatPercent(usagePercent(free, window))})
	}

	m.addCommandTable([]string{"Category", "Tokens", "% of window"}, rows, func(row, col int, style lipgloss.Style) lipgloss.Style {
		if row == table.HeaderRow {
			return style
		}
		if row < 0 || row >= len(rows) {
			return style
		}
		isTotalRow := rows[row][0] == "Total"
		if col == 0 {
			if isTotalRow {
				style = style.Inherit(repltheme.PrimaryBoldStyle)
			}
			return style
		}
		if col == 1 {
			style = style.Inherit(repltheme.MetaLabelStyle)
			if isTotalRow {
				style = style.Bold(true)
			}
			return style
		}
		if isTotalRow && knownWindow {
			style = style.Inherit(contextPercentStyle(usagePercent(breakdown.TotalEstimated, window))).Bold(true)
		}
		return style
	})

	if status.KnownTokens && breakdown.TotalEstimated == status.CurrentTokens {
		m.output.AddEmptyLine()
		m.output.AddStyledLine("  Anchored to provider-reported input tokens; per-category splits are heuristic estimates.", repltheme.MutedStyle)
	} else {
		m.output.AddEmptyLine()
		m.output.AddStyledLine("  Estimated (chars/3 heuristic); no provider-reported usage yet.", repltheme.MutedStyle)
	}

	if status.ShouldSuggestCompaction() {
		m.output.AddEmptyLine()
		m.output.AddStyledLine("  ⚠ Context is nearly full — consider running /compact.", repltheme.ContextStatusPercentWarnStyle)
	}

	m.output.AddEmptyLine()
	m.updateViewportContent()
	m.viewport.GotoBottom()
}
