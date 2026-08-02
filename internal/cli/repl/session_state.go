package repl

import (
	"time"

	"github.com/user/keen-code/internal/llm"
	"github.com/user/keen-code/internal/session"
	"github.com/user/keen-code/internal/tools"
)

const interruptedPromptText = "Interrupted...what should the agent do instead?"

type replSessionState struct {
	store   *session.Store
	current *session.Session
}

func newReplSessionState(workingDir string) *replSessionState {
	store, err := session.NewStore(workingDir)
	if err != nil {
		return nil
	}
	return &replSessionState{store: store}
}

func (s *replSessionState) ensureCurrent() error {
	if s == nil || s.store == nil || s.current != nil {
		return nil
	}

	current, err := s.store.Create()
	if err != nil {
		return err
	}
	s.current = current
	return nil
}

func (s *replSessionState) appendUserMessage(content string) error {
	if s == nil {
		return nil
	}
	if err := s.ensureCurrent(); err != nil {
		return err
	}
	return s.store.Append(s.current, session.Event{
		Kind:        session.KindUserMessage,
		UserMessage: &session.MessagePayload{Content: content},
	})
}

func (s *replSessionState) appendAssistantTurn(
	segments []streamSegment,
	message llm.Message,
	interrupted bool,
	errText string,
) error {
	if s == nil || s.current == nil {
		return nil
	}
	return s.store.Append(s.current, buildAssistantTurnEvent(segments, message, interrupted, errText))
}

func (s *replSessionState) appendCompaction(segments []streamSegment, messages []llm.Message, status string) error {
	if s == nil || s.current == nil {
		return nil
	}
	return s.store.Append(s.current, buildCompactionEvent(segments, messages, status))
}

func (s *replSessionState) appendAutoCompaction(checkpointSegments []streamSegment, checkpoint llm.Message, messages []llm.Message) error {
	if s == nil || s.current == nil {
		return nil
	}
	return s.store.AppendBatch(s.current, []session.Event{
		buildAssistantTurnEvent(checkpointSegments, checkpoint, false, ""),
		buildCompactionEvent(nil, messages, ""),
	})
}

func buildCompactionEvent(segments []streamSegment, messages []llm.Message, status string) session.Event {
	return session.Event{
		Kind: session.KindCompactionApplied,
		CompactionApplied: &session.CompactionAppliedPayload{
			Status:     status,
			Transcript: buildAssistantTurnTranscript(segments),
			Messages:   cloneLLMMessages(messages),
		},
	}
}

func (s *replSessionState) resetSession() {
	if s == nil {
		return
	}
	s.current = nil
}

func (s *replSessionState) currentID() string {
	if s == nil || s.current == nil {
		return ""
	}
	return s.current.ID
}

func (s *replSessionState) listSessions() ([]session.Summary, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	return s.store.List()
}

func (s *replSessionState) load(summary session.Summary) (*session.LoadedSession, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}

	loaded, err := s.store.Load(summary)
	if err != nil {
		return nil, err
	}

	s.current = loaded.Session
	return loaded, nil
}

func (s *replSessionState) setSession(session *session.Session) {
	if s == nil {
		return
	}
	s.current = session
}

func buildAssistantTurnEvent(
	segments []streamSegment,
	message llm.Message,
	interrupted bool,
	errText string,
) session.Event {
	return session.Event{
		Kind: session.KindAssistantTurn,
		AssistantTurn: &session.AssistantTurnPayload{
			Transcript:  buildAssistantTurnTranscript(segments),
			Message:     message.Content,
			TurnMemory:  llm.CloneTurnMemory(message.TurnMemory),
			Interrupted: interrupted,
			Error:       errText,
		},
	}
}

func buildAssistantTurnTranscript(segments []streamSegment) []session.TranscriptItem {
	items := make([]session.TranscriptItem, 0, len(segments))

	for _, seg := range segments {
		switch seg.kind {
		case segmentAssistant:
			if seg.content != "" {
				items = append(items, session.TranscriptItem{
					Kind:    session.TranscriptItemText,
					Content: seg.content,
				})
			}
		case segmentReasoning:
			if seg.content != "" {
				items = append(items, session.TranscriptItem{
					Kind:    session.TranscriptItemReasoning,
					Content: seg.content,
				})
			}
		case segmentToolStart:
			if seg.toolCall != nil {
				items = append(items, session.TranscriptItem{
					Kind: session.TranscriptItemToolStart,
					ToolStart: &session.ToolStartPayload{
						Name:  seg.toolCall.Name,
						Input: cloneInput(seg.toolCall.Input),
					},
				})
			}
		case segmentToolEnd:
			if seg.toolCall != nil {
				items = append(items, session.TranscriptItem{
					Kind: session.TranscriptItemToolEnd,
					ToolEnd: &session.ToolEndPayload{
						Name:       seg.toolCall.Name,
						Input:      cloneInput(seg.toolCall.Input),
						Output:     seg.toolCall.Output,
						Error:      seg.toolCall.Error,
						DurationNS: seg.toolCall.Duration.Nanoseconds(),
					},
				})
			}
		case segmentBash:
			duration := int64(0)
			errText := ""
			if seg.toolCall != nil {
				duration = seg.toolCall.Duration.Nanoseconds()
				errText = seg.toolCall.Error
			}
			items = append(items, session.TranscriptItem{
				Kind: session.TranscriptItemBash,
				Bash: &session.BashPayload{
					Command:    seg.command,
					Summary:    seg.summary,
					Output:     seg.output,
					Error:      errText,
					DurationNS: duration,
				},
			})
		case segmentDiff:
			if len(seg.diffLines) > 0 {
				lines := make([]tools.EditDiffLine, len(seg.diffLines))
				copy(lines, seg.diffLines)
				items = append(items, session.TranscriptItem{
					Kind: session.TranscriptItemDiff,
					Diff: &session.DiffPayload{Lines: lines},
				})
			}
		}
	}

	return items
}

func cloneInput(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}

	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func cloneLLMMessages(messages []llm.Message) []llm.Message {
	return llm.CloneMessages(messages)
}

func cloneStreamSegments(segments []streamSegment) []streamSegment {
	result := make([]streamSegment, len(segments))
	for i, seg := range segments {
		result[i] = seg
		if seg.toolCall != nil {
			toolCall := *seg.toolCall
			toolCall.Input = cloneInput(seg.toolCall.Input)
			result[i].toolCall = &toolCall
		}
		if len(seg.diffLines) > 0 {
			diffLines := make([]tools.EditDiffLine, len(seg.diffLines))
			copy(diffLines, seg.diffLines)
			result[i].diffLines = diffLines
		}
	}
	return result
}

func toolCallFromPayload(payload *session.ToolStartPayload) *llm.ToolCall {
	if payload == nil {
		return nil
	}
	return &llm.ToolCall{
		Name:  payload.Name,
		Input: cloneInput(payload.Input),
	}
}

func toolCallResultFromPayload(payload *session.ToolEndPayload) *llm.ToolCall {
	if payload == nil {
		return nil
	}
	return &llm.ToolCall{
		Name:     payload.Name,
		Input:    cloneInput(payload.Input),
		Output:   payload.Output,
		Error:    payload.Error,
		Duration: time.Duration(payload.DurationNS),
	}
}
