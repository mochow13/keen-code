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
	ResponseChan chan bool
}

type REPLPermissionRequester struct {
	requestChan  chan *PermissionRequest
	responseChan chan bool
	mu           sync.Mutex
	pending      *PermissionRequest
}

func NewREPLPermissionRequester() *REPLPermissionRequester {
	return &REPLPermissionRequester{
		requestChan:  make(chan *PermissionRequest, 1),
		responseChan: make(chan bool, 1),
	}
}

func (r *REPLPermissionRequester) RequestPermission(ctx context.Context, toolName, path, resolvedPath, operation string) (bool, error) {
	req := &PermissionRequest{
		ToolName:     toolName,
		Path:         path,
		ResolvedPath: resolvedPath,
		Operation:    operation,
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

func (r *REPLPermissionRequester) SendResponse(allowed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
