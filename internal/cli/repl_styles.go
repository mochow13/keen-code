package cli

import "github.com/charmbracelet/lipgloss"

var (
	primaryColor = lipgloss.AdaptiveColor{
		Light: "#7C3AED",
		Dark:  "#7C3AED",
	}
	secondaryColor = lipgloss.AdaptiveColor{
		Light: "#059669",
		Dark:  "#10B981",
	}
	mutedColor = lipgloss.AdaptiveColor{
		Light: "#6B7280",
		Dark:  "#9CA3AF",
	}
	accentColor = lipgloss.AdaptiveColor{
		Light: "#D97706",
		Dark:  "#F59E0B",
	}
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)
	infoLabelStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Width(18)
	infoValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{
			Light: "#374151",
			Dark:  "#E5E7EB",
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
	outputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{
			Light: "#1F2937",
			Dark:  "#E5E7EB",
		})
	promptStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor)
	helpCmdStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true).
			Width(12)
	inputLineStyle = lipgloss.NewStyle()
	helpDescStyle  = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{
			Light: "#374151",
			Dark:  "#E5E7EB",
		})
)
