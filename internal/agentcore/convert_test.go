package agentcore

import (
	"testing"

	"github.com/mochow13/keen-code/internal/llm/core"
)

func TestCloneTurnMemoryClonesCompressedRetainedOutput(t *testing.T) {
	original := &TurnMemory{ToolActivity: []HistoricalToolActivity{{
		Tool: "grep",
		RetainedOutput: map[string]any{
			"common_prefix": "internal/llm/",
			"matches": map[string][]map[string]any{
				"message.go": {{"line": "original"}},
			},
		},
	}}}

	cloned := CloneTurnMemory(original)
	retained := cloned.ToolActivity[0].RetainedOutput.(map[string]any)
	matches := retained["matches"].(map[string][]map[string]any)
	matches["message.go"][0]["line"] = "changed"
	matches["other.go"] = []map[string]any{{"line": "new"}}

	originalMatches := original.ToolActivity[0].RetainedOutput.(map[string]any)["matches"].(map[string][]map[string]any)
	if got := originalMatches["message.go"][0]["line"]; got != "original" {
		t.Fatalf("original nested match mutated: %v", got)
	}
	if _, exists := originalMatches["other.go"]; exists {
		t.Fatal("original matches map mutated")
	}
}

func TestToStreamEventPassesThroughUnknownTypes(t *testing.T) {
	event := toStreamEvent(core.StreamEvent{Type: "future_event", Content: "payload"})
	if event.Type != "future_event" || event.Content != "payload" {
		t.Fatalf("got %#v", event)
	}
}

func TestRoleConversionPassesThroughUnknownValues(t *testing.T) {
	if got := toRole("tool"); got != "tool" {
		t.Fatalf("toRole() = %q", got)
	}
	if got := fromRole("tool"); got != "tool" {
		t.Fatalf("fromRole() = %q", got)
	}
	msg := fromMessage(toMessage(core.Message{Role: "tool", Content: "call"}))
	if msg.Role != "tool" || msg.Content != "call" {
		t.Fatalf("role round-trip = %#v", msg)
	}
}

func TestModeConversionPassesThroughUnknownValues(t *testing.T) {
	if got := toMode("explore"); got != "explore" {
		t.Fatalf("toMode() = %q", got)
	}
	if got := fromMode("explore"); got != "explore" {
		t.Fatalf("fromMode() = %q", got)
	}
}
