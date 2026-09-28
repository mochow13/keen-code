package askuser

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
)

const HorizontalMargin = 4

const inputPrefixWidth = 4

func NewInput() textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "Type your answer"
	var styles textinput.Styles
	styles.Focused.Text = repltheme.AskUserTypedStyle
	styles.Focused.Placeholder = repltheme.AskUserCustomStyle
	styles.Blurred = styles.Focused
	styles.Cursor.Color = repltheme.TextDimColor
	input.SetStyles(styles)
	return input
}

type Card struct {
	Active bool
	state  State
}

func (c *Card) Clone() *Card {
	if c == nil {
		return nil
	}
	cp := *c
	cp.state.Answers = append([]string(nil), c.state.Answers...)
	cp.state.Resolved = append([]Answer(nil), c.state.Resolved...)
	return &cp
}

func (c *Card) IsActive() bool {
	return c != nil && c.Active
}

func (c *Card) Render(width int) string {
	if c == nil {
		return ""
	}
	return c.state.Render(width)
}

func (s State) Render(width int) string {
	if !s.Visible() {
		return ""
	}
	contentWidth := max(width-HorizontalMargin*2, 1)
	var content strings.Builder
	if s.Active() {
		renderActive(&content, s, contentWidth)
	} else {
		renderResolved(&content, s, contentWidth)
	}
	body := strings.TrimRight(content.String(), "\n")
	margin := strings.Repeat(" ", HorizontalMargin)
	body = margin + strings.ReplaceAll(body, "\n", "\n"+margin)
	rule := repltheme.AskUserRuleStyle.Render(strings.Repeat("─", max(width, 1)))
	return "\n" + rule + "\n" + body + "\n" + rule + "\n"
}

func renderActive(content *strings.Builder, s State, width int) {
	question := s.Request.Questionnaire.Questions[s.Index]
	for i, answer := range s.Answers {
		content.WriteString(repltheme.AskUserResolvedStyle.Render("• " + s.Request.Questionnaire.Questions[i].Question + ": " + answer))
		content.WriteString("\n")
	}
	if len(s.Answers) > 0 {
		content.WriteString("\n")
	}
	content.WriteString(repltheme.AskUserProgressStyle.Render("Question " + strconv.Itoa(s.Index+1) + " of " + strconv.Itoa(len(s.Request.Questionnaire.Questions))))
	content.WriteString("\n")
	content.WriteString(wrapText(question.Question, repltheme.AskUserQuestionStyle, width, "", ""))
	content.WriteString("\n\n")
	for i, option := range question.Options {
		badge := ""
		if i == 0 {
			badge = " " + repltheme.AskUserBadgeStyle.Render("(recommended)")
		}
		content.WriteString(renderOption(i == s.Selected, option+badge, repltheme.NormalStyle, true, width))
	}
	customRow := "Type your answer"
	customStyle := repltheme.AskUserCustomStyle
	if s.Editing || s.Input.Value() != "" {
		input := s.Input
		input.SetWidth(max(width-inputPrefixWidth, 1))
		customRow = input.View()
		customStyle = lipgloss.NewStyle()
	}
	content.WriteString(renderOption(s.Selected == len(question.Options), customRow, customStyle, false, width))
	content.WriteString("\n")
	hint := "↑/↓ navigate · Enter select · Esc cancel"
	if s.Selected == len(question.Options) && s.Editing {
		hint = "Enter submit · Esc cancel"
	}
	content.WriteString(repltheme.AskUserHintStyle.Render(hint))
	content.WriteString("\n")
}

func renderResolved(content *strings.Builder, s State, width int) {
	header := "Answers provided"
	if s.Cancelled {
		header = "↩ Questions cancelled"
	}
	content.WriteString(repltheme.AskUserProgressStyle.Render(header))
	if len(s.Resolved) == 0 {
		content.WriteString("\n")
		return
	}
	content.WriteString("\n\n")
	for _, answer := range s.Resolved {
		content.WriteString(wrapText(answer.Question+": "+answer.Answer, repltheme.AskUserResolvedStyle, width, "• ", "  "))
		content.WriteString("\n")
	}
}

func renderOption(selected bool, text string, style lipgloss.Style, highlightSelected bool, width int) string {
	cursor := "  "
	if selected {
		cursor = repltheme.AskUserSelectedStyle.Render("› ")
	}
	bulletStyle := style
	if selected && highlightSelected {
		style = repltheme.AskUserSelectedStyle
		bulletStyle = repltheme.AskUserSelectedStyle
	}
	prefix := cursor + bulletStyle.Render("• ")
	return wrapText(text, style, width, prefix, "    ") + "\n"
}

func wrapText(text string, style lipgloss.Style, width int, prefix, continuation string) string {
	available := max(width-lipgloss.Width(prefix), 1)
	wrapped := lipgloss.NewStyle().Width(available).Render(style.Render(text))
	lines := strings.Split(strings.TrimRight(wrapped, "\n"), "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = prefix + line
		} else {
			lines[i] = continuation + line
		}
	}
	return strings.Join(lines, "\n")
}
