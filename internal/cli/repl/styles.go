package repl

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

var (
	primaryColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#7C3AED"),
		Dark:  lipgloss.Color("#7C3AED"),
	}
	secondaryColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#059669"),
		Dark:  lipgloss.Color("#10B981"),
	}
	mutedColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#6B7280"),
		Dark:  lipgloss.Color("#9CA3AF"),
	}
	accentColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#D97706"),
		Dark:  lipgloss.Color("#F59E0B"),
	}
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)
	infoLabelStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Width(18)
	infoValueStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#374151"),
			Dark:  lipgloss.Color("#E5E7EB"),
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
			Light: lipgloss.Color("#374151"),
			Dark:  lipgloss.Color("#E5E7EB"),
		})
	assistantStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#1F2937"),
			Dark:  lipgloss.Color("#E5E7EB"),
		})
	reasoningStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#7F8A99"),
			Dark:  lipgloss.Color("#8A95A5"),
		}).
		Italic(true).
		Faint(true)
	errorStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#DC2626"),
			Dark:  lipgloss.Color("#EF4444"),
		})
	toolStartStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFA500")).
			Bold(true)
	interruptedStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true)
	toolSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00AA00"))
	toolErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000"))
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
				Foreground(lipgloss.Color("#DC2626"))
	bashCommandStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(secondaryColor)
	bashOutputStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#374151"),
			Dark:  lipgloss.Color("#E5E7EB"),
		})
	bashSummaryStyle = lipgloss.NewStyle().
				Foreground(mutedColor)
	bashRunningStyle = lipgloss.NewStyle().
				Foreground(secondaryColor).
				Bold(true)
	bashHintStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true)
	diffAddStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
		Light: lipgloss.Color("#166534"), Dark: lipgloss.Color("#4ADE80"),
	})
	diffRemoveStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
		Light: lipgloss.Color("#991B1B"), Dark: lipgloss.Color("#F87171"),
	})
	diffContextStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
		Light: lipgloss.Color("#374151"), Dark: lipgloss.Color("#9CA3AF"),
	})
	diffHunkStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#1D4ED8"), Dark: lipgloss.Color("#60A5FA"),
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
)
