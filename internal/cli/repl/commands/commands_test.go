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
