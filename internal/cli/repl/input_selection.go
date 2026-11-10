package repl

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type inputSelection struct {
	value   string
	maxLine int

	mouseDown bool
	anchor    selectionPoint
	cursor    selectionPoint

	lastClickTime time.Time
	lastClickX    int
	lastClickY    int
	clickCount    int
}

func (s *inputSelection) setContent(value string) {
	s.value = value
}

func (s *inputSelection) start(localX, localY, yOffset int) {
	p := selectionPoint{line: clampInt(yOffset+localY, 0, max(0, s.maxLine)), col: max(localX, 0)}
	s.mouseDown = true
	s.anchor = p
	s.cursor = p
}

func (s *inputSelection) drag(localX, localY, yOffset int) bool {
	if !s.mouseDown {
		return false
	}
	s.cursor = selectionPoint{line: clampInt(yOffset+localY, 0, max(0, s.maxLine)), col: max(localX, 0)}
	return true
}

func (s *inputSelection) release() bool {
	if !s.mouseDown {
		return false
	}
	s.mouseDown = false
	return true
}

func (s *inputSelection) clear() {
	s.mouseDown = false
	s.anchor = selectionPoint{}
	s.cursor = selectionPoint{}
	s.clickCount = 0
}

func (s inputSelection) hasSelection() bool {
	return s.anchor.line != s.cursor.line || s.anchor.col != s.cursor.col
}

func (s *inputSelection) registerClick(localX, localY int) int {
	now := time.Now()
	if now.Sub(s.lastClickTime) <= selectionClickThreshold &&
		absInt(localX-s.lastClickX) <= selectionClickTolerance &&
		absInt(localY-s.lastClickY) <= selectionClickTolerance {
		s.clickCount++
	} else {
		s.clickCount = 1
	}
	s.lastClickTime = now
	s.lastClickX = localX
	s.lastClickY = localY
	return s.clickCount
}

func (s *inputSelection) selectWord(localX, localY, yOffset int) bool {
	visualLine := clampInt(yOffset+localY, 0, max(0, s.maxLine))
	lines := strings.Split(s.value, "\n")
	lineIndex := min(visualLine, len(lines)-1)
	if lineIndex < 0 {
		return false
	}
	line := ansi.Strip(lines[lineIndex])
	startCol, endCol := findSelectionWordBoundaries(line, max(localX, 0))
	if startCol == endCol {
		return false
	}
	s.mouseDown = true
	s.anchor = selectionPoint{line: visualLine, col: startCol}
	s.cursor = selectionPoint{line: visualLine, col: endCol}
	return true
}

func (s *inputSelection) selectLine(localY, yOffset int) bool {
	visualLine := clampInt(yOffset+localY, 0, max(0, s.maxLine))
	lines := strings.Split(s.value, "\n")
	lineIndex := min(visualLine, len(lines)-1)
	if lineIndex < 0 {
		return false
	}
	lineWidth := ansi.StringWidth(lines[lineIndex])
	if lineWidth == 0 {
		return false
	}
	s.mouseDown = true
	s.anchor = selectionPoint{line: visualLine, col: 0}
	s.cursor = selectionPoint{line: visualLine, col: lineWidth}
	return true
}

func (s inputSelection) selectedText() string {
	if !s.hasSelection() {
		return ""
	}
	start, end := s.normalizedRange()
	lines := strings.Split(s.value, "\n")
	if len(lines) == 0 {
		return ""
	}
	end.line = min(end.line, len(lines)-1)
	start.line = min(start.line, len(lines)-1)
	if start.line > end.line {
		return ""
	}

	parts := make([]string, 0, end.line-start.line+1)
	for lineIndex := start.line; lineIndex <= end.line; lineIndex++ {
		line := ansi.Strip(lines[lineIndex])
		lineWidth := ansi.StringWidth(line)
		colStart := 0
		if lineIndex == start.line {
			colStart = clampInt(start.col, 0, lineWidth)
		}
		colEnd := lineWidth
		if lineIndex == end.line {
			colEnd = clampInt(end.col, 0, lineWidth)
		}
		if colEnd < colStart {
			colStart, colEnd = colEnd, colStart
		}
		parts = append(parts, ansi.Cut(line, colStart, colEnd))
	}
	return strings.TrimRight(strings.Join(parts, "\n"), "\n")
}

func (s inputSelection) normalizedRange() (selectionPoint, selectionPoint) {
	start, end := s.anchor, s.cursor
	if end.line < start.line || (end.line == start.line && end.col < start.col) {
		start, end = end, start
	}
	return start, end
}

func (s inputSelection) render(view string, width, height, yOffset, colOffset int) string {
	return renderSelection(view, width, height, yOffset, colOffset, s.anchor, s.cursor)
}

func (m *replModel) handleInputSelectionMouseDown(msg tea.MouseClickMsg) (bool, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return false, nil
	}
	if !m.mouseInInputTextArea(mouse.X, mouse.Y) {
		if m.inputSelection.hasSelection() {
			m.inputSelection.clear()
		}
		return false, nil
	}

	cmd := m.focusInput()
	m.inputSelection.setContent(m.textarea.Value())
	m.inputSelection.maxLine = m.textarea.ScrollYOffset() + m.textarea.Height() - 1
	m.selection.clear()
	x, y := m.inputSelectionLocalPosition(mouse.X, mouse.Y)
	clickCount := m.inputSelection.registerClick(x, y)
	switch clickCount {
	case 2:
		if !m.inputSelection.selectWord(x, y, m.textarea.ScrollYOffset()) {
			m.inputSelection.start(x, y, m.textarea.ScrollYOffset())
		}
	case 3:
		if !m.inputSelection.selectLine(y, m.textarea.ScrollYOffset()) {
			m.inputSelection.start(x, y, m.textarea.ScrollYOffset())
		}
		m.inputSelection.clickCount = 0
	default:
		m.inputSelection.start(x, y, m.textarea.ScrollYOffset())
	}
	return true, cmd
}

func (m *replModel) handleInputSelectionMouseDrag(msg tea.MouseMotionMsg) bool {
	if !m.inputSelection.mouseDown {
		return false
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft && mouse.Button != tea.MouseNone {
		return false
	}

	x, y := m.inputSelectionLocalPosition(mouse.X, mouse.Y)
	m.inputSelection.setContent(m.textarea.Value())
	m.inputSelection.maxLine = m.textarea.ScrollYOffset() + m.textarea.Height() - 1
	m.inputSelection.drag(x, y, m.textarea.ScrollYOffset())
	return true
}

func (m *replModel) handleInputSelectionMouseUp() (bool, tea.Cmd) {
	if !m.inputSelection.release() {
		return false, nil
	}
	return true, m.copySelectedTextCmd(m.inputSelection.selectedText())
}

func (m replModel) mouseInInputTextArea(x, y int) bool {
	inputTop := m.inputAreaTop()
	textTop := inputTop + 1
	return x >= 0 && y >= textTop && y < textTop+m.textarea.Height()
}

func (m replModel) inputSelectionLocalPosition(x, y int) (int, int) {
	localY := clampInt(y-m.inputAreaTop()-1, 0, max(0, m.textarea.Height()-1))
	localX := clampInt(x-inputPromptWidth, 0, max(0, m.textarea.Width()-1))
	return localX, localY
}

func (m replModel) inputAreaTop() int {
	return m.viewport.Height() + m.spinnerHeight() + m.notificationHeight() + m.todoPanelHeight() + m.queuedHeight()
}
