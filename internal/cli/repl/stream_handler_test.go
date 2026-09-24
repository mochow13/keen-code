package repl

import (
	"charm.land/lipgloss/v2"
	"errors"
	"github.com/mochow13/keen-code/internal/cli/repl/agentcore"
	"strings"
	"testing"
)

func TestStreamHandlerHidesSubagentFileNotFoundFailure(t *testing.T) {
	h := NewStreamHandler(nil)
	h.workingDir = "/repo"
	h.Start(make(chan agentcore.StreamEvent), "Working")
	h.HandleSubagentActivity(agentcore.ToolActivity{RunID: "run-1", CallID: "tool-1", Agent: "worker", Event: agentcore.StreamEvent{
		Type: agentcore.StreamEventTypeToolStart, ToolCall: &agentcore.ToolCall{Name: "read_file", Input: map[string]any{"path": "missing.go"}},
	}})
	if view := h.View(120); !strings.Contains(view, "missing.go") {
		t.Fatalf("expected active tool call before its result, got %q", view)
	}
	h.HandleSubagentActivity(agentcore.ToolActivity{RunID: "run-1", CallID: "tool-1", Agent: "worker", Event: agentcore.StreamEvent{
		Type: agentcore.StreamEventTypeToolEnd, ToolCall: &agentcore.ToolCall{Name: "read_file", Error: "not found: file missing.go"},
	}})
	if view := h.View(120); strings.Contains(view, "missing.go") || strings.Contains(view, "<worker>") {
		t.Fatalf("expected completed file-not-found call to be hidden, got %q", view)
	}
	if transcript := strings.Join(h.renderTranscriptLines(), "\n"); strings.Contains(transcript, "missing.go") || strings.Contains(transcript, "<worker>") {
		t.Fatalf("expected file-not-found call to be hidden from transcript, got %q", transcript)
	}
}

func TestStreamHandlerRendersInterleavedSubagentActivityWithoutResults(t *testing.T) {
	h := NewStreamHandler(nil)
	h.workingDir = "/repo"
	h.Start(make(chan agentcore.StreamEvent), "Working")
	h.HandleSubagentActivity(agentcore.ToolActivity{RunID: "run-1", CallID: "tool-1", Agent: "worker", Event: agentcore.StreamEvent{
		Type: agentcore.StreamEventTypeToolStart, ToolCall: &agentcore.ToolCall{Name: "bash", Input: map[string]any{"command": "go test ./..."}},
	}})
	h.HandleSubagentActivity(agentcore.ToolActivity{RunID: "run-2", CallID: "tool-1", Agent: "reviewer", Event: agentcore.StreamEvent{
		Type: agentcore.StreamEventTypeToolStart, ToolCall: &agentcore.ToolCall{Name: "read_file", Input: map[string]any{"path": "README.md"}},
	}})
	h.HandleSubagentActivity(agentcore.ToolActivity{RunID: "run-1", CallID: "tool-1", Agent: "worker", Event: agentcore.StreamEvent{
		Type: agentcore.StreamEventTypeToolEnd, ToolCall: &agentcore.ToolCall{Name: "bash", Output: map[string]any{"stdout": "hidden-output"}},
	}})
	view := h.View(120)
	for _, want := range []string{"[worker]", "go test ./...", "[reviewer]", "README.md"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, "hidden-output") {
		t.Fatalf("view leaked child result: %q", view)
	}
}

func TestStreamHandler_HandleChunk(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	sh.HandleChunk("Hello")
	if sh.GetResponse() != "Hello" {
		t.Errorf("expected response 'Hello', got '%s'", sh.GetResponse())
	}
	if sh.GetRawResponse() != "Hello" {
		t.Errorf("expected raw response 'Hello', got '%s'", sh.GetRawResponse())
	}
	if sh.HasContent() != true {
		t.Error("expected HasContent() to be true")
	}

	sh.HandleChunk(" World")
	if sh.GetResponse() != "Hello World" {
		t.Errorf("expected response 'Hello World', got '%s'", sh.GetResponse())
	}
	if sh.GetRawResponse() != "Hello World" {
		t.Errorf("expected raw response 'Hello World', got '%s'", sh.GetRawResponse())
	}
}

