package modelselection

import "github.com/charmbracelet/lipgloss"

var (
	primaryColor = lipgloss.AdaptiveColor{
		Light: "#7C3AED",
		Dark:  "#7C3AED",
	}
	mutedColor = lipgloss.AdaptiveColor{
		Light: "#6B7280",
		Dark:  "#9CA3AF",
	}
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)
	selectionStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)
	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{
			Light: "#374151",
			Dark:  "#9CA3AF",
		})
	hintStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true)
	promptStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{
			Light: "#DC2626",
			Dark:  "#EF4444",
		})
)
