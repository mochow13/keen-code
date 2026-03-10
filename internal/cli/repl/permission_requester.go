package repl

import (
	"context"
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
	if !isDangerous && r.sessionAllowedTools[toolName] {
		return true, nil
	}

	req := &PermissionRequest{
		ToolName:     toolName,
		Path:         path,
		ResolvedPath: resolvedPath,
		Operation:    operation,
		IsDangerous:  isDangerous,
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
