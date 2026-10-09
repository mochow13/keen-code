package widgets

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	"github.com/mochow13/keen-code/internal/usage"
)

func usageTestSummaries() []usage.Summary {
	all := usage.Summary{
		Rows: []usage.ModelUsage{
			{Provider: "anthropic", Model: "claude-sonnet-4-5", Input: 1240000, Output: 342000, CacheRead: 980000, CacheWrite: 120000},
			{Provider: "openai", Model: "gpt-5", Input: 412000, Output: 88000, CacheRead: 210000},
			{Provider: "google", Model: "gemini-2.5-pro", Input: 34000, Output: 9000},
		},
		Total: usage.ModelUsage{Input: 1686000, Output: 439000, CacheRead: 1190000, CacheWrite: 120000},
	}
	day := usage.Summary{
		Rows: []usage.ModelUsage{
			{Provider: "anthropic", Model: "claude-sonnet-4-5", Input: 12000, Output: 3000, CacheRead: 8000, CacheWrite: 500},
		},
		Total: usage.ModelUsage{Input: 12000, Output: 3000, CacheRead: 8000, CacheWrite: 500},
	}
	week := usage.Summary{
		Rows: []usage.ModelUsage{
			{Provider: "anthropic", Model: "claude-sonnet-4-5", Input: 24000, Output: 6000},
		},
		Total: usage.ModelUsage{Input: 24000, Output: 6000},
	}
	empty := usage.Summary{}
	sixMonths := usage.Summary{
		Rows: []usage.ModelUsage{
			{Provider: "anthropic", Model: "claude-sonnet-4-5", Input: 6000, Output: 1500},
		},
		Total: usage.ModelUsage{Input: 6000, Output: 1500},
	}
	year := usage.Summary{
		Rows: []usage.ModelUsage{
			{Provider: "anthropic", Model: "claude-sonnet-4-5", Input: 3000, Output: 750},
		},
		Total: usage.ModelUsage{Input: 3000, Output: 750},
	}
	return []usage.Summary{all, day, week, empty, sixMonths, year}
}

func TestUsageViewRangeWraps(t *testing.T) {
	v := NewUsageView(usageTestSummaries())
	if got := v.CurrentSummary(); len(got.Rows) != 3 {
		t.Fatalf("expected all-time summary first, got %d rows", len(got.Rows))
	}
	for i, want := range []int{1, 1, 0, 1, 1} {
		v.NextRange()
		if got := v.CurrentSummary(); len(got.Rows) != want {
			t.Fatalf("NextRange #%d expected %d rows, got %d", i+1, want, len(got.Rows))
		}
	}
	v.NextRange()
	if got := v.CurrentSummary(); len(got.Rows) != 3 {
		t.Fatalf("expected wrap back to all-time, got %d rows", len(got.Rows))
	}
	v.PrevRange()
	if got := v.CurrentSummary(); len(got.Rows) != 1 {
		t.Fatalf("expected wrap back to last range via PrevRange, got %d rows", len(got.Rows))
	}
}

func TestFormatUsageCardTableAndTotal(t *testing.T) {
	v := NewUsageView(usageTestSummaries())
	card := FormatUsageCard(v, 80)
	for _, want := range []string{"Usage Data", "All time", "Last 7 days", "Last 30 days", "TOTAL USAGE", "BY MODEL", "claude-sonnet-4-5", "gpt-5", "←/→ change window", "Esc close"} {
		if !strings.Contains(card, want) {
			t.Fatalf("expected card to contain %q, got %q", want, card)
		}
	}
	for _, want := range []string{"1.2M", "342k", "980k", "120k"} {
		if !strings.Contains(card, want) {
			t.Fatalf("expected compact value %q in card, got %q", want, card)
		}
	}
	totalSection := strings.SplitN(card, "BY MODEL", 2)[0]
	if strings.Contains(totalSection, "Cache Write") || strings.Contains(totalSection, "120k") {
		t.Fatalf("cache write should only appear per model, not in total metrics: %q", totalSection)
	}
	modelSection := strings.SplitN(card, "BY MODEL", 2)[1]
	if !strings.Contains(modelSection, "Cache Write") || !strings.Contains(modelSection, "120k") {
		t.Fatalf("expected per-model cache write metrics, got %q", modelSection)
	}
	if !strings.Contains(card, stylePrefix(repltheme.ModelChipStyle)+"All time") {
		t.Fatalf("expected active tab bold/primary, got %q", card)
	}
	if !strings.Contains(card, stylePrefix(repltheme.UsageMetricValueStyle)+"1.2M") {
		t.Fatalf("expected metric values to use a consistent bold style, got %q", card)
	}
}

