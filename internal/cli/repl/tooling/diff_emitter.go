package tooling

import "github.com/mochow13/keen-code/internal/tools"

type DiffRequest struct {
	Lines []tools.EditDiffLine
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

func (e *DiffEmitter) EmitDiff(lines []tools.EditDiffLine) {
	done := make(chan struct{})
	e.diffChan <- DiffRequest{Lines: lines, Done: done}
	<-done
}

func (e *DiffEmitter) GetDiffChan() <-chan DiffRequest {
	return e.diffChan
}
