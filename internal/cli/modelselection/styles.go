package modelselection

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

var (
	primaryColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#7C3AED"),
		Dark:  lipgloss.Color("#7C3AED"),
	}
	mutedColor = compat.AdaptiveColor{
		Light: lipgloss.Color("#6B7280"),
		Dark:  lipgloss.Color("#9CA3AF"),
	}
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)
	selectionStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)
	normalStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#374151"),
			Dark:  lipgloss.Color("#9CA3AF"),
		})
	hintStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true)
	promptStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)
	errorStyle = lipgloss.NewStyle().
			Foreground(compat.AdaptiveColor{
			Light: lipgloss.Color("#DC2626"),
			Dark:  lipgloss.Color("#EF4444"),
		})
)
