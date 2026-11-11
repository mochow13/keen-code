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
	Accent       compat.AdaptiveColor
	Error        compat.AdaptiveColor
	OnAccent     compat.AdaptiveColor
	OnError      compat.AdaptiveColor
	OnPrimary    compat.AdaptiveColor
	DiffAdd      compat.AdaptiveColor
	DiffRemove   compat.AdaptiveColor
	DiffAddBg    compat.AdaptiveColor
	DiffRemoveBg compat.AdaptiveColor
}

const DefaultThemeName = "default"

var Themes = []Theme{
	{
		Name:         "default",
		Label:        "Default (Indigo)",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#3F51B5"), Dark: lipgloss.Color("#5C6BC0")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#5C6BC0"), Dark: lipgloss.Color("#7986CB")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#287A8A"), Dark: lipgloss.Color("#649FA9")},
		Accent:       compat.AdaptiveColor{Light: lipgloss.Color("#FF8F00"), Dark: lipgloss.Color("#FFB300")},
		Error:        compat.AdaptiveColor{Light: lipgloss.Color("#D32F2F"), Dark: lipgloss.Color("#EF5350")},
		OnAccent:     compat.AdaptiveColor{Light: lipgloss.Color("#000000"), Dark: lipgloss.Color("#000000")},
		OnError:      compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")},
		OnPrimary:    WhiteColor,
		DiffAdd:      compat.AdaptiveColor{Light: lipgloss.Color("#2E7D32"), Dark: lipgloss.Color("#66BB6A")},
		DiffRemove:   compat.AdaptiveColor{Light: lipgloss.Color("#C62828"), Dark: lipgloss.Color("#EF5350")},
		DiffAddBg:    compat.AdaptiveColor{Light: lipgloss.Color("#DFF5E1"), Dark: lipgloss.Color("#1F3A24")},
		DiffRemoveBg: compat.AdaptiveColor{Light: lipgloss.Color("#FBE3E3"), Dark: lipgloss.Color("#3D1F22")},
	},
	{
		Name:         "claude",
		Label:        "Claude Code",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#B2502E"), Dark: lipgloss.Color("#D97757")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#C46849"), Dark: lipgloss.Color("#FAB283")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#4A7BA7"), Dark: lipgloss.Color("#6A9BCC")},
		Accent:       compat.AdaptiveColor{Light: lipgloss.Color("#A8741A"), Dark: lipgloss.Color("#E5B567")},
		Error:        compat.AdaptiveColor{Light: lipgloss.Color("#B3261E"), Dark: lipgloss.Color("#E8706B")},
		OnAccent:     compat.AdaptiveColor{Light: lipgloss.Color("#000000"), Dark: lipgloss.Color("#000000")},
		OnError:      compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")},
		OnPrimary:    WhiteColor,
		DiffAdd:      compat.AdaptiveColor{Light: lipgloss.Color("#2F7D3B"), Dark: lipgloss.Color("#7BC88B")},
		DiffRemove:   compat.AdaptiveColor{Light: lipgloss.Color("#B3261E"), Dark: lipgloss.Color("#E8706B")},
		DiffAddBg:    compat.AdaptiveColor{Light: lipgloss.Color("#DCF2DF"), Dark: lipgloss.Color("#24402A")},
		DiffRemoveBg: compat.AdaptiveColor{Light: lipgloss.Color("#F8DEDD"), Dark: lipgloss.Color("#4A2328")},
	},
	{
		Name:         "dracula",
		Label:        "Dracula",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#644AC9"), Dark: lipgloss.Color("#BD93F9")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#7E57C2"), Dark: lipgloss.Color("#D6ACFF")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#A3144D"), Dark: lipgloss.Color("#FF79C6")},
		Accent:       compat.AdaptiveColor{Light: lipgloss.Color("#8F7A00"), Dark: lipgloss.Color("#F1FA8C")},
		Error:        compat.AdaptiveColor{Light: lipgloss.Color("#CC2F2F"), Dark: lipgloss.Color("#FF5555")},
		OnAccent:     compat.AdaptiveColor{Light: lipgloss.Color("#000000"), Dark: lipgloss.Color("#000000")},
		OnError:      compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")},
		OnPrimary:    WhiteColor,
		DiffAdd:      compat.AdaptiveColor{Light: lipgloss.Color("#1F8F3A"), Dark: lipgloss.Color("#50FA7B")},
		DiffRemove:   compat.AdaptiveColor{Light: lipgloss.Color("#CC2F2F"), Dark: lipgloss.Color("#FF5555")},
		DiffAddBg:    compat.AdaptiveColor{Light: lipgloss.Color("#DDF5E3"), Dark: lipgloss.Color("#2B4A3A")},
		DiffRemoveBg: compat.AdaptiveColor{Light: lipgloss.Color("#FADDE1"), Dark: lipgloss.Color("#4D2A35")},
	},
	{
		Name:         "tokyonight",
		Label:        "Tokyo Night",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#3457B2"), Dark: lipgloss.Color("#7AA2F7")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#5A7BD5"), Dark: lipgloss.Color("#A8BDFB")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#6A3FB5"), Dark: lipgloss.Color("#BB9AF7")},
		Accent:       compat.AdaptiveColor{Light: lipgloss.Color("#8F5E15"), Dark: lipgloss.Color("#E0AF68")},
		Error:        compat.AdaptiveColor{Light: lipgloss.Color("#C53B53"), Dark: lipgloss.Color("#F7768E")},
		OnAccent:     compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")},
		OnError:      compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")},
		OnPrimary:    WhiteColor,
		DiffAdd:      compat.AdaptiveColor{Light: lipgloss.Color("#485E30"), Dark: lipgloss.Color("#9ECE6A")},
		DiffRemove:   compat.AdaptiveColor{Light: lipgloss.Color("#C53B53"), Dark: lipgloss.Color("#F7768E")},
		DiffAddBg:    compat.AdaptiveColor{Light: lipgloss.Color("#DCE9CF"), Dark: lipgloss.Color("#20303B")},
		DiffRemoveBg: compat.AdaptiveColor{Light: lipgloss.Color("#F3D9DE"), Dark: lipgloss.Color("#37222C")},
	},
	{
		Name:         "catppuccin",
		Label:        "Catppuccin",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#8839EF"), Dark: lipgloss.Color("#CBA6F7")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#7287FD"), Dark: lipgloss.Color("#B4BEFE")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#1E66F5"), Dark: lipgloss.Color("#89B4FA")},
		Accent:       compat.AdaptiveColor{Light: lipgloss.Color("#FE640B"), Dark: lipgloss.Color("#FAB387")},
		Error:        compat.AdaptiveColor{Light: lipgloss.Color("#D20F39"), Dark: lipgloss.Color("#F38BA8")},
		OnAccent:     compat.AdaptiveColor{Light: lipgloss.Color("#000000"), Dark: lipgloss.Color("#000000")},
		OnError:      compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")},
		OnPrimary:    WhiteColor,
		DiffAdd:      compat.AdaptiveColor{Light: lipgloss.Color("#2E8A1E"), Dark: lipgloss.Color("#A6E3A1")},
		DiffRemove:   compat.AdaptiveColor{Light: lipgloss.Color("#D20F39"), Dark: lipgloss.Color("#F38BA8")},
		DiffAddBg:    compat.AdaptiveColor{Light: lipgloss.Color("#D9EED5"), Dark: lipgloss.Color("#2F3F35")},
		DiffRemoveBg: compat.AdaptiveColor{Light: lipgloss.Color("#F5D6DD"), Dark: lipgloss.Color("#4A2E3A")},
	},
	{
		Name:         "github",
		Label:        "GitHub",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#0969DA"), Dark: lipgloss.Color("#58A6FF")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#0550AE"), Dark: lipgloss.Color("#79C0FF")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#8250DF"), Dark: lipgloss.Color("#BC8CFF")},
		Accent:       compat.AdaptiveColor{Light: lipgloss.Color("#9A6700"), Dark: lipgloss.Color("#D29922")},
		Error:        compat.AdaptiveColor{Light: lipgloss.Color("#CF222E"), Dark: lipgloss.Color("#F85149")},
		OnAccent:     compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")},
		OnError:      compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")},
		OnPrimary:    compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#0D1117")},
		DiffAdd:      compat.AdaptiveColor{Light: lipgloss.Color("#1A7F37"), Dark: lipgloss.Color("#3FB950")},
		DiffRemove:   compat.AdaptiveColor{Light: lipgloss.Color("#CF222E"), Dark: lipgloss.Color("#F85149")},
		DiffAddBg:    compat.AdaptiveColor{Light: lipgloss.Color("#DAFBE1"), Dark: lipgloss.Color("#12261E")},
		DiffRemoveBg: compat.AdaptiveColor{Light: lipgloss.Color("#FFEBE9"), Dark: lipgloss.Color("#25171C")},
	},
	{
		Name:         "nord",
		Label:        "Nord",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#356879"), Dark: lipgloss.Color("#88C0D0")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#456B82"), Dark: lipgloss.Color("#8FBCBB")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#49688E"), Dark: lipgloss.Color("#81A1C1")},
		Accent:       compat.AdaptiveColor{Light: lipgloss.Color("#A3732C"), Dark: lipgloss.Color("#EBCB8B")},
		Error:        compat.AdaptiveColor{Light: lipgloss.Color("#B04A55"), Dark: lipgloss.Color("#BF616A")},
		OnAccent:     compat.AdaptiveColor{Light: lipgloss.Color("#000000"), Dark: lipgloss.Color("#000000")},
		OnError:      compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")},
		OnPrimary:    compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#2E3440")},
		DiffAdd:      compat.AdaptiveColor{Light: lipgloss.Color("#4F6B3A"), Dark: lipgloss.Color("#A3BE8C")},
		DiffRemove:   compat.AdaptiveColor{Light: lipgloss.Color("#B04A55"), Dark: lipgloss.Color("#BF616A")},
		DiffAddBg:    compat.AdaptiveColor{Light: lipgloss.Color("#E1EBD9"), Dark: lipgloss.Color("#3B4A3F")},
		DiffRemoveBg: compat.AdaptiveColor{Light: lipgloss.Color("#F2DEE0"), Dark: lipgloss.Color("#3B2C33")},
	},
	{
		Name:         "atom",
		Label:        "Atom One",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#356AD8"), Dark: lipgloss.Color("#61AFEF")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#4078F2"), Dark: lipgloss.Color("#8CC5F4")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#A626A4"), Dark: lipgloss.Color("#C678DD")},
		Accent:       compat.AdaptiveColor{Light: lipgloss.Color("#986801"), Dark: lipgloss.Color("#E5C07B")},
		Error:        compat.AdaptiveColor{Light: lipgloss.Color("#CA1243"), Dark: lipgloss.Color("#E06C75")},
		OnAccent:     compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")},
		OnError:      compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")},
		OnPrimary:    compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#282C34")},
		DiffAdd:      compat.AdaptiveColor{Light: lipgloss.Color("#3E8A3D"), Dark: lipgloss.Color("#98C379")},
		DiffRemove:   compat.AdaptiveColor{Light: lipgloss.Color("#CA1243"), Dark: lipgloss.Color("#E06C75")},
		DiffAddBg:    compat.AdaptiveColor{Light: lipgloss.Color("#E3F2E2"), Dark: lipgloss.Color("#2C3B2D")},
		DiffRemoveBg: compat.AdaptiveColor{Light: lipgloss.Color("#FBE1E5"), Dark: lipgloss.Color("#45282D")},
	},
	{
		Name:         "monochrome",
		Label:        "Monochrome",
		Primary:      compat.AdaptiveColor{Light: lipgloss.Color("#424242"), Dark: lipgloss.Color("#D4D4D4")},
		PrimaryLight: compat.AdaptiveColor{Light: lipgloss.Color("#616161"), Dark: lipgloss.Color("#E8E8E8")},
		Secondary:    compat.AdaptiveColor{Light: lipgloss.Color("#6A6A6A"), Dark: lipgloss.Color("#9E9E9E")},
		Accent:       compat.AdaptiveColor{Light: lipgloss.Color("#2E2E2E"), Dark: lipgloss.Color("#F5F5F5")},
		Error:        compat.AdaptiveColor{Light: lipgloss.Color("#000000"), Dark: lipgloss.Color("#BDBDBD")},
		OnAccent:     compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")},
		OnError:      compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#000000")},
		OnPrimary:    compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#121212")},
		DiffAdd:      compat.AdaptiveColor{Light: lipgloss.Color("#000000"), Dark: lipgloss.Color("#F5F5F5")},
		DiffRemove:   compat.AdaptiveColor{Light: lipgloss.Color("#616161"), Dark: lipgloss.Color("#9E9E9E")},
		DiffAddBg:    compat.AdaptiveColor{Light: lipgloss.Color("#E0E0E0"), Dark: lipgloss.Color("#3A3A3A")},
		DiffRemoveBg: compat.AdaptiveColor{Light: lipgloss.Color("#F5F5F5"), Dark: lipgloss.Color("#262626")},
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
	AccentColor = t.Accent
	ErrorColor = t.Error
	OnPrimaryColor = t.OnPrimary
	DiffAddColor = t.DiffAdd
	DiffRemoveColor = t.DiffRemove
	DiffAddBgColor = t.DiffAddBg
	DiffRemoveBgColor = t.DiffRemoveBg
	rebuildThemedStyles(t.OnAccent, t.OnError)
	CurrentThemeName = t.Name
	return true
}

func rebuildThemedStyles(onAccent, onError compat.AdaptiveColor) {
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
	TodoActiveStyle = lipgloss.NewStyle().Foreground(PrimaryLightColor).Bold(true)
	TodoDoneMarkStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	AdversaryPromptStyle = lipgloss.NewStyle().Bold(true).Foreground(SecondaryColor).MarginTop(2)
	AdversaryInputRuleStyle = lipgloss.NewStyle().Foreground(SecondaryColor).Faint(true)
	PlanInputRuleStyle = lipgloss.NewStyle().Foreground(SecondaryColor).Faint(true)
	AdversaryBorderStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	AdversaryChipStyle = lipgloss.NewStyle().Background(SecondaryColor).Foreground(BlackColor).Bold(true).Padding(0, 1)
	ModePlanChipStyle = lipgloss.NewStyle().Background(SecondaryColor).Foreground(BlackColor).Bold(true).Padding(0, 1)
	InputRulePlanStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	PromptPlanStyle = lipgloss.NewStyle().Bold(true).Foreground(SecondaryColor).MarginTop(2)
	ModeBuildChipStyle = lipgloss.NewStyle().Background(PrimaryColor).Foreground(OnPrimaryColor).Bold(true).Padding(0, 1)

	DiffAddStyle = lipgloss.NewStyle().Foreground(DiffAddColor)
	DiffRemoveStyle = lipgloss.NewStyle().Foreground(DiffRemoveColor)
	DiffAddLineStyle = lipgloss.NewStyle().Background(DiffAddBgColor)
	DiffRemoveLineStyle = lipgloss.NewStyle().Background(DiffRemoveBgColor)

	AccentStyle = lipgloss.NewStyle().Foreground(AccentColor)
	ShellPromptStyle = lipgloss.NewStyle().Bold(true).Foreground(AccentColor).MarginTop(2)
	BtwPromptStyle = lipgloss.NewStyle().Bold(true).Foreground(AccentColor).MarginTop(2)
	ShellInputRuleStyle = lipgloss.NewStyle().Foreground(AccentColor).Faint(true)
	BtwInputRuleStyle = lipgloss.NewStyle().Foreground(AccentColor).Faint(true)
	ContextStatusPercentWarnStyle = lipgloss.NewStyle().Foreground(AccentColor)
	CompactionSuggestionStyle = lipgloss.NewStyle().Foreground(AccentColor)
	UpdateAvailableStyle = lipgloss.NewStyle().Foreground(AccentColor).Bold(true)
	BtwBorderStyle = lipgloss.NewStyle().Foreground(AccentColor)
	BtwChipStyle = lipgloss.NewStyle().Background(AccentColor).Foreground(onAccent).Bold(true).Padding(0, 1)
	ShellChipStyle = lipgloss.NewStyle().Background(AccentColor).Foreground(onAccent).Bold(true).Padding(0, 1)

	ErrorStyle = lipgloss.NewStyle().Foreground(ErrorColor)
	CompactionErrorStyle = lipgloss.NewStyle().Foreground(ErrorColor)
	ToolErrorStyle = lipgloss.NewStyle().Foreground(ErrorColor)
	WarningTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(ErrorColor)
	ContextStatusPercentCriticalStyle = lipgloss.NewStyle().Foreground(ErrorColor)
	ModeYoloChipStyle = lipgloss.NewStyle().Background(ErrorColor).Foreground(onError).Bold(true).Padding(0, 1)
	InputRuleYoloStyle = lipgloss.NewStyle().Foreground(ErrorColor)
	YoloInputRuleStyle = lipgloss.NewStyle().Foreground(ErrorColor).Faint(true)
	PromptYoloStyle = lipgloss.NewStyle().Bold(true).Foreground(ErrorColor).MarginTop(2)
}
