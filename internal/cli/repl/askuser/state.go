package askuser

import (
	"github.com/mochow13/keen-code/internal/agentcore"

	"charm.land/bubbles/v2/textinput"
)

type Answer struct {
	Question string
	Answer   string
}

type State struct {
	Requester *Requester
	Request   *Request
	Index     int
	Selected  int
	Answers   []string
	Input     textinput.Model
	Editing   bool
	Resolved  []Answer
	Cancelled bool
	Completed bool
}

func NewState(requester *Requester) State {
	return State{Requester: requester, Input: NewInput()}
}

func (s *State) Begin(request *Request) {
	*s = State{Requester: s.Requester, Request: request, Input: NewInput()}
}

func (s State) Active() bool { return s.Request != nil }

func (s State) Visible() bool { return s.Active() || s.Completed }

func (s State) Clone() *State {
	cloned := s
	cloned.Requester = nil
	cloned.Answers = append([]string(nil), s.Answers...)
	cloned.Resolved = append([]Answer(nil), s.Resolved...)
	return &cloned
}

func (s *State) Clear() {
	requester := s.Requester
	*s = State{Requester: requester}
}

func (s *State) SyncInputFocus() {
	if s.Editing {
		s.Input.Focus()
	} else {
		s.Input.Blur()
	}
}

func (s *State) Move(delta int) {
	if !s.Active() {
		return
	}
	rows := len(s.Request.Questionnaire.Questions[s.Index].Options) + 1
	s.Selected = (s.Selected + delta + rows) % rows
	s.Editing = s.Selected == len(s.Request.Questionnaire.Questions[s.Index].Options)
	s.SyncInputFocus()
}

func (s *State) Answer(value string) bool {
	s.Answers = append(s.Answers, value)
	s.Index++
	s.Selected, s.Editing = 0, false
	s.Input.SetValue("")
	s.Input.Blur()
	return s.Index == len(s.Request.Questionnaire.Questions)
}

func (s *State) Resolve(requester *Requester, cancelled bool) {
	if !s.Active() {
		return
	}
	result := agentcore.AskUserResult{
		Answers:   append([]string(nil), s.Answers...),
		Cancelled: cancelled,
	}
	if requester != nil {
		requester.Respond(s.Request, result)
	}
	s.Resolved = make([]Answer, len(s.Answers))
	for i, answer := range s.Answers {
		s.Resolved[i] = Answer{
			Question: s.Request.Questionnaire.Questions[i].Question,
			Answer:   answer,
		}
	}
	s.Request = nil
	s.Editing = false
	s.Cancelled = cancelled
	s.Completed = true
}

func NewResolvedState(request agentcore.AskUserRequest, result agentcore.AskUserResult) *State {
	state := &State{Completed: true, Cancelled: result.Cancelled}
	for i, answer := range result.Answers {
		if i >= len(request.Questions) {
			break
		}
		state.Resolved = append(state.Resolved, Answer{
			Question: request.Questions[i].Question,
			Answer:   answer,
		})
	}
	return state
}

func (s *State) Card() *Card {
	if s == nil {
		return nil
	}
	cloned := s.Clone()
	if !cloned.Visible() {
		return nil
	}
	return &Card{
		Active: cloned.Active(),
		state:  *cloned,
	}
}
