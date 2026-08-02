package repl

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	replpermissions "github.com/user/keen-code/internal/cli/repl/permissions"
	repltooling "github.com/user/keen-code/internal/cli/repl/tooling"
	"github.com/user/keen-code/internal/llm"
	"github.com/user/keen-code/internal/subagents"
	"github.com/user/keen-code/internal/tools"
)

func TestStreamHandlerHidesSubagentFileNotFoundFailure(t *testing.T) {
	h := NewStreamHandler(nil)
	h.workingDir = "/repo"
	h.Start(make(chan llm.StreamEvent), "Working")
	h.HandleSubagentActivity(subagents.ToolActivity{RunID: "run-1", CallID: "tool-1", Agent: "worker", Event: llm.StreamEvent{
		Type: llm.StreamEventTypeToolStart, ToolCall: &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "missing.go"}},
	}})
	if view := h.View(120); !strings.Contains(view, "missing.go") {
		t.Fatalf("expected active tool call before its result, got %q", view)
	}
	h.HandleSubagentActivity(subagents.ToolActivity{RunID: "run-1", CallID: "tool-1", Agent: "worker", Event: llm.StreamEvent{
		Type: llm.StreamEventTypeToolEnd, ToolCall: &llm.ToolCall{Name: "read_file", Error: "not found: file missing.go"},
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
	h.Start(make(chan llm.StreamEvent), "Working")
	h.HandleSubagentActivity(subagents.ToolActivity{RunID: "run-1", CallID: "tool-1", Agent: "worker", Event: llm.StreamEvent{
		Type: llm.StreamEventTypeToolStart, ToolCall: &llm.ToolCall{Name: "bash", Input: map[string]any{"command": "go test ./..."}},
	}})
	h.HandleSubagentActivity(subagents.ToolActivity{RunID: "run-2", CallID: "tool-1", Agent: "reviewer", Event: llm.StreamEvent{
		Type: llm.StreamEventTypeToolStart, ToolCall: &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "README.md"}},
	}})
	h.HandleSubagentActivity(subagents.ToolActivity{RunID: "run-1", CallID: "tool-1", Agent: "worker", Event: llm.StreamEvent{
		Type: llm.StreamEventTypeToolEnd, ToolCall: &llm.ToolCall{Name: "bash", Output: map[string]any{"stdout": "hidden-output"}},
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
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

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
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

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
	eventCh := make(chan llm.StreamEvent)
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
	eventCh := make(chan llm.StreamEvent)
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
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	// Iteration 1: assistant text + a completed tool call.
	sh.HandleChunk("Let me read the file. ")
	sh.HandleToolStart(&llm.ToolCall{Name: "read_file"})
	sh.HandleToolEnd(&llm.ToolCall{Name: "read_file", Duration: 5})

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
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

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

func TestStreamHandler_RewindForRetry_PreservesResolvedPermissionAndDiff(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	sh.HandleChunk("I'll edit this. ")
	sh.HandleDiff([]tools.EditDiffLine{{Kind: tools.DiffLineAdded, Content: "new line", NewLineNum: 1}})
	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(replpermissions.StatusAllowed)
	sh.HandleReasoningChunk("checking result")
	sh.HandleChunk("The edit completed")

	sh.RewindForRetry()

	if len(sh.segments) != 3 {
		t.Fatalf("expected 3 surviving segments after rewind, got %d", len(sh.segments))
	}
	if sh.segments[0].kind != segmentAssistant || sh.segments[1].kind != segmentDiff || sh.segments[2].kind != segmentPermission {
		t.Fatalf("expected assistant/diff/permission segments to remain, got %q/%q/%q", sh.segments[0].kind, sh.segments[1].kind, sh.segments[2].kind)
	}
	if sh.segments[2].permissionReq.Status != replpermissions.StatusAllowed {
		t.Fatalf("expected resolved permission to remain allowed, got %q", sh.segments[2].permissionReq.Status)
	}
	if got := sh.GetResponse(); got != "I'll edit this. " {
		t.Fatalf("expected rebuilt response %q, got %q", "I'll edit this. ", got)
	}
}

func TestStreamHandler_RewindForRetry_LeavesSealedTailUnchanged(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	sh.HandleChunk("Running tests. ")
	sh.HandleBashStart("go test ./...", "run tests")
	sh.HandleBashEnd(&llm.ToolCall{Output: map[string]any{"stdout": "ok"}})

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
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	sh.HandleChunk("First chunk")
	sh.HandleToolStart(&llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "go.mod"}})
	sh.HandleChunk(" Second chunk")
	sh.HandleToolEnd(&llm.ToolCall{Name: "read_file", Duration: 5})

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
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	sh.HandleToolStart(&llm.ToolCall{Name: "glob", Input: map[string]any{"pattern": "**/*.go"}})
	sh.HandleToolEnd(&llm.ToolCall{Name: "glob", Duration: 5})

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

func TestStreamHandler_ReadFileNotFoundIsHidden(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	sh.HandleChunk("Checking the file. ")
	sh.HandleToolStart(&llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "missing.go"}})
	sh.HandleToolEnd(&llm.ToolCall{Name: "read_file", Error: `not found: file "missing.go" does not exist`})
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

func TestStreamHandler_EditFileOldStringNotFoundIsHidden(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	sh.HandleChunk("Trying an edit. ")
	sh.HandleToolStart(&llm.ToolCall{Name: "edit_file", Input: map[string]any{"path": "output.go"}})
	sh.HandleToolEnd(&llm.ToolCall{Name: "edit_file", Error: "oldString not found"})
	sh.HandleChunk("The target changed.")

	view := sh.View(80)
	lines, _ := sh.HandleDone()
	transcript := strings.Join(lines, "\n")
	for _, rendered := range []string{view, transcript} {
		if strings.Contains(rendered, "Edit") || strings.Contains(rendered, "output.go") || strings.Contains(rendered, "oldString not found") {
			t.Fatalf("expected failed edit to be hidden, got %q", rendered)
		}
		if !strings.Contains(rendered, "Trying an edit.") || !strings.Contains(rendered, "The target changed.") {
			t.Fatalf("expected assistant text to remain, got %q", rendered)
		}
	}
}

func TestStreamHandler_DelegateTaskShowsBatchAndPartialFailure(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Brewing...")
	input := map[string]any{"tasks": []any{
		map[string]any{"agent": "explorer", "task": "one"},
		map[string]any{"agent": "explorer", "task": "two"},
		map[string]any{"agent": "reviewer", "task": "three"},
	}}
	sh.HandleToolStart(&llm.ToolCall{Name: "delegate_task", Input: input})

	view := sh.View(80)
	if !strings.Contains(view, "3 tasks (explorer ×2, reviewer ×1)") {
		t.Fatalf("expected running batch summary, got %q", view)
	}

	sh.HandleToolEnd(&llm.ToolCall{
		Name:  "delegate_task",
		Input: input,
		Output: map[string]any{
			"completed": 2, "failed": 1,
			"completed_by_agent": map[string]int{"explorer": 1, "reviewer": 1},
			"failed_by_agent":    map[string]int{"explorer": 1},
		},
	})
	view = sh.View(120)
	want := "2 completed (explorer ×1, reviewer ×1), 1 failed (explorer ×1)"
	if !strings.Contains(view, want) || !strings.Contains(view, "✗") {
		t.Fatalf("expected partial failure summary, got %q", view)
	}
}

func TestStreamHandler_CallMCPToolNeverShowsArguments(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Brewing...")
	sh.HandleToolStart(&llm.ToolCall{Name: "call_mcp_tool", Input: map[string]any{
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

	sh.HandleToolEnd(&llm.ToolCall{Name: "call_mcp_tool", Input: map[string]any{
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
	sh.Start(make(<-chan llm.StreamEvent), "Brewing...")

	view := sh.View(80)

	if strings.Contains(view, "Brewing...") {
		t.Error("expected view to not contain loading text (spinner is rendered outside StreamHandler)")
	}
}

func TestStreamHandler_View_WithRunningBashShowsCommand(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Brewing...")
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
	sh.Start(make(<-chan llm.StreamEvent), "Brewing...")
	longPattern := strings.Repeat("very-long-segment/", 8) + "*.go"
	sh.HandleToolStart(&llm.ToolCall{Name: "grep", Input: map[string]any{
		"pattern": longPattern,
		"path":    "internal/cli/repl",
	}})
	sh.HandleToolEnd(&llm.ToolCall{Name: "grep", Duration: 5})

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
	sh.Start(make(<-chan llm.StreamEvent), "Brewing...")
	sh.HandleToolStart(&llm.ToolCall{Name: "grep", Input: map[string]any{
		"pattern": strings.Repeat("long-pattern-", 12),
		"path":    "internal/cli/repl",
	}})
	sh.HandleToolEnd(&llm.ToolCall{Name: "grep", Duration: 5})
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
	sh.Start(make(<-chan llm.StreamEvent), "Brewing...")
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
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")
	sh.HandleChunk("Hello World")

	view := sh.View(80)

	if !strings.Contains(view, "Hello World") {
		t.Error("expected view to contain response content")
	}
}

func TestStreamHandler_View_NoSpinnerNoContent(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	view := sh.View(80)

	if view != "" {
		t.Errorf("expected empty view when no spinner and no content, got '%s'", view)
	}
}

func TestWaitForAsyncEvent_Chunk(t *testing.T) {
	eventCh := make(chan llm.StreamEvent, 1)
	eventCh <- llm.StreamEvent{
		Type:    llm.StreamEventTypeChunk,
		Content: "chunk data",
	}
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *replpermissions.Request), make(chan repltooling.DiffRequest), nil)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	streamMsg, ok := msg.(mainStreamMsg)
	if !ok {
		t.Fatalf("expected mainStreamMsg, got %T", msg)
	}
	if streamMsg.eventCh != eventCh {
		t.Fatal("expected source event channel to round-trip")
	}
	if streamMsg.closed {
		t.Fatal("expected open stream event")
	}
	if streamMsg.event.Type != llm.StreamEventTypeChunk || streamMsg.event.Content != "chunk data" {
		t.Fatalf("unexpected stream event: %#v", streamMsg.event)
	}
}

func TestWaitForAsyncEvent_Done(t *testing.T) {
	eventCh := make(chan llm.StreamEvent, 1)
	eventCh <- llm.StreamEvent{
		Type: llm.StreamEventTypeDone,
	}
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *replpermissions.Request), make(chan repltooling.DiffRequest), nil)
	msg := cmd()

	streamMsg, ok := msg.(mainStreamMsg)
	if !ok {
		t.Fatalf("expected mainStreamMsg, got %T", msg)
	}
	if streamMsg.closed || streamMsg.event.Type != llm.StreamEventTypeDone {
		t.Fatalf("expected done event, got %#v", streamMsg)
	}
}

func TestWaitForAsyncEvent_ReasoningChunk(t *testing.T) {
	eventCh := make(chan llm.StreamEvent, 1)
	eventCh <- llm.StreamEvent{
		Type:    llm.StreamEventTypeReasoningChunk,
		Content: "thinking",
	}
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *replpermissions.Request), make(chan repltooling.DiffRequest), nil)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	streamMsg, ok := msg.(mainStreamMsg)
	if !ok {
		t.Fatalf("expected mainStreamMsg, got %T", msg)
	}
	if streamMsg.closed || streamMsg.event.Type != llm.StreamEventTypeReasoningChunk || streamMsg.event.Content != "thinking" {
		t.Fatalf("unexpected stream event: %#v", streamMsg)
	}
}

func TestWaitForAsyncEvent_Error(t *testing.T) {
	testErr := errors.New("stream error")
	eventCh := make(chan llm.StreamEvent, 1)
	eventCh <- llm.StreamEvent{
		Type:  llm.StreamEventTypeError,
		Error: testErr,
	}
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *replpermissions.Request), make(chan repltooling.DiffRequest), nil)
	msg := cmd()

	streamMsg, ok := msg.(mainStreamMsg)
	if !ok {
		t.Fatalf("expected mainStreamMsg, got %T", msg)
	}
	if streamMsg.closed || streamMsg.event.Type != llm.StreamEventTypeError || streamMsg.event.Error != testErr {
		t.Fatalf("unexpected stream event: %#v", streamMsg)
	}
}

