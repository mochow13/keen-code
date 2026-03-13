package repl

import "github.com/user/keen-code/internal/tools"

type diffEmitRequest struct {
	lines []tools.EditDiffLine
	done  chan struct{}
}

type REPLDiffEmitter struct {
	diffChan chan diffEmitRequest
}

func NewREPLDiffEmitter() *REPLDiffEmitter {
	return &REPLDiffEmitter{
		diffChan: make(chan diffEmitRequest, 1),
	}
}

func (e *REPLDiffEmitter) EmitDiff(lines []tools.EditDiffLine) {
	done := make(chan struct{})
	e.diffChan <- diffEmitRequest{lines: lines, done: done}
	<-done
}

func (e *REPLDiffEmitter) GetDiffChan() <-chan diffEmitRequest {
	return e.diffChan
}
