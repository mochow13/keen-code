package repl

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type permissionKeyEnter struct{}
type permissionKeyCancel struct{}

type PermissionChoice int

const (
	PermissionChoiceAllow PermissionChoice = iota
	PermissionChoiceAllowSession
	PermissionChoiceDeny
)

type PermissionSelector struct {
	toolName     string
	path         string
	resolvedPath string
	operation    string
	cursor       int
	choices      []string
}

func NewPermissionSelector(toolName, path, resolvedPath, operation string) *PermissionSelector {
	return &PermissionSelector{
		toolName:     toolName,
		path:         path,
		resolvedPath: resolvedPath,
		operation:    operation,
		cursor:       0,
		choices:      []string{"Allow", "Allow for this session", "Deny"},
	}
}

func (ps *PermissionSelector) Init() tea.Cmd {
	return nil
}

func (ps *PermissionSelector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if ps.cursor > 0 {
				ps.cursor--
			}
		case "down", "j":
			if ps.cursor < len(ps.choices)-1 {
				ps.cursor++
			}
		case "enter":
			return ps, func() tea.Msg { return permissionKeyEnter{} }
		case "esc":
			ps.cursor = int(PermissionChoiceDeny)
			return ps, func() tea.Msg { return permissionKeyCancel{} }
		}
	}
	return ps, nil
}

func (ps *PermissionSelector) View() tea.View {
	return tea.NewView(ps.ViewString())
}

func (ps *PermissionSelector) ViewString() string {
	var view strings.Builder

	view.WriteString(titleStyle.Render(fmt.Sprintf("Allow %s?", ps.toolName)))
	view.WriteString("\n\n")

	view.WriteString("  " + infoLabelStyle.Render("Tool:") + " " + infoValueStyle.Render(ps.toolName))
	view.WriteString("\n")
	view.WriteString("  " + infoLabelStyle.Render("Path:") + " " + infoValueStyle.Render(ps.path))
	view.WriteString("\n")
	view.WriteString("  " + infoLabelStyle.Render("Resolved:") + " " + infoValueStyle.Render(ps.resolvedPath))
	view.WriteString("\n\n")

	for i, choice := range ps.choices {
		cursorStr := "  "
		style := normalStyle
		if i == ps.cursor {
			cursorStr = "> "
			style = selectionStyle
		}
		view.WriteString(cursorStr + style.Render(choice) + "\n")
	}

	view.WriteString("\n" + hintStyle.Render("[↑/↓ to navigate, Enter to confirm, Esc to cancel]"))

	return view.String()
}

func (ps *PermissionSelector) GetChoice() PermissionChoice {
	return PermissionChoice(ps.cursor)
}

func IsPermissionComplete(msg tea.Msg) bool {
	_, ok := msg.(permissionKeyEnter)
	return ok
}

func IsPermissionCancel(msg tea.Msg) bool {
	_, ok := msg.(permissionKeyCancel)
	return ok
}
