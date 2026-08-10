package tooling

import (
	"testing"

	"github.com/mochow13/keen-code/internal/tools"
)

func TestDiffEmitterRoundTrip(t *testing.T) {
	emitter := NewDiffEmitter()
	lines := []tools.EditDiffLine{{Kind: tools.DiffLineAdded, NewLineNum: 1, Content: "added"}}
	finished := make(chan struct{})
	go func() {
		emitter.EmitDiff(lines)
		close(finished)
	}()

	request := <-emitter.GetDiffChan()
	if len(request.Lines) != 1 || request.Lines[0].Content != "added" {
		t.Fatalf("unexpected diff request %#v", request)
	}
	close(request.Done)
	<-finished
}
