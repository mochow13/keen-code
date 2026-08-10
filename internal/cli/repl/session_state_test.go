package repl

import (
	"testing"
	"time"

	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/session"
	"github.com/mochow13/keen-code/internal/tools"
)

func TestBuildAssistantTurnEvent_MixedTranscript(t *testing.T) {
	diffLines := []tools.EditDiffLine{
		{Kind: tools.DiffLineAdded, Content: "added", NewLineNum: 1},
	}

	segments := []streamSegment{
		{kind: segmentAssistant, content: "draft"},
		{kind: segmentReasoning, content: "thinking"},
		{
			kind:     segmentToolStart,
			toolCall: toolCallFromPayload(&session.ToolStartPayload{Name: "read_file", Input: map[string]any{"path": "go.mod"}}),
		},
		{
			kind: segmentToolEnd,
			toolCall: toolCallResultFromPayload(&session.ToolEndPayload{
				Name:       "read_file",
				Input:      map[string]any{"path": "go.mod"},
				Output:     map[string]any{"content": "module github.com/user/keen-code"},
				DurationNS: int64(5 * time.Millisecond),
			}),
		},
		{
			kind:    segmentBash,
			command: "go test ./...",
			summary: "Run unit tests",
			output:  "ok",
			toolCall: &llm.ToolCall{
				Duration: 7 * time.Millisecond,
			},
		},
		{kind: segmentDiff, diffLines: diffLines},
	}

	event := buildAssistantTurnEvent(segments, llm.Message{
		Role:    llm.RoleAssistant,
		Content: "final answer",
		TurnMemory: &llm.TurnMemory{
			ToolActivity: []llm.HistoricalToolActivity{{Tool: "write_file", Input: map[string]any{"path": "a.go", "content": "content"}, Status: "success"}},
		},
	}, false, "")

	if event.Kind != session.KindAssistantTurn {
		t.Fatalf("expected assistant turn event, got %q", event.Kind)
	}
	if event.AssistantTurn == nil {
		t.Fatal("expected assistant turn payload")
	}
	if event.AssistantTurn.Message != "final answer" {
		t.Fatalf("unexpected assistant message %q", event.AssistantTurn.Message)
	}
	if event.AssistantTurn.TurnMemory == nil || len(event.AssistantTurn.TurnMemory.ToolActivity) != 1 || event.AssistantTurn.TurnMemory.ToolActivity[0].Input["path"] != "a.go" || event.AssistantTurn.TurnMemory.ToolActivity[0].Input["content"] != "content" {
		t.Fatalf("expected turn memory to be preserved, got %#v", event.AssistantTurn.TurnMemory)
	}
	if len(event.AssistantTurn.Transcript) != 6 {
		t.Fatalf("expected 6 transcript items, got %d", len(event.AssistantTurn.Transcript))
	}
	if event.AssistantTurn.Transcript[0].Kind != session.TranscriptItemText {
		t.Fatalf("expected first transcript item to be text, got %q", event.AssistantTurn.Transcript[0].Kind)
	}
	if event.AssistantTurn.Transcript[1].Kind != session.TranscriptItemReasoning {
		t.Fatalf("expected second transcript item to be reasoning, got %q", event.AssistantTurn.Transcript[1].Kind)
	}
	if event.AssistantTurn.Transcript[2].ToolStart == nil || event.AssistantTurn.Transcript[2].ToolStart.Name != "read_file" {
		t.Fatal("expected tool start payload")
	}
	if event.AssistantTurn.Transcript[3].ToolEnd == nil || event.AssistantTurn.Transcript[3].ToolEnd.Name != "read_file" {
		t.Fatal("expected tool end payload")
	}
	if event.AssistantTurn.Transcript[4].Bash == nil || event.AssistantTurn.Transcript[4].Bash.Command != "go test ./..." {
		t.Fatal("expected bash payload")
	}
	if event.AssistantTurn.Transcript[5].Diff == nil || len(event.AssistantTurn.Transcript[5].Diff.Lines) != 1 {
		t.Fatal("expected diff payload")
	}
}

func TestReplSessionState_SetSession(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)

	state := newReplSessionState(work)
	if state == nil {
		t.Fatal("newReplSessionState() returned nil")
	}

	if state.currentID() != "" {
		t.Fatalf("expected empty current ID, got %q", state.currentID())
	}

	sess := &session.Session{ID: "test-session-id"}
	state.setSession(sess)

	if state.currentID() != sess.ID {
		t.Fatalf("currentID() = %q, want %q", state.currentID(), sess.ID)
	}
}

func TestCloneStreamSegments_DeepCopiesMutableFields(t *testing.T) {
	segments := []streamSegment{
		{
			kind:     segmentToolStart,
			toolCall: toolCallFromPayload(&session.ToolStartPayload{Name: "read_file", Input: map[string]any{"path": "go.mod"}}),
		},
		{
			kind:      segmentDiff,
			diffLines: []tools.EditDiffLine{{Kind: tools.DiffLineAdded, Content: "added", NewLineNum: 1}},
		},
	}

	cloned := cloneStreamSegments(segments)

	segments[0].toolCall.Input["path"] = "go.sum"
	segments[1].diffLines[0].Content = "changed"

	if cloned[0].toolCall.Input["path"] != "go.mod" {
		t.Fatalf("expected cloned tool input to remain unchanged, got %v", cloned[0].toolCall.Input["path"])
	}
	if cloned[1].diffLines[0].Content != "added" {
		t.Fatalf("expected cloned diff content to remain unchanged, got %q", cloned[1].diffLines[0].Content)
	}
}
