package widgets

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	replcommands "github.com/user/keen-code/internal/cli/repl/commands"
)

func TestFilterCommandsEmpty(t *testing.T) {
	if got := replcommands.Filter(""); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestFilterCommandsC(t *testing.T) {
	got := replcommands.Filter("/c")
	if len(got) != 3 || got[0].Name != "/clear" || got[1].Name != "/compact" || got[2].Name != "/cleanup" {
		t.Errorf("expected /clear, /compact, and /cleanup, got %v", got)
	}
}

func TestFilterCommandsH(t *testing.T) {
	got := replcommands.Filter("/h")
	if len(got) != 1 || got[0].Name != "/help" {
		t.Errorf("expected /help only, got %v", got)
	}
}

func TestFilterCommandsM(t *testing.T) {
	got := replcommands.Filter("/m")
	if len(got) != 7 || got[0].Name != "/memory" || got[1].Name != "/memory show" || got[2].Name != "/mcp" || got[3].Name != "/mcp connect" || got[4].Name != "/mcp status" || got[5].Name != "/model" || got[6].Name != "/mode" {
		t.Errorf("expected /memory, /memory show, /mcp, /mcp connect, /mcp status, /model and /mode, got %v", got)
	}
}

func TestFilterCommandsE(t *testing.T) {
	got := replcommands.Filter("/e")
	if len(got) != 2 {
		t.Errorf("expected /emptyq and /exit, got %v", got)
	}
	names := []string{got[0].Name, got[1].Name}
	if !slices.Contains(names, "/exit") || !slices.Contains(names, "/emptyq") {
		t.Errorf("expected /emptyq and /exit, got %v", got)
	}
}

func TestFilterCommandsNoMatch(t *testing.T) {
	if got := replcommands.Filter("/xyz"); len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestFilterCommandsCaseInsensitive(t *testing.T) {
	got := replcommands.Filter("/EXIT")
	if len(got) != 1 || got[0].Name != "/exit" {
		t.Errorf("expected /exit, got %v", got)
	}
}

func TestFilterCommandsMCPConnect(t *testing.T) {
	got := replcommands.Filter("/mcp c")
	if len(got) != 1 || got[0].Name != "/mcp connect" {
		t.Errorf("expected /mcp connect, got %v", got)
	}
}

func TestFilterCommandsSkillsSubcommands(t *testing.T) {
	got := replcommands.Filter("/skills e")
	if len(got) != 1 || got[0].Name != "/skills enable" {
		t.Errorf("expected /skills enable, got %v", got)
	}

	got = replcommands.Filter("/skills d")
	if len(got) != 1 || got[0].Name != "/skills disable" {
		t.Errorf("expected /skills disable, got %v", got)
	}

	got = replcommands.Filter("/skills r")
	if len(got) != 1 || got[0].Name != "/skills reload" {
		t.Errorf("expected /skills reload, got %v", got)
	}

	got = replcommands.Filter("/skills s")
	if len(got) != 1 || got[0].Name != "/skills status" {
		t.Errorf("expected /skills status, got %v", got)
	}
}

func TestFilterCommandsExactMatch(t *testing.T) {
	got := replcommands.Filter("/help")
	if len(got) != 1 || got[0].Name != "/help" {
		t.Errorf("expected exactly /help, got %v", got)
	}
}

func TestSuggestionMoveDown(t *testing.T) {
	s := NewSuggestionModel()
	s.Refresh("/")
	s.selected = 0
	s.MoveDown()
	if s.selected != 1 {
		t.Errorf("expected 1, got %d", s.selected)
	}
	s.selected = len(s.items) - 1
	s.MoveDown()
	if s.selected != 0 {
		t.Errorf("expected wrap to 0, got %d", s.selected)
	}
}

func TestSuggestionMoveUp(t *testing.T) {
	s := NewSuggestionModel()
	s.Refresh("/")
	s.selected = 2
	s.MoveUp()
	if s.selected != 1 {
		t.Errorf("expected 1, got %d", s.selected)
	}
	s.selected = 0
	s.MoveUp()
	if s.selected != len(s.items)-1 {
		t.Errorf("expected wrap to %d, got %d", len(s.items)-1, s.selected)
	}
}

func TestSuggestionCurrentNilWhenInvisible(t *testing.T) {
	s := NewSuggestionModel()
	if s.Current() != nil {
		t.Error("expected nil when not visible")
	}
}

func TestSuggestionHeight(t *testing.T) {
	s := NewSuggestionModel()
	if s.Height() != 0 {
		t.Errorf("expected 0 when not visible, got %d", s.Height())
	}
	s.Refresh("/")
	expected := len(s.items)
	if expected > maxVisibleItems {
		expected = maxVisibleItems
	}
	if s.Height() != expected {
		t.Errorf("expected %d, got %d", expected, s.Height())
	}
}

func TestSuggestionViewMarksSelectedItem(t *testing.T) {
	s := NewSuggestionModel()
	s.RefreshFiles([]string{"first.go", "second.go"})
	s.MoveDown()

	lines := strings.Split(ansi.Strip(s.View(80)), "\n")
	if !strings.HasPrefix(lines[0], "   ") {
		t.Errorf("expected unselected first row to have an empty marker, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], " ▸ ") {
		t.Errorf("expected selected second row to have an arrow marker, got %q", lines[1])
	}
}

func TestSuggestionRefreshSlash(t *testing.T) {
	s := NewSuggestionModel()
	s.Refresh("/")
	if !s.visible {
		t.Error("expected visible after refresh('/')")
	}
	if len(s.items) == 0 {
		t.Error("expected items populated")
	}
}

func TestSuggestionRefreshWithSkills(t *testing.T) {
	s := NewSuggestionModel()
	s.RefreshWithSkills("/de", []SuggestionItem{{Name: "/deploy", Description: "Deploy app"}})
	if !s.visible {
		t.Fatal("expected suggestions visible")
	}
	if len(s.items) != 1 || s.items[0].Name != "/deploy" {
		t.Fatalf("unexpected items: %#v", s.items)
	}
}

func TestSuggestionRefreshEmpty(t *testing.T) {
	s := NewSuggestionModel()
	s.Refresh("/")
	s.Refresh("")
	if s.visible {
		t.Error("expected not visible after refresh('')")
	}
}

// File mode tests

func TestRefreshFilesVisible(t *testing.T) {
	s := NewSuggestionModel()
	s.RefreshFiles([]string{"internal/main.go", "internal/model.go"})
	if !s.visible {
		t.Error("expected visible after RefreshFiles")
	}
	if len(s.items) != 2 {
		t.Errorf("expected 2 items, got %d", len(s.items))
	}
	if s.items[0].Name != "internal/main.go" {
		t.Errorf("unexpected first item: %s", s.items[0].Name)
	}
}

func TestRefreshFilesEmptyHides(t *testing.T) {
	s := NewSuggestionModel()
	s.RefreshFiles([]string{})
	if s.visible {
		t.Error("expected not visible with empty file list")
	}
}

func TestIsFileMode(t *testing.T) {
	s := NewSuggestionModel()
	s.Refresh("/")
	if s.IsFileMode() {
		t.Error("command mode should not report file mode")
	}
	s.RefreshFiles([]string{"foo.go"})
	if !s.IsFileMode() {
		t.Error("expected file mode after RefreshFiles")
	}
}

func TestCurrentInFileMode(t *testing.T) {
	s := NewSuggestionModel()
	s.RefreshFiles([]string{"a.go", "b.go", "c.go"})
	s.MoveDown()
	cur := s.Current()
	if cur == nil || cur.Name != "b.go" {
		t.Errorf("expected b.go, got %v", cur)
	}
}

func TestNavigationFileMode(t *testing.T) {
	s := NewSuggestionModel()
	s.RefreshFiles([]string{"a.go", "b.go", "c.go"})
	s.MoveDown()
	s.MoveDown()
	s.MoveUp()
	cur := s.Current()
	if cur == nil || cur.Name != "b.go" {
		t.Errorf("expected b.go after down/down/up, got %v", cur)
	}
}