func TestFormatUsageCardZeroCacheRendersDash(t *testing.T) {
	v := NewUsageView(usageTestSummaries())
	card := FormatUsageCard(v, 80)
	lines := strings.Split(card, "\n")
	var geminiLine string
	for i, line := range lines {
		if strings.Contains(line, "gemini-2.5-pro") && i+4 < len(lines) {
			geminiLine = strings.Join(lines[i+1:i+5], "")
		}
	}
	if geminiLine == "" {
		t.Fatalf("expected gemini metrics, got %q", card)
	}
	if dashCount := strings.Count(geminiLine, "-"); dashCount < 2 {
		t.Fatalf("expected '-' for zero cache read/write, got %q", geminiLine)
	}
}

func TestFormatUsageCardShowsProviderAndModel(t *testing.T) {
	sums := []usage.Summary{{
		Rows: []usage.ModelUsage{
			{Provider: "anthropic", Model: "claude-sonnet-4-5", Input: 2000, Output: 10},
			{Provider: "anthropic", Model: "shared", Input: 1500, Output: 10},
			{Provider: "openai", Model: "shared", Input: 1000, Output: 10},
		},
		Total: usage.ModelUsage{Input: 4500, Output: 30},
	}, {}, {}, {}, {}, {}}
	card := FormatUsageCard(NewUsageView(sums), 80)
	for _, want := range []string{"anthropic/claude-sonnet-4-5", "anthropic/shared", "openai/shared"} {
		if !strings.Contains(card, want) {
			t.Fatalf("expected %q in card, got %q", want, card)
		}
	}
}

func TestFormatUsageCardEmptyState(t *testing.T) {
	v := NewUsageView(usageTestSummaries())
	v.NextRange()
	v.NextRange()
	v.NextRange()
	card := FormatUsageCard(v, 80)
	if !strings.Contains(card, "No usage recorded yet.") {
		t.Fatalf("expected empty state, got %q", card)
	}
	if !strings.Contains(card, stylePrefix(repltheme.ModelChipStyle)+"Last 30 days") {
		t.Fatalf("expected active month tab, got %q", card)
	}
}

func TestFormatUsageCardWindowSwap(t *testing.T) {
	v := NewUsageView(usageTestSummaries())
	v.NextRange()
	card := FormatUsageCard(v, 80)
	if !strings.Contains(card, "12k") || strings.Contains(card, "gpt-5") {
		t.Fatalf("expected week window values only, got %q", card)
	}
}

func TestFormatUsageCardNilAndWidth(t *testing.T) {
	if got := FormatUsageCard(nil, 80); got != "" {
		t.Fatalf("expected empty card for nil view, got %q", got)
	}
	v := NewUsageView(usageTestSummaries())
	card := FormatUsageCard(v, 80)
	if !strings.Contains(card, stylePrefix(repltheme.ModelSelectionRuleStyle)+"─") {
		t.Fatalf("expected card chrome rules, got %q", card)
	}
	if !strings.Contains(card, stylePrefix(repltheme.ModelSelectionTitleStyle)+"Usage Data") {
		t.Fatalf("expected card title chrome, got %q", card)
	}
}

func TestUsageModelChartBarsScaleToSelectedWindow(t *testing.T) {
	summary := usage.Summary{Rows: []usage.ModelUsage{
		{Provider: "test", Model: "model-a", Input: 100, Output: 50, CacheRead: 25, CacheWrite: 8},
		{Provider: "test", Model: "model-b", Input: 25, Output: 10, CacheRead: 0},
	}}
	chart := formatUsageModelRows(summary, 30, 0)
	for _, want := range []string{
		"model-a", "model-b", "Input", "Output", "Cache Read", "Cache Write",
		stylePrefix(repltheme.UsageInputBarStyle) + strings.Repeat("█", 14),
		stylePrefix(repltheme.UsageOutputBarStyle) + strings.Repeat("█", 7),
	} {
		if !strings.Contains(chart, want) {
			t.Errorf("expected model chart to contain %q, got %q", want, chart)
		}
	}
	if !strings.Contains(chart, stylePrefix(repltheme.UsageMetricValueStyle)+"8") {
		t.Fatalf("expected model cache write value to remain visible: %q", chart)
	}
	for _, tc := range []struct {
		metric string
		style  lipgloss.Style
	}{
		{"input", repltheme.UsageInputBarStyle},
		{"output", repltheme.UsageOutputBarStyle},
		{"cache read", repltheme.UsageCacheReadBarStyle},
		{"cache write", repltheme.UsageCacheWriteBarStyle},
	} {
		if !strings.Contains(chart, stylePrefix(tc.style)) {
			t.Fatalf("expected %s bar to use its own color style: %q", tc.metric, chart)
		}
	}
	if strings.Contains(chart, "Cache write 0") {
		t.Fatalf("expected zero cache write to use dash: %q", chart)
	}
}

