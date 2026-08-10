package widgets

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	"github.com/mochow13/keen-code/internal/session"
)

func TestSessionPickerVisibleRange(t *testing.T) {
	picker := NewSessionPicker(makeSessionSummaries(10))

	start, end := picker.visibleRange(3)
	if start != 0 || end != 3 {
		t.Fatalf("expected initial visible range 0..3, got %d..%d", start, end)
	}

	picker.cursor = 9
	start, end = picker.visibleRange(3)
	if start != 7 || end != 10 {
		t.Fatalf("expected trailing visible range 7..10, got %d..%d", start, end)
	}
}

func TestFormatSessionPickerCard_RespectsHeightBudget(t *testing.T) {
	picker := NewSessionPicker(makeSessionSummaries(10))

	card := FormatSessionPickerCard(picker, 80, 12)
	lines := strings.Split(strings.TrimRight(card, "\n"), "\n")
	if len(lines) > 12 {
		t.Fatalf("expected picker card to fit height budget, got %d lines", len(lines))
	}
	if !strings.Contains(card, "session 0") {
		t.Fatalf("expected top of list to be shown initially, got %q", card)
	}
	if strings.Contains(card, "session 9") {
		t.Fatalf("expected tall picker to window visible items, got %q", card)
	}
}

func TestFormatSessionPickerCard_KeepsSelectedItemVisible(t *testing.T) {
	picker := NewSessionPicker(makeSessionSummaries(10))
	picker.cursor = 9

	card := FormatSessionPickerCard(picker, 80, 12)
	if !strings.Contains(card, stylePrefix(repltheme.ModelSelectionCursorStyle)+"▶ ") || !strings.Contains(card, stylePrefix(repltheme.ModelSelectionSelectedTextStyle)+"session 9") {
		t.Fatalf("expected selected item to remain visible, got %q", card)
	}
}

func TestFormatSessionPickerCard_LongListFitsWidth(t *testing.T) {
	picker := NewSessionPicker(makeSessionSummaries(10))

	width := 80
	card := FormatSessionPickerCard(picker, width, 12)
	for _, line := range strings.Split(strings.TrimRight(card, "\n"), "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("line exceeds expected width heuristic (%d > %d): %q", w, width, line)
		}
	}
}

func TestFormatSessionPickerCard_UsesModelSelectionStyles(t *testing.T) {
	picker := NewSessionPicker(makeSessionSummaries(2))

	card := FormatSessionPickerCard(picker, 80, 12)
	if !strings.Contains(card, stylePrefix(repltheme.ModelSelectionTitleStyle)+"Saved Sessions") {
		t.Fatalf("expected bold, dim, faint title, got %q", card)
	}
	if !strings.Contains(card, stylePrefix(repltheme.ModelSelectionCursorStyle)+"▶ ") || !strings.Contains(card, stylePrefix(repltheme.ModelSelectionSelectedTextStyle)+"session 0") {
		t.Fatalf("expected suggestion-style cursor and selected session text, got %q", card)
	}
	if !strings.Contains(card, "  "+stylePrefix(repltheme.ModelSelectionTextStyle)+"session 1") {
		t.Fatalf("expected dim, faint unselected session text, got %q", card)
	}
	if !strings.Contains(card, stylePrefix(repltheme.ModelSelectionRuleStyle)+"─") {
		t.Fatalf("expected dim, faint rules, got %q", card)
	}
}

func TestFormatSessionPickerCard_UsesViewportWidthRules(t *testing.T) {
	picker := NewSessionPicker(makeSessionSummaries(3))

	card := FormatSessionPickerCard(picker, 24, 12)
	lines := strings.Split(strings.TrimRight(card, "\n"), "\n")
	nonEmpty := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmpty = append(nonEmpty, line)
		}
	}
	if len(nonEmpty) < 3 {
		t.Fatalf("expected ruled picker output, got %v", nonEmpty)
	}
	if !strings.Contains(nonEmpty[0], "─") || !strings.Contains(nonEmpty[len(nonEmpty)-1], "─") {
		t.Fatalf("expected top and bottom rules, got %q", card)
	}
	if ruleWidth := lipgloss.Width(nonEmpty[0]); ruleWidth != 24 {
		t.Fatalf("expected rules to match viewport width, got width %d", ruleWidth)
	}
}

func makeSessionSummaries(count int) []session.Summary {
	summaries := make([]session.Summary, 0, count)
	now := time.Now()
	for i := 0; i < count; i++ {
		summaries = append(summaries, session.Summary{
			ID:              fmt.Sprintf("session-%d", i),
			CreatedAt:       now.Add(-time.Duration(i) * time.Hour),
			UpdatedAt:       now.Add(-time.Duration(i) * time.Minute),
			LastUserMessage: fmt.Sprintf("session %d", i),
		})
	}
	return summaries
}
