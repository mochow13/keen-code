package theme

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

type Theme struct {
	Name         string
	Label        string
	Primary      compat.AdaptiveColor
	PrimaryLight compat.AdaptiveColor
	Secondary    compat.AdaptiveColor
	OnPrimary    compat.AdaptiveColor
}

const DefaultThemeName = "default"

var Themes = []Theme{
	{
		Name:         "default",
		Label:        "Default (Indigo)",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#3F51B5"), Dark: lipgloss.Color("#5C6BC0")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#5C6BC0"), Dark: lipgloss.Color("#7986CB")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#287A8A"), Dark: lipgloss.Color("#649FA9")},
		OnPrimary:    WhiteColor,
	},
	{
		Name:         "claude",
		Label:        "Claude Code",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#B2502E"), Dark: lipgloss.Color("#D97757")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#C46849"), Dark: lipgloss.Color("#FAB283")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#4A7BA7"), Dark: lipgloss.Color("#6A9BCC")},
		OnPrimary:    WhiteColor,
	},
	{
		Name:         "dracula",
		Label:        "Dracula",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#644AC9"), Dark: lipgloss.Color("#BD93F9")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#7E57C2"), Dark: lipgloss.Color("#D6ACFF")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#A3144D"), Dark: lipgloss.Color("#FF79C6")},
		OnPrimary:    WhiteColor,
	},
	{
		Name:         "tokyonight",
		Label:        "Tokyo Night",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#3457B2"), Dark: lipgloss.Color("#7AA2F7")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#5A7BD5"), Dark: lipgloss.Color("#A8BDFB")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#6A3FB5"), Dark: lipgloss.Color("#BB9AF7")},
		OnPrimary:    WhiteColor,
	},
	{
		Name:         "catppuccin",
		Label:        "Catppuccin",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#8839EF"), Dark: lipgloss.Color("#CBA6F7")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#7287FD"), Dark: lipgloss.Color("#B4BEFE")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#1E66F5"), Dark: lipgloss.Color("#89B4FA")},
		OnPrimary:    WhiteColor,
	},
	{
		Name:         "github",
		Label:        "GitHub",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#0969DA"), Dark: lipgloss.Color("#58A6FF")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#0550AE"), Dark: lipgloss.Color("#79C0FF")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#8250DF"), Dark: lipgloss.Color("#BC8CFF")},
		OnPrimary:    compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#0D1117")},
	},
	{
		Name:         "nord",
		Label:        "Nord",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#356879"), Dark: lipgloss.Color("#88C0D0")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#456B82"), Dark: lipgloss.Color("#8FBCBB")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#49688E"), Dark: lipgloss.Color("#81A1C1")},
		OnPrimary:    compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#2E3440")},
	},
	{
		Name:         "monochrome",
		Label:        "Monochrome",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#424242"), Dark: lipgloss.Color("#D4D4D4")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#616161"), Dark: lipgloss.Color("#E8E8E8")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#6A6A6A"), Dark: lipgloss.Color("#9E9E9E")},
		OnPrimary:    compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#121212")},
	},
}

var CurrentThemeName = DefaultThemeName

func ThemeByName(name string) (Theme, bool) {
	for _, t := range Themes {
		if t.Name == name {
			return t, true
		}
	}
	return Theme{}, false
}

func ThemeNames() []string {
	names := make([]string, 0, len(Themes))
	for _, t := range Themes {
		names = append(names, t.Name)
	}
	return names
}

func ApplyTheme(name string) bool {
	t, ok := ThemeByName(name)
	if !ok {
		return false
	}
	PrimaryColor = t.Primary
	PrimaryLightColor = t.PrimaryLight
	SecondaryColor = t.Secondary
	OnPrimaryColor = t.OnPrimary
	rebuildThemedStyles()
	CurrentThemeName = t.Name
	return true
}

func rebuildThemedStyles() {
	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(PrimaryColor)
	TipCodeStyle = lipgloss.NewStyle().Foreground(PrimaryLightColor).Bold(true)
	UsageHintStyle = lipgloss.NewStyle().Foreground(SecondaryColor).Bold(true)
	BoxStyle = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(SecondaryColor).
		Padding(1, 2).
		MarginTop(1)
	HighlightStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	PrimaryBoldStyle = lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
	InitialScreenWordmarkStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	InitialScreenArtStyles = []lipgloss.Style{
		lipgloss.NewStyle().Foreground(PrimaryLightColor).Bold(true),
		lipgloss.NewStyle().Foreground(PrimaryLightColor),
		PrimaryBoldStyle,
		lipgloss.NewStyle().Foreground(PrimaryColor),
	}
	PromptStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(PrimaryColor).
		MarginTop(2)
	InputRuleStyle = lipgloss.NewStyle().Foreground(PrimaryColor).Faint(true)
	HelpCmdStyle = lipgloss.NewStyle().Foreground(SecondaryColor).Bold(true).Width(12)
	InterruptedStyle = lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
	LoadingTextCodeStyle = lipgloss.NewStyle().Foreground(PrimaryLightColor).Bold(true)
	CompactionSuccessStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	ToolNameStyle = PrimaryBoldStyle
	BashCommandStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	ModelChipStyle = lipgloss.NewStyle().Background(PrimaryColor).Foreground(OnPrimaryColor).Bold(true).Padding(0, 1)
	UserPromptCardStyle = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(SecondaryColor).Padding(1, 2)
	UserPromptStyle = lipgloss.NewStyle().Bold(true).Foreground(SecondaryColor)
	UserPromptSelectionStyle = lipgloss.NewStyle().Foreground(SecondaryColor).Bold(true)
	AskUserProgressStyle = lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
	AskUserSelectedStyle = lipgloss.NewStyle().Foreground(SecondaryColor).Bold(true)
	AskUserTypedStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	SuggestionSelectedStyle = lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
	SuggestionCmdStyle = lipgloss.NewStyle().Foreground(PrimaryColor)
	SuggestionSelectedCmdStyle = lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
	ModelSelectionCursorStyle = SuggestionSelectedStyle
	ContextStatusPercentStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	QueueChipStyle = lipgloss.NewStyle().Background(SecondaryColor).Foreground(BlackColor).Bold(true).Padding(0, 1)
	AdversaryPromptStyle = lipgloss.NewStyle().Bold(true).Foreground(SecondaryColor).MarginTop(2)
	AdversaryInputRuleStyle = lipgloss.NewStyle().Foreground(SecondaryColor).Faint(true)
	PlanInputRuleStyle = lipgloss.NewStyle().Foreground(SecondaryColor).Faint(true)
	AdversaryBorderStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	AdversaryChipStyle = lipgloss.NewStyle().Background(SecondaryColor).Foreground(BlackColor).Bold(true).Padding(0, 1)
	ModePlanChipStyle = lipgloss.NewStyle().Background(SecondaryColor).Foreground(BlackColor).Bold(true).Padding(0, 1)
	InputRulePlanStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	PromptPlanStyle = lipgloss.NewStyle().Bold(true).Foreground(SecondaryColor).MarginTop(2)
	ModeBuildChipStyle = lipgloss.NewStyle().Background(PrimaryColor).Foreground(OnPrimaryColor).Bold(true).Padding(0, 1)
}
