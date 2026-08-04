package widgets

import (
	"fmt"
	"strings"

	repltheme "github.com/user/keen-code/internal/cli/repl/theme"
	"github.com/user/keen-code/internal/session"
)

type SessionPicker struct {
	summaries []session.Summary
	cursor    int
}

const (
	sessionPickerLinesPerItem = 3
	sessionPickerFixedLines   = 3
	defaultCardWidth          = 80
)

func NewSessionPicker(summaries []session.Summary) *SessionPicker {
	return &SessionPicker{summaries: summaries}
}

func (p *SessionPicker) Move(delta int) {
	if p == nil || len(p.summaries) == 0 {
		return
	}

	p.cursor += delta
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(p.summaries) {
		p.cursor = len(p.summaries) - 1
	}
}

func (p *SessionPicker) Current() *session.Summary {
	if p == nil || len(p.summaries) == 0 {
		return nil
	}
	return &p.summaries[p.cursor]
}

func (p *SessionPicker) visibleRange(maxItems int) (int, int) {
	if p == nil || len(p.summaries) == 0 {
		return 0, 0
	}
	if maxItems <= 0 || maxItems >= len(p.summaries) {
		return 0, len(p.summaries)
	}

	start := p.cursor - maxItems/2
	if start < 0 {
		start = 0
	}
	if start+maxItems > len(p.summaries) {
		start = len(p.summaries) - maxItems
	}
	return start, start + maxItems
}

func FormatSessionPickerCard(picker *SessionPicker, width, maxHeight int) string {
	if picker == nil {
		return ""
	}

	ruleWidth := defaultCardWidth - 2
	if width > 0 {
		ruleWidth = width - 2
	}
	if ruleWidth < 1 {
		ruleWidth = 1
	}
	maxItems := len(picker.summaries)
	if maxHeight > 0 {
		availableBodyLines := maxHeight - 3
		if availableBodyLines <= sessionPickerFixedLines {
			maxItems = 1
		} else {
			maxItems = (availableBodyLines - sessionPickerFixedLines) / sessionPickerLinesPerItem
			if maxItems < 1 {
				maxItems = 1
			}
		}
	}
	start, end := picker.visibleRange(maxItems)

	rule := "  " + repltheme.ModelSelectionRuleStyle.Render(strings.Repeat("─", ruleWidth))
	var body strings.Builder
	body.WriteString(repltheme.ModelSelectionTitleStyle.Render("Saved Sessions"))
	body.WriteString("\n\n")

	for i := start; i < end; i++ {
		summary := picker.summaries[i]
		selected := i == picker.cursor

		preview := strings.TrimSpace(summary.LastUserMessage)
		if preview == "" {
			preview = "(no user message)"
		}
		if len(preview) > 72 {
			preview = preview[:69] + "..."
		}

		if selected {
			body.WriteString(repltheme.ModelSelectionCursorStyle.Render("▶ "))
			body.WriteString(repltheme.ModelSelectionSelectedTextStyle.Render(preview))
		} else {
			body.WriteString("  ")
			body.WriteString(repltheme.ModelSelectionTextStyle.Render(preview))
		}
		body.WriteString("\n")
		body.WriteString(repltheme.ModelSelectionTextStyle.Render(fmt.Sprintf(
			"    Created: %s   Updated: %s",
			summary.CreatedAt.Local().Format("2006-01-02 15:04"),
			summary.UpdatedAt.Local().Format("2006-01-02 15:04"),
		)))
		body.WriteString("\n\n")
	}

	body.WriteString(repltheme.ModelSelectionTextStyle.Render("[↑/↓ navigate  Enter to resume  Esc to cancel]"))

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
		out.WriteString("  " + line + "\n")
	}
	out.WriteString(rule)
	out.WriteString("\n")
	return out.String()
}
