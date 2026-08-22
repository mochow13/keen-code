package askuser

import (
	"context"
	"fmt"
	"sync"

	"github.com/mochow13/keen-code/internal/tools"
)

type Request struct {
	Context       context.Context
	Questionnaire tools.AskUserRequest
	ResponseChan  chan tools.AskUserResult
}

type Requester struct {
	requestChan chan *Request
	mu          sync.Mutex
	pending     *Request
}

func NewRequester() *Requester {
	return &Requester{requestChan: make(chan *Request, 1)}
}

func (r *Requester) RequestUser(ctx context.Context, questionnaire tools.AskUserRequest) (tools.AskUserResult, error) {
	req := &Request{Context: ctx, Questionnaire: questionnaire, ResponseChan: make(chan tools.AskUserResult, 1)}
	r.mu.Lock()
	if r.pending != nil {
		r.mu.Unlock()
		return tools.AskUserResult{}, fmt.Errorf("ask_user already has a pending questionnaire")
	}
	r.pending = req
	r.mu.Unlock()
	defer r.clear(req)

	select {
	case r.requestChan <- req:
	case <-ctx.Done():
		return tools.AskUserResult{}, ctx.Err()
	}
	select {
	case result := <-req.ResponseChan:
		return result, nil
	case <-ctx.Done():
		return tools.AskUserResult{}, ctx.Err()
	}
}

func (r *Requester) GetRequestChan() <-chan *Request { return r.requestChan }

func (r *Requester) IsPending(req *Request) bool {
	if req == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pending == req && req.Context.Err() == nil
}

func (r *Requester) Respond(req *Request, result tools.AskUserResult) {
	if req == nil {
		return
	}
	select {
	case req.ResponseChan <- result:
	default:
	}
}

func (r *Requester) clear(req *Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending == req {
		r.pending = nil
	}
}
