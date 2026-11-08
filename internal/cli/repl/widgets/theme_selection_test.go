package widgets

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNewThemeSelectionPreselectsCurrent(t *testing.T) {
	sel := NewThemeSelection("dracula")
	if sel.SelectedName() != "dracula" {
		t.Fatalf("SelectedName = %q, want dracula", sel.SelectedName())
	}
	sel = NewThemeSelection("unknown-fallback")
	if sel.SelectedName() != "default" {
		t.Fatalf("SelectedName = %q, want default", sel.SelectedName())
	}
}

func TestThemeSelectionNavigationWraps(t *testing.T) {
	sel := NewThemeSelection("default")
	sel, _ = sel.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if got := sel.SelectedName(); got != "monochrome" {
		t.Fatalf("up wrap = %q, want monochrome", got)
	}
	sel, _ = sel.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if got := sel.SelectedName(); got != "default" {
		t.Fatalf("down wrap = %q, want default", got)
	}
}

func TestThemeSelectionEnterAndEsc(t *testing.T) {
	sel := NewThemeSelection("claude")
	_, cmd := sel.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected complete cmd on enter")
	}
	if _, ok := cmd().(ThemeSelectionCompleteMsg); !ok {
		t.Fatalf("enter msg = %T, want ThemeSelectionCompleteMsg", cmd())
	}

	sel = NewThemeSelection("claude")
	_, cmd = sel.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("expected cancel cmd on esc")
	}
	if _, ok := cmd().(ThemeSelectionCancelMsg); !ok {
		t.Fatalf("esc msg = %T, want ThemeSelectionCancelMsg", cmd())
	}
}

func TestThemeSelectionViewListsOptions(t *testing.T) {
	sel := NewThemeSelection("default")
	view := sel.View(80)
	for _, opt := range []string{"Default (Indigo)", "Claude Code", "Dracula", "Tokyo Night", "Catppuccin", "GitHub", "Nord", "Monochrome"} {
		if !strings.Contains(view, opt) {
			t.Fatalf("view missing %q", opt)
		}
	}
}

func TestThemeSelectionViewShowsAccentAndErrorPreview(t *testing.T) {
	sel := NewThemeSelection("default")
	view := sel.View(80)
	for _, s := range []string{"warn", "error", "██"} {
		if !strings.Contains(view, s) {
			t.Fatalf("view missing %q", s)
		}
	}
}
