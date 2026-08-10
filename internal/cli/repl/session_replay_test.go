package repl

import (
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/session"
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
