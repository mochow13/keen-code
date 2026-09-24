package repl

import (
	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/llm/core"
	"strings"
	"testing"
	"time"

	"github.com/mochow13/keen-code/internal/session"
	"github.com/mochow13/keen-code/internal/tools"
)

func TestSessionReplay_InterruptedTurnRendersTranscriptAndPrompt(t *testing.T) {
	replay := newSessionReplay(80, nil, "")
	replay.applyEvent(session.Event{
		Kind: session.KindAssistantTurn,
		AssistantTurn: &session.AssistantTurnPayload{
			Transcript: []session.TranscriptItem{
				{Kind: session.TranscriptItemText, Content: "partial reply"},
			},
			Message:     "partial reply\n\n[Response interrupted by user]",
			Interrupted: true,
		},
	})

	joined := replay.output.Join()
	if !strings.Contains(joined, "partial reply") {
		t.Fatalf("expected partial reply in replay output, got %q", joined)
	}
	if !strings.Contains(joined, "Interrupted") {
		t.Fatalf("expected interrupted prompt in replay output, got %q", joined)
	}
}

func TestSessionReplay_ErrorTurnRendersTranscriptAndError(t *testing.T) {
	replay := newSessionReplay(80, nil, "")
	replay.applyEvent(session.Event{
		Kind: session.KindAssistantTurn,
		AssistantTurn: &session.AssistantTurnPayload{
			Transcript: []session.TranscriptItem{
				{Kind: session.TranscriptItemReasoning, Content: "thinking"},
				{Kind: session.TranscriptItemText, Content: "partial reply"},
			},
			Error: "stream failed",
		},
	})

	joined := replay.output.Join()
	if !strings.Contains(joined, "partial reply") {
		t.Fatalf("expected partial reply in replay output, got %q", joined)
	}
	if !strings.Contains(joined, "stream failed") {
		t.Fatalf("expected error message in replay output, got %q", joined)
	}
}

func TestSessionReplay_CompactionRendersTranscript(t *testing.T) {
	replay := newSessionReplay(80, nil, "")
	replay.applyEvent(session.Event{
		Kind: session.KindCompactionApplied,
		CompactionApplied: &session.CompactionAppliedPayload{
			Status: "Context compacted.",
			Transcript: []session.TranscriptItem{
				{Kind: session.TranscriptItemReasoning, Content: "condensing"},
				{Kind: session.TranscriptItemText, Content: "summary"},
			},
			Messages: []core.Message{
				{Role: core.RoleUser, Content: "summary"},
			},
		},
	})

	joined := replay.output.Join()
	if !strings.Contains(joined, "condensing") {
		t.Fatalf("expected compaction reasoning in replay output, got %q", joined)
	}
	if !strings.Contains(joined, "summary") {
		t.Fatalf("expected compaction summary in replay output, got %q", joined)
	}
	if strings.Contains(joined, "Context compacted.") {
		t.Fatalf("expected replay to match streamed compaction output without status line, got %q", joined)
	}
}

func TestSessionReplay_CompactionFallsBackToLegacyStatus(t *testing.T) {
	replay := newSessionReplay(80, nil, "")
	replay.applyEvent(session.Event{
		Kind: session.KindCompactionApplied,
		CompactionApplied: &session.CompactionAppliedPayload{
			Status: "Context compacted.",
		},
	})

	joined := replay.output.Join()
	if !strings.Contains(joined, "Context compacted.") {
		t.Fatalf("expected legacy compaction status in replay output, got %q", joined)
	}
}

func TestSessionReplay_RendersAskUserActivityInOrder(t *testing.T) {
	replay := newSessionReplay(80, nil, "")
	input := map[string]any{"questions": []any{map[string]any{"question": "Pick a mode", "options": []any{"Build", "Plan"}}}}
	replay.applyEvent(session.Event{
		Kind: session.KindAssistantTurn,
		AssistantTurn: &session.AssistantTurnPayload{Transcript: []session.TranscriptItem{
			{Kind: session.TranscriptItemText, Content: "Before"},
			{Kind: session.TranscriptItemToolStart, ToolStart: &session.ToolStartPayload{Name: tools.AskUserToolName, Input: input}},
			{Kind: session.TranscriptItemToolEnd, ToolEnd: &session.ToolEndPayload{Name: tools.AskUserToolName, Input: input, Output: map[string]any{"tool": tools.AskUserToolName, "answers": []any{"Plan"}, "cancelled": false}}},
			{Kind: session.TranscriptItemText, Content: "After"},
		}},
	})
	got := replay.output.Join()
	for _, text := range []string{"Before", "Pick a mode: Plan", "After"} {
		if !strings.Contains(got, text) {
			t.Fatalf("expected %q in ask_user replay output, got %q", text, got)
		}
	}
	if before, answer, after := strings.Index(got, "Before"), strings.Index(got, "Pick a mode: Plan"), strings.Index(got, "After"); !(before < answer && answer < after) {
		t.Fatalf("ask_user replay output is out of order: %q", got)
	}
}

func TestBuildAssistantTurnEvent_MixedTranscript(t *testing.T) {
	diffLines := []agentcore.EditDiffLine{
		{Kind: agentcore.EditDiffLineAdded, Content: "added", NewLineNum: 1},
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
			toolCall: &agentcore.ToolCall{
				Duration: 7 * time.Millisecond,
			},
		},
		{kind: segmentDiff, diffLines: diffLines},
	}

	event := buildAssistantTurnEvent(segments, agentcore.Message{
		Role:    agentcore.RoleAssistant,
		Content: "final answer",
		TurnMemory: &agentcore.TurnMemory{
			ToolActivity: []agentcore.HistoricalToolActivity{{Tool: "write_file", Input: map[string]any{"path": "a.go", "content": "content"}, Status: "success"}},
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