func TestWaitForAsyncEvent_ChannelClosed(t *testing.T) {
	eventCh := make(chan llm.StreamEvent)
	close(eventCh)

	cmd := waitForAsyncEvent(eventCh, make(chan *replpermissions.Request), make(chan repltooling.DiffRequest), nil)
	msg := cmd()

	streamMsg, ok := msg.(mainStreamMsg)
	if !ok {
		t.Fatalf("expected mainStreamMsg, got %T", msg)
	}
	if !streamMsg.closed {
		t.Fatalf("expected closed stream message, got %#v", streamMsg)
	}
}

func TestFormatResponseLines(t *testing.T) {
	input := "Line 1\nLine 2\nLine 3"
	result := formatResponseLines(input)

	if len(result) != 3 {
		t.Errorf("expected 3 lines, got %d", len(result))
	}
	if result[0] != "  Line 1" {
		t.Errorf("expected '  Line 1', got '%s'", result[0])
	}
	if result[1] != "  Line 2" {
		t.Errorf("expected '  Line 2', got '%s'", result[1])
	}
}

func TestFormatResponseLines_Empty(t *testing.T) {
	result := formatResponseLines("")
	if len(result) != 1 {
		t.Errorf("expected 1 line for empty input, got %d", len(result))
	}
}

func TestStreamHandler_Start(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)

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
	eventCh := make(chan llm.StreamEvent)

	sh.Start(eventCh, "First")
	sh.HandleChunk("previous content")

	newEventCh := make(chan llm.StreamEvent)
	sh.Start(newEventCh, "Second")

	if sh.GetResponse() != "" {
		t.Error("expected response to be reset after new Start")
	}
	if sh.GetLoadingText() != "Second" {
		t.Error("expected loading text to be updated")
	}
}

