package stream

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/agentcore"
	reploutput "github.com/mochow13/keen-code/internal/cli/repl/output"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
	segment := &Segment{Kind: SegmentBash, Command: "go test ./...", Summary: "testing", Output: strings.Join(output, "\n")}
	lines := handler.renderBashSegment(segment, 20, false)
	got := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"$ go test", "testing", "2 earlier"} {
		if !strings.Contains(got, want) {
			t.Fatalf("bash render %q missing %q", got, want)
		}
	}

	withoutWidth := ansi.Strip(strings.Join(handler.renderBashSegment(&Segment{Command: "pwd", Output: "path"}, 0, false), "\n"))
	if !strings.Contains(withoutWidth, "$ pwd") || !strings.Contains(withoutWidth, "path") {
		t.Fatalf("unbounded bash render = %q", withoutWidth)
	}
}

func TestRenderViewAndTranscriptHandleStandaloneToolEnd(t *testing.T) {
	handler := NewStreamHandler(nil)
	handler.lastWidth = 40
	handler.showThinking = true
	handler.segments = []Segment{
		{Kind: SegmentToolEnd, ToolCall: &agentcore.ToolCall{Name: "read_file", Output: map[string]any{"total_lines": 2}}},
		{Kind: SegmentAssistant, Content: "done"},
		{Kind: SegmentReasoning, Content: "thought"},
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
	read := func(path string, lines, bytes int) []Segment {
		return []Segment{
			{Kind: SegmentToolStart, ToolCall: &agentcore.ToolCall{Name: "read_file", Input: map[string]any{"path": path}}},
			{Kind: SegmentToolEnd, ToolCall: &agentcore.ToolCall{Name: "read_file", Output: map[string]any{"lines_read": lines, "bytes_read": bytes}}},
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
	segments := []Segment{
		{Kind: SegmentToolStart, ToolCall: &agentcore.ToolCall{Name: "read_file", Input: map[string]any{"path": "same.go"}}},
		{Kind: SegmentToolEnd, ToolCall: &agentcore.ToolCall{Name: "read_file"}},
		{Kind: SegmentAssistant, Content: "between"},
		{Kind: SegmentToolStart, ToolCall: &agentcore.ToolCall{Name: "read_file", Input: map[string]any{"path": "same.go"}}},
		{Kind: SegmentToolEnd, ToolCall: &agentcore.ToolCall{Name: "read_file", Error: "failed"}},
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
	if lines := renderDiffSegment(&Segment{}, 20); lines != nil {
		t.Fatalf("empty diff segment = %#v", lines)
	}
	contextOnly := &Segment{DiffLines: []agentcore.EditDiffLine{{Kind: agentcore.EditDiffLineContext, Content: "same", OldLineNum: 1, NewLineNum: 1}}}
	if lines := renderDiffSegment(contextOnly, 20); lines != nil {
		t.Fatalf("context-only diff segment = %#v", lines)
	}
	unbounded := &Segment{DiffLines: []agentcore.EditDiffLine{{Kind: agentcore.EditDiffLineAdded, Content: "content", NewLineNum: 1}}}
	lines := renderDiffSegment(unbounded, 0)
	if len(lines) != 4 || lipgloss.Width(lines[2]) != DefaultWidth-diffRightPadding {
		t.Fatalf("unbounded diff should fall back to default width, got %#v", lines)
	}
}

func TestRenderDiffSegment_TintsChangedRowsToFullWidth(t *testing.T) {
	seg := &Segment{DiffLines: []agentcore.EditDiffLine{
		{Kind: agentcore.EditDiffLineHunk, Content: "@@ -1,2 +1,2 @@"},
		{Kind: agentcore.EditDiffLineContext, Content: "keep", OldLineNum: 1, NewLineNum: 1},
		{Kind: agentcore.EditDiffLineRemoved, Content: "old", OldLineNum: 2},
		{Kind: agentcore.EditDiffLineAdded, Content: "\tnew", NewLineNum: 2},
		{Kind: agentcore.EditDiffLineHunk, Content: "@@ -40,1 +40,1 @@"},
		{Kind: agentcore.EditDiffLineAdded, Content: "later", NewLineNum: 40},
	}}
	lines := renderDiffSegment(seg, 40)
	plain := make([]string, len(lines))
	for i, line := range lines {
		plain[i] = ansi.Strip(line)
	}
	want := []string{
		"",
		"  " + strings.Repeat("─", 36),
		"     1    1   keep",
		"     2      - old" + strings.Repeat(" ", 21),
		"          2 +     new" + strings.Repeat(" ", 17),
		"     ⋮",
		"         40 + later" + strings.Repeat(" ", 19),
		"  " + strings.Repeat("─", 36),
	}
	if !reflect.DeepEqual(plain, want) {
		t.Fatalf("diff rows = %q, want %q", plain, want)
	}
	for _, i := range []int{3, 4, 6} {
		if !strings.Contains(lines[i], "\x1b[48;") {
			t.Fatalf("changed row %d should carry a background color, got %q", i, lines[i])
		}
	}
	if strings.Contains(lines[2], "\x1b[48;") {
		t.Fatalf("context row should not be tinted, got %q", lines[2])
	}
}

func TestRenderDiffSegment_FollowsTheme(t *testing.T) {
	prev := repltheme.CurrentThemeName
	t.Cleanup(func() { repltheme.ApplyTheme(prev) })
	seg := &Segment{DiffLines: []agentcore.EditDiffLine{
		{Kind: agentcore.EditDiffLineRemoved, Content: "old", OldLineNum: 1},
		{Kind: agentcore.EditDiffLineAdded, Content: "new", NewLineNum: 1},
	}}

	repltheme.ApplyTheme("default")
	before := renderDiffSegment(seg, 40)
	repltheme.ApplyTheme("dracula")
	after := renderDiffSegment(seg, 40)
	for _, i := range []int{2, 3} {
		if before[i] == after[i] {
			t.Fatalf("diff row %d did not change with theme: %q", i, after[i])
		}
	}
}

func TestRenderDiffSegment_RendersRulesUsingViewportWidth(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")
	sh.HandleDiff([]agentcore.EditDiffLine{
		{Kind: agentcore.EditDiffLineHunk, Content: "@@ -1,2 +1,3 @@"},
		{Kind: agentcore.EditDiffLineContext, Content: "unchanged context", OldLineNum: 1, NewLineNum: 1},
		{Kind: agentcore.EditDiffLineRemoved, Content: strings.Repeat("short ", 6), OldLineNum: 2},
		{Kind: agentcore.EditDiffLineAdded, Content: strings.Repeat("shorter ", 6), NewLineNum: 2},
	})

	wideView := sh.View(80)
	wideLines := strings.Split(strings.TrimRight(wideView, "\n"), "\n")
	if len(wideLines) < 4 {
		t.Fatalf("expected ruled diff lines, got %v", wideLines)
	}

	nonEmpty := make([]string, 0, len(wideLines))
	for _, line := range wideLines {
		if strings.TrimSpace(line) != "" {
			nonEmpty = append(nonEmpty, line)
		}
	}
	if len(nonEmpty) < 4 {
		t.Fatalf("expected non-empty diff lines, got %v", nonEmpty)
	}
	plainWideView := ansi.Strip(wideView)
	if strings.Contains(plainWideView, "@@ -1,2 +1,3 @@") {
		t.Fatalf("diff view should omit hunk headers, got %q", plainWideView)
	}
	if !strings.Contains(plainWideView, "unchanged context") {
		t.Fatalf("diff view should show context lines, got %q", plainWideView)
	}
	for _, shown := range []string{"short ", "shorter "} {
		if !strings.Contains(plainWideView, shown) {
			t.Fatalf("diff view should retain changed line %q, got %q", shown, plainWideView)
		}
	}
	if !strings.Contains(nonEmpty[0], "─") || !strings.Contains(nonEmpty[len(nonEmpty)-1], "─") {
		t.Fatalf("expected top and bottom diff rules, got %q", wideView)
	}
	wideRuleWidth := lipgloss.Width(nonEmpty[0])
	if wideRuleWidth != 78 {
		t.Fatalf("expected diff rules to leave right padding, got width %d", wideRuleWidth)
	}

	narrowView := sh.View(24)
	narrowLines := strings.Split(strings.TrimRight(narrowView, "\n"), "\n")
	for _, line := range narrowLines {
		if w := lipgloss.Width(line); w > 24 {
			t.Fatalf("line exceeds viewport width (%d > %d): %q", w, 24, line)
		}
	}
	narrowNonEmpty := make([]string, 0, len(narrowLines))
	for _, line := range narrowLines {
		if strings.TrimSpace(line) != "" {
			narrowNonEmpty = append(narrowNonEmpty, line)
		}
	}
	if len(narrowNonEmpty) <= len(nonEmpty) {
		t.Fatalf("expected narrow diff view to wrap long lines, got wide=%d narrow=%d", len(nonEmpty), len(narrowNonEmpty))
	}
	if len(narrowNonEmpty) < 4 {
		t.Fatalf("expected non-empty narrow diff lines, got %v", narrowNonEmpty)
	}
	if narrowRuleWidth := lipgloss.Width(narrowNonEmpty[0]); narrowRuleWidth != 22 {
		t.Fatalf("expected narrow diff rules to leave right padding, got width %d", narrowRuleWidth)
	}
	for _, line := range narrowNonEmpty {
		if w := lipgloss.Width(line); w > 22 {
			t.Fatalf("expected non-empty diff line to leave right padding (%d > %d): %q", w, 22, line)
		}
	}
}

func TestCompletedTranscriptReflows(t *testing.T) {
	tests := []struct {
		name string
		seed func(handler *StreamHandler)
	}{
		{
			name: "bash",
			seed: func(handler *StreamHandler) {
				handler.HandleBashStart("printf a long command with multiple arguments", "running a long command")
				handler.HandleBashEnd(&agentcore.ToolCall{Output: map[string]any{"stdout": "some long output with multiple words for wrapping"}})
			},
		},
		{
			name: "assistant and tool",
			seed: func(handler *StreamHandler) {
				handler.HandleChunk("A completed assistant response with several words to wrap correctly.")
				handler.HandleToolStart(&agentcore.ToolCall{Name: agentcore.ToolNameGrep, Input: map[string]any{"pattern": "a pattern with several words to search for"}})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewStreamHandler(nil)
			test.seed(handler)
			segments := handler.Snapshot()
			handler.HandleDone()
			output := reploutput.NewOutputBuilder(70, "")
			handler.AppendTranscript(output, segments)
			wide := output.Join()
			output.SetWidth(25)
			narrow := output.Join()
			if wide == narrow {
				t.Fatal("completed transcript did not reflow")
			}
			for _, line := range strings.Split(narrow, "\n") {
				if ansi.StringWidth(line) > 25 {
					t.Fatalf("line exceeds width: %q", line)
				}
			}
			output.SetWidth(70)
			if output.Join() != wide {
				t.Fatal("completed transcript did not restore layout")
			}
		})
	}
}

func TestRenderBashSegmentShowsOutputTail(t *testing.T) {
	output := make([]string, 30)
	for i := range output {
		output[i] = fmt.Sprintf("line %02d", i)
	}
	handler := NewStreamHandler(nil)
	handler.HandleBashStart("go test ./...", "")
	handler.HandleBashEnd(&agentcore.ToolCall{Output: map[string]any{"stdout": strings.Join(output, "\n") + "\n", "exit_code": 0}})

	got := ansi.Strip(strings.Join(handler.renderBashSegment(&handler.segments[0], 80, false), "\n"))
	if !strings.Contains(got, "… 14 earlier lines") || !strings.Contains(got, "line 29") || strings.Contains(got, "line 13") {
		t.Fatalf("expected last %d lines with earlier-lines marker, got %q", bashOutputMaxLines, got)
	}
}

func TestRenderBashSegmentShowsStderrFirstOnFailure(t *testing.T) {
	handler := NewStreamHandler(nil)
	handler.HandleBashStart("make", "")
	handler.HandleBashEnd(&agentcore.ToolCall{Output: map[string]any{
		"stdout":      "building\n",
		"stderr":      "error: missing target\n",
		"stdout_file": "/tmp/keen-bash-1.stdout",
		"exit_code":   2,
	}})

	lines := handler.renderBashSegment(&handler.segments[0], 80, false)
	got := ansi.Strip(strings.Join(lines, "\n"))
	errAt, outAt := strings.Index(got, "error: missing target"), strings.Index(got, "building")
	if errAt < 0 || outAt < 0 || errAt > outAt {
		t.Fatalf("expected stderr before stdout on failure, got %q", got)
	}
	if !strings.Contains(got, "full stdout: /tmp/keen-bash-1.stdout") {
		t.Fatalf("expected saved output hint, got %q", got)
	}
	if !strings.Contains(strings.Join(lines, "\n"), repltheme.ToolErrorStyle.Render("error: missing target")) {
		t.Fatal("expected stderr in the error style")
	}
}

func TestSplitLineBudget(t *testing.T) {
	tests := []struct{ first, second, wantFirst, wantSecond int }{
		{3, 4, 3, 4},
		{30, 2, 14, 2},
		{2, 30, 2, 14},
		{30, 30, 8, 8},
	}
	for _, test := range tests {
		gotFirst, gotSecond := splitLineBudget(test.first, test.second, 16)
		if gotFirst != test.wantFirst || gotSecond != test.wantSecond {
			t.Fatalf("splitLineBudget(%d, %d) = %d, %d; want %d, %d", test.first, test.second, gotFirst, gotSecond, test.wantFirst, test.wantSecond)
		}
	}
}

func TestSanitizeCommandOutput(t *testing.T) {
	tests := map[string]string{
		"progress 10%\rprogress 50%\rprogress 100%\ndone": "progress 100%\ndone",
		"windows\r\nline": "windows\nline",
		"trailing\r":      "trailing",
		"\x1b[31mred\x1b[0m and \x1b[1mbold\x1b[0m": "red and bold",
		"bell\a and\bback\ttab":                     "bell andback\ttab",
	}
	for input, want := range tests {
		if got := sanitizeCommandOutput(input); got != want {
			t.Fatalf("sanitizeCommandOutput(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRenderBashSegmentExpandsWithDisplayOptions(t *testing.T) {
	output := make([]string, 250)
	for i := range output {
		output[i] = fmt.Sprintf("line %03d", i)
	}
	display := &DisplayOptions{}
	handler := NewStreamHandler(nil)
	handler.SetDisplayOptions(display)
	handler.HandleBashStart("go test ./...", "")
	handler.HandleBashEnd(&agentcore.ToolCall{Output: map[string]any{"stdout": strings.Join(output, "\n"), "exit_code": 0}})
	render := func() string {
		return ansi.Strip(strings.Join(handler.renderBashSegment(&handler.segments[0], 80, false), "\n"))
	}

	collapsed := render()
	if !strings.Contains(collapsed, "… 234 earlier lines · ctrl+o to expand") || strings.Contains(collapsed, "collapse") {
		t.Fatalf("collapsed block = %q", collapsed)
	}

	display.BashExpanded = true
	expanded := render()
	for _, want := range []string{"… 50 earlier lines", "line 050", "line 249", "ctrl+o to collapse"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded block missing %q: %q", want, expanded)
		}
	}
	if strings.Contains(expanded, "line 049") || strings.Contains(expanded, "to expand") {
		t.Fatalf("expanded block should cap at %d lines without an expand hint: %q", bashOutputExpandedMaxLines, expanded)
	}
}

func TestRenderBashSegmentHintsOnlyWhenTruncated(t *testing.T) {
	display := &DisplayOptions{BashExpanded: true}
	handler := NewStreamHandler(nil)
	handler.SetDisplayOptions(display)
	handler.HandleBashStart("echo hi", "")
	handler.HandleBashEnd(&agentcore.ToolCall{Output: map[string]any{"stdout": "hi", "exit_code": 0}, Duration: 1})
	handler.HandleBashStart("make", "")
	handler.HandleBashEnd(&agentcore.ToolCall{Output: map[string]any{
		"stdout":    strings.Repeat("out\n", 30),
		"stderr":    strings.Repeat("err\n", 30),
		"exit_code": 1,
	}})

	if got := ansi.Strip(strings.Join(handler.renderBashSegment(&handler.segments[0], 80, false), "\n")); strings.Contains(got, "ctrl+o") {
		t.Fatalf("short output should not show a ctrl+o hint: %q", got)
	}
	display.BashExpanded = false
	got := ansi.Strip(strings.Join(handler.renderBashSegment(&handler.segments[1], 80, false), "\n"))
	if strings.Count(got, "earlier lines") != 2 || strings.Count(got, "ctrl+o to expand") != 1 {
		t.Fatalf("expected one expand hint across both truncated streams: %q", got)
	}
}

func TestAppendedTranscriptFollowsDisplayOptions(t *testing.T) {
	display := &DisplayOptions{}
	handler := NewStreamHandler(nil)
	handler.SetDisplayOptions(display)
	handler.HandleBashStart("seq 40", "")
	handler.HandleBashEnd(&agentcore.ToolCall{Output: map[string]any{"stdout": strings.Repeat("n\n", 40), "exit_code": 0}})
	segments := handler.Snapshot()
	handler.HandleDone()

	output := reploutput.NewOutputBuilder(80, "")
	handler.AppendTranscript(output, segments)
	before := len(output.GetLines())

	display.BashExpanded = true
	output.Rerender()
	if after := len(output.GetLines()); after <= before {
		t.Fatalf("expected rerendered transcript to grow after expanding, got %d -> %d lines", before, after)
	}
}

func TestRenderBashSegmentTruncatesLongLines(t *testing.T) {
	display := &DisplayOptions{}
	handler := NewStreamHandler(nil)
	handler.SetDisplayOptions(display)
	handler.HandleBashStart("cat bundle.min.js", "")
	handler.HandleBashEnd(&agentcore.ToolCall{Output: map[string]any{
		"stdout":    "short\n" + strings.Repeat("x", 2500),
		"exit_code": 0,
	}})
	render := func() string {
		return ansi.Strip(strings.Join(handler.renderBashSegment(&handler.segments[0], 0, false), "\n"))
	}

	collapsed := render()
	if !strings.Contains(collapsed, strings.Repeat("x", bashOutputMaxLineChars)+" … +2300 chars") || strings.Contains(collapsed, strings.Repeat("x", bashOutputMaxLineChars+1)) {
		t.Fatalf("expected long line cut at %d chars, got %q", bashOutputMaxLineChars, collapsed)
	}
	if !strings.Contains(collapsed, "short") || strings.Contains(collapsed, "short …") {
		t.Fatalf("short lines should be untouched, got %q", collapsed)
	}

	display.BashExpanded = true
	expanded := render()
	if !strings.Contains(expanded, strings.Repeat("x", bashOutputExpandedMaxLineChars)+" … +500 chars") {
		t.Fatalf("expected expanded line cut at %d chars, got %q", bashOutputExpandedMaxLineChars, expanded)
	}
}
