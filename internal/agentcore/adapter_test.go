package agentcore

import (
	"context"
	"testing"
	"time"

	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/subagents"
)

func TestAdaptStreamCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan core.StreamEvent)
	out := (&adapter{}).adaptStream(ctx, in)

	in <- core.StreamEvent{Type: core.StreamEventTypeChunk, Content: "a"}
	got := <-out
	if got.Content != "a" {
		t.Fatalf("got content %q, want a", got.Content)
	}

	in <- core.StreamEvent{Type: core.StreamEventTypeChunk, Content: "b"}
	cancel()

	done := make(chan struct{})
	go func() {
		select {
		case in <- core.StreamEvent{Type: core.StreamEventTypeChunk, Content: "c"}:
		case <-ctx.Done():
		}
		close(in)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("producer blocked after stream cancel")
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-out:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("adapted channel did not close after cancel")
		}
	}
}

func TestAdaptStreamNilChannel(t *testing.T) {
	if got := (&adapter{}).adaptStream(context.Background(), nil); got != nil {
		t.Fatalf("expected nil adapted channel, got %#v", got)
	}
}

func TestAdaptStreamPassesThroughUnknownEventTypes(t *testing.T) {
	in := make(chan core.StreamEvent, 1)
	out := (&adapter{}).adaptStream(context.Background(), in)

	in <- core.StreamEvent{Type: core.StreamEventType("not_a_real_event"), Content: "keep"}
	close(in)

	got := <-out
	if got.Type != "not_a_real_event" || got.Content != "keep" {
		t.Fatalf("got %#v", got)
	}
	if _, ok := <-out; ok {
		t.Fatal("expected adapted channel to close")
	}
}

func TestSubagentActivityChannelsCloseOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, out := subagentActivityChannels(ctx, true)
	cancel()

	select {
	case _, ok := <-out:
		if ok {
			t.Fatal("expected activity channel to close after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("activity channel did not close after cancellation")
	}
}

func TestSubagentActivityChannelsDrainsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	in, out := subagentActivityChannels(ctx, true)

	// Fill both buffers so the pump blocks on out and the next send blocks on in.
	for i := 0; i < cap(in)+cap(out); i++ {
		select {
		case in <- subagents.ToolActivity{RunID: "fill"}:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out filling activity buffers")
		}
	}

	sent := make(chan struct{})
	go func() {
		in <- subagents.ToolActivity{RunID: "blocked"}
		close(sent)
	}()

	cancel()

	select {
	case <-sent:
	case <-time.After(2 * time.Second):
		t.Fatal("producer blocked after activity cancel")
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-out:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("activity channel did not close after cancel")
		}
	}
}

func TestAdaptStreamCancelWithoutUpstreamClose(t *testing.T) {
	for _, blockedOutput := range []bool{false, true} {
		name := "idle input"
		if blockedOutput {
			name = "blocked output"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			in := make(chan core.StreamEvent)
			defer close(in)
			out := (&adapter{}).adaptStream(ctx, in)
			if blockedOutput {
				select {
				case in <- core.StreamEvent{Type: core.StreamEventTypeChunk}:
				case <-time.After(time.Second):
					t.Fatal("adapter did not receive event")
				}
			}
			cancel()
			deadline := time.After(time.Second)
			for {
				select {
				case _, ok := <-out:
					if !ok {
						return
					}
				case <-deadline:
					t.Fatal("output waits for upstream closure after cancellation")
				}
			}
		})
	}
}

type testPermissionRequester struct {
	called bool
}

func (p *testPermissionRequester) RequestPermission(context.Context, string, string, string, bool) (bool, error) {
	p.called = true
	return true, nil
}

func TestPermissionRequesterAdapterFailsClosed(t *testing.T) {
	var typedNil *testPermissionRequester
	for name, p := range map[string]*permissionRequesterAdapter{
		"nil adapter":         nil,
		"nil requester":       {},
		"typed nil requester": {inner: typedNil},
	} {
		t.Run(name, func(t *testing.T) {
			allowed, err := p.RequestPermission(context.Background(), "bash", "command", "", true)
			if allowed || err == nil {
				t.Fatalf("missing requester must fail closed: allowed=%v, err=%v", allowed, err)
			}
		})
	}
}

func TestPermissionRequesterAdapterDelegates(t *testing.T) {
	requester := &testPermissionRequester{}
	p := &permissionRequesterAdapter{inner: requester}
	allowed, err := p.RequestPermission(context.Background(), "bash", "command", "", true)
	if !allowed || err != nil || !requester.called {
		t.Fatalf("request was not delegated: allowed=%v, err=%v, called=%v", allowed, err, requester.called)
	}
}
