package markdown

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRendererRenderPlainTextUsesTerminalForeground(t *testing.T) {
	renderer, err := New(80)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rendered := renderer.Render("plain assistant response")
	if hasForegroundColorEscape(rendered) {
		t.Fatalf("expected no foreground color escapes, got %q", rendered)
	}
}

func TestRendererRenderCodeBlockUsesSyntaxHighlighting(t *testing.T) {
	renderer, err := New(80)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rendered := renderer.Render("```go\nfunc main() {\n\tprintln(\"hello\")\n}\n```")
	if !hasForegroundColorEscape(rendered) {
		t.Fatalf("expected syntax foreground color escapes, got %q", rendered)
	}
}

func TestRendererRenderInlineCodeUsesColor(t *testing.T) {
	renderer, err := New(80)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rendered := renderer.Render("Use `keen --help` for details.")
	if !hasForegroundColorEscape(rendered) {
		t.Fatalf("expected inline code foreground color escapes, got %q", rendered)
	}
}

func TestRendererRenderHorizontalRuleUsesContentWidth(t *testing.T) {
	tests := []struct {
		name      string
		width     int
		wantWidth int
	}{
		{
			name:      "wide",
			width:     80,
			wantWidth: 72,
		},
		{
			name:      "narrow",
			width:     24,
			wantWidth: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer, err := New(tt.width)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			rendered := stripANSI(renderer.Render("before\n\n---\n\nafter"))
			rule := findLineContaining(rendered, "─")
			if rule == "" {
				t.Fatalf("expected horizontal rule, got %q", rendered)
			}

			trimmed := strings.TrimSpace(rule)
			if got := lipgloss.Width(trimmed); got != tt.wantWidth {
				t.Fatalf("expected rule width %d, got %d in %q", tt.wantWidth, got, rendered)
			}
			if strings.Trim(trimmed, "─") != "" {
				t.Fatalf("expected rule to contain only rule characters, got %q", trimmed)
			}
		})
	}
}

func TestRendererRenderTableUsesRulesAndColumns(t *testing.T) {
	renderer, err := New(80)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rendered := stripANSI(renderer.Render("| Name | Status |\n| --- | --- |\n| Build | Passing |"))
	if !strings.Contains(rendered, "│") {
		t.Fatalf("expected table column separators, got %q", rendered)
	}
	if !strings.Contains(rendered, "┼") {
		t.Fatalf("expected table row separator intersections, got %q", rendered)
	}
	for _, want := range []string{"┌", "┬", "┐", "├", "┤", "└", "┴", "┘"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected table outer border %q, got %q", want, rendered)
		}
	}
}

func TestRendererRenderTableAddsRulesBetweenBodyRows(t *testing.T) {
	renderer, err := New(80)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rendered := stripANSI(renderer.Render("| Name | Status |\n| --- | --- |\n| Build | Passing |\n| Test | Passing |\n| Race | Passing |"))
	lines := strings.Split(rendered, "\n")
	for _, row := range []string{"Build", "Test"} {
		index := findLineIndexContaining(lines, row)
		if index == -1 {
			t.Fatalf("expected row %q in %q", row, rendered)
		}
		if index+1 >= len(lines) || !isFramedTableSeparatorLine(lines[index+1]) {
			t.Fatalf("expected table rule after row %q, got %q in %q", row, lines[index+1], rendered)
		}
	}
}

func TestRendererRenderTableStaysWithinWidth(t *testing.T) {
	width := 40
	renderer, err := New(width)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rendered := stripANSI(renderer.Render("| Name | Description |\n| --- | --- |\n| Alpha | This description is deliberately long so it must wrap within the markdown viewport. |"))
	for _, line := range strings.Split(rendered, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("expected line width <= %d, got %d for %q in %q", width, got, line, rendered)
		}
	}
}

