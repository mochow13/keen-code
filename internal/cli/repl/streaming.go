package repl

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/keen-cli/internal/llm"
)

type StreamHandler struct {
	isActive        bool
	currentResponse string
	eventCh         <-chan llm.StreamEvent
	loadingText     string
	mdRenderer      *MarkdownRenderer
}

func NewStreamHandler(mdRenderer *MarkdownRenderer) *StreamHandler {
	return &StreamHandler{
		mdRenderer: mdRenderer,
	}
}

func (sh *StreamHandler) Start(eventCh <-chan llm.StreamEvent, loadingText string) {
	sh.isActive = true
	sh.currentResponse = ""
	sh.eventCh = eventCh
	sh.loadingText = loadingText
}

func (sh *StreamHandler) IsActive() bool {
	return sh.isActive
}

func (sh *StreamHandler) GetResponse() string {
	return sh.currentResponse
}

func (sh *StreamHandler) GetLoadingText() string {
	return sh.loadingText
}

func (sh *StreamHandler) HasContent() bool {
	return sh.currentResponse != ""
}

func (sh *StreamHandler) HandleChunk(chunk string) tea.Cmd {
	sh.currentResponse += chunk
	return sh.waitForNextEvent()
}

func (sh *StreamHandler) HandleDone() ([]string, string) {
	response := sh.currentResponse
	sh.isActive = false
	sh.currentResponse = ""
	sh.eventCh = nil
	sh.loadingText = ""

	if sh.mdRenderer != nil {
		rendered := sh.mdRenderer.Render(response)
		lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
		formattedLines := make([]string, len(lines))
		for i, line := range lines {
			formattedLines[i] = "  " + line
		}
		return formattedLines, response
	}

	return formatResponseLines(response), response
}

func (sh *StreamHandler) HandleError(err error) string {
	sh.isActive = false
	sh.currentResponse = ""
	sh.eventCh = nil
	sh.loadingText = ""
	return err.Error()
}

func (sh *StreamHandler) WaitForEvent() tea.Cmd {
	return sh.waitForNextEvent()
}

func (sh *StreamHandler) waitForNextEvent() tea.Cmd {
	return func() tea.Msg {
		event, ok := <-sh.eventCh
		if !ok {
			return llmDoneMsg{}
		}

		switch event.Type {
		case llm.StreamEventTypeChunk:
			return llmChunkMsg(event.Content)
		case llm.StreamEventTypeDone:
			return llmDoneMsg{}
		case llm.StreamEventTypeError:
			return llmErrorMsg{err: event.Error}
		default:
			return llmDoneMsg{}
		}
	}
}

func (sh *StreamHandler) View(width int, showSpinner bool, spinnerView string) string {
	var view strings.Builder

	if showSpinner {
		view.WriteString("\n  " + spinnerView + " " + sh.loadingText)
	}

	if sh.isActive && sh.currentResponse != "" {
		if sh.mdRenderer != nil {
			rendered := sh.mdRenderer.Render(sh.currentResponse)
			lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
			for _, line := range lines {
				view.WriteString("\n  " + line)
			}
		} else {
			responseLines := strings.Split(sh.currentResponse, "\n")
			wrapStyle := lipgloss.NewStyle().Width(width - 4)
			for _, line := range responseLines {
				view.WriteString("\n  " + wrapStyle.Render(assistantStyle.Render(line)))
			}
		}
	}

	return view.String()
}

func formatResponseLines(response string) []string {
	lines := strings.Split(response, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = "  " + line
	}
	return result
}

type llmChunkMsg string
type llmDoneMsg struct{}
type llmErrorMsg struct {
	err error
}
