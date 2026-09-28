package stream

import (
	"testing"

	"github.com/mochow13/keen-code/internal/agentcore"
	replaskuser "github.com/mochow13/keen-code/internal/cli/repl/askuser"
	replpermissions "github.com/mochow13/keen-code/internal/cli/repl/permissions"
)

func TestHandlerChunkAndSnapshot(t *testing.T) {
	h := NewStreamHandler(nil, WithWorkingDir("/repo"), WithShowThinking(true))
	h.Start(make(chan agentcore.StreamEvent), "Loading...")
	h.HandleChunk("Hello")
	h.HandleChunk(" World")
	if got := h.GetResponse(); got != "Hello World" {
		t.Fatalf("response = %q", got)
	}
	snap := h.Snapshot()
	if len(snap) != 1 || snap[0].Kind != SegmentAssistant || snap[0].Content != "Hello World" {
		t.Fatalf("snapshot = %#v", snap)
	}
	snap[0].Content = "mutated"
	if got := h.Snapshot()[0].Content; got != "Hello World" {
		t.Fatalf("snapshot not isolated, got %q", got)
	}
}

func TestHandlerOptionsAndAccessors(t *testing.T) {
	h := NewStreamHandler(nil, WithWorkingDir("/w"), WithShowThinking(false), WithWidth(42))
	if h.WorkingDir() != "/w" {
		t.Fatalf("working dir = %q", h.WorkingDir())
	}
	if h.ShowThinking() {
		t.Fatal("expected showThinking false")
	}
	if h.LastWidth() != 42 {
		t.Fatalf("width = %d", h.LastWidth())
	}
	h.SetWorkingDir("/x")
	h.SetShowThinking(true)
	h.SetLastWidth(80)
	if h.WorkingDir() != "/x" || !h.ShowThinking() || h.LastWidth() != 80 {
		t.Fatalf("accessors did not stick: %#v", h)
	}
	ch := make(chan agentcore.StreamEvent)
	h.Start(ch, "hi")
	if h.EventChannel() == nil {
		t.Fatal("expected event channel")
	}
}

func TestHandlerAskUserCard(t *testing.T) {
	h := NewStreamHandler(nil)
	h.Start(make(chan agentcore.StreamEvent), "Loading...")
	questionnaire := agentcore.AskUserRequest{Questions: []agentcore.AskUserQuestion{{Question: "Pick", Options: []string{"one"}}}}
	active := replaskuser.NewState(nil)
	active.Begin(&replaskuser.Request{Questionnaire: questionnaire})
	h.SetAskUser(active.Card())
	if len(h.Snapshot()) != 1 {
		t.Fatalf("expected ask-user segment, got %#v", h.Snapshot())
	}
	h.SetAskUser(nil)
	if len(h.Snapshot()) != 0 {
		t.Fatalf("expected removal, got %#v", h.Snapshot())
	}
	resolved := replaskuser.NewResolvedState(questionnaire, agentcore.AskUserResult{Answers: []string{"one"}})
	h.SetAskUser(resolved.Card())
	if got := h.View(80); got == "" {
		t.Fatal("expected resolved card view")
	}
}

func TestHandlerPermissionFlow(t *testing.T) {
	h := NewStreamHandler(nil)
	h.Start(make(chan agentcore.StreamEvent), "Loading...")
	req := &replpermissions.Request{ToolName: "bash", Status: replpermissions.StatusPending}
	h.HandlePermissionRequest(req)
	if !h.HasPendingPermission() {
		t.Fatal("expected pending permission")
	}
	h.MovePendingCursor(1)
	h.ResolvePendingPermission(replpermissions.StatusAllowed)
	if h.HasPendingPermission() {
		t.Fatal("expected resolved permission")
	}
}

func TestCloneSegmentsDeepCopies(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentToolStart, ToolCall: &agentcore.ToolCall{Name: "read_file", Input: map[string]any{"path": "a"}}},
		{Kind: SegmentDiff, DiffLines: []agentcore.EditDiffLine{{Kind: agentcore.EditDiffLineAdded, Content: "x"}}},
	}
	cloned := CloneSegments(segs)
	segs[0].ToolCall.Input["path"] = "b"
	segs[1].DiffLines[0].Content = "y"
	if cloned[0].ToolCall.Input["path"] != "a" || cloned[1].DiffLines[0].Content != "x" {
		t.Fatalf("clone not deep: %#v", cloned)
	}
	if FinalAssistantRun([]Segment{{Kind: SegmentAssistant, Content: "hi"}}) != "hi" {
		t.Fatal("FinalAssistantRun mismatch")
	}
	if !HasNonTextActivity([]Segment{{Kind: SegmentToolEnd}}) {
		t.Fatal("HasNonTextActivity mismatch")
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
