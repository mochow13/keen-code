package repl

import (
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"
	replcommands "github.com/mochow13/keen-code/internal/cli/repl/commands"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	replwidgets "github.com/mochow13/keen-code/internal/cli/repl/widgets"
	"github.com/mochow13/keen-code/internal/config"
)

func TestHandleThemeCommandSetValid(t *testing.T) {
	prev := repltheme.CurrentThemeName
	t.Cleanup(func() { repltheme.ApplyTheme(prev) })

	m := newTestModel()
	m.ctx.globalCfg = config.DefaultGlobalConfig()
	m.ctx.loader = config.NewLoaderWithPath(filepath.Join(t.TempDir(), "configs.json"))
	m = m.handleThemeCommand("/theme dracula")
	if repltheme.CurrentThemeName != "dracula" {
		t.Fatalf("CurrentThemeName = %q, want dracula", repltheme.CurrentThemeName)
	}
	if m.ctx.globalCfg.Theme != "dracula" {
		t.Fatalf("persisted Theme = %q, want dracula", m.ctx.globalCfg.Theme)
	}
	if !m.ctx.loader.Exists() {
		t.Fatal("expected config file to be written")
	}
}

func TestHandleThemeCommandShowsSelectedName(t *testing.T) {
	prev := repltheme.CurrentThemeName
	t.Cleanup(func() { repltheme.ApplyTheme(prev) })

	m := newTestModel()
	m.ctx.globalCfg = config.DefaultGlobalConfig()
	m.ctx.loader = config.NewLoaderWithPath(filepath.Join(t.TempDir(), "configs.json"))
	m = m.handleThemeCommand("/theme dracula")
	joined := strings.Join(m.output.GetLines(), "\n")
	if !strings.Contains(joined, "Selected theme Dracula") {
		t.Fatalf("expected selected theme message, got %q", joined)
	}
}

func TestHandleThemeCommandInvalid(t *testing.T) {
	prev := repltheme.CurrentThemeName
	t.Cleanup(func() { repltheme.ApplyTheme(prev) })
	repltheme.ApplyTheme("default")

	m := newTestModel()
	m = m.handleThemeCommand("/theme nope")
	if repltheme.CurrentThemeName != "default" {
		t.Fatalf("invalid theme changed theme to %q", repltheme.CurrentThemeName)
	}
	joined := strings.Join(m.output.GetLines(), "\n")
	if !strings.Contains(joined, "Usage: /theme") {
		t.Fatalf("expected usage error, got %q", joined)
	}
}

func TestThemeDispatchOpensPicker(t *testing.T) {
	prev := repltheme.CurrentThemeName
	t.Cleanup(func() { repltheme.ApplyTheme(prev) })

	m := newTestModel()
	updated, _, handled := m.dispatchCommand(replcommands.Theme)
	if !handled || updated.themeSelection == nil {
		t.Fatal("expected /theme to open picker")
	}
}

func TestConsumeThemeSelectionComplete(t *testing.T) {
	prev := repltheme.CurrentThemeName
	t.Cleanup(func() { repltheme.ApplyTheme(prev) })

	m := newTestModel()
	m.ctx.globalCfg = config.DefaultGlobalConfig()
	m.ctx.loader = config.NewLoaderWithPath(filepath.Join(t.TempDir(), "configs.json"))
	m.themeSelection = replwidgets.NewThemeSelection("default")
	updated, _, handled := m.consumeThemeSelectionResult(replwidgets.ThemeSelectionCompleteMsg{Name: "tokyonight"})
	if !handled || updated.themeSelection != nil {
		t.Fatal("expected picker to close on complete")
	}
	if repltheme.CurrentThemeName != "tokyonight" {
		t.Fatalf("CurrentThemeName = %q, want tokyonight", repltheme.CurrentThemeName)
	}
}

func TestConsumeThemeSelectionCancel(t *testing.T) {
	m := newTestModel()
	m.themeSelection = replwidgets.NewThemeSelection("default")
	updated, _, handled := m.consumeThemeSelectionResult(replwidgets.ThemeSelectionCancelMsg{})
	if !handled || updated.themeSelection != nil {
		t.Fatal("expected picker to close on cancel")
	}
}

func TestThemeSelectionKeyFlow(t *testing.T) {
	prev := repltheme.CurrentThemeName
	t.Cleanup(func() { repltheme.ApplyTheme(prev) })

	m := newTestModel()
	m.ctx.globalCfg = config.DefaultGlobalConfig()
	m.ctx.loader = config.NewLoaderWithPath(filepath.Join(t.TempDir(), "configs.json"))
	m = m.startThemeSelection()
	if m.themeSelection == nil {
		t.Fatal("expected picker to open")
	}
	updated, cmd := m.handleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected complete cmd from picker enter")
	}
	picked := m.themeSelection.SelectedName()
	updated, _, handled := updated.consumeThemeSelectionResult(cmd())
	if !handled {
		t.Fatal("expected consume to handle complete msg")
	}
	if updated.themeSelection != nil {
		t.Fatal("expected picker to close")
	}
	if repltheme.CurrentThemeName != picked {
		t.Fatalf("CurrentThemeName = %q, want %q", repltheme.CurrentThemeName, picked)
	}
}

func TestApplyThemeSyncsSpinnerStyles(t *testing.T) {
	prev := repltheme.CurrentThemeName
	t.Cleanup(func() { repltheme.ApplyTheme(prev) })

	repltheme.ApplyTheme("default")
	m := newTestModel()
	m.ctx.globalCfg = config.DefaultGlobalConfig()
	m.syncSpinnerStyles()
	beforeAdversary := m.adversary.spinner.Style.Render("x")
	beforeLoading := m.loading.spinner.Style.Render("x")
	wantAdversary := lipgloss.NewStyle().Foreground(repltheme.SecondaryColor).Render("x")
	wantLoading := lipgloss.NewStyle().Foreground(repltheme.PrimaryColor).Render("x")
	if beforeAdversary != wantAdversary {
		t.Fatalf("adversary spinner = %q, want secondary %q", beforeAdversary, wantAdversary)
	}
	if beforeLoading != wantLoading {
		t.Fatalf("loading spinner = %q, want primary %q", beforeLoading, wantLoading)
	}

	if !m.applyTheme("dracula") {
		t.Fatal("applyTheme(dracula) = false")
	}
	afterAdversary := m.adversary.spinner.Style.Render("x")
	if afterAdversary == beforeAdversary {
		t.Fatal("adversary spinner did not follow Secondary on theme change")
	}
	if repltheme.AdversaryPromptStyle.Render("x") == repltheme.PromptStyle.Render("x") {
		t.Fatal("adversary prompt should follow Secondary, not Primary")
	}
	if repltheme.AdversaryChipStyle.Render("x") != repltheme.QueueChipStyle.Render("x") {
		t.Fatal("adversary chip should match Secondary-driven queue chip")
	}
}
