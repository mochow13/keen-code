package repl

import (
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/llm"
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
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "summary"},
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
