package repl

import (
	"testing"

	"github.com/mochow13/keen-code/internal/config"
)

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

func TestNewClassificationManagerInitialization(t *testing.T) {
	for _, tc := range []struct {
		name    string
		global  *config.GlobalConfig
		wantErr bool
	}{
		{name: "unconfigured"},
		{name: "disabled", global: &config.GlobalConfig{Decision: &config.DecisionConfig{}}},
		{name: "enabled without provider", global: &config.GlobalConfig{Decision: &config.DecisionConfig{Enabled: true}}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager, err := newClassificationManager(tc.global)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tc.wantErr)
			}
			if manager == nil {
				t.Fatal("expected manager even when initialization fails")
			}
		})
	}
}
