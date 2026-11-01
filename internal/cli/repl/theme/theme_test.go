package theme

import (
	"strings"
	"testing"

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
	for _, c := range []compat.AdaptiveColor{theme.Primary, theme.PrimaryLight, theme.Secondary, theme.OnPrimary} {
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