func TestRendererRenderTableDoesNotAddRulesWithinWrappedBodyRows(t *testing.T) {
	renderer, err := New(40)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rendered := stripANSI(renderer.Render("| Name | Description |\n| --- | --- |\n| Alpha | First segment continuation marker that wraps. |\n| Beta | Done |"))
	lines := strings.Split(rendered, "\n")
	alphaIndex := findLineIndexContaining(lines, "Alpha")
	continuationIndex := findLineIndexContaining(lines, "continuation")
	betaIndex := findLineIndexContaining(lines, "Beta")
	if alphaIndex == -1 || continuationIndex == -1 || betaIndex == -1 {
		t.Fatalf("expected wrapped rows in %q", rendered)
	}
	if alphaIndex >= continuationIndex || continuationIndex >= betaIndex {
		t.Fatalf("expected Alpha row to wrap before Beta row in %q", rendered)
	}
	for _, line := range lines[alphaIndex+1 : continuationIndex] {
		if isFramedTableSeparatorLine(line) {
			t.Fatalf("expected no table rule inside wrapped Alpha row, got %q in %q", line, rendered)
		}
	}
	if !isFramedTableSeparatorLine(lines[betaIndex-1]) {
		t.Fatalf("expected table rule before Beta row, got %q in %q", lines[betaIndex-1], rendered)
	}
}

func TestRendererRenderTableOuterBordersDoNotLeaveRightGap(t *testing.T) {
	renderer, err := New(80)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rendered := stripANSI(renderer.Render("| ID | Name | Score | Status |\n| --- | --- | --- | --- |\n| 1 | Alice | 87 | Active |\n| 2 | Bob | 92 | Active |\n| 3 | Charlie | 78 | Pending |\n| 4 | Diana | 95 | Active |\n| 5 | Edward | 64 | Inactive |\n| 6 | Fiona | 88 | Active |\n| 7 | George | 71 | Pending |\n| 8 | Hannah | 99 | Active |"))
	for _, line := range strings.Split(rendered, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "┌") && !strings.HasPrefix(trimmed, "├") && !strings.HasPrefix(trimmed, "└") {
			continue
		}
		if strings.Contains(trimmed, " ┐") || strings.Contains(trimmed, " ┤") || strings.Contains(trimmed, " ┘") {
			t.Fatalf("expected border line without right gap, got %q in %q", line, rendered)
		}
	}
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(value string) string {
	return ansiEscapePattern.ReplaceAllString(value, "")
}

func findLineContaining(value, needle string) string {
	for _, line := range strings.Split(value, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

func findLineIndexContaining(lines []string, needle string) int {
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

func isFramedTableSeparatorLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "├") &&
		strings.Contains(trimmed, "┼") &&
		strings.HasSuffix(trimmed, "┤")
}

func hasForegroundColorEscape(value string) bool {
	return strings.Contains(value, "\x1b[38;") ||
		strings.Contains(value, "\x1b[30m") ||
		strings.Contains(value, "\x1b[31m") ||
		strings.Contains(value, "\x1b[32m") ||
		strings.Contains(value, "\x1b[33m") ||
		strings.Contains(value, "\x1b[34m") ||
		strings.Contains(value, "\x1b[35m") ||
		strings.Contains(value, "\x1b[36m") ||
		strings.Contains(value, "\x1b[37m") ||
		strings.Contains(value, "\x1b[90m") ||
		strings.Contains(value, "\x1b[91m") ||
		strings.Contains(value, "\x1b[92m") ||
		strings.Contains(value, "\x1b[93m") ||
		strings.Contains(value, "\x1b[94m") ||
		strings.Contains(value, "\x1b[95m") ||
		strings.Contains(value, "\x1b[96m") ||
		strings.Contains(value, "\x1b[97m") ||
		strings.Contains(value, "\x1b[39m")
}

func TestRendererUpdateWidth(t *testing.T) {
	renderer, err := New(80)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	original := renderer.renderer
	if err := renderer.UpdateWidth(80); err != nil {
		t.Fatalf("UpdateWidth(same) error = %v", err)
	}
	if renderer.renderer != original {
		t.Fatal("same width rebuilt renderer")
	}
	if err := renderer.UpdateWidth(24); err != nil {
		t.Fatalf("UpdateWidth(new) error = %v", err)
	}
	if renderer.width != 24 || renderer.renderer == original {
		t.Fatal("new width did not rebuild renderer")
	}
}