func TestStreamHandler_HandleReasoningChunk_DoesNotAffectAssistantResponse(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	sh.HandleReasoningChunk("thinking ")
	sh.HandleReasoningChunk("more")
	sh.HandleChunk("answer")

	if got := sh.GetResponse(); got != "answer" {
		t.Fatalf("expected assistant response 'answer', got %q", got)
	}
	if len(sh.segments) != 2 {
		t.Fatalf("expected 2 segments (reasoning + assistant), got %d", len(sh.segments))
	}
	if sh.segments[0].kind != segmentReasoning {
		t.Fatalf("expected first segment reasoning, got %q", sh.segments[0].kind)
	}
	if sh.segments[0].content != "thinking more" {
		t.Fatalf("unexpected reasoning content %q", sh.segments[0].content)
	}
}

func TestStreamHandler_HandleDone(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan agentcore.StreamEvent)
	sh.Start(eventCh, "Loading...")
	sh.HandleChunk("Line 1\nLine 2")

	lines, fullResponse := sh.HandleDone()

	if fullResponse != "Line 1\nLine 2" {
		t.Errorf("expected full response 'Line 1\\nLine 2', got '%s'", fullResponse)
	}

	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "  Line 1") {
		t.Errorf("expected first line to start with '  Line 1', got '%s'", lines[0])
	}

	if sh.IsActive() {
		t.Error("expected IsActive to be false after HandleDone")
	}
	if sh.HasContent() {
		t.Error("expected HasContent to be false after HandleDone")
	}
}

func TestStreamHandler_HandleError(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan agentcore.StreamEvent)
	sh.Start(eventCh, "Loading...")
	sh.HandleChunk("some content")

	testErr := errors.New("stream failed")
	lines, errMsg := sh.HandleError(testErr)

	if len(lines) != 1 {
		t.Fatalf("expected 1 pending transcript line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "some content") {
		t.Errorf("expected pending line to include chunk content, got %q", lines[0])
	}

	if errMsg != "stream failed" {
		t.Errorf("expected error message 'stream failed', got '%s'", errMsg)
	}

	if sh.IsActive() {
		t.Error("expected IsActive to be false after HandleError")
	}
	if sh.HasContent() {
		t.Error("expected HasContent to be false after HandleError")
	}
}

func TestStreamHandler_RewindForRetry_PreservesSealedSegments(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	// Iteration 1: assistant text + a completed tool call.
	sh.HandleChunk("Let me read the file. ")
	sh.HandleToolStart(&agentcore.ToolCall{Name: "read_file"})
	sh.HandleToolEnd(&agentcore.ToolCall{Name: "read_file", Duration: 5})

	// Iteration 2: in-flight reasoning + assistant chunks before a stream failure.
	sh.HandleReasoningChunk("checking the contents")
	sh.HandleChunk("Based on the file, I think")

	sh.RewindForRetry()

	// The two completed iteration-1 segments (assistant + tool start/end pair) survive;
	// the trailing in-flight reasoning + assistant segments are dropped.
	if len(sh.segments) != 3 {
		t.Fatalf("expected 3 surviving segments after rewind, got %d", len(sh.segments))
	}
	if sh.segments[0].kind != segmentAssistant || sh.segments[0].content != "Let me read the file. " {
		t.Fatalf("expected first segment to be the iteration-1 assistant message, got %+v", sh.segments[0])
	}
	if sh.segments[1].kind != segmentToolStart || sh.segments[2].kind != segmentToolEnd {
		t.Fatalf("expected tool start/end pair to remain, got %q/%q", sh.segments[1].kind, sh.segments[2].kind)
	}

	// currentResponse and rawResponse must be rebuilt to match what's still in the slice,
	// so the upcoming retry's chunks accumulate on top of iteration-1 text only.
	if got := sh.GetResponse(); got != "Let me read the file. " {
		t.Fatalf("expected rebuilt response %q, got %q", "Let me read the file. ", got)
	}
	if got := sh.GetRawResponse(); got != "Let me read the file. " {
		t.Fatalf("expected rebuilt raw response %q, got %q", "Let me read the file. ", got)
	}
}

