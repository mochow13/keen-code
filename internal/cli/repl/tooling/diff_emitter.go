package tooling

import "github.com/mochow13/keen-code/internal/agentcore"

type DiffRequest struct {
	Lines []agentcore.EditDiffLine
	Done  chan struct{}
}

type DiffEmitter struct {
	diffChan chan DiffRequest
}

func NewDiffEmitter() *DiffEmitter {
	return &DiffEmitter{
		diffChan: make(chan DiffRequest, 1),
	}
}

func (e *DiffEmitter) EmitDiff(lines []agentcore.EditDiffLine) {
	done := make(chan struct{})
	e.diffChan <- DiffRequest{Lines: lines, Done: done}
	<-done
}

func (e *DiffEmitter) GetDiffChan() <-chan DiffRequest {
	return e.diffChan
}
