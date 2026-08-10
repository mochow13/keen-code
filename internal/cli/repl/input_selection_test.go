package repl

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestInputSelection_MaxLineAllowsVisualLines(t *testing.T) {
	var selection inputSelection
	selection.setContent("hello\nworld")
	selection.maxLine = 4

	selection.start(0, 3, 0)
	selection.drag(5, 4, 0)

	if selection.anchor.line != 3 {
		t.Fatalf("expected anchor on visual line 3, got %d", selection.anchor.line)
	}
	if selection.cursor.line != 4 {
		t.Fatalf("expected cursor on visual line 4, got %d", selection.cursor.line)
	}

	if got := selection.selectedText(); got != "world" {
		t.Fatalf("expected selected text to fall back to last logical line, got %q", got)
	}
}

func TestInputSelection_RenderWithMaxLine(t *testing.T) {
	var selection inputSelection
	selection.setContent("hello\nworld")
	selection.maxLine = 4

	selection.start(0, 3, 0)
	selection.drag(5, 4, 0)

	view := "hello\nworld\nthird\nline\nfifth"
	rendered := selection.render(view, 20, 5, 0, 0)
	if rendered == view {
		t.Fatal("expected rendered selection to add styling")
	}
	if !strings.Contains(rendered, "third") {
		t.Fatalf("expected highlight on third visual line, got %q", rendered)
	}
	if !strings.Contains(rendered, "fifth") {
		t.Fatalf("expected highlight on fifth visual line, got %q", rendered)
	}
}

func TestInputSelection_MouseDragCopiesOnRelease(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("hello world")
	m.blurInput()
	textY := m.inputAreaTop() + 1

	updated, cmd := m.updateNormalMode(tea.MouseClickMsg(tea.Mouse{X: inputPromptWidth, Y: textY, Button: tea.MouseLeft}))
	if cmd == nil {
		t.Fatal("expected focus command on input mouse down")
	}
	updated, cmd = updated.updateNormalMode(tea.MouseMotionMsg(tea.Mouse{X: inputPromptWidth + 5, Y: textY, Button: tea.MouseLeft}))
	if cmd != nil {
		t.Fatal("expected no command while dragging input selection")
	}
	updated, cmd = updated.updateNormalMode(tea.MouseReleaseMsg(tea.Mouse{X: inputPromptWidth + 5, Y: textY, Button: tea.MouseLeft}))
	if cmd == nil {
		t.Fatal("expected copy command on input mouse release")
	}
	if updated.notification.text != copyNotificationMessage {
		t.Fatalf("expected copy notification, got %q", updated.notification.text)
	}
	if got := updated.inputSelection.selectedText(); got != "hello" {
		t.Fatalf("expected input selection to remain, got %q", got)
	}
	if strings.Contains(updated.selection.selectedText(), "hello") {
		t.Fatalf("expected viewport selection to stay clear, got %q", updated.selection.selectedText())
	}
	if !updated.textarea.Focused() {
		t.Fatal("expected input click to focus textarea")
	}
}

func TestInputSelection_CtrlCDoesNotCopySelection(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("hello world")
	m.inputSelection.setContent(m.textarea.Value())
	m.inputSelection.start(0, 0, 0)
	m.inputSelection.drag(5, 0, 0)

	updated, cmd := m.updateNormalMode(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatal("expected ctrl+c to clear input without copying")
	}
	if updated.quitting {
		t.Fatal("expected ctrl+c with input value not to quit")
	}
	if got := updated.textarea.Value(); got != "" {
		t.Fatalf("expected ctrl+c to clear input, got %q", got)
	}
}

func TestInputSelection_RendersInView(t *testing.T) {
	m := newTestModel()
	m.textarea.SetValue("hello world")
	m.inputSelection.setContent(m.textarea.Value())
	m.inputSelection.start(0, 0, 0)
	m.inputSelection.drag(5, 0, 0)

	view := m.View().Content
	plainView := ansi.Strip(view)
	if !strings.Contains(plainView, "hello") {
		t.Fatalf("expected input content in view, got %q", view)
	}
	withoutSelection := newTestModel()
	withoutSelection.textarea.SetValue("hello world")
	if view == withoutSelection.View().Content {
		t.Fatal("expected input selection to affect rendered view")
	}
}

func TestInputSelection_SelectWordAndLine(t *testing.T) {
	var selection inputSelection
	selection.setContent("hello world\nsecond line")
	selection.maxLine = 1

	if !selection.selectWord(7, 0, 0) {
		t.Fatal("selectWord did not select a word")
	}
	if got := selection.selectedText(); got != "world" {
		t.Fatalf("selected word = %q, want world", got)
	}
	if !selection.selectLine(1, 0) {
		t.Fatal("selectLine did not select a line")
	}
	if got := selection.selectedText(); got != "second line" {
		t.Fatalf("selected line = %q, want second line", got)
	}
}

func TestInputSelection_RejectsWhitespaceAndEmptyLines(t *testing.T) {
	var selection inputSelection
	selection.setContent("hello world\n")
	selection.maxLine = 1

	if selection.selectWord(5, 0, 0) {
		t.Fatal("selectWord selected whitespace")
	}
	if selection.selectLine(1, 0) {
		t.Fatal("selectLine selected an empty line")
	}
	if selection.drag(1, 0, 0) || selection.release() {
		t.Fatal("inactive selection accepted drag or release")
	}
}

func TestInputSelection_SelectsVisibleTextWithoutANSI(t *testing.T) {
	var selection inputSelection
	selection.setContent("\x1b[31mred\x1b[0m text")
	selection.maxLine = 0
	if !selection.selectWord(1, 0, 0) {
		t.Fatal("selectWord did not select styled text")
	}
	if got := selection.selectedText(); got != "red" {
		t.Fatalf("selected styled word = %q, want red", got)
	}
}
