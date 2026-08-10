package markdown

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
)

type Renderer struct {
	renderer *glamour.TermRenderer
	width    int
}

func wordWrapWidth(width int) int {
	wordWrap := width - 4
	if wordWrap < 1 {
		return 1
	}
	return wordWrap
}

func newGlamourRenderer(width int) (*glamour.TermRenderer, error) {
	wordWrap := wordWrapWidth(width)

	return glamour.NewTermRenderer(
		glamour.WithStyles(repltheme.MarkdownStyleConfig(wordWrap)),
		glamour.WithChromaFormatter("terminal256"),
		glamour.WithWordWrap(wordWrap),
		glamour.WithTableWrap(true),
		glamour.WithInlineTableLinks(true),
	)
}

func New(width int) (*Renderer, error) {
	renderer, err := newGlamourRenderer(width)
	if err != nil {
		return nil, err
	}

	return &Renderer{
		renderer: renderer,
		width:    width,
	}, nil
}

func (r *Renderer) Render(markdown string) string {
	if markdown == "" {
		return ""
	}

	tables := markdownTableBlocks(markdown)
	rendered, err := r.renderer.Render(markdown)
	if err != nil {
		return markdown
	}
	return makeURLsClickable(addTableOuterBorders(rendered, tables))
}

func (r *Renderer) UpdateWidth(width int) error {
	if r.width == width {
		return nil
	}

	renderer, err := newGlamourRenderer(width)
	if err != nil {
		return err
	}

	r.renderer = renderer
	r.width = width
	return nil
}

type markdownTableBlock struct {
	bodyRows [][]string
}

func addTableOuterBorders(rendered string, tables []markdownTableBlock) string {
	lines := strings.Split(rendered, "\n")
	out := make([]string, 0, len(lines))
	tableIndex := 0

	for i := 0; i < len(lines); {
		if !isTableLine(lines[i]) {
			out = append(out, lines[i])
			i++
			continue
		}

		start := i
		hasSeparator := false
		for i < len(lines) && isTableLine(lines[i]) {
			hasSeparator = hasSeparator || isTableSeparatorLine(lines[i])
			i++
		}

		block := lines[start:i]
		if hasSeparator {
			var table *markdownTableBlock
			if tableIndex < len(tables) {
				table = &tables[tableIndex]
				tableIndex++
			}
			out = append(out, renderTableBlockWithOuterBorders(block, table)...)
			continue
		}
		out = append(out, block...)
	}

	return strings.Join(out, "\n")
}

func renderTableBlockWithOuterBorders(lines []string, table *markdownTableBlock) []string {
	if len(lines) == 0 {
		return lines
	}

	indent := tableIndent(lines)
	bodies := make([]string, len(lines))
	width := 0
	separator := ""
	headerSeparatorIndex := -1
	for i, line := range lines {
		body := strings.TrimPrefix(line, indent)
		bodies[i] = body
		width = max(width, lipgloss.Width(body))
		if separator == "" && isTableSeparatorLine(line) {
			separator = body
			headerSeparatorIndex = i
		}
	}
	if separator == "" {
		separator = strings.Repeat("─", width)
	}
	separator = normalizeTableSeparator(separator, width)
	bodyRuleBreaks := tableBodyRuleBreaks(lines, separator, headerSeparatorIndex, table)

	framed := make([]string, 0, len(lines)+2)
	framed = append(framed, indent+"┌"+strings.ReplaceAll(separator, "┼", "┬")+"┐")
	bodyLine := 0
	for i, body := range bodies {
		if isTableSeparatorLine(lines[i]) {
			framed = append(framed, indent+"├"+normalizeTableSeparator(body, width)+"┤")
			continue
		}
		framed = append(framed, indent+"│"+padTableBody(body, width)+"│")
		if headerSeparatorIndex >= 0 && i > headerSeparatorIndex {
			bodyLine++
		}
		if bodyRuleBreaks[bodyLine] {
			framed = append(framed, indent+"├"+separator+"┤")
		}
	}
	framed = append(framed, indent+"└"+strings.ReplaceAll(separator, "┼", "┴")+"┘")
	return framed
}