func TestUsageModelChartUsesSelectedSummary(t *testing.T) {
	view := NewUsageView(usageTestSummaries())
	allTime := FormatUsageCard(view, 80)
	view.NextRange()
	week := FormatUsageCard(view, 80)

	if !strings.Contains(allTime, "1.2M") || !strings.Contains(week, "12k") {
		t.Fatalf("expected chart values to follow selected window; all-time=%q week=%q", allTime, week)
	}
	if !strings.Contains(allTime, "█") || !strings.Contains(week, "█") {
		t.Fatalf("expected model bar charts for both windows")
	}
}

func TestUsageViewPagination(t *testing.T) {
	for _, count := range []int{0, 1, 3, 4, 6, 7} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			summary := usage.Summary{Total: usage.ModelUsage{Input: 987654}}
			for i := 0; i < count; i++ {
				summary.Rows = append(summary.Rows, usage.ModelUsage{Model: fmt.Sprintf("model-%02d", i), Input: 100 + i})
			}
			view := NewUsageView([]usage.Summary{summary, {}})
			pages := max(1, (count+2)/3)
			if got := view.PageCount(); got != pages {
				t.Fatalf("PageCount() = %d, want %d", got, pages)
			}
			view.PrevPage()
			if view.PageIndex() != 0 {
				t.Fatal("previous page should stop at first page")
			}
			for page := 0; page < pages; page++ {
				if got := view.PageIndex(); got != page {
					t.Fatalf("PageIndex() = %d, want %d", got, page)
				}
				card := FormatUsageCard(view, 80)
				if count == 0 {
					if !strings.Contains(card, "No usage recorded yet.") {
						t.Fatal("expected empty state")
					}
				} else {
					for _, want := range []string{fmt.Sprintf("Page %d of %d", page+1, pages), "987.7k", "↑/↓ change page"} {
						if !strings.Contains(card, want) {
							t.Fatalf("expected %q in card, got %q", want, card)
						}
					}
				}
				for i, row := range summary.Rows {
					wantVisible := i >= page*3 && i < (page+1)*3
					if strings.Contains(card, row.Model) != wantVisible {
						t.Fatalf("page %d model %q visibility should be %v", page, row.Model, wantVisible)
					}
				}
				view.NextPage()
			}
			if view.PageIndex() != pages-1 {
				t.Fatal("next page should stop at last page")
			}
			for page := pages - 2; page >= 0; page-- {
				view.PrevPage()
				if view.PageIndex() != page {
					t.Fatalf("previous page = %d, want %d", view.PageIndex(), page)
				}
			}
		})
	}
}

func TestUsageViewRangeChangeResetsPage(t *testing.T) {
	rows := make([]usage.ModelUsage, 11)
	view := NewUsageView([]usage.Summary{{Rows: rows}, {Rows: rows}, {}})
	view.NextPage()
	view.NextRange()
	if view.PageIndex() != 0 {
		t.Fatal("next range should reset page")
	}
	view.NextPage()
	view.PrevRange()
	if view.PageIndex() != 0 {
		t.Fatal("previous range should reset page")
	}
	view.NextPage()
	view.PrevRange()
	if view.PageIndex() != 0 || len(view.CurrentSummary().Rows) != 0 {
		t.Fatal("switching to empty range should reset page")
	}
}

func TestUsageViewEmptySummaries(t *testing.T) {
	for _, view := range []*UsageView{nil, NewUsageView(nil)} {
		view.NextRange()
		view.PrevRange()
		view.NextPage()
		view.PrevPage()
		if view.PageIndex() != 0 || view.PageCount() != 1 || len(view.CurrentSummary().Rows) != 0 {
			t.Fatal("expected empty view at first page")
		}
	}
}

func TestUsageModelChartScaleIsStableAcrossPages(t *testing.T) {
	summary := usage.Summary{Rows: []usage.ModelUsage{
		{Model: "largest", Input: 100},
		{}, {},
		{Model: "smaller", Input: 50},
	}}
	chart := formatUsageModelRows(summary, 30, 1)
	if !strings.Contains(chart, "smaller") || strings.Contains(chart, "largest") {
		t.Fatalf("expected only second page models, got %q", chart)
	}
	if !strings.Contains(chart, stylePrefix(repltheme.UsageInputBarStyle)+strings.Repeat("█", 7)) || strings.Contains(chart, strings.Repeat("█", 14)) {
		t.Fatalf("expected bar scale to use full window maximum, got %q", chart)
	}
}
