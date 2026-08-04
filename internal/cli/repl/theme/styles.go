package theme

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

var (
	PrimaryColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#3F51B5"),
		Dark:  lipgloss.Color("#5C6BC0"),
	}
	PrimaryLightColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#5C6BC0"),
		Dark:  lipgloss.Color("#7986CB"),
	}
	SecondaryColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#287A8A"),
		Dark:  lipgloss.Color("#649FA9"),
	}
	MutedColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#757575"),
		Dark:  lipgloss.Color("#BDBDBD"),
	}
	AccentColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#FF8F00"),
		Dark:  lipgloss.Color("#FFB300"),
	}

	TextPrimaryColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#242323"),
		Dark:  lipgloss.Color("#BDBDBD"),
	}
	TextSecondaryColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#424242"),
		Dark:  lipgloss.Color("#BDBDBD"),
	}
	TextDimColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#5e5d5d"),
		Dark:  lipgloss.Color("#B3B3B3"),
	}
	LoadingShimmerColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#212121"),
		Dark:  lipgloss.Color("#FFFFFF"),
	}
	LoadingShimmerMidColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#424242"),
		Dark:  lipgloss.Color("#D0D0D0"),
	}
	LoadingCodeShimmerColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#283593"),
		Dark:  lipgloss.Color("#E8EAF6"),
	}
	LoadingCodeShimmerMidColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#3F51B5"),
		Dark:  lipgloss.Color("#C5CAE9"),
	}
	RuleColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#bdbdbd"),
		Dark:  lipgloss.Color("#3b3b3b"),
	}
	UserInputBlockBackground = compat.AdaptiveColor{
		Light: lipgloss.Color("#E0E0E0"),
		Dark:  lipgloss.Color("#2E2E2E"),
	}

	ErrorColor = compat.AdaptiveColor{Light: lipgloss.Color("#D32F2F"), Dark: lipgloss.Color("#EF5350")}
	WhiteColor = compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#FFFFFF")}
	BlackColor = compat.AdaptiveColor{Light: lipgloss.Color("#000000"), Dark: lipgloss.Color("#000000")}

	DiffAddColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#2E7D32"),
		Dark:  lipgloss.Color("#66BB6A"),
	}
	DiffRemoveColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#C62828"),
		Dark:  lipgloss.Color("#EF5350"),
	}
	DiffContextColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#616161"),
		Dark:  lipgloss.Color("#9E9E9E"),
	}
	DiffHunkColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#1565C0"),
		Dark:  lipgloss.Color("#42A5F5"),
	}

	NormalStyle    = lipgloss.NewStyle()
	TitleStyle     = lipgloss.NewStyle().Bold(true).Foreground(PrimaryColor)
	TipStyle       = lipgloss.NewStyle().Foreground(TextDimColor)
	TipCodeStyle   = lipgloss.NewStyle().Foreground(PrimaryLightColor).Bold(true)
	HintStyle      = lipgloss.NewStyle().Foreground(TextDimColor)
	UsageHintStyle = lipgloss.NewStyle().Foreground(SecondaryColor).Bold(true)
	BoxStyle       = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(SecondaryColor).
			Padding(1, 2).
			MarginTop(1)
	HighlightStyle   = lipgloss.NewStyle().Foreground(SecondaryColor)
	MutedStyle       = lipgloss.NewStyle().Foreground(MutedColor)
	PrimaryBoldStyle = lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
	AccentStyle      = lipgloss.NewStyle().Foreground(AccentColor)

	InitialScreenMetadataStyle = lipgloss.NewStyle().Foreground(TextDimColor).Faint(true)
	InitialScreenWordmarkStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	InitialScreenTipLabelStyle = lipgloss.NewStyle().Foreground(TextDimColor).Faint(true).Bold(true)
	InitialScreenRuleStyle     = lipgloss.NewStyle().Foreground(RuleColor).Faint(true)
	InitialScreenArtStyles     = []lipgloss.Style{
		lipgloss.NewStyle().Foreground(PrimaryLightColor).Bold(true),
		lipgloss.NewStyle().Foreground(PrimaryLightColor),
		PrimaryBoldStyle,
		lipgloss.NewStyle().Foreground(PrimaryColor),
	}

	PromptStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(PrimaryColor).
			MarginTop(2)
	ShellPromptStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(AccentColor).
				MarginTop(2)
	BtwPromptStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(AccentColor).
			MarginTop(2)
	AdversaryPromptStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(SecondaryColor).
				MarginTop(2)
	InputRuleStyle          = lipgloss.NewStyle().Foreground(PrimaryColor).Faint(true)
	InputRuleBlurredStyle   = lipgloss.NewStyle().Foreground(TextDimColor).Faint(true)
	ShellInputRuleStyle     = lipgloss.NewStyle().Foreground(AccentColor).Faint(true)
	BtwInputRuleStyle       = lipgloss.NewStyle().Foreground(AccentColor).Faint(true)
	AdversaryInputRuleStyle = lipgloss.NewStyle().Foreground(SecondaryColor).Faint(true)
	PlanInputRuleStyle      = lipgloss.NewStyle().Foreground(SecondaryColor).Faint(true)
	UserInputBlockStyle     = lipgloss.NewStyle().
				Background(UserInputBlockBackground).
				Padding(1, 1)

	InfoLabelStyle = lipgloss.NewStyle().Foreground(MutedColor).Width(18)
	InfoValueStyle = lipgloss.NewStyle().Foreground(TextDimColor)

	HelpCmdStyle   = lipgloss.NewStyle().Foreground(SecondaryColor).Bold(true).Width(12)
	HelpDescStyle  = lipgloss.NewStyle().Foreground(TextSecondaryColor)
	TimestampStyle = lipgloss.NewStyle().Foreground(TextDimColor)

	AssistantStyle              = lipgloss.NewStyle().Foreground(TextPrimaryColor)
	ReasoningStyle              = lipgloss.NewStyle().Foreground(TextDimColor).Faint(true)
	ErrorStyle                  = lipgloss.NewStyle().Foreground(ErrorColor)
	InterruptedStyle            = lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
	LoadingTextStyled           = lipgloss.NewStyle().Foreground(TextDimColor).Faint(true)
	LoadingTextCodeStyle        = lipgloss.NewStyle().Foreground(PrimaryLightColor).Bold(true)
	LoadingTextCodeShimmerMid   = lipgloss.NewStyle().Foreground(LoadingCodeShimmerMidColor).Bold(true)
	LoadingTextCodeShimmerStyle = lipgloss.NewStyle().Foreground(LoadingCodeShimmerColor).Bold(true)
	LoadingTextShimmerStyle     = lipgloss.NewStyle().Foreground(LoadingShimmerColor).Bold(true)
	LoadingTextShimmerMid       = lipgloss.NewStyle().Foreground(LoadingShimmerMidColor)
	LoadingTimerStyle           = lipgloss.NewStyle().Foreground(TextDimColor).Faint(true)
	TurnElapsedStyle            = lipgloss.NewStyle().Foreground(TextDimColor).Faint(true)
	CompactionSuccessStyle      = lipgloss.NewStyle().Foreground(SecondaryColor)
	CompactionErrorStyle        = lipgloss.NewStyle().Foreground(ErrorColor)
	CompactionCancelledStyle    = lipgloss.NewStyle().Foreground(TextDimColor)

	ToolNameStyle     = PrimaryBoldStyle
	ToolErrorStyle    = lipgloss.NewStyle().Foreground(ErrorColor)
	ToolMetaStyle     = lipgloss.NewStyle().Foreground(TextDimColor).Faint(true)
	WarningTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(ErrorColor)

	BashCommandStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	BashOutputStyle  = lipgloss.NewStyle().Foreground(TextDimColor)
	BashSummaryStyle = lipgloss.NewStyle().Foreground(MutedColor).Faint(true)

	DiffAddStyle     = lipgloss.NewStyle().Foreground(DiffAddColor)
	DiffRemoveStyle  = lipgloss.NewStyle().Foreground(DiffRemoveColor)
	DiffContextStyle = lipgloss.NewStyle().Foreground(DiffContextColor)
	DiffHunkStyle    = lipgloss.NewStyle().Foreground(DiffHunkColor).Bold(true)
	DiffLineNumStyle = lipgloss.NewStyle().Foreground(TextDimColor)
	RuleStyle        = lipgloss.NewStyle().Foreground(RuleColor)

	ModelChipStyle                  = lipgloss.NewStyle().Background(PrimaryColor).Foreground(WhiteColor).Bold(true).Padding(0, 1)
	ModelSelectionCursorStyle       = SuggestionSelectedStyle
	ModelSelectionSelectedTextStyle = SuggestionSelectedFileStyle
	ModelSelectionTextStyle         = lipgloss.NewStyle().Foreground(TextDimColor).Faint(true)
	ModelSelectionTitleStyle        = ModelSelectionTextStyle.Bold(true)
	ModelSelectionThinkingStyle     = ModelSelectionTitleStyle
	ModelSelectionRuleStyle         = ModelSelectionTextStyle
	UserPromptCardStyle             = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(SecondaryColor).Padding(1, 2)
	UserPromptStyle                 = lipgloss.NewStyle().Bold(true).Foreground(SecondaryColor)
	UserPromptSelectionStyle        = lipgloss.NewStyle().Foreground(SecondaryColor).Bold(true)

	SuggestionContainerStyle    = lipgloss.NewStyle().Padding(0, 1)
	SuggestionSelectedStyle     = lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
	SuggestionCmdStyle          = lipgloss.NewStyle().Foreground(PrimaryColor)
	SuggestionSelectedCmdStyle  = lipgloss.NewStyle().Foreground(PrimaryColor).Bold(true)
	SuggestionDescStyle         = lipgloss.NewStyle().Foreground(TextDimColor).Faint(true).PaddingLeft(2)
	SuggestionSelectedDescStyle = lipgloss.NewStyle().Foreground(TextDimColor).Bold(true).PaddingLeft(2)
	SuggestionFileStyle         = lipgloss.NewStyle().Foreground(TextDimColor).Faint(true)
	SuggestionSelectedFileStyle = lipgloss.NewStyle().Foreground(TextDimColor).Bold(true)

	MetaLabelStyle                    = lipgloss.NewStyle().Foreground(TextDimColor).Faint(true)
	ContextStatusLabelStyle           = lipgloss.NewStyle().Foreground(TextDimColor)
	ContextStatusPercentStyle         = lipgloss.NewStyle().Foreground(SecondaryColor)
	ContextStatusPercentWarnStyle     = lipgloss.NewStyle().Foreground(AccentColor)
	ContextStatusPercentCriticalStyle = lipgloss.NewStyle().Foreground(ErrorColor)
	ContextStatusUnknownStyle         = lipgloss.NewStyle().Foreground(TextDimColor)
	CompactionSuggestionStyle         = lipgloss.NewStyle().Foreground(AccentColor)

	UpdateAvailableStyle = lipgloss.NewStyle().Foreground(AccentColor).Bold(true)
	UpdateCommandStyle   = lipgloss.NewStyle().Foreground(TextDimColor)

	BtwBorderStyle = lipgloss.NewStyle().Foreground(AccentColor)
	BtwLabelStyle  = lipgloss.NewStyle().Foreground(TextPrimaryColor)
	BtwChipStyle   = lipgloss.NewStyle().Background(AccentColor).Foreground(BlackColor).Bold(true).Padding(0, 1)

	AdversaryBorderStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	AdversaryLabelStyle  = lipgloss.NewStyle().Foreground(TextPrimaryColor)
	AdversaryChipStyle   = lipgloss.NewStyle().Background(SecondaryColor).Foreground(BlackColor).Bold(true).Padding(0, 1)

	QueueItemStyle = lipgloss.NewStyle().Foreground(TextDimColor).Faint(true)
	QueueChipStyle = lipgloss.NewStyle().Background(SecondaryColor).Foreground(BlackColor).Bold(true).Padding(0, 1)

	ShellChipStyle       = lipgloss.NewStyle().Background(AccentColor).Foreground(BlackColor).Bold(true).Padding(0, 1)
	ShellChipOutputStyle = lipgloss.NewStyle().Background(MutedColor).Foreground(BlackColor).Bold(true).Padding(0, 1)

	ModeBuildChipStyle = lipgloss.NewStyle().Background(PrimaryColor).Foreground(WhiteColor).Bold(true).Padding(0, 1)
	ModePlanChipStyle  = lipgloss.NewStyle().Background(SecondaryColor).Foreground(BlackColor).Bold(true).Padding(0, 1)

	InputRulePlanStyle = lipgloss.NewStyle().Foreground(SecondaryColor)
	PromptPlanStyle    = lipgloss.NewStyle().
				Bold(true).
				Foreground(SecondaryColor).
				MarginTop(2)
)
