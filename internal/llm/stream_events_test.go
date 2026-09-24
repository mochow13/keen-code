package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/tools"
)

func TestSendStreamEventDelivery(t *testing.T) {
	events := make(chan core.StreamEvent, 1)
	want := core.StreamEvent{Type: core.StreamEventTypeChunk, Content: "hello"}
	if !sendStreamEvent(context.Background(), events, want) {
		t.Fatal("event not sent")
	}
	if got := <-events; got.Type != want.Type || got.Content != want.Content {
		t.Fatalf("unexpected event: %#v", got)
	}
}

func TestSendStreamEventCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan core.StreamEvent)
	done := make(chan bool, 1)
	go func() {
		done <- sendStreamEvent(ctx, events, core.StreamEvent{Type: core.StreamEventTypeDone})
	}()
	select {
	case <-done:
		t.Fatal("send returned without a consumer or cancellation")
	case <-time.After(10 * time.Millisecond):
	}
	cancel()
	select {
	case sent := <-done:
		if sent {
			t.Fatal("event sent without a consumer")
		}
	case <-time.After(time.Second):
		t.Fatal("send blocked after cancellation")
	}
}

func TestSendStreamEventAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	events := make(chan core.StreamEvent, 1)
	if sendStreamEvent(ctx, events, core.StreamEvent{}) || len(events) != 0 {
		t.Fatal("cancelled stream should not enqueue events")
	}
}

func TestTerminalEventsDoNotBlockAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	events := make(chan core.StreamEvent)
	for name, emit := range map[string]func(){
		"anthropic": func() { (&AnthropicClient{}).emitTerminalEvent(ctx, events, nil, 0, nil, ctx.Err()) },
		"bedrock":   func() { (&BedrockClient{}).emitTerminalEvent(ctx, events, nil, 0, nil, ctx.Err()) },
		"genkit":    func() { (&GenkitClient{}).emitTerminalEvent(ctx, events, nil, 0, nil, ctx.Err()) },
		"openai":    func() { (&OpenAICompatibleClient{}).emitTerminalEvent(ctx, events, nil, 0, nil, ctx.Err()) },
		"codex":     func() { (&OpenAICodexClient{}).emitTerminalEvent(ctx, events, nil, 0, nil, ctx.Err()) },
		"responses": func() { (&OpenAIResponsesClient{}).emitTerminalEvent(ctx, events, nil, 0, nil, ctx.Err()) },
	} {
		t.Run(name, func(t *testing.T) {
			done := make(chan struct{})
			go func() { emit(); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("terminal event blocked after cancellation")
			}
		})
	}
}

func TestCancelledToolStartDoesNotExecute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tool := &validatingExecutionTool{}
	registry := tools.NewRegistry()
	if err := registry.Register(tool); err != nil {
		t.Fatal(err)
	}
	result := executeTool(ctx, registry, tool.Name(), map[string]any{"value": "ok"}, make(chan core.StreamEvent))
	if !errors.Is(result.Err, context.Canceled) || tool.executed {
		t.Fatalf("cancelled tool executed=%v, err=%v", tool.executed, result.Err)
	}
}
