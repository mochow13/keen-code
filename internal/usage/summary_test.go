package usage

import (
	"testing"
	"time"
)

func TestSummarizeOrderingAndTotal(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	records := []Record{
		{TS: now, Provider: "openai", Model: "gpt-5", Input: 100, Output: 50, CacheRead: 10},
		{TS: now, Provider: "anthropic", Model: "claude-sonnet-4-5", Input: 500, Output: 20, CacheRead: 300, CacheWrite: 40, Reasoning: 7},
		{TS: now, Provider: "openai", Model: "gpt-5", Input: 50, Output: 5},
		{TS: now, Provider: "gemini", Model: "gemini-2.5-pro", Input: 200, Output: 10},
	}
	sum := Summarize(records, time.Time{})
	if len(sum.Rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(sum.Rows))
	}
	if sum.Rows[0].Model != "claude-sonnet-4-5" || sum.Rows[0].Input != 500 {
		t.Fatalf("first row should be top input: %+v", sum.Rows[0])
	}
	if sum.Rows[1].Model != "gemini-2.5-pro" || sum.Rows[2].Model != "gpt-5" {
		t.Fatalf("wrong order: %+v", sum.Rows)
	}
	if sum.Rows[2].Input != 150 || sum.Rows[2].Output != 55 {
		t.Fatalf("gpt-5 should aggregate: %+v", sum.Rows[2])
	}
	if sum.Total.Input != 850 || sum.Total.Output != 85 || sum.Total.CacheRead != 310 ||
		sum.Total.CacheWrite != 40 || sum.Total.Reasoning != 7 {
		t.Fatalf("wrong total: %+v", sum.Total)
	}
}

func TestSummarizeWindowFiltering(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	records := []Record{
		{TS: now.Add(-time.Hour), Provider: "a", Model: "recent", Input: 100, Output: 10},
		{TS: now.Add(-10 * 24 * time.Hour), Provider: "a", Model: "mid", Input: 200, Output: 20},
		{TS: now.Add(-40 * 24 * time.Hour), Provider: "a", Model: "old", Input: 300, Output: 30},
		{Date: "2026-07-01", Provider: "a", Model: "rollup-old", Input: 400, Output: 40, Rollup: true},
	}
	last1 := Summarize(records, rangeSince(RangeLast1Day, now))
	if len(last1.Rows) != 1 || last1.Rows[0].Model != "recent" {
		t.Fatalf("1d window wrong: %+v", last1.Rows)
	}
	last7 := Summarize(records, rangeSince(RangeLast7Days, now))
	if len(last7.Rows) != 1 || last7.Rows[0].Model != "recent" {
		t.Fatalf("7d window wrong: %+v", last7.Rows)
	}
	last30 := Summarize(records, rangeSince(RangeLast30Days, now))
	if len(last30.Rows) != 2 {
		t.Fatalf("30d window wrong: %+v", last30.Rows)
	}
	all := Summarize(records, rangeSince(RangeAllTime, now))
	if len(all.Rows) != 4 || all.Total.Input != 1000 {
		t.Fatalf("all-time wrong: %+v", all)
	}
	lastYear := Summarize(records, rangeSince(RangeLast1Year, now))
	if len(lastYear.Rows) != 4 {
		t.Fatalf("1y window wrong: %+v", lastYear.Rows)
	}
}

func TestSummarizeBoundaryInclusive(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	since := rangeSince(RangeLast30Days, now)
	records := []Record{
		{TS: since, Provider: "a", Model: "edge", Input: 11, Output: 1},
		{TS: since.Add(-time.Nanosecond), Provider: "a", Model: "outside", Input: 22, Output: 2},
	}
	sum := Summarize(records, since)
	if len(sum.Rows) != 1 || sum.Rows[0].Model != "edge" {
		t.Fatalf("boundary should be inclusive: %+v", sum.Rows)
	}
}

func TestSummarizeRollupOutsideWindowExcluded(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	records := []Record{
		{Date: "2026-08-28", Provider: "a", Model: "m", Input: 50, Output: 5, Rollup: true},
		{TS: now, Provider: "a", Model: "m", Input: 10, Output: 1},
	}
	sum := Summarize(records, rangeSince(RangeLast30Days, now))
	if len(sum.Rows) != 1 || sum.Rows[0].Input != 10 {
		t.Fatalf("stale rollup must not contaminate 30d window: %+v", sum.Rows)
	}
	all := Summarize(records, time.Time{})
	if all.Total.Input != 60 {
		t.Fatalf("all-time must include rollups: %+v", all.Total)
	}
}

func TestRangeSince(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		r    Range
		want time.Time
	}{
		{RangeAllTime, time.Time{}},
		{RangeLast1Day, now.AddDate(0, 0, -1)},
		{RangeLast7Days, now.AddDate(0, 0, -7)},
		{RangeLast30Days, now.AddDate(0, 0, -30)},
		{RangeLast6Months, now.AddDate(0, -6, 0)},
		{RangeLast1Year, now.AddDate(-1, 0, 0)},
	}
	for _, tc := range cases {
		if got := tc.r.Since(now); !got.Equal(tc.want) {
			t.Errorf("%s Since = %v, want %v", tc.r, got, tc.want)
		}
		if got := rangeSince(tc.r, now); !got.Equal(tc.want) {
			t.Errorf("%s rangeSince = %v, want %v", tc.r, got, tc.want)
		}
	}
}

func TestRangesAndLabels(t *testing.T) {
	want := []string{"All time", "Last 1 day", "Last 7 days", "Last 30 days", "Last 6 months", "Last 1 year"}
	if len(Ranges) != len(want) {
		t.Fatalf("Ranges has %d entries, want %d", len(Ranges), len(want))
	}
	for i, r := range Ranges {
		if got := r.String(); got != want[i] {
			t.Errorf("Ranges[%d].String() = %q, want %q", i, got, want[i])
		}
	}
}