func TestStreamHandler_RewindForRetry_EmptyStream(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	sh.HandleChunk("partial")
	sh.HandleReasoningChunk("hmm")

	sh.RewindForRetry()

	if len(sh.segments) != 0 {
		t.Fatalf("expected no segments after rewinding a stream with no sealed work, got %d", len(sh.segments))
	}
	if sh.GetResponse() != "" || sh.GetRawResponse() != "" {
		t.Fatalf("expected response strings to be cleared, got %q / %q", sh.GetResponse(), sh.GetRawResponse())
	}
}

func TestStreamHandler_RewindForRetry_LeavesSealedTailUnchanged(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	sh.HandleChunk("Running tests. ")
	sh.HandleBashStart("go test ./...", "run tests")
	sh.HandleBashEnd(&agentcore.ToolCall{Output: map[string]any{"stdout": "ok"}})

	sh.RewindForRetry()

	if len(sh.segments) != 2 {
		t.Fatalf("expected sealed assistant/bash segments to remain, got %d", len(sh.segments))
	}
	if sh.segments[0].kind != segmentAssistant || sh.segments[1].kind != segmentBash {
		t.Fatalf("expected assistant/bash segments to remain, got %q/%q", sh.segments[0].kind, sh.segments[1].kind)
	}
	if got := sh.GetResponse(); got != "Running tests. " {
		t.Fatalf("expected response to remain %q, got %q", "Running tests. ", got)
	}
}

