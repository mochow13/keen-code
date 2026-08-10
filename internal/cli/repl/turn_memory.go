package repl

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/mochow13/keen-code/internal/llm"
)

const maxHistoricalToolInputFieldBytes = 4 * 1024

var retainedHistoricalToolInputs = map[string]struct{}{
	"read_file":     {},
	"grep":          {},
	"glob":          {},
	"web_fetch":     {},
	"bash":          {},
	"delegate_task": {},
	"call_mcp_tool": {},
	"write_file":    {},
	"edit_file":     {},
}

type turnMemoryAccumulator struct {
	toolActivity []llm.HistoricalToolActivity
}

func newTurnMemoryAccumulator() *turnMemoryAccumulator {
	return &turnMemoryAccumulator{}
}

func (a *turnMemoryAccumulator) RecordToolActivity(segments []streamSegment, workingDir string) {
	if a == nil {
		return
	}
	a.toolActivity = collectHistoricalToolActivity(segments, workingDir)
}

func collectHistoricalToolActivity(segments []streamSegment, workingDir string) []llm.HistoricalToolActivity {
	textOffset := 0
	activities := make([]llm.HistoricalToolActivity, 0)

	for _, segment := range segments {
		switch segment.kind {
		case segmentAssistant:
			textOffset += len(segment.content)
		case segmentToolEnd:
			if segment.toolCall != nil {
				activities = append(activities, historicalToolActivity(segment.toolCall, textOffset, workingDir, ""))
			}
		case segmentBash:
			if segment.toolCall != nil {
				activities = append(activities, historicalToolActivity(segment.toolCall, textOffset, workingDir, segment.command))
			}
		}
	}

	return activities
}

func historicalToolActivity(toolCall *llm.ToolCall, textOffset int, workingDir, bashCommand string) llm.HistoricalToolActivity {
	activity := llm.HistoricalToolActivity{
		TextOffset: textOffset,
		Tool:       toolCall.Name,
		Status:     "success",
	}
	if toolCall.Error != "" {
		activity.Status = "error"
	}

	if _, ok := retainedHistoricalToolInputs[toolCall.Name]; ok {
		input := toolCall.Input
		if retainsPathInput(toolCall.Name) {
			input = cloneToolInput(input)
			if path, ok := input["path"].(string); ok {
				input["path"] = relativizePath(path, workingDir)
			}
		}
		if toolCall.Name == "bash" && bashCommand != "" {
			input = cloneToolInput(input)
			input["command"] = bashCommand
		}
		activity.Input = boundedHistoricalToolInput(input, truncatesHistoricalToolInput(toolCall.Name))
	}

	if toolCall.Name == "bash" {
		exitCode, ok := extractIntField(toolCall.Output, "exit_code")
		if ok && exitCode != 0 {
			activity.ExitCode = &exitCode
		}
	}
	return activity
}

func boundedHistoricalToolInput(input map[string]any, truncateOversizedStrings bool) map[string]any {
	if len(input) == 0 {
		return nil
	}

	bounded := make(map[string]any, len(input))
	for key, value := range input {
		if text, ok := value.(string); ok && truncateOversizedStrings {
			bounded[key] = truncateUTF8(text, maxHistoricalToolInputFieldBytes)
			continue
		}
		encoded, err := json.Marshal(value)
		if err == nil && len(encoded) <= maxHistoricalToolInputFieldBytes {
			bounded[key] = value
		}
	}
	if len(bounded) == 0 {
		return nil
	}
	return bounded
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && !utf8.RuneStart(value[maxBytes]) {
		maxBytes--
	}
	return value[:maxBytes]
}

func cloneToolInput(input map[string]any) map[string]any {
	cloned := make(map[string]any, len(input)+1)
	maps.Copy(cloned, input)
	return cloned
}

func retainsPathInput(tool string) bool {
	return tool == "read_file" || tool == "grep" || tool == "glob" || tool == "write_file" || tool == "edit_file"
}

func truncatesHistoricalToolInput(tool string) bool {
	return tool == "write_file" || tool == "edit_file"
}

func (a *turnMemoryAccumulator) Build() *llm.TurnMemory {
	if a == nil || len(a.toolActivity) == 0 {
		return nil
	}

	return llm.CloneTurnMemory(&llm.TurnMemory{ToolActivity: a.toolActivity})
}

func extractIntField(output any, key string) (int, bool) {
	result, ok := output.(map[string]any)
	if !ok {
		return 0, false
	}

	switch value := result[key].(type) {
	case int:
		return value, true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

func (m *replModel) startAssistantTurnMemory() {
	if m == nil {
		return
	}
	m.turnMemory = newTurnMemoryAccumulator()
}

func (m *replModel) recordHistoricalToolActivity(segments []streamSegment) {
	if m == nil || m.turnMemory == nil {
		return
	}
	m.turnMemory.RecordToolActivity(segments, m.turnMemoryWorkingDir())
}

func (m *replModel) consumeTurnMemory() *llm.TurnMemory {
	if m == nil || m.turnMemory == nil {
		return nil
	}
	memory := m.turnMemory.Build()
	m.turnMemory = nil
	return memory
}

func (m *replModel) clearTurnMemory() {
	if m == nil {
		return
	}
	m.turnMemory = nil
}

func (m *replModel) turnMemoryWorkingDir() string {
	if m == nil {
		return ""
	}
	if m.appState != nil && m.appState.WorkingDir() != "" {
		return m.appState.WorkingDir()
	}
	if m.ctx != nil {
		return m.ctx.workingDir
	}
	return ""
}

func relativizePath(path string, workingDir string) string {
	if path == "" || workingDir == "" || !filepath.IsAbs(path) {
		return path
	}

	relativePath, err := filepath.Rel(workingDir, path)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return path
	}
	return relativePath
}
