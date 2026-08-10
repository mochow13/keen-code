package session

import (
	"testing"

	"github.com/mochow13/keen-code/internal/llm"
)

func TestBuildConversation_AppendsAssistantTurnMessages(t *testing.T) {
	events := []Event{
		{
			Kind:        KindUserMessage,
			UserMessage: &MessagePayload{Content: "user"},
		},
		{
			Kind: KindAssistantTurn,
			AssistantTurn: &AssistantTurnPayload{
				Message: "assistant",
				TurnMemory: &llm.TurnMemory{
					ToolActivity: []llm.HistoricalToolActivity{{Tool: "write_file", Input: map[string]any{"path": "a.go", "content": "content"}, Status: "success"}},
				},
				Interrupted: true,
				Error:       "ignored for conversation projection",
			},
		},
	}

	got := BuildConversation(events)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].Role != llm.RoleUser || got[0].Content != "user" {
		t.Fatalf("unexpected user message: %#v", got[0])
	}
	if got[1].Role != llm.RoleAssistant || got[1].Content != "assistant" {
		t.Fatalf("unexpected assistant message: %#v", got[1])
	}
	if got[1].TurnMemory == nil || len(got[1].TurnMemory.ToolActivity) != 1 || got[1].TurnMemory.ToolActivity[0].Input["path"] != "a.go" || got[1].TurnMemory.ToolActivity[0].Input["content"] != "content" {
		t.Fatalf("expected assistant turn memory to be preserved, got %#v", got[1].TurnMemory)
	}
}

func TestBuildConversation_IgnoresAssistantTurnWithoutMessage(t *testing.T) {
	events := []Event{
		{
			Kind: KindAssistantTurn,
			AssistantTurn: &AssistantTurnPayload{
				Error: "stream failed",
			},
		},
	}

	got := BuildConversation(events)
	if len(got) != 0 {
		t.Fatalf("expected no messages, got %#v", got)
	}
}

func TestBuildConversation_PreservesToolOnlyAssistantTurn(t *testing.T) {
	events := []Event{{
		Kind: KindAssistantTurn,
		AssistantTurn: &AssistantTurnPayload{
			TurnMemory: &llm.TurnMemory{ToolActivity: []llm.HistoricalToolActivity{{
				Tool:   "read_file",
				Status: "success",
			}}},
		},
	}}

	got := BuildConversation(events)
	if len(got) != 1 || got[0].Content != "" || got[0].TurnMemory.IsEmpty() {
		t.Fatalf("expected tool-only assistant turn, got %#v", got)
	}
}

func TestBuildConversation_CompactionCloneIsIndependent(t *testing.T) {
	compacted := []llm.Message{
		{Role: llm.RoleUser, Content: "summary"},
	}
	events := []Event{
		{
			Kind: KindCompactionApplied,
			CompactionApplied: &CompactionAppliedPayload{
				Status:   "Context compacted.",
				Messages: compacted,
			},
		},
	}

	got := BuildConversation(events)
	compacted[0].Content = "mutated"

	if got[0].Content != "summary" {
		t.Fatalf("expected cloned compacted content to remain unchanged, got %#v", got)
	}
}
