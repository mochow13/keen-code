package repl

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/mochow13/keen-code/internal/cli/repl/agentcore"
	"github.com/mochow13/keen-code/internal/llm"

	replmarkdown "github.com/mochow13/keen-code/internal/cli/repl/markdown"
	reploutput "github.com/mochow13/keen-code/internal/cli/repl/output"
	repltheme "github.com/mochow13/keen-code/internal/cli/repl/theme"
	"github.com/mochow13/keen-code/internal/session"
)

type sessionReplay struct {
	output  *reploutput.OutputBuilder
	handler *StreamHandler
}

func newSessionReplay(width int, mdRenderer *replmarkdown.Renderer, workingDir string) *sessionReplay {
	outputWidth := defaultWidth
	if width > 0 {
		outputWidth = width
	}

	handler := NewStreamHandler(mdRenderer)
	handler.lastWidth = width
	handler.workingDir = workingDir
	if handler.lastWidth <= 0 {
		handler.lastWidth = defaultWidth
	}

	return &sessionReplay{
		output:  reploutput.NewOutputBuilder(outputWidth, workingDir),
		handler: handler,
	}
}

func (r *sessionReplay) applyEvent(event session.Event) {
	switch event.Kind {
	case session.KindUserMessage:
		r.flushDone()
		if event.UserMessage != nil {
			r.output.AddUserInput(llm.StripModeSuffix(event.UserMessage.Content), repltheme.PromptStyle)
		}
	case session.KindAssistantTurn:
		r.applyAssistantTurn(event.AssistantTurn)
	case session.KindCompactionApplied:
		r.applyCompaction(event.CompactionApplied)
	}
}

func (r *sessionReplay) applyAssistantTurn(turn *session.AssistantTurnPayload) {
	if turn == nil {
		return
	}

	replayTranscript(r.handler, turn.Transcript)

	switch {
	case turn.Interrupted:
		r.flushInterrupt()
	case turn.Error != "":
		r.flushError(turn.Error)
	default:
		r.flushDone()
	}
}

func (r *sessionReplay) applyCompaction(compaction *session.CompactionAppliedPayload) {
	r.flushDone()
	if compaction == nil {
		return
	}

	if len(compaction.Transcript) > 0 {
		replayTranscript(r.handler, compaction.Transcript)
		r.flushDone()
		return
	}

	if compaction.Status != "" {
		reploutput.AddCompactionSuccessStatus(r.output, compaction.Status)
	}
}

func (r *sessionReplay) flushDone() {
	if !r.handler.HasContent() {
		return
	}
	lines, _ := r.handler.HandleDone()
	for _, line := range lines {
		r.output.AddLine(line)
	}
	r.output.AddEmptyLine()
}

func (r *sessionReplay) flushInterrupt() {
	if r.handler.HasContent() {
		lines := r.handler.HandleInterrupt()
		for _, line := range lines {
			r.output.AddLine(line)
		}
	}
	r.output.AddStyledLine("\n  "+interruptedPromptText, repltheme.InterruptedStyle)
	r.output.AddEmptyLine()
}

func (r *sessionReplay) flushError(errText string) {
	if r.handler.HasContent() {
		lines, _ := r.handler.HandleError(errors.New(errText))
		for _, line := range lines {
			r.output.AddLine(line)
		}
	}
	if errText != "" {
		r.output.AddError(errText, repltheme.ErrorStyle)
	}
}

func replayTranscript(handler *StreamHandler, transcript []session.TranscriptItem) {
	if handler == nil {
		return
	}

	for _, item := range transcript {
		switch item.Kind {
		case session.TranscriptItemText:
			handler.HandleChunk(item.Content)
		case session.TranscriptItemReasoning:
			handler.HandleReasoningChunk(item.Content)
		case session.TranscriptItemToolStart:
			if item.ToolStart == nil || item.ToolStart.Name != agentcore.ToolNameAskUser {
				handler.HandleToolStart(toolCallFromPayload(item.ToolStart))
			}
		case session.TranscriptItemToolEnd:
			if item.ToolEnd != nil && item.ToolEnd.Name == agentcore.ToolNameAskUser {
				if state := askUserStateFromPayload(item.ToolEnd); state != nil {
					handler.SetAskUser(state)
				}
			} else {
				handler.HandleToolEnd(toolCallResultFromPayload(item.ToolEnd))
			}
		case session.TranscriptItemBash:
			replayBashPayload(handler, item.Bash)
		case session.TranscriptItemDiff:
			if item.Diff != nil {
				handler.HandleDiff(agentcore.FromToolDiffLines(item.Diff.Lines))
			}
		}
	}
}

func askUserStateFromPayload(payload *session.ToolEndPayload) *askUserState {
	if payload == nil {
		return nil
	}

	var request agentcore.AskUserRequest
	var result agentcore.AskUserResult
	if !decodeSessionPayload(payload.Input, &request) || !decodeSessionPayload(payload.Output, &result) {
		return nil
	}

	state := &askUserState{completed: true, cancelled: result.Cancelled}
	for i, answer := range result.Answers {
		if i >= len(request.Questions) {
			break
		}
		state.resolved = append(state.resolved, askUserAnswer{question: request.Questions[i].Question, answer: answer})
	}
	return state
}

func decodeSessionPayload(value any, target any) bool {
	encoded, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return json.Unmarshal(encoded, target) == nil
}

func replayBashPayload(handler *StreamHandler, payload *session.BashPayload) {
	if handler == nil || payload == nil {
		return
	}

	handler.HandleBashStart(payload.Command, payload.Summary)
	handler.HandleBashEnd(&agentcore.ToolCall{
		Name:     agentcore.ToolNameBash,
		Output:   map[string]any{"stdout": payload.Output},
		Error:    payload.Error,
		Duration: time.Duration(payload.DurationNS),
	})
}
