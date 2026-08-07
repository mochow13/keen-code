package repl

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/user/keen-code/internal/llm"
)

func TestUsagePercent(t *testing.T) {
	if got := usagePercent(1000, 2000); got != 50.0 {
		t.Fatalf("usagePercent(1000, 2000) = %f, want 50", got)
	}
	if got := usagePercent(2500, 2000); got != 100.0 {
		t.Fatalf("usagePercent should clamp to 100, got %f", got)
	}
	if got := usagePercent(100, 0); got != 0.0 {
		t.Fatalf("usagePercent with zero context window should be 0, got %f", got)
	}
}

func TestRenderContextStatusUnknownWindow(t *testing.T) {
	got := renderContextStatus(contextStatus{KnownWindow: false, KnownTokens: true, CurrentTokens: 42})
	if !strings.Contains(got, "N/A") {
		t.Fatalf("expected N/A for unknown window, got %q", got)
	}
}

func TestRenderContextStatusUnknownTokens(t *testing.T) {
	got := renderContextStatus(contextStatus{KnownWindow: true, ContextWindow: 100000, KnownTokens: false})
	if !strings.Contains(got, "0.0%") {
		t.Fatalf("expected 0.0%% for unknown tokens, got %q", got)
	}
}

func TestRenderContextStatusKnown(t *testing.T) {
	cases := []struct {
		name   string
		status contextStatus
		want   string
	}{
		{
			name: "includes percent",
			status: contextStatus{
				CurrentTokens: 1000,
				ContextWindow: 2000,
				Percent:       50.0,
				KnownWindow:   true,
				KnownTokens:   true,
			},
			want: "50%",
		},
		{
			name: "shows two decimals when needed",
			status: contextStatus{
				CurrentTokens: 1,
				ContextWindow: 3,
				Percent:       33.3333,
				KnownWindow:   true,
				KnownTokens:   true,
			},
			want: "33.33%",
		},
		{
			name: "with totals",
			status: contextStatus{
				CurrentTokens:     1000,
				ContextWindow:     2000,
				Percent:           50.0,
				KnownWindow:       true,
				KnownTokens:       true,
				TotalInputTokens:  1234,
				TotalOutputTokens: 567,
			},
			want: "1.2k ↑ / 567 ↓",
		},
		{
			name: "without totals",
			status: contextStatus{
				CurrentTokens: 1000,
				ContextWindow: 2000,
				Percent:       50.0,
				KnownWindow:   true,
				KnownTokens:   true,
			},
			want: "50%",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderContextStatus(c.status)
			if !strings.Contains(got, c.want) {
				t.Fatalf("expected %q in status, got %q", c.want, got)
			}
		})
	}
}

func TestContextStatus_ShouldSuggestCompaction(t *testing.T) {
	if !(contextStatus{KnownWindow: true, KnownTokens: true, Percent: 70}).ShouldSuggestCompaction() {
		t.Fatal("expected compaction suggestion at 70%")
	}
	if (contextStatus{KnownWindow: true, KnownTokens: true, Percent: 69.99}).ShouldSuggestCompaction() {
		t.Fatal("did not expect compaction suggestion below 70%")
	}
	if (contextStatus{KnownWindow: false, KnownTokens: true, Percent: 90}).ShouldSuggestCompaction() {
		t.Fatal("did not expect compaction suggestion when context window is unknown")
	}
	if (contextStatus{KnownWindow: true, KnownTokens: false, Percent: 90}).ShouldSuggestCompaction() {
		t.Fatal("did not expect compaction suggestion when tokens are unknown")
	}
}

func TestContextStatus_AddUsage(t *testing.T) {
	var s contextStatus
	s.AddUsage(&llm.TokenUsage{InputTokens: 100, OutputTokens: 50})
	if s.TotalInputTokens != 100 || s.TotalOutputTokens != 50 {
		t.Fatalf("expected totals 100/50, got %d/%d", s.TotalInputTokens, s.TotalOutputTokens)
	}
	s.AddUsage(&llm.TokenUsage{InputTokens: 200, OutputTokens: 100})
	if s.TotalInputTokens != 300 || s.TotalOutputTokens != 150 {
		t.Fatalf("expected totals 300/150, got %d/%d", s.TotalInputTokens, s.TotalOutputTokens)
	}
	s.AddUsage(nil)
	if s.TotalInputTokens != 300 || s.TotalOutputTokens != 150 {
		t.Fatal("expected no change on nil usage")
	}
}

func TestContextStatus_ResetTotals(t *testing.T) {
	var s contextStatus
	s.AddUsage(&llm.TokenUsage{InputTokens: 100, OutputTokens: 50})
	s.ResetTotals()
	if s.TotalInputTokens != 0 || s.TotalOutputTokens != 0 {
		t.Fatalf("expected totals reset to 0, got %d/%d", s.TotalInputTokens, s.TotalOutputTokens)
	}
}

func TestFormatCompactTokens(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1k"},
		{1234, "1.2k"},
		{12_345, "12.3k"},
		{999_999, "1M"},
		{1_000_000, "1M"},
		{1_500_000, "1.5M"},
		{2_000_000, "2M"},
		{3_412_000, "3.4M"},
	}
	for _, tt := range tests {
		if got := formatCompactTokens(tt.n); got != tt.want {
			t.Errorf("formatCompactTokens(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestHandleContextCommand_RendersBreakdown(t *testing.T) {
	m := newTestModel()
	m.appState.AppendMessage(llm.Message{Role: llm.RoleUser, Content: "hello world"})
	m.appState.AppendMessage(llm.Message{Role: llm.RoleAssistant, Content: "hi there"})
	m.appState.SetLastUsage(&llm.TokenUsage{InputTokens: 10000, OutputTokens: 100})

	_, _, handled := m.dispatchCommand("/context")
	if !handled {
		t.Fatal("expected /context to be handled")
	}

	out := ansi.Strip(m.output.Join())
	for _, want := range []string{"Context Usage", "System prompt", "Tool definitions", "User messages", "Assistant messages", "Tool results", "Total", "10k"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestHandleContextCommand_NoUsageShowsEstimateNote(t *testing.T) {
	m := newTestModel()
	m.appState.AppendMessage(llm.Message{Role: llm.RoleUser, Content: "hi"})

	m.handleContextCommand()

	out := ansi.Strip(m.output.Join())
	if !strings.Contains(out, "Estimated (chars/3 heuristic)") {
		t.Errorf("expected estimate note, got:\n%s", out)
	}
}

func TestHandleContextCommand_UnknownWindowOmitsFreeRow(t *testing.T) {
	m := newTestModel()
	m.appState.AppendMessage(llm.Message{Role: llm.RoleUser, Content: "hi"})
	m.appState.SetLastUsage(&llm.TokenUsage{InputTokens: 5000})

	m.handleContextCommand()

	out := ansi.Strip(m.output.Join())
	if strings.Contains(out, "Free") {
		t.Errorf("expected no Free row with unknown window, got:\n%s", out)
	}
}
