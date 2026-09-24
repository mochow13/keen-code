package repl

import (
	"testing"

	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/session"
)

func TestCloneStreamSegments_DeepCopiesMutableFields(t *testing.T) {
	segments := []streamSegment{
		{
			kind:     segmentToolStart,
			toolCall: toolCallFromPayload(&session.ToolStartPayload{Name: "read_file", Input: map[string]any{"path": "go.mod"}}),
		},
		{
			kind:      segmentDiff,
			diffLines: []agentcore.EditDiffLine{{Kind: agentcore.EditDiffLineAdded, Content: "added", NewLineNum: 1}},
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
