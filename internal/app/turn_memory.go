package app

import (
	"encoding/json"
	"maps"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/llm/compress"
)

const maxHistoricalToolInputFieldBytes = 4 * 1024

var retainedHistoricalToolInputs = map[string]struct{}{
	agentcore.ToolNameReadFile: {}, agentcore.ToolNameGrep: {}, agentcore.ToolNameGlob: {},
	agentcore.ToolNameWebFetch: {}, agentcore.ToolNameBash: {}, agentcore.ToolNameDelegate: {},
	agentcore.ToolNameCallMCP: {}, agentcore.ToolNameWriteFile: {}, agentcore.ToolNameEditFile: {},
	agentcore.ToolNameAskUser: {},
}

type turnMemory struct {
	workingDir   string
	retainOutput bool
	activity     []agentcore.HistoricalToolActivity
}

func (m *turnMemory) record(call *agentcore.ToolCall, offset int) {
	if m == nil || call == nil {
		return
	}
	activity := agentcore.HistoricalToolActivity{TextOffset: offset, Tool: call.Name, Status: "success"}
	if call.Error != "" {
		activity.Status = "error"
	}
	if m.retainOutput {
		activity.HasRawOutput = true
		activity.RawOutput = call.Output
		if call.Error != "" {
			activity.RawOutput = map[string]any{"error": call.Error}
		} else {
			activity.RetainedOutput = compress.ForLLM(call.Name, call.Output)
		}
	} else if call.Name == agentcore.ToolNameAskUser {
		activity.RetainedOutput = call.Output
	}
	if _, ok := retainedHistoricalToolInputs[call.Name]; ok {
		input := call.Input
		if retainsPathInput(call.Name) {
			input = maps.Clone(input)
			if path, ok := input["path"].(string); ok {
				input["path"] = relativizePath(path, m.workingDir)
			}
		}
		if call.Name == agentcore.ToolNameAskUser {
			activity.Input = maps.Clone(input)
		} else {
			activity.Input = boundedHistoricalToolInput(input, truncatesHistoricalToolInput(call.Name))
		}
	}
	if call.Name == agentcore.ToolNameBash {
		if code, ok := extractIntField(call.Output, "exit_code"); ok && code != 0 {
			activity.ExitCode = &code
		}
	}
	m.activity = append(m.activity, activity)
}

func (m *turnMemory) build() *agentcore.TurnMemory {
	if m == nil || len(m.activity) == 0 {
		return nil
	}
	return agentcore.CloneTurnMemory(&agentcore.TurnMemory{ToolActivity: m.activity})
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

func retainsPathInput(tool string) bool {
	return tool == agentcore.ToolNameReadFile || tool == agentcore.ToolNameGrep || tool == agentcore.ToolNameGlob || tool == agentcore.ToolNameWriteFile || tool == agentcore.ToolNameEditFile
}

func truncatesHistoricalToolInput(tool string) bool {
	return tool == agentcore.ToolNameWriteFile || tool == agentcore.ToolNameEditFile
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

func relativizePath(path, workingDir string) string {
	if path == "" || workingDir == "" || !filepath.IsAbs(path) {
		return path
	}
	relativePath, err := filepath.Rel(workingDir, path)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return path
	}
	return relativePath
}
