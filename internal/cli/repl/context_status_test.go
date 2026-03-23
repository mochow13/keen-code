package repl

import (
	"strings"
	"testing"

	"github.com/user/keen-code/internal/llm"
)

func TestEstimateTokensFromWordCount(t *testing.T) {
	tests := []struct {
		words int
		want  int
	}{
		{words: 0, want: 0},
		{words: 1, want: 1},
		{words: 3, want: 4},
		{words: 1000, want: 1333},
	}

	for _, tt := range tests {
		if got := estimateTokensFromWordCount(tt.words); got != tt.want {
			t.Fatalf("estimateTokensFromWordCount(%d) = %d, want %d", tt.words, got, tt.want)
		}
	}
}

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

func TestProgressFillWidth(t *testing.T) {
	if got := progressFillWidth(50.0, 20); got != 10 {
		t.Fatalf("progressFillWidth(50, 20) = %d, want 10", got)
	}
	if got := progressFillWidth(100.0, 20); got != 20 {
		t.Fatalf("progressFillWidth(100, 20) = %d, want 20", got)
	}
	if got := progressFillWidth(0.0, 20); got != 0 {
		t.Fatalf("progressFillWidth(0, 20) = %d, want 0", got)
	}
}

func TestBuildConversationForEstimation(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "hello world"},
		{Role: llm.RoleAssistant, Content: "response"},
	}
	got := buildConversationForEstimation("", messages, "partial")
	if !strings.Contains(got, "hello world") || !strings.Contains(got, "response") || !strings.Contains(got, "partial") {
		t.Fatalf("conversation text is missing expected parts: %q", got)
	}
}

func TestRenderContextStatusUnknown(t *testing.T) {
	got := renderContextStatus(contextStatus{KnownWindow: false, CurrentTokens: 42})
	if !strings.Contains(got, "N/A") {
		t.Fatalf("expected N/A for unknown context status, got %q", got)
	}
}

func TestRenderContextStatusKnownIncludesPercent(t *testing.T) {
	got := renderContextStatus(contextStatus{
		CurrentTokens: 1000,
		ContextWindow: 2000,
		Percent:       50.0,
		KnownWindow:   true,
	})
	if !strings.Contains(got, "50%") {
		t.Fatalf("expected percent in status, got %q", got)
	}
}

func TestRenderContextStatusKnownShowsTwoDecimalPlacesWhenNeeded(t *testing.T) {
	got := renderContextStatus(contextStatus{
		CurrentTokens: 1,
		ContextWindow: 3,
		Percent:       33.3333,
		KnownWindow:   true,
	})
	if !strings.Contains(got, "33.33%") {
		t.Fatalf("expected 33.33%% in status, got %q", got)
	}
}