func tableBodyRuleBreaks(lines []string, separator string, headerSeparatorIndex int, table *markdownTableBlock) map[int]bool {
	breaks := map[int]bool{}
	if table == nil || len(table.bodyRows) < 2 || headerSeparatorIndex < 0 {
		return breaks
	}

	bodyLineCount := 0
	for _, line := range lines[headerSeparatorIndex+1:] {
		if !isTableSeparatorLine(line) {
			bodyLineCount++
		}
	}

	heights := tableBodyRowHeights(table.bodyRows, separator, bodyLineCount)
	if len(heights) == 0 {
		return breaks
	}

	line := 0
	for i, height := range heights[:len(heights)-1] {
		line += height
		if line > 0 {
			breaks[line] = true
		}
		if i == len(heights)-2 && line >= bodyLineCount {
			delete(breaks, line)
		}
	}
	return breaks
}

func tableBodyRowHeights(rows [][]string, separator string, bodyLineCount int) []int {
	if bodyLineCount == len(rows) {
		heights := make([]int, len(rows))
		for i := range heights {
			heights[i] = 1
		}
		return heights
	}

	widths := tableColumnWidths(separator)
	if len(widths) == 0 {
		return nil
	}

	heights := make([]int, len(rows))
	total := 0
	for i, row := range rows {
		height := 1
		for col, cell := range row {
			if col >= len(widths) {
				break
			}
			contentWidth := widths[col] - 2
			if contentWidth < 1 {
				contentWidth = 1
			}
			height = max(height, lipgloss.Height(lipgloss.NewStyle().Width(contentWidth).Render(markdownCellText(cell))))
		}
		heights[i] = height
		total += height
	}
	if total != bodyLineCount {
		return nil
	}
	return heights
}

func tableColumnWidths(separator string) []int {
	parts := strings.Split(separator, "┼")
	widths := make([]int, 0, len(parts))
	for _, part := range parts {
		widths = append(widths, lipgloss.Width(part))
	}
	return widths
}

func markdownTableBlocks(markdown string) []markdownTableBlock {
	lines := strings.Split(markdown, "\n")
	tables := []markdownTableBlock{}

	for i := 0; i+1 < len(lines); {
		header := parseMarkdownTableRow(lines[i])
		if len(header) == 0 || !isMarkdownTableSeparator(lines[i+1]) {
			i++
			continue
		}

		i += 2
		bodyRows := [][]string{}
		for i < len(lines) {
			row := parseMarkdownTableRow(lines[i])
			if len(row) == 0 {
				break
			}
			bodyRows = append(bodyRows, row)
			i++
		}
		tables = append(tables, markdownTableBlock{bodyRows: bodyRows})
	}

	return tables
}

func isMarkdownTableSeparator(line string) bool {
	cells := parseMarkdownTableRow(line)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if strings.Trim(cell, ":-") != "" || !strings.Contains(cell, "---") {
			return false
		}
	}
	return true
}

func parseMarkdownTableRow(line string) []string {
	if !strings.Contains(line, "|") {
		return nil
	}
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "|") {
		line = line[1:]
	}
	if strings.HasSuffix(line, "|") {
		line = line[:len(line)-1]
	}

	cells := []string{}
	var cell strings.Builder
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			cell.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '|':
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
		default:
			cell.WriteRune(r)
		}
	}
	if escaped {
		cell.WriteRune('\\')
	}
	cells = append(cells, strings.TrimSpace(cell.String()))
	return cells
}

func markdownCellText(cell string) string {
	cell = strings.ReplaceAll(cell, "\\|", "|")
	cell = strings.ReplaceAll(cell, "`", "")
	cell = strings.ReplaceAll(cell, "**", "")
	cell = strings.ReplaceAll(cell, "__", "")
	cell = strings.ReplaceAll(cell, "*", "")
	cell = strings.ReplaceAll(cell, "_", "")
	return cell
}

func isTableLine(line string) bool {
	return strings.Contains(line, "│") || isTableSeparatorLine(line)
}

func isTableSeparatorLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.Contains(trimmed, "─") || !strings.Contains(trimmed, "┼") {
		return false
	}
	return strings.Trim(trimmed, "─┼") == ""
}

func tableIndent(lines []string) string {
	indent := ""
	found := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		current := leadingSpaces(line)
		if !found || len(current) < len(indent) {
			indent = current
			found = true
		}
	}
	return indent
}

func leadingSpaces(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " "))]
}

func padTableBody(value string, width int) string {
	if missing := width - lipgloss.Width(value); missing > 0 {
		return value + strings.Repeat(" ", missing)
	}
	return value
}

func normalizeTableSeparator(value string, width int) string {
	value = strings.ReplaceAll(value, " ", "─")
	if missing := width - lipgloss.Width(value); missing > 0 {
		return value + strings.Repeat("─", missing)
	}
	return value
}
