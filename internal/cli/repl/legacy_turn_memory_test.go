package repl

import (
	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/app"
	replstream "github.com/mochow13/keen-code/internal/cli/repl/stream"
)

const maxHistoricalToolInputFieldBytes = 4 * 1024

func (m *replModel) startAssistantTurnMemory() {
	m.turn = &app.Turn{}
	m.turn.ConfigureMemory(m.turnMemoryWorkingDir(), m.toolHistory == toolHistoryFull)
	var response string
	for _, segment := range m.stream.handler.Snapshot() {
		if segment.Kind == replstream.SegmentAssistant {
			response += segment.Content
		}
		if segment.Kind == replstream.SegmentToolEnd {
			m.turn.RecordToolCall(segment.ToolCall, response)
		}
	}
}

func collectHistoricalToolActivity(segments []replstream.Segment, workingDir string, retainOutput bool) []agentcore.HistoricalToolActivity {
	turn := &app.Turn{}
	turn.ConfigureMemory(workingDir, retainOutput)
	response := ""
	for _, segment := range segments {
		switch segment.Kind {
		case replstream.SegmentAssistant:
			response += segment.Content
		case replstream.SegmentToolEnd:
			turn.RecordToolCall(segment.ToolCall, response)
		case replstream.SegmentBash:
			if segment.ToolCall != nil {
				call := *segment.ToolCall
				call.Input = make(map[string]any, len(segment.ToolCall.Input)+1)
				for key, value := range segment.ToolCall.Input {
					call.Input[key] = value
				}
				if segment.Command != "" {
					call.Input["command"] = segment.Command
				}
				turn.RecordToolCall(&call, response)
			}
		}
	}
	if memory := turn.Memory(); memory != nil {
		return memory.ToolActivity
	}
	return nil
}
