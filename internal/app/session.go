package app

import (
	"context"
	"errors"
	"sync"

	"github.com/mochow13/keen-code/internal/agentcore"
	"github.com/mochow13/keen-code/internal/session"
)

var ErrTurnActive = errors.New("a turn is already active")

type Agent interface {
	FormatUserMessage(content string) string
	Submit(ctx context.Context, sessionID string, input string) (<-chan agentcore.StreamEvent, error)
	ReplaceMessages(messages []agentcore.Message)
}

type Session struct {
	agent   Agent
	store   *session.Store
	current *session.Session
	active  *Turn
}

// Turn is one prompt submission. Close releases the session's active-turn slot
// and cancels the turn; it is safe to call more than once.
type Turn struct {
	Events <-chan agentcore.StreamEvent

	session *Session
	cancel  context.CancelFunc
	close   sync.Once
	memory  turnMemory
}

func NewSession(agent Agent, workingDir string) (*Session, error) {
	store, err := session.NewStore(workingDir)
	if err != nil {
		return nil, err
	}
	return &Session{agent: agent, store: store}, nil
}

func NewSessionWithoutStore(agent Agent) *Session {
	return &Session{agent: agent}
}

func (s *Session) SetAgent(agent Agent) {
	if s == nil {
		return
	}
	s.agent = agent
}

func (s *Session) StartTurn(ctx context.Context, input string) (*Turn, error) {
	if s == nil {
		return nil, errors.New("session is unavailable")
	}
	if s.active != nil {
		return nil, ErrTurnActive
	}
	if s.agent == nil {
		return nil, errors.New("agent is unavailable")
	}
	if err := s.appendUserMessage(s.agent.FormatUserMessage(input)); err != nil {
		return nil, err
	}

	turnCtx, cancel := context.WithCancel(ctx)
	events, err := s.agent.Submit(turnCtx, s.currentID(), input)
	if err != nil {
		cancel()
		return nil, err
	}

	turn := &Turn{Events: events, session: s, cancel: cancel}
	s.active = turn
	return turn, nil
}

func (t *Turn) Close() {
	if t == nil {
		return
	}
	t.close.Do(func() {
		t.cancel()
		if t.session != nil && t.session.active == t {
			t.session.active = nil
		}
	})
}

func (t *Turn) ConfigureMemory(workingDir string, retainOutput bool) {
	if t != nil {
		t.memory.workingDir = workingDir
		t.memory.retainOutput = retainOutput
	}
}

func (t *Turn) RecordToolCall(call *agentcore.ToolCall, response string) {
	if t == nil || call == nil {
		return
	}
	t.memory.record(call, len(response))
}

func (t *Turn) Memory() *agentcore.TurnMemory {
	if t == nil {
		return nil
	}
	return t.memory.build()
}

func (t *Turn) ResetMemory() {
	if t != nil {
		t.memory.activity = nil
	}
}

func (s *Session) AppendUserMessage(content string) error {
	if s == nil {
		return nil
	}
	return s.appendUserMessage(content)
}

func (s *Session) appendUserMessage(content string) error {
	if s.store == nil {
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

func (s *Session) AppendAssistantTurn(transcript []session.TranscriptItem, message agentcore.Message, interrupted bool, errText string) error {
	if s == nil || s.store == nil || s.current == nil {
		return nil
	}
	return s.store.Append(s.current, buildAssistantTurnEvent(transcript, message, interrupted, errText))
}

func (s *Session) AppendCompaction(transcript []session.TranscriptItem, messages []agentcore.Message, status string) error {
	if s == nil || s.store == nil || s.current == nil {
		return nil
	}
	return s.store.Append(s.current, buildCompactionEvent(transcript, messages, status))
}

func (s *Session) AppendAutoCompaction(checkpointTranscript []session.TranscriptItem, checkpoint agentcore.Message, messages []agentcore.Message) error {
	if s == nil || s.store == nil || s.current == nil {
		return nil
	}
	return s.store.AppendBatch(s.current, []session.Event{
		buildAssistantTurnEvent(checkpointTranscript, checkpoint, false, ""),
		buildCompactionEvent(nil, messages, ""),
	})
}

func buildAssistantTurnEvent(transcript []session.TranscriptItem, message agentcore.Message, interrupted bool, errText string) session.Event {
	return session.Event{
		Kind: session.KindAssistantTurn,
		AssistantTurn: &session.AssistantTurnPayload{
			Transcript:  transcript,
			Message:     message.Content,
			TurnMemory:  agentcore.ToCoreTurnMemory(message.TurnMemory),
			Interrupted: interrupted,
			Error:       errText,
		},
	}
}

func buildCompactionEvent(transcript []session.TranscriptItem, messages []agentcore.Message, status string) session.Event {
	return session.Event{
		Kind: session.KindCompactionApplied,
		CompactionApplied: &session.CompactionAppliedPayload{
			Status:     status,
			Transcript: transcript,
			Messages:   agentcore.ToCoreMessages(messages),
		},
	}
}

func (s *Session) ensureCurrent() error {
	if s.store == nil || s.current != nil {
		return nil
	}
	current, err := s.store.Create()
	if err != nil {
		return err
	}
	s.current = current
	return nil
}

func (s *Session) CurrentID() string {
	if s == nil {
		return ""
	}
	return s.currentID()
}

func (s *Session) currentID() string {
	if s.current == nil {
		return ""
	}
	return s.current.ID
}

func (s *Session) List() ([]session.Summary, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	return s.store.List()
}

func (s *Session) Load(summary session.Summary) (*session.LoadedSession, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	loaded, err := s.store.Load(summary)
	if err != nil {
		return nil, err
	}
	s.Resume(loaded)
	return loaded, nil
}

func (s *Session) Resume(loaded *session.LoadedSession) {
	if s == nil || loaded == nil {
		return
	}
	s.current = loaded.Session
	if s.agent != nil {
		s.agent.ReplaceMessages(agentcore.FromCoreMessages(session.BuildConversation(loaded.Events)))
	}
}

func (s *Session) Reset() {
	if s == nil {
		return
	}
	s.current = nil
}
