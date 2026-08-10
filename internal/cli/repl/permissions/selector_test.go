package permissions

import "testing"

func TestChoices(t *testing.T) {
	if got := len(Choices(true)); got != 3 {
		t.Fatalf("dangerous choices = %d, want 3", got)
	}
	if got := len(Choices(false)); got != 4 {
		t.Fatalf("safe choices = %d, want 4", got)
	}
}

func TestChoiceAt(t *testing.T) {
	for _, tt := range []struct {
		cursor    int
		dangerous bool
		want      Choice
	}{
		{0, true, ChoiceAllow}, {1, true, ChoiceDeny}, {2, true, ChoiceAskWhatToDo},
		{0, false, ChoiceAllow}, {1, false, ChoiceAllowSession}, {2, false, ChoiceDeny}, {3, false, ChoiceAskWhatToDo},
	} {
		if got := ChoiceAt(tt.cursor, tt.dangerous); got != tt.want {
			t.Errorf("ChoiceAt(%d, %v) = %v, want %v", tt.cursor, tt.dangerous, got, tt.want)
		}
	}
}
