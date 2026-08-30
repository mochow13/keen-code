package repl

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mochow13/keen-code/internal/llm"
)

func TestStreamRenderPlainAssistantAndReasoningBranches(t *testing.T) {
	handler := NewStreamHandler(nil)
	if lines := handler.renderAssistantViewLines("", 20); lines != nil {
		t.Fatalf("empty assistant view = %#v", lines)
	}
	if lines := handler.renderAssistantTranscriptLines(""); lines != nil {
		t.Fatalf("empty assistant transcript = %#v", lines)
	}
	if lines := handler.renderReasoningViewLines("", 20); lines != nil {
		t.Fatalf("empty reasoning view = %#v", lines)
	}
	if lines := handler.renderReasoningTranscriptLines(""); lines != nil {
		t.Fatalf("empty reasoning transcript = %#v", lines)
	}

	view := ansi.Strip(strings.Join(handler.renderAssistantViewLines("first\nsecond", 12), "\n"))
	if !strings.Contains(view, "first") || !strings.Contains(view, "second") {
		t.Fatalf("assistant view = %q", view)
	}
	transcript := strings.Join(handler.renderAssistantTranscriptLines("first\nsecond"), "\n")
	if transcript != "  first\n  second" {
		t.Fatalf("assistant transcript = %q", transcript)
	}
	handler.lastWidth = 0
	reasoning := ansi.Strip(strings.Join(handler.renderReasoningTranscriptLines("reasoning"), "\n"))
	if !strings.Contains(reasoning, "reasoning") {
		t.Fatalf("reasoning transcript = %q", reasoning)
	}
}

func TestRenderBashSegmentCoversWidthSummaryAndTruncation(t *testing.T) {
	output := make([]string, bashOutputMaxLines+2)
	for i := range output {
		output[i] = "line"
	}
	handler := NewStreamHandler(nil)
	segment := &streamSegment{kind: segmentBash, command: "go test ./...", summary: "testing", output: strings.Join(output, "\n")}
	lines := handler.renderBashSegment(segment, 20)
	got := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"$ go test", "testing", "2 more lines"} {
		if !strings.Contains(got, want) {
			t.Fatalf("bash render %q missing %q", got, want)
		}
	}

	withoutWidth := ansi.Strip(strings.Join(handler.renderBashSegment(&streamSegment{command: "pwd", output: "path"}, 0), "\n"))
	if !strings.Contains(withoutWidth, "$ pwd") || !strings.Contains(withoutWidth, "path") {
		t.Fatalf("unbounded bash render = %q", withoutWidth)
	}
}

func TestRenderViewAndTranscriptHandleStandaloneToolEnd(t *testing.T) {
	handler := NewStreamHandler(nil)
	handler.lastWidth = 40
	handler.showThinking = true
	handler.segments = []streamSegment{
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "read_file", Output: map[string]any{"total_lines": 2}}},
		{kind: segmentAssistant, content: "done"},
		{kind: segmentReasoning, content: "thought"},
	}
	view := ansi.Strip(strings.Join(handler.renderViewLines(40), "\n"))
	transcript := ansi.Strip(strings.Join(handler.renderTranscriptLines(), "\n"))
	for _, rendered := range []string{view, transcript} {
		for _, want := range []string{"Read", "done", "thought"} {
			if !strings.Contains(rendered, want) {
				t.Fatalf("rendered output %q missing %q", rendered, want)
			}
		}
	}
}

func TestRenderFoldsOnlyConsecutiveReadsOfSameFile(t *testing.T) {
	read := func(path string, lines, bytes int) []streamSegment {
		return []streamSegment{
			{kind: segmentToolStart, toolCall: &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": path}}},
			{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "read_file", Output: map[string]any{"lines_read": lines, "bytes_read": bytes}}},
		}
	}

	handler := NewStreamHandler(nil)
	handler.lastWidth = 120
	handler.segments = append(handler.segments, read("same.go", 10, 100)...)
	handler.segments = append(handler.segments, read("same.go", 5, 50)...)
	handler.segments = append(handler.segments, read("other.go", 2, 20)...)
	handler.segments = append(handler.segments, read("same.go", 1, 10)...)

	for _, rendered := range []string{
		ansi.Strip(strings.Join(handler.renderViewLines(120), "\n")),
		ansi.Strip(strings.Join(handler.renderTranscriptLines(), "\n")),
	} {
		if strings.Count(rendered, "same.go") != 2 {
			t.Fatalf("expected consecutive reads to fold without crossing other.go: %q", rendered)
		}
		for _, want := range []string{"2 chunks", "15 lines", "150 bytes", "other.go"} {
			if !strings.Contains(rendered, want) {
				t.Fatalf("rendered output %q missing %q", rendered, want)
			}
		}
	}
}

func TestConsecutiveReadCallsRequireSuccessfulAdjacentPairs(t *testing.T) {
	segments := []streamSegment{
		{kind: segmentToolStart, toolCall: &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "same.go"}}},
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "read_file"}},
		{kind: segmentAssistant, content: "between"},
		{kind: segmentToolStart, toolCall: &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "same.go"}}},
		{kind: segmentToolEnd, toolCall: &llm.ToolCall{Name: "read_file", Error: "failed"}},
	}

	calls, endIndex := consecutiveReadCalls(segments, 0)
	if len(calls) != 1 || endIndex != 1 {
		t.Fatalf("consecutiveReadCalls = %d calls ending at %d", len(calls), endIndex)
	}
	calls, _ = consecutiveReadCalls(segments, 3)
	if len(calls) != 0 {
		t.Fatalf("failed read should not fold: %d calls", len(calls))
	}
}

func TestRenderDiffBoundaryBranches(t *testing.T) {
	if lines := renderDiffSegment(&streamSegment{}, 20); lines != nil {
		t.Fatalf("empty diff segment = %#v", lines)
	}
	lines := renderWrappedDiffLine("prefix", "content", replthemeZeroStyle(), 0)
	if len(lines) != 1 || lines[0] != "prefixcontent" {
		t.Fatalf("unbounded wrapped diff = %#v", lines)
	}
}

func replthemeZeroStyle() lipgloss.Style {
	return lipgloss.NewStyle()
}