func TestWaitForAsyncEvent_Permission(t *testing.T) {
	permissionCh := make(chan *replpermissions.Request, 1)
	req := makeTestPermissionRequest(false)
	permissionCh <- req

	cmd := waitForAsyncEvent(make(chan llm.StreamEvent), permissionCh, make(chan repltooling.DiffRequest), nil)
	msg := cmd()

	permissionMsg, ok := msg.(permissionReadyMsg)
	if !ok {
		t.Fatalf("expected permissionReadyMsg, got %T", msg)
	}
	if permissionMsg.req != req {
		t.Fatal("expected permission request payload to round-trip")
	}
}

func TestWaitForAsyncEvent_Diff(t *testing.T) {
	diffCh := make(chan repltooling.DiffRequest, 1)
	req := repltooling.DiffRequest{Done: make(chan struct{})}
	diffCh <- req

	cmd := waitForAsyncEvent(make(chan llm.StreamEvent), make(chan *replpermissions.Request), diffCh, nil)
	msg := cmd()

	diffMsg, ok := msg.(diffReadyMsg)
	if !ok {
		t.Fatalf("expected diffReadyMsg, got %T", msg)
	}
	if diffMsg.req.Done != req.Done {
		t.Fatal("expected diff request payload to round-trip")
	}
}

