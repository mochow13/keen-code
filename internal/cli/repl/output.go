package repl

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/user/keen-cli/internal/llm"
)

type OutputBuilder struct {
	lines []string
	width int
}

func NewOutputBuilder(width int) *OutputBuilder {
	return &OutputBuilder{
		lines: []string{},
		width: width,
	}
}

func (ob *OutputBuilder) SetLines(lines []string) {
	ob.lines = lines
}

func (ob *OutputBuilder) GetLines() []string {
	return ob.lines
}

func (ob *OutputBuilder) AddLine(line string) {
	ob.lines = append(ob.lines, line)
}

func (ob *OutputBuilder) AddEmptyLine() {
	ob.lines = append(ob.lines, "")
}

func (ob *OutputBuilder) AddUserInput(input string, promptStyle lipgloss.Style) {
	inputLines := strings.Split(input, "\n")
	wrapStyle := lipgloss.NewStyle().Width(ob.width - 4)
	ob.lines = append(ob.lines, promptStyle.Render("> ")+wrapStyle.Render(inputLines[0]))
	for i := 1; i < len(inputLines); i++ {
		ob.lines = append(ob.lines, "  "+wrapStyle.Render(inputLines[i]))
	}
	ob.AddEmptyLine()
}

func (ob *OutputBuilder) AddAssistantResponse(response string, assistantStyle lipgloss.Style) {
	wrapStyle := lipgloss.NewStyle().Width(ob.width - 4)
	responseLines := strings.Split(response, "\n")
	for _, line := range responseLines {
		ob.lines = append(ob.lines, "  "+wrapStyle.Render(assistantStyle.Render(line)))
	}
	ob.AddEmptyLine()
}

func (ob *OutputBuilder) AddError(err string, errorStyle lipgloss.Style) {
	wrapStyle := lipgloss.NewStyle().Width(ob.width - 4)
	ob.lines = append(ob.lines, wrapStyle.Render(errorStyle.Render("  Error: "+err)))
	ob.AddEmptyLine()
}

func (ob *OutputBuilder) AddStyledLine(content string, style lipgloss.Style) {
	ob.lines = append(ob.lines, style.Render(content))
}

func (ob *OutputBuilder) Join() string {
	if len(ob.lines) == 0 {
		return ""
	}
	return strings.Join(ob.lines, "\n")
}

func (ob *OutputBuilder) IsEmpty() bool {
	return len(ob.lines) == 0
}

func (ob *OutputBuilder) AddToolStart(toolCall *llm.ToolCall) {
	ob.lines = append(ob.lines, formatToolStart(toolCall))
}

func (ob *OutputBuilder) AddToolEnd(toolCall *llm.ToolCall) {
	ob.lines = append(ob.lines, formatToolEnd(toolCall))
}

func formatToolStart(toolCall *llm.ToolCall) string {
	inputJSON := jsonMarshalCompact(toolCall.Input)
	return "  " + toolStartStyle.Render(fmt.Sprintf("🔧 %s(%s)...", toolCall.Name, inputJSON))
}

func formatToolEnd(toolCall *llm.ToolCall) string {
	if toolCall.Error != "" {
		return "  " + toolErrorStyle.Render(fmt.Sprintf("✗ %s failed: %s", toolCall.Name, toolCall.Error))
	}
	return "  " + toolSuccessStyle.Render(fmt.Sprintf("✓ %s (%s)", toolCall.Name, toolCall.Duration))
}

func jsonMarshalCompact(v map[string]any) string {
	if v == nil {
		return ""
	}
	var parts []string
	for k, val := range v {
		parts = append(parts, fmt.Sprintf("%s=%v", k, val))
	}
	return strings.Join(parts, ", ")
}
