package output

import (
	"charm.land/lipgloss/v2"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
)

func AddCompactionSuccessStatus(output *OutputBuilder, status string) {
	addCompactionStatus(output, "✓", status, repltheme.CompactionSuccessStyle)
}

func AddCompactionErrorStatus(output *OutputBuilder, status string) {
	addCompactionStatus(output, "✗", status, repltheme.CompactionErrorStyle)
}

func AddCompactionCancelledStatus(output *OutputBuilder, status string) {
	addCompactionStatus(output, "", status, repltheme.CompactionCancelledStyle)
}

func addCompactionStatus(output *OutputBuilder, icon, status string, style lipgloss.Style) {
	if output == nil || status == "" {
		return
	}

	line := "  "
	if icon != "" {
		line += icon + " "
	}
	line += status

	output.AddStyledLine(line, style)
	output.AddEmptyLine()
}
