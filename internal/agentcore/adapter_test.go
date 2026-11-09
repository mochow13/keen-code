package agentcore

import (
	"context"
	"testing"
	"time"

	"github.com/mochow13/keen-code/internal/appstate"
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

func messageTextFixture() []Message {
	exitCode := 0
	return []Message{
		{Role: RoleSystem, Content: "system prompt"},
		{Role: RoleUser, Content: "hello"},
		{Role: RoleAssistant, Content: "working", TurnMemory: &TurnMemory{ToolActivity: []HistoricalToolActivity{{
			TextOffset: 3, Tool: "read_file",
			Input:  map[string]any{"path": "a.go", "opts": map[string]any{"nested": []any{"x", map[string]any{"y": 1}}}},
			Status: "success", ExitCode: &exitCode, HasRawOutput: true,
			RawOutput: map[string]any{"rows": []any{map[string]any{"a": 1}}}, RetainedOutput: "kept",
		}}}},
		{Role: RoleAssistant, Content: ""},
		{Role: RoleUser, Content: "follow-up"},
	}
}

func newMessageTextAdapter(msgs []Message) *adapter {
	a := &adapter{appState: &appstate.AppState{}}
	a.ReplaceMessages(msgs)
	return a
}

func TestAdapterGetMessageTextPreservesOrderAndStripsMemory(t *testing.T) {
	a := newMessageTextAdapter(messageTextFixture())
	want := messageTextFixture()
	got := a.GetMessageText()
	if len(got) != len(want) {
		t.Fatalf("got %d messages, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Role != want[i].Role || got[i].Content != want[i].Content {
			t.Fatalf("index %d: got %+v, want role/content %+v", i, got[i], want[i])
		}
		if got[i].TurnMemory != nil {
			t.Fatalf("index %d: TurnMemory = %+v, want nil", i, got[i].TurnMemory)
		}
	}
}

func TestAdapterGetMessageTextSnapshotUnaffectedByStateChanges(t *testing.T) {
	a := newMessageTextAdapter(messageTextFixture())
	snap := a.GetMessageText()
	if len(snap) == 0 {
		t.Fatal("expected non-empty snapshot")
	}
	first := snap[0]
	a.AppendMessage(Message{Role: RoleUser, Content: "new"})
	a.ReplaceMessages([]Message{{Role: RoleUser, Content: "replaced"}})
	if snap[0] != first {
		t.Fatalf("snapshot mutated by state change: got %+v, want %+v", snap[0], first)
	}
	if len(snap) != len(messageTextFixture()) {
		t.Fatalf("snapshot length changed: got %d", len(snap))
	}
}

func TestAdapterGetMessageTextMutationDoesNotAffectState(t *testing.T) {
	a := newMessageTextAdapter(messageTextFixture())
	snap := a.GetMessageText()
	snap[0].Role = RoleUser
	snap[0].Content = "mutated"
	snap[1].Content = "mutated"
	fresh := a.GetMessageText()
	want := messageTextFixture()
	for i := range want {
		if fresh[i].Role != want[i].Role || fresh[i].Content != want[i].Content {
			t.Fatalf("index %d: state affected by snapshot mutation: got %+v", i, fresh[i])
		}
	}
	withMemory := a.GetMessages()
	if len(withMemory) != len(want) {
		t.Fatalf("got %d messages, want %d", len(withMemory), len(want))
	}
	if withMemory[2].TurnMemory == nil || len(withMemory[2].TurnMemory.ToolActivity) != 1 {
		t.Fatalf("GetMessages lost TurnMemory: %+v", withMemory[2])
	}
}

var messageTextBenchSink any

func messageTextBenchmarkMessages(n int) []Message {
	msgs := make([]Message, 0, n)
	for i := range n {
		m := Message{Role: RoleUser, Content: "message"}
		switch i % 4 {
		case 0:
			m.Role = RoleSystem
		case 2:
			m.Role = RoleAssistant
			m.Content = "assistant output"
		case 3:
			m.Role = RoleAssistant
			exitCode := i
			m.TurnMemory = &TurnMemory{ToolActivity: []HistoricalToolActivity{
				{TextOffset: i, Tool: "read_file",
					Input: map[string]any{"path": "file.go", "offset": i, "opts": map[string]any{
						"tags":   []any{"a", i, map[string]any{"k": "v"}},
						"nested": map[string]any{"depth": []any{map[string]any{"n": i}}},
					}},
					Status: "success", ExitCode: &exitCode, HasRawOutput: true,
					RawOutput:      map[string]any{"rows": []any{map[string]any{"line": i, "text": "content"}}},
					RetainedOutput: map[string]any{"summary": "kept", "index": i},
				},
				{TextOffset: i, Tool: "bash", Input: map[string]any{"cmd": "echo hi", "args": []any{"a", "b"}},
					Status: "success", RawOutput: "raw output", RetainedOutput: "retained"},
			}}
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func BenchmarkAdapterGetMessages(b *testing.B) {
	a := newMessageTextAdapter(messageTextBenchmarkMessages(1000))
	b.ReportAllocs()
	for b.Loop() {
		messageTextBenchSink = a.GetMessages()
	}
}

func BenchmarkAdapterGetMessageText(b *testing.B) {
	a := newMessageTextAdapter(messageTextBenchmarkMessages(1000))
	b.ReportAllocs()
	for b.Loop() {
		messageTextBenchSink = a.GetMessageText()
	}
}
