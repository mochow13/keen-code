package tooling

import (
	"testing"

	"github.com/mochow13/keen-code/internal/cli/repl/agentcore"
)

func TestDiffEmitterRoundTrip(t *testing.T) {
	emitter := NewDiffEmitter()
	lines := []agentcore.EditDiffLine{{Kind: agentcore.EditDiffLineAdded, NewLineNum: 1, Content: "added"}}
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