var _ tea.Msg = llmChunkMsg("")
var _ tea.Msg = llmReasoningChunkMsg("")
var _ tea.Msg = llmDoneMsg{}
var _ tea.Msg = llmErrorMsg{}
var _ tea.Msg = mainStreamMsg{}
var _ tea.Msg = permissionReadyMsg{}
var _ tea.Msg = diffReadyMsg{}

func makeTestPermissionRequest(isDangerous bool) *replpermissions.Request {
	return &replpermissions.Request{
		RequestID:    "test-1",
		ToolName:     "read_file",
		Path:         "../secret.txt",
		ResolvedPath: "/home/user/secret.txt",
		IsDangerous:  isDangerous,
		Status:       replpermissions.StatusPending,
		ResponseChan: make(chan bool, 1),
	}
}

func TestStreamHandler_HandlePermissionRequest_AddsSegment(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	if len(sh.segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(sh.segments))
	}
	if sh.segments[0].kind != segmentPermission {
		t.Errorf("expected segmentPermission, got %q", sh.segments[0].kind)
	}
	if sh.segments[0].permissionReq != req {
		t.Error("expected permission request to be stored in segment")
	}
}

func TestStreamHandler_HasPendingPermission_True(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	if !sh.HasPendingPermission() {
		t.Error("expected HasPendingPermission to be true")
	}
}

func TestStreamHandler_HasPendingPermission_FalseWhenResolved(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(replpermissions.StatusAllowed)

	if sh.HasPendingPermission() {
		t.Error("expected HasPendingPermission to be false after resolution")
	}
}

func TestStreamHandler_MovePendingCursor(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	sh.MovePendingCursor(1)
	if sh.segments[0].permissionCursor != 1 {
		t.Errorf("expected cursor at 1, got %d", sh.segments[0].permissionCursor)
	}

	sh.MovePendingCursor(100)
	if sh.segments[0].permissionCursor != 3 {
		t.Errorf("expected cursor clamped at 3, got %d", sh.segments[0].permissionCursor)
	}

	sh.MovePendingCursor(-100)
	if sh.segments[0].permissionCursor != 0 {
		t.Errorf("expected cursor clamped at 0, got %d", sh.segments[0].permissionCursor)
	}
}

