package commands

import "testing"

func TestFilterIncludesSkillsReloadSuggestion(t *testing.T) {
	results := Filter("/skills r")

	for _, result := range results {
		if result.Name == SkillsReload {
			return
		}
	}

	t.Fatalf("expected %q suggestion, got %#v", SkillsReload, results)
}

func TestFilterIncludesSkillsStatusSuggestion(t *testing.T) {
	results := Filter("/skills s")

	for _, result := range results {
		if result.Name == SkillsStatus {
			return
		}
	}

	t.Fatalf("expected %q suggestion, got %#v", SkillsStatus, results)
}

func TestIsKnownCommand(t *testing.T) {
	for _, input := range []string{Help, Model + " openai/gpt", SkillsEnable + " demo"} {
		if !IsKnownCommand(input) {
			t.Errorf("IsKnownCommand(%q) = false", input)
		}
	}
	for _, input := range []string{"", "help", "/unknown", Help + "ful"} {
		if IsKnownCommand(input) {
			t.Errorf("IsKnownCommand(%q) = true", input)
		}
	}
}

func TestYoloCommandRemoved(t *testing.T) {
	for _, cmd := range All {
		if cmd.Name == "/yolo" {
			t.Fatalf("unexpected /yolo in All")
		}
	}
	for _, cmd := range Suggestions {
		if cmd.Name == "/yolo" {
			t.Fatalf("unexpected /yolo in Suggestions")
		}
	}
	if IsKnownCommand("/yolo") {
		t.Fatal("expected IsKnownCommand(/yolo) = false")
	}
	for _, result := range Filter("/yolo") {
		if result.Name == "/yolo" {
			t.Fatalf("unexpected /yolo suggestion, got %#v", result)
		}
	}
}

func TestUsageCommandRegisteredAndSuggested(t *testing.T) {
	if !IsKnownCommand(Usage) {
		t.Fatalf("expected %q to be a known command", Usage)
	}
	for _, result := range Filter("/us") {
		if result.Name == Usage {
			return
		}
	}
	t.Fatalf("expected %q to be suggested", Usage)
}
