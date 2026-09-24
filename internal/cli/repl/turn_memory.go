package repl

import (
	"encoding/json"
	"github.com/mochow13/keen-code/internal/agentcore"
	"maps"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/mochow13/keen-code/internal/llm/compress"
)

const maxHistoricalToolInputFieldBytes = 4 * 1024

var retainedHistoricalToolInputs = map[string]struct{}{
	agentcore.ToolNameReadFile:  {},
	agentcore.ToolNameGrep:      {},
	agentcore.ToolNameGlob:      {},
	agentcore.ToolNameWebFetch:  {},
	agentcore.ToolNameBash:      {},
	agentcore.ToolNameDelegate:  {},
	agentcore.ToolNameCallMCP:   {},
	agentcore.ToolNameWriteFile: {},
	agentcore.ToolNameEditFile:  {},
	agentcore.ToolNameAskUser:   {},
}

type turnMemoryAccumulator struct {
	toolActivity []agentcore.HistoricalToolActivity
	retainOutput bool
}

func newTurnMemoryAccumulator(retainOutput bool) *turnMemoryAccumulator {
	return &turnMemoryAccumulator{retainOutput: retainOutput}
}

func (a *turnMemoryAccumulator) RecordToolActivity(segments []streamSegment, workingDir string) {
	if a == nil {
		return
	}
	a.toolActivity = collectHistoricalToolActivity(segments, workingDir, a.retainOutput)
}

func collectHistoricalToolActivity(segments []streamSegment, workingDir string, retainOutput bool) []agentcore.HistoricalToolActivity {
	textOffset := 0
	activities := make([]agentcore.HistoricalToolActivity, 0)

	for _, segment := range segments {
		switch segment.kind {
		case segmentAssistant:
			textOffset += len(segment.content)
		case segmentToolEnd:
			if segment.toolCall != nil {
				activities = append(activities, historicalToolActivity(segment.toolCall, textOffset, workingDir, "", retainOutput))
			}
		case segmentBash:
			if segment.toolCall != nil {
				activities = append(activities, historicalToolActivity(segment.toolCall, textOffset, workingDir, segment.command, retainOutput))
			}
		}
	}

	return activities
}

func historicalToolActivity(toolCall *agentcore.ToolCall, textOffset int, workingDir, bashCommand string, retainOutput bool) agentcore.HistoricalToolActivity {
	activity := agentcore.HistoricalToolActivity{
		TextOffset: textOffset,
		Tool:       toolCall.Name,
		Status:     "success",
	}
	if toolCall.Error != "" {
		activity.Status = "error"
	}
	if retainOutput {
		activity.HasRawOutput = true
		activity.RawOutput = toolCall.Output
		if toolCall.Error != "" {
			activity.RawOutput = map[string]any{"error": toolCall.Error}
		} else {
			activity.RetainedOutput = compress.ForLLM(toolCall.Name, toolCall.Output)
		}
	} else if toolCall.Name == agentcore.ToolNameAskUser {
		activity.RetainedOutput = toolCall.Output
	}

	if _, ok := retainedHistoricalToolInputs[toolCall.Name]; ok {
		input := toolCall.Input
		if retainsPathInput(toolCall.Name) {
			input = cloneToolInput(input)
			if path, ok := input["path"].(string); ok {
				input["path"] = relativizePath(path, workingDir)
			}
		}
		if toolCall.Name == agentcore.ToolNameBash && bashCommand != "" {
			input = cloneToolInput(input)
			input["command"] = bashCommand
		}
		if toolCall.Name == agentcore.ToolNameAskUser {
			activity.Input = cloneToolInput(input)
		} else {
			activity.Input = boundedHistoricalToolInput(input, truncatesHistoricalToolInput(toolCall.Name))
		}
	}

	if toolCall.Name == agentcore.ToolNameBash {
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
	return tool == agentcore.ToolNameReadFile || tool == agentcore.ToolNameGrep || tool == agentcore.ToolNameGlob || tool == agentcore.ToolNameWriteFile || tool == agentcore.ToolNameEditFile
}

func truncatesHistoricalToolInput(tool string) bool {
	return tool == agentcore.ToolNameWriteFile || tool == agentcore.ToolNameEditFile
}

func (a *turnMemoryAccumulator) Build() *agentcore.TurnMemory {
	if a == nil || len(a.toolActivity) == 0 {
		return nil
	}

	return agentcore.CloneTurnMemory(&agentcore.TurnMemory{ToolActivity: a.toolActivity})
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
	m.turnMemory = newTurnMemoryAccumulator(m.toolHistory == toolHistoryFull)
}

func (m *replModel) recordHistoricalToolActivity(segments []streamSegment) {
	if m == nil || m.turnMemory == nil {
		return
	}
	m.turnMemory.RecordToolActivity(segments, m.turnMemoryWorkingDir())
}

func (m *replModel) consumeTurnMemory() *agentcore.TurnMemory {
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
	if m.agentCore != nil && m.agentCore.WorkingDir() != "" {
		return m.agentCore.WorkingDir()
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
