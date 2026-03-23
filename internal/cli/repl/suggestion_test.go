package repl

import (
	"testing"
)

func TestFilterCommandsEmpty(t *testing.T) {
	if got := filterCommands(""); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestFilterCommandsSlashOnly(t *testing.T) {
	got := filterCommands("/")
	if len(got) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(got))
	}
	if got[0].Name != "/exit" || got[1].Name != "/help" || got[2].Name != "/model" {
		t.Errorf("unexpected order: %v", got)
	}
}

func TestFilterCommandsH(t *testing.T) {
	got := filterCommands("/h")
	if len(got) != 1 || got[0].Name != "/help" {
		t.Errorf("expected /help only, got %v", got)
	}
}

func TestFilterCommandsM(t *testing.T) {
	got := filterCommands("/m")
	if len(got) != 1 || got[0].Name != "/model" {
		t.Errorf("expected /model only, got %v", got)
	}
}

func TestFilterCommandsE(t *testing.T) {
	got := filterCommands("/e")
	if len(got) != 1 || got[0].Name != "/exit" {
		t.Errorf("expected /exit only, got %v", got)
	}
}

func TestFilterCommandsNoMatch(t *testing.T) {
	if got := filterCommands("/xyz"); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestFilterCommandsCaseInsensitive(t *testing.T) {
	got := filterCommands("/EXIT")
	if len(got) != 1 || got[0].Name != "/exit" {
		t.Errorf("expected /exit, got %v", got)
	}
}

func TestFilterCommandsExactMatch(t *testing.T) {
	got := filterCommands("/help")
	if len(got) != 1 || got[0].Name != "/help" {
		t.Errorf("expected exactly /help, got %v", got)
	}
}

func TestSuggestionMoveDown(t *testing.T) {
	s := newSuggestionModel()
	s.refresh("/")
	s.selected = 0
	s.moveDown()
	if s.selected != 1 {
		t.Errorf("expected 1, got %d", s.selected)
	}
	// wrap at max
	s.selected = len(s.items) - 1
	s.moveDown()
	if s.selected != 0 {
		t.Errorf("expected wrap to 0, got %d", s.selected)
	}
}

func TestSuggestionMoveUp(t *testing.T) {
	s := newSuggestionModel()
	s.refresh("/")
	s.selected = 2
	s.moveUp()
	if s.selected != 1 {
		t.Errorf("expected 1, got %d", s.selected)
	}
	// wrap at 0
	s.selected = 0
	s.moveUp()
	if s.selected != len(s.items)-1 {
		t.Errorf("expected wrap to %d, got %d", len(s.items)-1, s.selected)
	}
}

func TestSuggestionCurrentNilWhenInvisible(t *testing.T) {
	s := newSuggestionModel()
	if s.current() != nil {
		t.Error("expected nil when not visible")
	}
}

func TestSuggestionHeight(t *testing.T) {
	s := newSuggestionModel()
	if s.height() != 0 {
		t.Errorf("expected 0 when not visible, got %d", s.height())
	}
	s.refresh("/")
	if s.height() != len(s.items)+2 {
		t.Errorf("expected %d, got %d", len(s.items)+2, s.height())
	}
}

func TestSuggestionRefreshSlash(t *testing.T) {
	s := newSuggestionModel()
	s.refresh("/")
	if !s.visible {
		t.Error("expected visible after refresh('/')")
	}
	if len(s.items) == 0 {
		t.Error("expected items populated")
	}
}

func TestSuggestionRefreshEmpty(t *testing.T) {
	s := newSuggestionModel()
	s.refresh("/")
	s.refresh("")
	if s.visible {
		t.Error("expected not visible after refresh('')")
	}
}
