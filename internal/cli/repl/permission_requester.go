package repl

import (
	"context"
	"sync"
)

type PermissionRequest struct {
	ToolName     string
	Path         string
	ResolvedPath string
	Operation    string
	IsDangerous  bool
	ResponseChan chan bool
}

type REPLPermissionRequester struct {
	requestChan         chan *PermissionRequest
	responseChan        chan bool
	mu                  sync.Mutex
	pending             *PermissionRequest
	sessionAllowedTools map[string]bool
}

func NewREPLPermissionRequester() *REPLPermissionRequester {
	return &REPLPermissionRequester{
		requestChan:         make(chan *PermissionRequest, 1),
		responseChan:        make(chan bool, 1),
		sessionAllowedTools: make(map[string]bool),
	}
}

func (r *REPLPermissionRequester) RequestPermission(ctx context.Context, toolName, path, resolvedPath, operation string, isDangerous bool) (bool, error) {
	r.mu.Lock()
	if !isDangerous && r.sessionAllowedTools[toolName] {
		r.mu.Unlock()
		return true, nil
	}
	r.mu.Unlock()

	req := &PermissionRequest{
		ToolName:     toolName,
		Path:         path,
		ResolvedPath: resolvedPath,
		Operation:    operation,
		IsDangerous:  isDangerous,
		ResponseChan: make(chan bool, 1),
	}

	r.mu.Lock()
	r.pending = req
	r.mu.Unlock()

	select {
	case r.requestChan <- req:
		select {
		case response := <-req.ResponseChan:
			r.mu.Lock()
			r.pending = nil
			r.mu.Unlock()
			return response, nil
		case <-ctx.Done():
			r.mu.Lock()
			r.pending = nil
			r.mu.Unlock()
			return false, ctx.Err()
		}
	case <-ctx.Done():
		r.mu.Lock()
		r.pending = nil
		r.mu.Unlock()
		return false, ctx.Err()
	}
}

func (r *REPLPermissionRequester) GetRequestChan() <-chan *PermissionRequest {
	return r.requestChan
}

func (r *REPLPermissionRequester) SendResponse(choice PermissionChoice, toolName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

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
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pending != nil
}
