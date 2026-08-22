package repl

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	replaskuser "github.com/mochow13/keen-code/internal/cli/repl/askuser"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	"github.com/mochow13/keen-code/internal/tools"
)

const askUserHorizontalMargin = 4

type askUserAnswer struct {
	question string
	answer   string
}

type askUserState struct {
	requester *replaskuser.Requester
	request   *replaskuser.Request
	index     int
	selected  int
	answers   []string
	draft     string
	editing   bool
	resolved  []askUserAnswer
	cancelled bool
	completed bool
}

func (s *askUserState) begin(request *replaskuser.Request) {
	*s = askUserState{requester: s.requester, request: request}
}
func (s askUserState) active() bool  { return s.request != nil }
func (s askUserState) visible() bool { return s.active() || s.completed }

func cloneAskUserState(s askUserState) *askUserState {
	cloned := s
	cloned.requester = nil
	cloned.answers = append([]string(nil), s.answers...)
	cloned.resolved = append([]askUserAnswer(nil), s.resolved...)
	return &cloned
}

func (s *askUserState) clear() {
	requester := s.requester
	*s = askUserState{requester: requester}
}

func (s *askUserState) move(delta int) {
	if !s.active() {
		return
	}
	rows := len(s.request.Questionnaire.Questions[s.index].Options) + 1
	s.selected = (s.selected + delta + rows) % rows
	s.editing = s.selected == len(s.request.Questionnaire.Questions[s.index].Options)
}

func (s *askUserState) answer(value string) bool {
	s.answers = append(s.answers, value)
	s.index++
	s.selected, s.draft, s.editing = 0, "", false
	return s.index == len(s.request.Questionnaire.Questions)
}

func (s *askUserState) resolve(requester *replaskuser.Requester, cancelled bool) {
	if !s.active() {
		return
	}
	result := tools.AskUserResult{Answers: append([]string(nil), s.answers...), Cancelled: cancelled}
	if requester != nil {
		requester.Respond(s.request, result)
	}
	s.resolved = make([]askUserAnswer, len(s.answers))
	for i, answer := range s.answers {
		s.resolved[i] = askUserAnswer{question: s.request.Questionnaire.Questions[i].Question, answer: answer}
	}
	s.request = nil
	s.editing = false
	s.cancelled = cancelled
	s.completed = true
}

func (m *replModel) clearAskUser() {
	if m.stream.handler != nil {
		m.stream.handler.SetAskUser(nil)
	}
	m.askUser.clear()
}

func (m *replModel) appendResolvedAskUserSegment() {
	if !m.askUser.completed || m.stream.handler == nil {
		return
	}
	m.stream.handler.SetAskUser(&m.askUser)
	m.clearAskUser()
}

func renderAskUserCard(s askUserState, width int) string {
	if !s.visible() {
		return ""
	}
	contentWidth := max(width-askUserHorizontalMargin*2, 1)
	var content strings.Builder
	if s.active() {
		renderActiveAskUserCard(&content, s, contentWidth)
	} else {
		renderResolvedAskUserCard(&content, s, contentWidth)
	}
	body := strings.TrimRight(content.String(), "\n")
	margin := strings.Repeat(" ", askUserHorizontalMargin)
	body = margin + strings.ReplaceAll(body, "\n", "\n"+margin)
	rule := repltheme.AskUserRuleStyle.Render(strings.Repeat("─", max(width, 1)))
	return "\n" + rule + "\n" + body + "\n" + rule + "\n"
}

func renderActiveAskUserCard(content *strings.Builder, s askUserState, width int) {
	question := s.request.Questionnaire.Questions[s.index]
	for i, answer := range s.answers {
		content.WriteString(repltheme.AskUserResolvedStyle.Render("• " + s.request.Questionnaire.Questions[i].Question + ": " + answer))
		content.WriteString("\n")
	}
	if len(s.answers) > 0 {
		content.WriteString("\n")
	}
	content.WriteString(repltheme.AskUserProgressStyle.Render("Question " + strconv.Itoa(s.index+1) + " of " + strconv.Itoa(len(s.request.Questionnaire.Questions))))
	content.WriteString("\n")
	content.WriteString(wrapAskUserText(question.Question, repltheme.AskUserQuestionStyle, width, "", ""))
	content.WriteString("\n\n")
	for i, option := range question.Options {
		badge := ""
		if i == 0 {
			badge = " " + repltheme.AskUserBadgeStyle.Render("(recommended)")
		}
		content.WriteString(renderAskUserOption(i == s.selected, option+badge, repltheme.NormalStyle, true, width))
	}
	custom := "Type your answer"
	if s.draft != "" {
		custom = s.draft + repltheme.AskUserCursorStyle.Render(" ")
	} else if s.editing {
		custom = repltheme.AskUserCursorStyle.Render("T") + "ype your answer"
	}
	customStyle := repltheme.AskUserCustomStyle
	if s.draft != "" {
		customStyle = repltheme.AskUserTypedStyle
	}
	content.WriteString(renderAskUserOption(s.selected == len(question.Options), custom, customStyle, false, width))
	content.WriteString("\n")
	hint := "↑/↓ navigate · Enter select · Esc cancel"
	if s.selected == len(question.Options) && s.editing {
		hint = "Enter submit · Esc cancel"
	}
	content.WriteString(repltheme.AskUserHintStyle.Render(hint))
	content.WriteString("\n")
}

func renderResolvedAskUserCard(content *strings.Builder, s askUserState, width int) {
	header := "Answers provided"
	if s.cancelled {
		header = "↩ Questions cancelled"
	}
	content.WriteString(repltheme.AskUserProgressStyle.Render(header))
	if len(s.resolved) == 0 {
		content.WriteString("\n")
		return
	}
	content.WriteString("\n\n")
	for _, answer := range s.resolved {
		content.WriteString(wrapAskUserText(answer.question+": "+answer.answer, repltheme.AskUserResolvedStyle, width, "• ", "  "))
		content.WriteString("\n")
	}
}

func renderAskUserOption(selected bool, text string, style lipgloss.Style, highlightSelected bool, width int) string {
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
	return wrapAskUserText(text, style, width, prefix, "    ") + "\n"
}

func wrapAskUserText(text string, style lipgloss.Style, width int, prefix, continuation string) string {
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
