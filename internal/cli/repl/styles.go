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
	modeStyle = lipgloss.NewStyle().
			Foreground(accentColor).
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
			Foreground(primaryColor)
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
	errorStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#DC2626"),
			Dark:  lipgloss.Color("#EF4444"),
		})
	toolStartStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFA500")).
			Italic(true)
	toolSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00AA00"))
	toolErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000"))
	normalStyle    = lipgloss.NewStyle()
	selectionStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)
	hintStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true)
	inputBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.ThickBorder()).
				BorderForeground(primaryColor)
)