func TestStreamHandler_GetPendingChoice_NonDangerous(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	if sh.GetPendingChoice() != replpermissions.ChoiceAllow {
		t.Error("expected initial choice to be Allow")
	}

	sh.MovePendingCursor(1)
	if sh.GetPendingChoice() != replpermissions.ChoiceAllowSession {
		t.Error("expected choice at cursor 1 to be AllowSession")
	}

	sh.MovePendingCursor(1)
	if sh.GetPendingChoice() != replpermissions.ChoiceDeny {
		t.Error("expected choice at cursor 2 to be Deny")
	}
}

func TestStreamHandler_GetPendingChoice_Dangerous(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(true)
	sh.HandlePermissionRequest(req)

	sh.MovePendingCursor(1)
	if sh.GetPendingChoice() != replpermissions.ChoiceDeny {
		t.Error("expected cursor 1 to be Deny for dangerous (no AllowSession)")
	}
}

func TestStreamHandler_ResolvePendingPermission(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(replpermissions.StatusAllowedSession)

	if sh.segments[0].permissionReq.Status != replpermissions.StatusAllowedSession {
		t.Errorf("expected status AllowedSession, got %q", sh.segments[0].permissionReq.Status)
	}
}

func TestRenderPermissionCard_Pending(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)

	view := sh.View(80)

	if !strings.Contains(view, "Permission Required") {
		t.Error("expected 'Permission Required' in pending card")
	}
	if !strings.Contains(view, "read_file") {
		t.Error("expected tool name in card")
	}
	if !strings.Contains(view, "Allow for this session") {
		t.Error("expected 'Allow for this session' choice in card")
	}
	if !strings.Contains(view, "↑/↓") {
		t.Error("expected keyboard hint in card")
	}
}

func TestRenderPermissionCard_Dangerous(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(true)
	sh.HandlePermissionRequest(req)

	view := sh.View(80)

	if !strings.Contains(view, "Allow Dangerous Command") {
		t.Error("expected dangerous warning in card")
	}
	if strings.Contains(view, "Allow for this session") {
		t.Error("expected no 'Allow for this session' for dangerous operations")
	}
}

func TestRenderPermissionCard_Resolved_Allowed(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(replpermissions.StatusAllowed)

	view := sh.View(80)

	if !strings.Contains(view, "✓") {
		t.Error("expected checkmark in resolved allowed card")
	}
	if strings.Contains(view, "Permission Required") {
		t.Error("expected no card title in resolved state")
	}
}

func TestRenderPermissionCard_Resolved_Denied(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(replpermissions.StatusDenied)

	view := sh.View(80)

	if !strings.Contains(view, "✗") {
		t.Error("expected X mark in resolved denied card")
	}
}

func TestRenderDiffSegment_RendersRulesUsingViewportWidth(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")
	sh.HandleDiff([]tools.EditDiffLine{
		{Kind: tools.DiffLineHunk, Content: "@@ -1,2 +1,3 @@"},
		{Kind: tools.DiffLineRemoved, Content: strings.Repeat("short ", 6), OldLineNum: 1},
		{Kind: tools.DiffLineAdded, Content: strings.Repeat("shorter ", 6), NewLineNum: 1},
	})

	wideView := sh.View(80)
	wideLines := strings.Split(strings.TrimRight(wideView, "\n"), "\n")
	if len(wideLines) < 5 {
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

func TestRenderPermissionCard_PreviewTruncation(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	var previewLines []string
	for i := range permissionPreviewMaxLines + 10 {
		previewLines = append(previewLines, strings.Repeat("x", i%40))
	}
	req.Preview = strings.Join(previewLines, "\n")
	sh.HandlePermissionRequest(req)

	view := sh.View(80)

	if !strings.Contains(view, "more preview lines omitted") {
		t.Error("expected truncation message in card with long preview")
	}
}

func TestRenderPermissionCard_LongPathWrapsWithinWidth(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(false)
	req.Path = "/very/long/path/" + strings.Repeat("nested-directory/", 12) + "file.go"
	req.ResolvedPath = "/Users/example/" + strings.Repeat("really-long-segment/", 10) + "file.go"
	sh.HandlePermissionRequest(req)

	width := 50
	view := sh.View(width)

	for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("line exceeds viewport width (%d > %d): %q", w, width, line)
		}
	}

	if !strings.Contains(view, "Path:") {
		t.Error("expected Path field to be present")
	}
	if !strings.Contains(view, "Resolved:") {
		t.Error("expected Resolved field to be present")
	}
}

