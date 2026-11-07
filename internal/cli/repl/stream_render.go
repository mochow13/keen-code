package repl

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
)

func renderBtwQuestionHeader(question string) string {
	chip := repltheme.BtwChipStyle.Render("btw")
	return chip + " " + repltheme.BtwLabelStyle.Render(question)
}

func renderBtwLeftBorder(line string) string {
	border := repltheme.BtwBorderStyle.Render("▌")
	return border + " " + line
}

func (m *replModel) renderBtwInline(width int) string {
	contentWidth := width - contentHorizontalPadding
	if contentWidth < 1 {
		contentWidth = 1
	}

	var view strings.Builder
	view.WriteString("\n\n")

	header := renderBtwQuestionHeader(m.btw.question)
	for _, line := range strings.Split(ansi.Wrap(header, max(1, width-2), ""), "\n") {
		view.WriteString(renderBtwLeftBorder(line))
		view.WriteString("\n")
	}

	streamView := strings.TrimLeft(m.btw.streamHandler.View(contentWidth), "\n")
	for _, line := range strings.Split(streamView, "\n") {
		view.WriteString(renderBtwLeftBorder(line))
		view.WriteString("\n")
	}

	if m.btw.showSpinner {
		view.WriteString(renderBtwLeftBorder(m.btw.spinner.View()))
		view.WriteString("\n")
	}

	return view.String()
}

func (m *replModel) renderBtwInlineFinished(width int) string {
	var view strings.Builder
	view.WriteString("\n")

	header := renderBtwQuestionHeader(m.btw.question)
	for _, line := range strings.Split(ansi.Wrap(header, max(1, width-2), ""), "\n") {
		view.WriteString(renderBtwLeftBorder(line))
		view.WriteString("\n")
	}

	lines := m.btw.lines
	if m.btw.segments != nil && m.btw.streamHandler != nil {
		lines = m.btw.streamHandler.RenderSegments(m.btw.segments, max(1, width-contentHorizontalPadding))
		if m.btw.errorText != "" {
			lines = append(lines, "  "+repltheme.ErrorStyle.Render(m.btw.errorText))
		}
	}
	for _, line := range lines {
		view.WriteString(renderBtwLeftBorder(line))
		view.WriteString("\n")
	}

	return view.String()
}

func renderAdversaryHeader(focus string) string {
	chip := repltheme.AdversaryChipStyle.Render("adversary")
	if focus == "" {
		return chip
	}
	return chip + " " + repltheme.AdversaryLabelStyle.Render(focus)
}

func renderAdversaryLeftBorder(line string) string {
	border := repltheme.AdversaryBorderStyle.Render("▌")
	return border + " " + line
}

func (m *replModel) renderAdversaryInline(width int) string {
	contentWidth := max(width-contentHorizontalPadding, 1)

	var view strings.Builder
	view.WriteString("\n\n")

	for _, line := range strings.Split(ansi.Wrap(renderAdversaryHeader(m.adversary.focus), max(1, width-2), ""), "\n") {
		view.WriteString(renderAdversaryLeftBorder(line))
		view.WriteString("\n")
	}

	streamView := strings.TrimLeft(m.adversary.streamHandler.View(contentWidth), "\n")
	for _, line := range strings.Split(streamView, "\n") {
		view.WriteString(renderAdversaryLeftBorder(line))
		view.WriteString("\n")
	}

	if m.adversary.showSpinner {
		view.WriteString(renderAdversaryLeftBorder(m.adversary.spinner.View()))
		view.WriteString("\n")
	}

	return view.String()
}

func (m *replModel) renderAdversaryInlineFinished(width int) string {
	var view strings.Builder
	view.WriteString("\n")

	for _, line := range strings.Split(ansi.Wrap(renderAdversaryHeader(m.adversary.focus), max(1, width-2), ""), "\n") {
		view.WriteString(renderAdversaryLeftBorder(line))
		view.WriteString("\n")
	}

	lines := m.adversary.lines
	if m.adversary.segments != nil && m.adversary.streamHandler != nil {
		lines = m.adversary.streamHandler.RenderSegments(m.adversary.segments, max(1, width-contentHorizontalPadding))
		if m.adversary.errorText != "" {
			lines = append(lines, "  "+repltheme.ErrorStyle.Render(m.adversary.errorText))
		}
	}
	for _, line := range lines {
		view.WriteString(renderAdversaryLeftBorder(line))
		view.WriteString("\n")
	}

	return view.String()
}
