package theme

import (
	"image/color"
	"math"
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

func TestApplyThemeAllValid(t *testing.T) {
	prev := CurrentThemeName
	t.Cleanup(func() { ApplyTheme(prev) })

	for _, name := range ThemeNames() {
		if !ApplyTheme(name) {
			t.Fatalf("ApplyTheme(%q) = false, want true", name)
		}
		if CurrentThemeName != name {
			t.Fatalf("CurrentThemeName = %q, want %q", CurrentThemeName, name)
		}
	}
}

func TestApplyThemeInvalidKeepsCurrent(t *testing.T) {
	prev := CurrentThemeName
	t.Cleanup(func() { ApplyTheme(prev) })

	ApplyTheme("dracula")
	beforePrompt := PromptStyle.Render("x")
	beforeBox := BoxStyle.Render("x")
	if ApplyTheme("nope") {
		t.Fatal("ApplyTheme(nope) = true, want false")
	}
	if CurrentThemeName != "dracula" {
		t.Fatalf("CurrentThemeName = %q, want dracula", CurrentThemeName)
	}
	if PromptStyle.Render("x") != beforePrompt || BoxStyle.Render("x") != beforeBox {
		t.Fatal("invalid theme changed styles")
	}
}

func TestApplyThemeChangesPrimaryAndSecondary(t *testing.T) {
	prev := CurrentThemeName
	t.Cleanup(func() { ApplyTheme(prev) })

	ApplyTheme("default")
	defaultPrompt := PromptStyle.Render("x")
	defaultBox := BoxStyle.Render("x")
	ApplyTheme("dracula")
	if PromptStyle.Render("x") == defaultPrompt {
		t.Fatal("expected PromptStyle to change between default and dracula")
	}
	if BoxStyle.Render("x") == defaultBox {
		t.Fatal("expected Secondary-driven BoxStyle to change between default and dracula")
	}
}

func TestApplyThemePlanFollowsSecondary(t *testing.T) {
	prev := CurrentThemeName
	t.Cleanup(func() { ApplyTheme(prev) })

	ApplyTheme("default")
	defaultPrompt := AdversaryPromptStyle.Render("x")
	defaultRule := PlanInputRuleStyle.Render("x")
	defaultChip := ModePlanChipStyle.Render("x")
	defaultPlanPrompt := PromptPlanStyle.Render("x")
	ApplyTheme("dracula")
	if AdversaryPromptStyle.Render("x") == defaultPrompt {
		t.Fatal("expected AdversaryPromptStyle to follow Secondary between default and dracula")
	}
	if PlanInputRuleStyle.Render("x") == defaultRule {
		t.Fatal("expected PlanInputRuleStyle to follow Secondary between default and dracula")
	}
	if ModePlanChipStyle.Render("x") == defaultChip {
		t.Fatal("expected ModePlanChipStyle to follow Secondary between default and dracula")
	}
	if PromptPlanStyle.Render("x") == defaultPlanPrompt {
		t.Fatal("expected PromptPlanStyle to follow Secondary between default and dracula")
	}
	for _, name := range ThemeNames() {
		ApplyTheme(name)
		if AdversaryChipStyle.Render("x") != QueueChipStyle.Render("x") {
			t.Fatalf("AdversaryChipStyle should match Secondary-driven QueueChipStyle under theme %q", name)
		}
		if AdversaryInputRuleStyle.Render("x") != PlanInputRuleStyle.Render("x") {
			t.Fatalf("adversary and plan input rules should both follow Secondary under theme %q", name)
		}
	}
}

func TestMonochromeThemeIsAdaptiveAndNeutral(t *testing.T) {
	prev := CurrentThemeName
	t.Cleanup(func() { ApplyTheme(prev) })

	theme, ok := ThemeByName("monochrome")
	if !ok {
		t.Fatal("monochrome theme missing")
	}
	for _, c := range []compat.AdaptiveColor{theme.Primary, theme.PrimaryLight, theme.Secondary, theme.Accent, theme.Error, theme.OnPrimary} {
		if c.Light == c.Dark {
			t.Fatalf("expected adaptive light/dark pair, got %v", c)
		}
	}
	if !ApplyTheme("monochrome") {
		t.Fatal("ApplyTheme(monochrome) = false")
	}
	if OnPrimaryColor != theme.OnPrimary {
		t.Fatal("OnPrimaryColor not applied")
	}
	chip := ModelChipStyle.Render("x")
	if !strings.Contains(chip, "\x1b[") {
		t.Fatalf("expected styled chip, got %q", chip)
	}
	planPrompt := PromptPlanStyle.Render("x")
	ApplyTheme("default")
	if PromptPlanStyle.Render("x") == planPrompt {
		t.Fatal("plan styles must follow Secondary across themes")
	}
}

func TestApplyThemeChangesAccentAndError(t *testing.T) {
	prev := CurrentThemeName
	t.Cleanup(func() { ApplyTheme(prev) })

	ApplyTheme("default")
	defaultAccent := AccentStyle.Render("x")
	defaultError := ErrorStyle.Render("x")
	defaultShellChip := ShellChipStyle.Render("x")
	defaultYoloChip := ModeYoloChipStyle.Render("x")
	ApplyTheme("dracula")
	if AccentStyle.Render("x") == defaultAccent {
		t.Fatal("expected AccentStyle to change between default and dracula")
	}
	if ErrorStyle.Render("x") == defaultError {
		t.Fatal("expected ErrorStyle to change between default and dracula")
	}
	if ShellChipStyle.Render("x") == defaultShellChip {
		t.Fatal("expected ShellChipStyle to follow Accent between default and dracula")
	}
	if ModeYoloChipStyle.Render("x") == defaultYoloChip {
		t.Fatal("expected ModeYoloChipStyle to follow Error between default and dracula")
	}
}

func TestChipColorsHaveContrast(t *testing.T) {
	previous := CurrentThemeName
	t.Cleanup(func() { ApplyTheme(previous) })
	luminance := func(c color.Color) float64 {
		r, g, b, _ := c.RGBA()
		linear := func(v uint32) float64 {
			x := float64(v) / 65535
			if x <= 0.04045 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
	}
	for _, theme := range Themes {
		ApplyTheme(theme.Name)
		for _, pair := range []struct {
			name   string
			bg, fg compat.AdaptiveColor
		}{
			{"accent", theme.Accent, theme.OnAccent}, {"error", theme.Error, theme.OnError},
		} {
			for _, colors := range [][2]color.Color{{pair.bg.Light, pair.fg.Light}, {pair.bg.Dark, pair.fg.Dark}} {
				a, b := luminance(colors[0]), luminance(colors[1])
				if a < b {
					a, b = b, a
				}
				if ratio := (a + 0.05) / (b + 0.05); ratio < 4.5 {
					t.Errorf("%s %s contrast = %.2f", theme.Name, pair.name, ratio)
				}
			}
		}
		if !reflect.DeepEqual(BtwChipStyle.GetForeground(), theme.OnAccent) || !reflect.DeepEqual(ShellChipStyle.GetForeground(), theme.OnAccent) || !reflect.DeepEqual(ModeYoloChipStyle.GetForeground(), theme.OnError) {
			t.Errorf("%s chip foregrounds do not match theme", theme.Name)
		}
	}
}

func TestApplyThemeSetsDiffColors(t *testing.T) {
	previous := CurrentThemeName
	t.Cleanup(func() { ApplyTheme(previous) })
	for _, theme := range Themes {
		ApplyTheme(theme.Name)
		for _, pair := range []struct {
			name   string
			fg, bg compat.AdaptiveColor
			style  lipgloss.Style
		}{
			{"add", theme.DiffAdd, theme.DiffAddBg, DiffAddLineStyle},
			{"remove", theme.DiffRemove, theme.DiffRemoveBg, DiffRemoveLineStyle},
		} {
			if pair.fg.Light == nil || pair.fg.Dark == nil || pair.bg.Light == nil || pair.bg.Dark == nil {
				t.Fatalf("%s diff %s colors are not set", theme.Name, pair.name)
			}
			if !reflect.DeepEqual(pair.style.GetBackground(), pair.bg) {
				t.Errorf("%s diff %s line background not applied", theme.Name, pair.name)
			}
			for _, colors := range [][2]color.Color{{pair.fg.Light, pair.bg.Light}, {pair.fg.Dark, pair.bg.Dark}} {
				if ratio := contrastRatio(colors[0], colors[1]); ratio < 3 {
					t.Errorf("%s diff %s sign contrast = %.2f", theme.Name, pair.name, ratio)
				}
			}
		}
		if !reflect.DeepEqual(DiffAddStyle.GetForeground(), theme.DiffAdd) || !reflect.DeepEqual(DiffRemoveStyle.GetForeground(), theme.DiffRemove) {
			t.Errorf("%s diff sign colors not applied", theme.Name)
		}
	}
}

func contrastRatio(a, b color.Color) float64 {
	luminance := func(c color.Color) float64 {
		r, g, bl, _ := c.RGBA()
		linear := func(v uint32) float64 {
			x := float64(v) / 65535
			if x <= 0.04045 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(bl)
	}
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}
