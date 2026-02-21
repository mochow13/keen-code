package llm

import (
	"testing"

	"github.com/user/keen-cli/internal/tools"
)

func TestToGenkitTool(t *testing.T) {
	dummyTool := tools.NewDummyTool()
	genkitTool := ToGenkitTool(dummyTool)

	if genkitTool == nil {
		t.Fatal("expected non-nil genkit tool")
	}

	// The genkit tool should have the same name
	if genkitTool.Name() != "dummy_echo" {
		t.Errorf("expected tool name 'dummy_echo', got %q", genkitTool.Name())
	}

	// Check definition contains expected description
	def := genkitTool.Definition()
	expectedDesc := "Echoes back the input message with a timestamp."
	if def == nil || len(def.Description) < len(expectedDesc) || def.Description[:len(expectedDesc)] != expectedDesc {
		t.Errorf("unexpected tool description")
	}
}

func TestToGenkitTools(t *testing.T) {
	registry := tools.NewRegistry()

	// Empty registry should return empty slice
	genkitTools := ToGenkitTools(registry)
	if len(genkitTools) != 0 {
		t.Errorf("expected 0 tools for empty registry, got %d", len(genkitTools))
	}

	// Register a tool
	if err := registry.Register(tools.NewDummyTool()); err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	// Should return 1 tool
	genkitTools = ToGenkitTools(registry)
	if len(genkitTools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(genkitTools))
	}
}
