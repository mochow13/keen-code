package widgets

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	"github.com/mochow13/keen-code/internal/usage"
)

type UsageView struct {
	rangeIndex int
	summaries  []usage.Summary
}

func NewUsageView(summaries []usage.Summary) *UsageView {
	return &UsageView{summaries: summaries}
}

func (v *UsageView) RangeIndex() int {
	if v == nil {
		return 0
	}
	return v.rangeIndex
}

func (v *UsageView) NextRange() {
	if v == nil {
		return
	}
	v.rangeIndex = (v.rangeIndex + 1) % len(v.summaries)
}

func (v *UsageView) PrevRange() {
	if v == nil {
		return
	}
	v.rangeIndex = (v.rangeIndex + len(v.summaries) - 1) % len(v.summaries)
}

func (v *UsageView) CurrentSummary() usage.Summary {
	if v == nil {
		return usage.Summary{}
	}
	return v.summaries[v.rangeIndex]
}

func FormatUsageCard(view *UsageView, width int) string {
	if view == nil {
		return ""
	}
	ruleWidth := defaultCardWidth - 2
	if width > 0 {
		ruleWidth = width - 2
	}
	if ruleWidth < 1 {
		ruleWidth = 1
	}
	rule := "  " + repltheme.ModelSelectionRuleStyle.Render(strings.Repeat("─", ruleWidth))
	innerRule := repltheme.ModelSelectionRuleStyle.Render(strings.Repeat("─", ruleWidth))

	var body strings.Builder
	body.WriteString(repltheme.ModelSelectionTitleStyle.Render("Usage Data"))
	body.WriteString("\n\n")

	tabs := make([]string, 0, len(view.summaries))
	for i := range view.summaries {
		label := usage.Ranges[i].String()
		if i == view.rangeIndex {
			tabs = append(tabs, repltheme.ModelChipStyle.Render(label))
		} else {
			tabs = append(tabs, repltheme.ModelSelectionTextStyle.Render(label))
		}
	}
	body.WriteString(strings.Join(tabs, "  "))
	body.WriteString("\n")
	body.WriteString(innerRule)
	body.WriteString("\n")

	sum := view.CurrentSummary()
	if len(sum.Rows) == 0 {
		body.WriteString("\n")
		body.WriteString(repltheme.ModelSelectionTextStyle.Render("No usage recorded yet."))
		body.WriteString("\n")
	} else {
		body.WriteString("\n")
		body.WriteString(repltheme.ModelSelectionTitleStyle.Render("TOTAL USAGE"))
		body.WriteString("\n")
		body.WriteString(formatUsageMetricRows(sum.Total, ruleWidth-2))
		body.WriteString("\n")
		body.WriteString(innerRule)
		body.WriteString("\n\n")
		body.WriteString(repltheme.ModelSelectionTitleStyle.Render("BY MODEL"))
		body.WriteString("\n\n")
		body.WriteString(formatUsageModelRows(sum, ruleWidth-2))
		body.WriteString("\n")
	}

	body.WriteString("\n")
	body.WriteString(repltheme.ModelSelectionTextStyle.Render("←/→ change window   Esc close"))

	lines := strings.Split(strings.TrimRight(body.String(), "\n"), "\n")
	var out strings.Builder
	out.WriteString("\n")
	out.WriteString(rule)
	out.WriteString("\n")
	for _, line := range lines {
		if line == "" {
			out.WriteString("\n")
			continue
		}
		out.WriteString("  ")
		out.WriteString(line)
		out.WriteString("\n")
	}
	out.WriteString(rule)
	out.WriteString("\n")
	return out.String()
}

type usageRowCells struct {
	name                              string
	inputTokens, outputTokens         int
	cacheReadTokens, cacheWriteTokens int
}

func usageTableCells(sum usage.Summary) []usageRowCells {
	cells := make([]usageRowCells, 0, len(sum.Rows))
	for _, row := range sum.Rows {
		name := row.Model
		switch {
		case row.Provider == "":
		case name == "":
			name = row.Provider
		default:
			name = row.Provider + "/" + row.Model
		}
		cells = append(cells, usageRowCells{
			name:             name,
			inputTokens:      row.Input,
			outputTokens:     row.Output,
			cacheReadTokens:  row.CacheRead,
			cacheWriteTokens: row.CacheWrite,
		})
	}
	return cells
}

func formatUsageMetricRows(total usage.ModelUsage, width int) string {
	labels := [3]string{"Input", "Output", "Cache read"}
	values := [3]string{
		formatUsageTokens(total.Input),
		formatUsageTokens(total.Output),
		formatUsageCache(total.CacheRead),
	}
	return formatUsageTotalMetrics(labels, values, width)
}

