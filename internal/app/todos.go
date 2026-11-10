package app

import (
	"slices"

	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/session"
	"github.com/mochow13/keen-code/internal/tools"
)

type Todos = tools.TodoList

func TodosFromToolCall(name string, input map[string]any, errText string) (Todos, bool) {
	if name != tools.TodoWriteToolName || errText != "" {
		return nil, false
	}
	todos, err := tools.ParseTodoWriteInput(input)
	if err != nil {
		return nil, false
	}
	return todos, true
}

func TodosFromEvents(events []session.Event) Todos {
	for _, event := range slices.Backward(events) {
		var transcript []session.TranscriptItem
		switch {
		case event.AssistantTurn != nil:
			transcript = event.AssistantTurn.Transcript
		case event.CompactionApplied != nil:
			transcript = event.CompactionApplied.Transcript
		}
		for _, item := range slices.Backward(transcript) {
			if item.Kind != session.TranscriptItemToolEnd || item.ToolEnd == nil {
				continue
			}
			if todos, ok := TodosFromToolCall(item.ToolEnd.Name, item.ToolEnd.Input, item.ToolEnd.Error); ok {
				return todos
			}
		}
	}
	return nil
}

func (s *Session) Todos() Todos {
	if s == nil {
		return nil
	}
	return s.todos
}

func (s *Session) ObserveToolCall(call *agentcore.ToolCall) bool {
	if s == nil || call == nil {
		return false
	}
	todos, ok := TodosFromToolCall(call.Name, call.Input, call.Error)
	if !ok {
		return false
	}
	s.todos = todos
	return true
}
