package repl

import (
	"strings"

	"charm.land/lipgloss/v2"
)

type suggestionModel struct {
	visible  bool
	items    []slashCommand
	selected int
}

func newSuggestionModel() suggestionModel {
	return suggestionModel{}
}

func (s *suggestionModel) refresh(input string) {
	s.items = filterCommands(input)
	if len(s.items) > 0 {
		s.visible = true
		s.selected = 0
	} else {
		s.visible = false
		s.items = nil
	}
}

func (s *suggestionModel) moveDown() {
	if len(s.items) == 0 {
		return
	}
	s.selected = (s.selected + 1) % len(s.items)
}

func (s *suggestionModel) moveUp() {
	if len(s.items) == 0 {
		return
	}
	s.selected = (s.selected - 1 + len(s.items)) % len(s.items)
}

func (s suggestionModel) current() *slashCommand {
	if !s.visible || len(s.items) == 0 {
		return nil
	}
	return &s.items[s.selected]
}

func (s suggestionModel) height() int {
	if !s.visible {
		return 0
	}
	return len(s.items) + 2
}

func (s suggestionModel) view(width int) string {
	if !s.visible {
		return ""
	}

	cmdColWidth := 0
	for _, item := range s.items {
		if len(item.Name) > cmdColWidth {
			cmdColWidth = len(item.Name)
		}
	}
	cmdColWidth += 2

	var rows []string
	for i, item := range s.items {
		isSelected := i == s.selected

		var cmdStyle, descStyle lipgloss.Style
		if isSelected {
			cmdStyle = suggestionSelectedCmdStyle.Width(cmdColWidth)
			descStyle = suggestionSelectedDescStyle
		} else {
			cmdStyle = suggestionCmdStyle.Width(cmdColWidth)
			descStyle = suggestionDescStyle
		}

		row := lipgloss.JoinHorizontal(lipgloss.Left,
			cmdStyle.Render(item.Name),
			descStyle.Render(item.Description),
		)
		rows = append(rows, row)
	}

	inner := strings.Join(rows, "\n")

	hasSelection := s.selected >= 0 && len(s.items) > 0
	containerStyle := suggestionContainerStyle
	if hasSelection {
		containerStyle = containerStyle.BorderForeground(primaryColor)
	}

	box := containerStyle.Render(inner)

	boxWidth := lipgloss.Width(box)
	if boxWidth < width {
		lines := strings.Split(box, "\n")
		var padded []string
		for _, l := range lines {
			lw := lipgloss.Width(l)
			if lw < width {
				padded = append(padded, l+strings.Repeat(" ", width-lw))
			} else {
				padded = append(padded, l)
			}
		}
		return strings.Join(padded, "\n")
	}
	return box
}