func formatUsageModelRows(sum usage.Summary, width int) string {
	var body strings.Builder
	cells := usageTableCells(sum)
	maxValue := 0
	for _, row := range cells {
		for _, value := range []int{row.inputTokens, row.outputTokens, row.cacheReadTokens, row.cacheWriteTokens} {
			if value > maxValue {
				maxValue = value
			}
		}
	}

	for i, row := range cells {
		if i > 0 {
			body.WriteString("\n\n")
		}
		body.WriteString(repltheme.PrimaryBoldStyle.Render(truncateUsageName(row.name, width)))
		body.WriteString("\n")
		for _, metric := range usageBarMetrics(row) {
			body.WriteString(formatUsageModelChartRow(metric, maxValue, width))
		}
	}
	return body.String()
}

const usageBarLabelWidth = len("Cache Write")

type usageBarMetric struct {
	label string
	value int
	cache bool
	style lipgloss.Style
}

func (m usageBarMetric) valueText() string {
	if m.cache {
		return formatUsageCache(m.value)
	}
	return formatUsageTokens(m.value)
}

func usageBarMetrics(row usageRowCells) [4]usageBarMetric {
	return [4]usageBarMetric{
		{label: "Input", value: row.inputTokens, style: repltheme.UsageInputBarStyle},
		{label: "Output", value: row.outputTokens, style: repltheme.UsageOutputBarStyle},
		{label: "Cache Read", value: row.cacheReadTokens, cache: true, style: repltheme.UsageCacheReadBarStyle},
		{label: "Cache Write", value: row.cacheWriteTokens, cache: true, style: repltheme.UsageCacheWriteBarStyle},
	}
}

func formatUsageModelChartRow(metric usageBarMetric, maxValue, width int) string {
	if width < 1 {
		width = 1
	}
	valueText := metric.valueText()
	valueWidth := len(formatUsageTokens(maxValue))
	barWidth := width - usageBarLabelWidth - valueWidth - 2
	if barWidth > 32 {
		barWidth = 32
	}
	if barWidth < 1 {
		barWidth = 1
	}

	filled := 0
	if metric.value > 0 && maxValue > 0 {
		filled = int(float64(metric.value)/float64(maxValue)*float64(barWidth) + 0.5)
		if filled == 0 {
			filled = 1
		}
	}
	if filled > barWidth {
		filled = barWidth
	}
	bar := metric.style.Render(strings.Repeat("█", filled)) + repltheme.ModelSelectionTextStyle.Render(strings.Repeat("░", barWidth-filled))
	var line strings.Builder
	line.WriteString(repltheme.ModelSelectionTextStyle.Render(padUsageRight(metric.label, usageBarLabelWidth)))
	line.WriteString(" ")
	line.WriteString(bar)
	line.WriteString(" ")
	line.WriteString(repltheme.UsageMetricValueStyle.Render(valueText))
	line.WriteString("\n")
	return line.String()
}

func formatUsageTotalMetrics(labels, values [3]string, width int) string {
	if width < 1 {
		width = 1
	}
	if width < 40 {
		labels = [3]string{"In", "Out", "Read"}
	}
	columnWidth := (width - 4) / 3
	if columnWidth < 1 {
		columnWidth = 1
	}
	var body strings.Builder
	for i := range labels {
		plainCell := labels[i] + " " + values[i]
		cell := repltheme.ModelSelectionTextStyle.Render(labels[i]) + " " + repltheme.UsageMetricValueStyle.Render(values[i])
		body.WriteString(cell)
		if i < len(labels)-1 {
			if padding := columnWidth - len(plainCell); padding > 0 {
				body.WriteString(strings.Repeat(" ", padding))
			}
			body.WriteString("  ")
		}
	}
	body.WriteString("\n")
	return body.String()
}

func truncateUsageName(name string, width int) string {
	if len(name) <= width {
		return name
	}
	if width <= 1 {
		return name[:width]
	}
	return name[:width-1] + "…"
}

func padUsageRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func formatUsageCache(n int) string {
	if n <= 0 {
		return "-"
	}
	return formatUsageTokens(n)
}

func formatUsageTokens(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	if n < 1_000_000 {
		v := float64(n) / 1000.0
		if v >= 999.95 {
			return formatUsageFloat(v/1000.0) + "M"
		}
		return formatUsageFloat(v) + "k"
	}
	return formatUsageFloat(float64(n)/1_000_000.0) + "M"
}

func formatUsageFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', 1, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}
