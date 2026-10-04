package repl

import "testing"

func TestClassificationManagerTracksAndResetsComplexityExchange(t *testing.T) {
	manager := &classificationManager{}
	manager.RecordUserMessage("first request")
	manager.RecordAssistantMessage("first response")
	if len(manager.complexityExchange) != 2 {
		t.Fatalf("exchange length = %d, want 2", len(manager.complexityExchange))
	}

	manager.Reset()
	if len(manager.complexityExchange) != 0 {
		t.Fatalf("exchange length after reset = %d, want 0", len(manager.complexityExchange))
	}
}
