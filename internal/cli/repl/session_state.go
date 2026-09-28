package repl

import (
	"time"

	"github.com/mochow13/keen-code/internal/agentcore"
	replstream "github.com/mochow13/keen-code/internal/cli/repl/stream"
	"github.com/mochow13/keen-code/internal/session"
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
	segments []replstream.Segment,
	message agentcore.Message,
	interrupted bool,
	errText string,
) error {
	if s == nil || s.current == nil {
		return nil
	}
	return s.store.Append(s.current, buildAssistantTurnEvent(segments, message, interrupted, errText))
}

func (s *replSessionState) appendCompaction(segments []replstream.Segment, messages []agentcore.Message, status string) error {
	if s == nil || s.current == nil {
		return nil
	}
	return s.store.Append(s.current, buildCompactionEvent(segments, messages, status))
}

func (s *replSessionState) appendAutoCompaction(checkpointSegments []replstream.Segment, checkpoint agentcore.Message, messages []agentcore.Message) error {
	if s == nil || s.current == nil {
		return nil
	}
	return s.store.AppendBatch(s.current, []session.Event{
		buildAssistantTurnEvent(checkpointSegments, checkpoint, false, ""),
		buildCompactionEvent(nil, messages, ""),
	})
}

func buildCompactionEvent(segments []replstream.Segment, messages []agentcore.Message, status string) session.Event {
	return session.Event{
		Kind: session.KindCompactionApplied,
		CompactionApplied: &session.CompactionAppliedPayload{
			Status:     status,
			Transcript: buildAssistantTurnTranscript(segments),
			Messages:   agentcore.ToCoreMessages(messages),
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
	segments []replstream.Segment,
	message agentcore.Message,
	interrupted bool,
	errText string,
) session.Event {
	return session.Event{
		Kind: session.KindAssistantTurn,
		AssistantTurn: &session.AssistantTurnPayload{
			Transcript:  buildAssistantTurnTranscript(segments),
			Message:     message.Content,
			TurnMemory:  agentcore.ToCoreTurnMemory(message.TurnMemory),
			Interrupted: interrupted,
			Error:       errText,
		},
	}
}

func buildAssistantTurnTranscript(segments []replstream.Segment) []session.TranscriptItem {
	items := make([]session.TranscriptItem, 0, len(segments))

	for _, seg := range segments {
		switch seg.Kind {
		case replstream.SegmentAssistant:
			if seg.Content != "" {
				items = append(items, session.TranscriptItem{
					Kind:    session.TranscriptItemText,
					Content: seg.Content,
				})
			}
		case replstream.SegmentReasoning:
			if seg.Content != "" {
				items = append(items, session.TranscriptItem{
					Kind:    session.TranscriptItemReasoning,
					Content: seg.Content,
				})
			}
		case replstream.SegmentToolStart:
			if seg.ToolCall != nil {
				items = append(items, session.TranscriptItem{
					Kind: session.TranscriptItemToolStart,
					ToolStart: &session.ToolStartPayload{
						Name:  seg.ToolCall.Name,
						Input: cloneInput(seg.ToolCall.Input),
					},
				})
			}
		case replstream.SegmentToolEnd:
			if seg.ToolCall != nil {
				items = append(items, session.TranscriptItem{
					Kind: session.TranscriptItemToolEnd,
					ToolEnd: &session.ToolEndPayload{
						Name:       seg.ToolCall.Name,
						Input:      cloneInput(seg.ToolCall.Input),
						Output:     seg.ToolCall.Output,
						Error:      seg.ToolCall.Error,
						DurationNS: seg.ToolCall.Duration.Nanoseconds(),
					},
				})
			}
		case replstream.SegmentBash:
			duration := int64(0)
			errText := ""
			if seg.ToolCall != nil {
				duration = seg.ToolCall.Duration.Nanoseconds()
				errText = seg.ToolCall.Error
			}
			items = append(items, session.TranscriptItem{
				Kind: session.TranscriptItemBash,
				Bash: &session.BashPayload{
					Command:    seg.Command,
					Summary:    seg.Summary,
					Output:     seg.Output,
					Error:      errText,
					DurationNS: duration,
				},
			})
		case replstream.SegmentDiff:
			if len(seg.DiffLines) > 0 {
				lines := make([]agentcore.EditDiffLine, len(seg.DiffLines))
				copy(lines, seg.DiffLines)
				items = append(items, session.TranscriptItem{
					Kind: session.TranscriptItemDiff,
					Diff: &session.DiffPayload{Lines: agentcore.ToToolDiffLines(lines)},
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

func cloneStreamSegments(segments []replstream.Segment) []replstream.Segment {
	return replstream.CloneSegments(segments)
}

func toolCallFromPayload(payload *session.ToolStartPayload) *agentcore.ToolCall {
	if payload == nil {
		return nil
	}
	return &agentcore.ToolCall{
		Name:  payload.Name,
		Input: cloneInput(payload.Input),
	}
}

func toolCallResultFromPayload(payload *session.ToolEndPayload) *agentcore.ToolCall {
	if payload == nil {
		return nil
	}
	return &agentcore.ToolCall{
		Name:     payload.Name,
		Input:    cloneInput(payload.Input),
		Output:   payload.Output,
		Error:    payload.Error,
		Duration: time.Duration(payload.DurationNS),
	}
}