func TestRenderPermissionCard_LongDangerousCommandWrapsWithinWidth(t *testing.T) {
	sh := NewStreamHandler(nil)
	sh.Start(make(<-chan llm.StreamEvent), "Loading...")

	req := makeTestPermissionRequest(true)
	req.Path = "rm -rf " + strings.Repeat("/tmp/very-long-segment-name/", 12)
	sh.HandlePermissionRequest(req)

	width := 48
	view := sh.View(width)

	for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		if w := lipgloss.Width(line); w > width {
			t.Fatalf("line exceeds viewport width (%d > %d): %q", w, width, line)
		}
	}

	if !strings.Contains(view, "Allow Dangerous Command") {
		t.Error("expected dangerous command title to be present")
	}
}

func TestPermissionTranscript_ResolvedBeforeDone(t *testing.T) {
	sh := NewStreamHandler(nil)
	eventCh := make(chan llm.StreamEvent)
	sh.Start(eventCh, "Loading...")

	sh.HandleChunk("before permission")

	req := makeTestPermissionRequest(false)
	sh.HandlePermissionRequest(req)
	sh.ResolvePendingPermission(replpermissions.StatusAllowedSession)

	sh.HandleChunk(" after permission")

	lines, _ := sh.HandleDone()

	foundBefore, foundStatus, foundAfter := false, false, false
	for _, l := range lines {
		if strings.Contains(l, "before permission") {
			foundBefore = true
		}
		if strings.Contains(l, "✓") && strings.Contains(l, "this session") {
			foundStatus = true
		}
		if strings.Contains(l, "after permission") {
			foundAfter = true
		}
	}

	if !foundBefore {
		t.Error("expected 'before permission' in transcript")
	}
	if !foundStatus {
		t.Error("expected resolved permission status line in transcript")
	}
	if !foundAfter {
		t.Error("expected 'after permission' in transcript")
	}
}

func TestHandleKeyMsg_PermissionEnter_ResolvesAllowed(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.stream.handler.HandlePermissionRequest(req)

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})

	if newM.stream.handler.HasPendingPermission() {
		t.Error("expected permission to be resolved after Enter")
	}
	if req.Status != replpermissions.StatusAllowed {
		t.Errorf("expected status Allowed, got %q", req.Status)
	}
}

func TestHandleKeyMsg_PermissionEsc_Denies(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.stream.handler.HandlePermissionRequest(req)

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEsc})

	if newM.stream.handler.HasPendingPermission() {
		t.Error("expected permission to be resolved after Esc")
	}
	if req.Status != replpermissions.StatusDenied {
		t.Errorf("expected status Denied, got %q", req.Status)
	}
}

func TestHandleKeyMsg_PermissionEnter_AllowSession(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.stream.handler.HandlePermissionRequest(req)

	m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})

	if req.Status != replpermissions.StatusAllowedSession {
		t.Errorf("expected status AllowedSession, got %q", req.Status)
	}
	if !newM.permissionRequester.IsSessionAllowed("read_file") {
		t.Error("expected read_file to be session-allowed after AllowSession choice")
	}
}

func TestHandleKeyMsg_NonPermissionKey_PassesToTextarea(t *testing.T) {
	m := newTestModel()
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.stream.handler.HandlePermissionRequest(req)

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'a', Text: "a"})

	if !newM.stream.handler.HasPendingPermission() {
		t.Error("expected permission to still be pending after non-permission key")
	}
}

func TestHandleKeyMsg_Enter_WhenPermissionPending_DoesNotSubmit(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("some user input")
	eventCh := make(chan llm.StreamEvent)
	m.stream.handler.Start(eventCh, "Loading...")

	req := makeTestPermissionRequest(false)
	m.stream.handler.HandlePermissionRequest(req)

	newM, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})

	if newM.textarea.Value() != "some user input" {
		t.Error("expected textarea to keep its value when Enter resolves permission")
	}
	if newM.stream.handler.HasPendingPermission() {
		t.Error("expected permission to be resolved")
	}
}

func TestStreamHandlerCheckpointPreservesActiveStream(t *testing.T) {
	events := make(chan llm.StreamEvent)
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
