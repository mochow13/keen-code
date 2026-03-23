package repl

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

var (
	primaryColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#3F51B5"),
		Dark:  lipgloss.Color("#5C6BC0"),
	}
	secondaryColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#00897B"),
		Dark:  lipgloss.Color("#4DB6AC"),
	}
	mutedColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#757575"),
		Dark:  lipgloss.Color("#BDBDBD"),
	}
	accentColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#FF8F00"),
		Dark:  lipgloss.Color("#FFB300"),
	}
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)
	infoLabelStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Width(18)
	infoValueStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#424242"),
			Dark:  lipgloss.Color("#BDBDBD"),
		})
	highlightStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)
	tipStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true)
	boxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(mutedColor).
			Padding(1, 2).
			MarginTop(1)
	promptStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginTop(2)
	helpCmdStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true).
			Width(12)
	helpDescStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#424242"),
			Dark:  lipgloss.Color("#BDBDBD"),
		})
	assistantStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#212121"),
			Dark:  lipgloss.Color("#EEEEEE"),
		})
	reasoningStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#9E9E9E"),
			Dark:  lipgloss.Color("#757575"),
		}).
		Italic(true).
		Faint(true)
	errorStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#D32F2F"),
			Dark:  lipgloss.Color("#EF5350"),
		})
	toolStartStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F57C00")).
			Bold(true)
	interruptedStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true)
	toolSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#388E3C"))
	toolErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D32F2F"))
	normalStyle         = lipgloss.NewStyle()
	modelSelectionStyle = lipgloss.NewStyle().
				Foreground(secondaryColor).
				Bold(true)
	hintStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true)
	inputBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.ThickBorder()).
				BorderForeground(primaryColor)
	warningTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#D32F2F"))
	bashCommandStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(secondaryColor)
	bashOutputStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#424242"),
			Dark:  lipgloss.Color("#BDBDBD"),
		})
	bashSummaryStyle = lipgloss.NewStyle().
				Foreground(mutedColor)
	diffAddStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
		Light: lipgloss.Color("#2E7D32"), Dark: lipgloss.Color("#66BB6A"),
	})
	diffRemoveStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
		Light: lipgloss.Color("#C62828"), Dark: lipgloss.Color("#EF5350"),
	})
	diffContextStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
		Light: lipgloss.Color("#616161"), Dark: lipgloss.Color("#9E9E9E"),
	})
	diffHunkStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#1565C0"), Dark: lipgloss.Color("#42A5F5"),
		}).Bold(true)
	diffLineNumStyle    = lipgloss.NewStyle().Foreground(mutedColor)
	userPromptCardStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(secondaryColor).
				Padding(1, 2)
	userPromptStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(secondaryColor)
	userPromptSelectionStyle = lipgloss.NewStyle().
					Foreground(secondaryColor).
					Bold(true)
	loadingTextStyled = lipgloss.NewStyle().Foreground(accentColor).Bold(true)

	suggestionContainerStyle = lipgloss.NewStyle().
					BorderStyle(lipgloss.RoundedBorder()).
					BorderForeground(mutedColor).
					Padding(0, 1)
	suggestionCmdStyle = lipgloss.NewStyle().
				Foreground(secondaryColor)
	suggestionDescStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				PaddingLeft(2)
	suggestionSelectedCmdStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FFFFFF")).
					Background(primaryColor).
					Bold(true)
	suggestionSelectedDescStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FFFFFF")).
					Background(primaryColor).
					PaddingLeft(2)
)
