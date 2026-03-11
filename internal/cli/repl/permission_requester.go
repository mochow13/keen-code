package repl

import (
	"context"
	"fmt"
	"sync/atomic"
)

type PermissionStatus string

const (
	PermissionStatusPending            PermissionStatus = "pending"
	PermissionStatusAllowed            PermissionStatus = "allowed"
	PermissionStatusAllowedSession     PermissionStatus = "allowed_session"
	PermissionStatusDenied             PermissionStatus = "denied"
	PermissionStatusAutoAllowedSession PermissionStatus = "auto_allowed_session"
)

var permissionRequestCounter uint64

type PermissionRequest struct {
	RequestID    string
	ToolName     string
	Path         string
	ResolvedPath string
	Operation    string
	IsDangerous  bool
	Preview      string
	PreviewKind  string
	AutoApproved bool
	Status       PermissionStatus
	ResponseChan chan bool
}

type REPLPermissionRequester struct {
	requestChan         chan *PermissionRequest
	pending             *PermissionRequest
	sessionAllowedTools map[string]bool
}

func NewREPLPermissionRequester() *REPLPermissionRequester {
	return &REPLPermissionRequester{
		requestChan:         make(chan *PermissionRequest, 1),
		sessionAllowedTools: make(map[string]bool),
	}
}

func (r *REPLPermissionRequester) RequestPermission(ctx context.Context, toolName, path, resolvedPath, operation string, isDangerous bool) (bool, error) {
	if !isDangerous && r.sessionAllowedTools[toolName] {
		return true, nil
	}

	id := atomic.AddUint64(&permissionRequestCounter, 1)
	req := &PermissionRequest{
		RequestID:    fmt.Sprintf("%d", id),
		ToolName:     toolName,
		Path:         path,
		ResolvedPath: resolvedPath,
		Operation:    operation,
		IsDangerous:  isDangerous,
		Status:       PermissionStatusPending,
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

func (r *REPLPermissionRequester) GetRequestChan() <-chan *PermissionRequest {
	return r.requestChan
}

func (r *REPLPermissionRequester) SendResponse(choice PermissionChoice, toolName string) {
	isDangerous := r.pending != nil && r.pending.IsDangerous
	allowed := choice == PermissionChoiceAllow || choice == PermissionChoiceAllowSession

	if choice == PermissionChoiceAllowSession && !isDangerous {
		r.sessionAllowedTools[toolName] = true
	}

	if r.pending != nil {
		select {
		case r.pending.ResponseChan <- allowed:
		default:
		}
	}
}

func (r *REPLPermissionRequester) HasPendingRequest() bool {
	return r.pending != nil
}
