package repl

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/user/keen-code/internal/llm"
)

const contextProgressBarWidth = 20

type contextStatus struct {
	CurrentTokens int
	ContextWindow int
	Percent       float64
	KnownWindow   bool
}

func estimateTokensFromWordCount(words int) int {
	if words <= 0 {
		return 0
	}
	// Requirement approximation: words / 0.75 ~= words * 4 / 3.
	return (words * 4) / 3
}

func usagePercent(currentTokens, contextWindow int) float64 {
	if currentTokens <= 0 || contextWindow <= 0 {
		return 0
	}
	percent := (float64(currentTokens) * 100.0) / float64(contextWindow)
	if percent > 100 {
		return 100
	}
	if percent < 0 {
		return 0
	}
	return percent
}

func progressFillWidth(percent float64, total int) int {
	if total <= 0 {
		return 0
	}
	if percent <= 0 {
		return 0
	}
	if percent >= 100 {
		return total
	}
	return int((float64(total) * percent) / 100.0)
}

func countWords(text string) int {
	return len(strings.Fields(text))
}

func buildConversationForEstimation(workingDir string, messages []llm.Message, partialAssistant string) string {
	parts := make([]string, 0, len(messages)+2)
	if workingDir != "" {
		parts = append(parts, llm.Build(workingDir))
	}
	for _, msg := range messages {
		if msg.Content != "" {
			parts = append(parts, msg.Content)
		}
	}
	if partialAssistant != "" {
		parts = append(parts, partialAssistant)
	}
	return strings.Join(parts, "\n")
}

func (m replModel) computeContextStatus(includePartialAssistant bool) contextStatus {
	var providerID, modelID string
	if m.ctx != nil && m.ctx.cfg != nil {
		providerID = m.ctx.cfg.Provider
		modelID = m.ctx.cfg.Model
	}

	var contextWindow int
	var knownWindow bool
	if m.ctx != nil && m.ctx.registry != nil && providerID != "" && modelID != "" {
		contextWindow, knownWindow = m.ctx.registry.GetModelContextWindow(providerID, modelID)
	}

	var messages []llm.Message
	if m.appState != nil {
		messages = m.appState.GetMessages()
	}

	partial := ""
	if includePartialAssistant && m.streamHandler != nil && m.streamHandler.IsActive() {
		partial = m.streamHandler.GetResponse()
	}

	workingDir := ""
	if m.ctx != nil {
		workingDir = m.ctx.workingDir
	}

	conversation := buildConversationForEstimation(workingDir, messages, partial)
	wordCount := countWords(conversation)
	currentTokens := estimateTokensFromWordCount(wordCount)

	status := contextStatus{
		CurrentTokens: currentTokens,
		ContextWindow: contextWindow,
		KnownWindow:   knownWindow,
	}
	if knownWindow {
		status.Percent = usagePercent(currentTokens, contextWindow)
	}
	return status
}

func (m *replModel) refreshContextStatus(includePartialAssistant bool) {
	if m == nil {
		return
	}
	m.contextStatus = m.computeContextStatus(includePartialAssistant)
}

func formatPercent(percent float64) string {
	p := strconv.FormatFloat(percent, 'f', 2, 64)
	p = strings.TrimRight(p, "0")
	p = strings.TrimRight(p, ".")
	return p + "%"
}

func contextPercentStyle(percent float64) lipgloss.Style {
	if percent >= 95 {
		return contextStatusPercentCriticalStyle
	}
	if percent >= 80 {
		return contextStatusPercentWarnStyle
	}
	return contextStatusPercentStyle
}

func renderContextStatus(status contextStatus) string {
	label := contextStatusLabelStyle.Render("✎ᝰ")
	if !status.KnownWindow || status.ContextWindow <= 0 {
		return label + " " + contextStatusUnknownStyle.Render("N/A")
	}

	filled := progressFillWidth(status.Percent, contextProgressBarWidth)
	empty := contextProgressBarWidth - filled

	fill := contextBarFillStyle.Render(strings.Repeat("█", filled))
	rest := contextBarEmptyStyle.Render(strings.Repeat("░", empty))
	bar := fill + rest
	percent := contextPercentStyle(status.Percent).Render(formatPercent(status.Percent))
	return label + " " + bar + " " + percent
}
