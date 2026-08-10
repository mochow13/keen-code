package permissions

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/mochow13/keen-code/internal/config"
)

type Status string

const (
	StatusPending            Status = "pending"
	StatusAllowed            Status = "allowed"
	StatusAllowedSession     Status = "allowed_session"
	StatusDenied             Status = "denied"
	StatusAutoAllowedSession Status = "auto_allowed_session"
	StatusRedirected         Status = "redirected"
)

var requestCounter uint64

type Request struct {
	RequestID    string
	ToolName     string
	Path         string
	ResolvedPath string
	IsDangerous  bool
	Preview      string
	PreviewKind  string
	AutoApproved bool
	Status       Status
	ResponseChan chan bool
}

type Requester struct {
	requestChan         chan *Request
	pending             *Request
	sessionAllowedTools map[string]bool
	autoApprove         bool
	projectPerms        *config.ProjectPermissions
}

func NewRequester(projectPerms *config.ProjectPermissions) *Requester {
	return &Requester{
		requestChan:         make(chan *Request, 1),
		sessionAllowedTools: make(map[string]bool),
		projectPerms:        projectPerms,
	}
}

func NewAutoApproveRequester() *Requester {
	r := NewRequester(nil)
	r.autoApprove = true
	return r
}

func (r *Requester) RequestPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous bool) (bool, error) {
	if r.autoApprove {
		return true, nil
	}

	if r.projectPerms != nil && r.projectPerms.Allow.Contains(toolName) {
		return true, nil
	}

	if !isDangerous && r.sessionAllowedTools[toolName] {
		return true, nil
	}

	id := atomic.AddUint64(&requestCounter, 1)
	req := &Request{
		RequestID:    fmt.Sprintf("%d", id),
		ToolName:     toolName,
		Path:         path,
		ResolvedPath: resolvedPath,
		IsDangerous:  isDangerous,
		Status:       StatusPending,
		ResponseChan: make(chan bool, 1),
	}

	r.pending = req

	select {
	case r.requestChan <- req:
		select {
		case response := <-req.ResponseChan:
			r.pending = nil
			return response, nil
		case <-ctx.Done():
			r.pending = nil
			return false, ctx.Err()
		}
	case <-ctx.Done():
		r.pending = nil
		return false, ctx.Err()
	}
}

func (r *Requester) GetRequestChan() <-chan *Request {
	return r.requestChan
}

func (r *Requester) SendResponse(choice Choice, toolName string) {
	isDangerous := r.pending != nil && r.pending.IsDangerous
	allowed := choice == ChoiceAllow || choice == ChoiceAllowSession

	if choice == ChoiceAllowSession && !isDangerous {
		r.sessionAllowedTools[toolName] = true
	}

	if r.pending != nil {
		select {
		case r.pending.ResponseChan <- allowed:
		default:
		}
	}
}

func (r *Requester) HasPendingRequest() bool {
	return r.pending != nil
}

func (r *Requester) IsSessionAllowed(toolName string) bool {
	return r.sessionAllowedTools[toolName]
}

func (r *Requester) ResetSessionPermissions() {
	r.sessionAllowedTools = make(map[string]bool)
}