func TestStreamHandler_HandleDone_MixedSegmentsChronological(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan agentcore.StreamEvent)
	sh.Start(eventCh, "Loading...")

	sh.HandleChunk("First chunk")
	sh.HandleToolStart(&agentcore.ToolCall{Name: "read_file", Input: map[string]any{"path": "go.mod"}})
	sh.HandleChunk(" Second chunk")
	sh.HandleToolEnd(&agentcore.ToolCall{Name: "read_file", Duration: 5})

	lines, fullResponse := sh.HandleDone()

	if fullResponse != "First chunk Second chunk" {
		t.Fatalf("unexpected full response: %q", fullResponse)
	}

	// start and end are not adjacent (chunk between them), so both lines are emitted
	if len(lines) != 4 {
		t.Fatalf("expected 4 transcript lines, got %d", len(lines))
	}

	if !strings.Contains(lines[0], "First chunk") {
		t.Fatalf("expected first line to be first assistant chunk, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "Read") || !strings.Contains(lines[1], "●") {
		t.Fatalf("expected second line to be tool start, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "Second chunk") {
		t.Fatalf("expected third line to be second assistant chunk, got %q", lines[2])
	}
	if !strings.Contains(lines[3], "Read") || !strings.Contains(lines[3], "✓") {
		t.Fatalf("expected fourth line to be tool end, got %q", lines[3])
	}
}

func TestStreamHandler_HandleDone_AdjacentToolStartEnd_CollapsedToOneLine(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan agentcore.StreamEvent)
	sh.Start(eventCh, "Loading...")

	sh.HandleToolStart(&agentcore.ToolCall{Name: "glob", Input: map[string]any{"pattern": "**/*.go"}})
	sh.HandleToolEnd(&agentcore.ToolCall{Name: "glob", Duration: 5})

	lines, _ := sh.HandleDone()

	if len(lines) != 1 {
		t.Fatalf("expected 1 line for adjacent start/end, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "Find") || !strings.Contains(lines[0], "✓") {
		t.Fatalf("expected combined done line, got %q", lines[0])
	}
	if strings.Contains(lines[0], "●") {
		t.Fatalf("expected no tool-start marker in combined line, got %q", lines[0])
	}
}

func TestFinalAssistantRun(t *testing.T) {
	segments := []streamSegment{
		{kind: segmentReasoning, content: "thinking"},
		{kind: segmentAssistant, content: "Let me check the config first."},
		{kind: segmentToolStart, toolCall: &agentcore.ToolCall{Name: "read_file"}},
		{kind: segmentToolEnd, toolCall: &agentcore.ToolCall{Name: "read_file"}},
		{kind: segmentAssistant, content: "## Goal\nShip it."},
	}
	if got := finalAssistantRun(segments); got != "## Goal\nShip it." {
		t.Fatalf("finalAssistantRun() = %q", got)
	}

	noTools := []streamSegment{{kind: segmentAssistant, content: "whole response"}}
	if got := finalAssistantRun(noTools); got != "whole response" {
		t.Fatalf("finalAssistantRun() = %q", got)
	}

	noTrailingText := []streamSegment{{kind: segmentToolEnd, toolCall: &agentcore.ToolCall{Name: "grep"}}}
	if got := finalAssistantRun(noTrailingText); got != "" {
		t.Fatalf("expected empty final run, got %q", got)
	}
}

func TestHasNonTextActivity(t *testing.T) {
	cases := []struct {
		name     string
		segments []streamSegment
		want     bool
	}{
		{name: "empty"},
		{name: "assistant only", segments: []streamSegment{{kind: segmentAssistant, content: "text"}}},
		{name: "reasoning only", segments: []streamSegment{{kind: segmentReasoning, content: "thinking"}}},
		{name: "reasoning and assistant", segments: []streamSegment{{kind: segmentReasoning}, {kind: segmentAssistant, content: "text"}}},
		{name: "tool start", segments: []streamSegment{{kind: segmentToolStart}}, want: true},
		{name: "tool end", segments: []streamSegment{{kind: segmentToolEnd}}, want: true},
		{name: "bash", segments: []streamSegment{{kind: segmentBash}}, want: true},
		{name: "permission", segments: []streamSegment{{kind: segmentPermission}}, want: true},
		{name: "diff", segments: []streamSegment{{kind: segmentDiff}}, want: true},
		{name: "subagent", segments: []streamSegment{{kind: segmentSubagent}}, want: true},
		{name: "ask user", segments: []streamSegment{{kind: segmentAskUser}}, want: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasNonTextActivity(tt.segments); got != tt.want {
				t.Fatalf("hasNonTextActivity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStreamHandler_ReadFileNotFoundIsHidden(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	sh.HandleChunk("Checking the file. ")
	sh.HandleToolStart(&agentcore.ToolCall{Name: "read_file", Input: map[string]any{"path": "missing.go"}})
	sh.HandleToolEnd(&agentcore.ToolCall{Name: "read_file", Error: `not found: file "missing.go" does not exist`})
	sh.HandleChunk("It is absent.")

	view := sh.View(80)
	lines, _ := sh.HandleDone()
	transcript := strings.Join(lines, "\n")
	for _, rendered := range []string{view, transcript} {
		if strings.Contains(rendered, "Read") || strings.Contains(rendered, "missing.go") || strings.Contains(rendered, "not found") {
			t.Fatalf("expected missing read to be hidden, got %q", rendered)
		}
		if !strings.Contains(rendered, "Checking the file.") || !strings.Contains(rendered, "It is absent.") {
			t.Fatalf("expected assistant text to remain, got %q", rendered)
		}
	}
}

func TestStreamHandler_HidesExpectedEditFileFailures(t *testing.T) {
	tests := []struct {
		name  string
		err   string
		match string
	}{
		{
			name:  "missing anchor",
			err:   `op 1: anchor "2:fff" does not exist in the current file snapshot; re-read the file and retry`,
			match: "does not exist in the current file snapshot",
		},
		{
			name:  "empty file insert tail",
			err:   "op 1: only insert_head is valid for an empty file",
			match: "only insert_head is valid for an empty file",
		},
		{
			name:  "overlapping operations",
			err:   "ops 1 and 2 have overlapping ranges (lines 2-4 and 3-5)",
			match: "overlapping ranges",
		},
		{
			name:  "conflicting operations",
			err:   "ops 1 and 2 conflict: both insert at the same position; combine them into one op",
			match: "both insert at the same position",
		},
		{
			name:  "file not found",
			err:   `not found: file "output.go" does not exist`,
			match: "not found: file",
		},
		{
			name:  "target is directory",
			err:   `not a file: "output.go" is a directory`,
			match: "is a directory",
		},
		{
			name:  "path resolution failed",
			err:   "path resolution failed: path escapes working directory",
			match: "path resolution failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sh := NewStreamHandler(nil)
			sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

			sh.HandleChunk("Trying an edit. ")
			sh.HandleToolStart(&agentcore.ToolCall{Name: "edit_file", Input: map[string]any{"path": "output.go"}})
			sh.HandleToolEnd(&agentcore.ToolCall{Name: "edit_file", Error: tt.err})
			sh.HandleChunk("The edit did not apply.")

			view := sh.View(80)
			lines, _ := sh.HandleDone()
			transcript := strings.Join(lines, "\n")
			for _, rendered := range []string{view, transcript} {
				if strings.Contains(rendered, "Edit") || strings.Contains(rendered, "output.go") || strings.Contains(rendered, tt.match) {
					t.Fatalf("expected failed edit to be hidden, got %q", rendered)
				}
				if !strings.Contains(rendered, "Trying an edit.") || !strings.Contains(rendered, "The edit did not apply.") {
					t.Fatalf("expected assistant text to remain, got %q", rendered)
				}
			}
		})
	}
}

func TestStreamHandler_DelegateTaskShowsBatchAndPartialFailure(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Brewing...")
	input := map[string]any{"tasks": []any{
		map[string]any{"agent": "explorer", "task": "one"},
		map[string]any{"agent": "explorer", "task": "two"},
		map[string]any{"agent": "reviewer", "task": "three"},
	}}
	sh.HandleToolStart(&agentcore.ToolCall{Name: "delegate_task", Input: input})

	view := sh.View(80)
	if !strings.Contains(view, "3 tasks (explorer ×2, reviewer ×1)") {
		t.Fatalf("expected running batch summary, got %q", view)
	}

	sh.HandleToolEnd(&agentcore.ToolCall{
		Name:  "delegate_task",
		Input: input,
		Output: map[string]any{"results": []map[string]any{
			{"agent": "explorer"}, {"agent": "reviewer"}, {"agent": "explorer", "error": "failed"},
		}},
	})
	view = sh.View(120)
	want := "2 completed (explorer ×1, reviewer ×1), 1 failed (explorer ×1)"
	if !strings.Contains(view, want) || !strings.Contains(view, "✗") {
		t.Fatalf("expected partial failure summary, got %q", view)
	}
}

func TestStreamHandler_CallMCPToolNeverShowsArguments(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Brewing...")
	sh.HandleToolStart(&agentcore.ToolCall{Name: "call_mcp_tool", Input: map[string]any{
		"server": "context7",
		"tool":   "query-docs",
		"arguments": map[string]any{
			"query":     "React useEffect API reference",
			"libraryId": "/reactjs/react.dev",
		},
	}})

	view := sh.View(80)
	if !strings.Contains(view, "MCP") || !strings.Contains(view, "context7/query-docs") {
		t.Fatalf("expected MCP tool summary, got %q", view)
	}
	if strings.Contains(view, "libraryId") || strings.Contains(view, "query:") || strings.Contains(view, "React useEffect") {
		t.Fatalf("expected MCP arguments to be hidden, got %q", view)
	}

	sh.HandleToolEnd(&agentcore.ToolCall{Name: "call_mcp_tool", Input: map[string]any{
		"server": "context7",
		"tool":   "query-docs",
		"arguments": map[string]any{
			"query":     "React useEffect API reference",
			"libraryId": "/reactjs/react.dev",
		},
	}, Duration: 5})

	view = sh.View(80)
	if !strings.Contains(view, "MCP") || !strings.Contains(view, "context7/query-docs") {
		t.Fatalf("expected completed MCP tool summary, got %q", view)
	}
	if strings.Contains(view, "libraryId") || strings.Contains(view, "query:") || strings.Contains(view, "React useEffect") {
		t.Fatalf("expected completed MCP arguments to be hidden, got %q", view)
	}

	lines, _ := sh.HandleDone()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "MCP") || !strings.Contains(joined, "context7/query-docs") {
		t.Fatalf("expected transcript MCP tool summary, got %q", joined)
	}
	if strings.Contains(joined, "libraryId") || strings.Contains(joined, "query:") || strings.Contains(joined, "React useEffect") {
		t.Fatalf("expected transcript MCP arguments to be hidden, got %q", joined)
	}
}

func TestStreamHandler_View_NoSpinnerInView(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Brewing...")

	view := sh.View(80)

	if strings.Contains(view, "Brewing...") {
		t.Error("expected view to not contain loading text (spinner is rendered outside StreamHandler)")
	}
}

func TestStreamHandler_View_WithRunningBashShowsCommand(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Brewing...")
	sh.HandleBashStart("npm test", "running tests")

	view := sh.View(80)

	if !strings.Contains(view, "npm test") {
		t.Fatal("expected bash command in view")
	}
	if !strings.Contains(view, "running tests") {
		t.Fatal("expected bash summary in view")
	}
}

func TestStreamHandler_View_LongToolStatusWrapsWithinWidth(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Brewing...")
	longPattern := strings.Repeat("very-long-segment/", 8) + "*.go"
	sh.HandleToolStart(&agentcore.ToolCall{Name: "grep", Input: map[string]any{
		"pattern": longPattern,
		"path":    "internal/cli/repl",
	}})
	sh.HandleToolEnd(&agentcore.ToolCall{Name: "grep", Duration: 5})

	width := 40
	view := sh.View(width)
	lines := strings.Split(strings.TrimPrefix(strings.TrimRight(view, "\n"), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected long tool status to wrap, got %v", lines)
	}
	for _, line := range lines {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("line exceeds viewport width (%d > %d): %q", w, width, line)
		}
		if !strings.HasPrefix(line, "  ") {
			t.Fatalf("expected wrapped tool status line to stay indented, got %q", line)
		}
	}
}

func TestStreamHandler_HandleDone_LongToolStatusWrapsToLastWidth(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Brewing...")
	sh.HandleToolStart(&agentcore.ToolCall{Name: "grep", Input: map[string]any{
		"pattern": strings.Repeat("long-pattern-", 12),
		"path":    "internal/cli/repl",
	}})
	sh.HandleToolEnd(&agentcore.ToolCall{Name: "grep", Duration: 5})
	sh.View(42)

	lines, _ := sh.HandleDone()
	if len(lines) < 2 {
		t.Fatalf("expected long transcript tool status to wrap, got %v", lines)
	}
	for _, line := range lines {
		if w := lipgloss.Width(line); w > 42 {
			t.Fatalf("line exceeds transcript width (%d > %d): %q", w, 42, line)
		}
	}
}

func TestStreamHandler_View_BashUsesViewportWidthRules(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Brewing...")
	sh.HandleBashStart("npm test", "running tests")

	wideView := sh.View(80)
	wideLines := strings.Split(strings.TrimRight(wideView, "\n"), "\n")
	wideNonEmpty := make([]string, 0, len(wideLines))
	for _, line := range wideLines {
		if strings.TrimSpace(line) != "" {
			wideNonEmpty = append(wideNonEmpty, line)
		}
	}
	if len(wideNonEmpty) < 3 {
		t.Fatalf("expected ruled bash block, got %v", wideNonEmpty)
	}
	if !strings.Contains(wideNonEmpty[0], "─") || !strings.Contains(wideNonEmpty[len(wideNonEmpty)-1], "─") {
		t.Fatalf("expected top and bottom bash rules, got %q", wideView)
	}
	if strings.HasPrefix(wideNonEmpty[0], "  ") || strings.HasPrefix(wideNonEmpty[len(wideNonEmpty)-1], "  ") {
		t.Fatalf("expected bash rules to span edge-to-edge without left inset, got %q", wideView)
	}
	if wideRuleWidth := lipgloss.Width(wideNonEmpty[0]); wideRuleWidth != 80 {
		t.Fatalf("expected bash rules to match viewport width, got width %d", wideRuleWidth)
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
	if len(narrowNonEmpty) < 3 {
		t.Fatalf("expected non-empty narrow bash lines, got %v", narrowNonEmpty)
	}
	if narrowRuleWidth := lipgloss.Width(narrowNonEmpty[0]); narrowRuleWidth != 24 {
		t.Fatalf("expected narrow bash rules to match viewport width, got width %d", narrowRuleWidth)
	}
}

func TestStreamHandler_View_WithContent(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")
	sh.HandleChunk("Hello World")

	view := sh.View(80)

	if !strings.Contains(view, "Hello World") {
		t.Error("expected view to contain response content")
	}
}

func TestStreamHandler_View_NoSpinnerNoContent(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan agentcore.StreamEvent), "Loading...")

	view := sh.View(80)

	if view != "" {
		t.Errorf("expected empty view when no spinner and no content, got '%s'", view)
	}
}

func TestStreamHandler_Start(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan agentcore.StreamEvent)

	sh.Start(eventCh, "Cooking...")

	if !sh.IsActive() {
		t.Error("expected IsActive to be true after Start")
	}
	if sh.GetLoadingText() != "Cooking..." {
		t.Errorf("expected loading text 'Cooking...', got '%s'", sh.GetLoadingText())
	}
	if sh.HasContent() {
		t.Error("expected HasContent to be false initially")
	}
}

func TestStreamHandler_Start_ResetsPreviousState(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan agentcore.StreamEvent)

	sh.Start(eventCh, "First")
	sh.HandleChunk("previous content")

	newEventCh := make(chan agentcore.StreamEvent)
	sh.Start(newEventCh, "Second")

	if sh.GetResponse() != "" {
		t.Error("expected response to be reset after new Start")
	}
	if sh.GetLoadingText() != "Second" {
		t.Error("expected loading text to be updated")
	}
}

func TestStreamHandlerCheckpointPreservesActiveStream(t *testing.T) {
	events := make(chan agentcore.StreamEvent)
	handler := NewStreamHandler(nil)
	handler.Start(events, "Working...")
	handler.HandleChunk("before checkpoint")

	_, response, segments := handler.Checkpoint()

	if response != "before checkpoint" {
		t.Fatalf("response = %q, want checkpoint response", response)
	}
	if len(segments) != 1 {
		t.Fatalf("segments = %d, want 1", len(segments))
	}
	if !handler.IsActive() {
		t.Fatal("checkpoint deactivated the stream")
	}
	if handler.eventCh != events {
		t.Fatal("checkpoint disconnected the event channel")
	}
	if handler.GetResponse() != "" || handler.HasContent() {
		t.Fatal("checkpoint did not clear current content")
	}
}
