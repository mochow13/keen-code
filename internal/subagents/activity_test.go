package subagents

import (
	"context"
	"testing"
	"time"

	"github.com/mochow13/keen-code/internal/llm/core"
)

func TestCollectResultClosedChannelNormalEOF(t *testing.T) {
	events := make(chan core.StreamEvent)
	close(events)

	text, err := collectResult(context.Background(), events, "agent", "run", nil)
	if err != nil {
		t.Fatalf("expected nil error for normal EOF, got %v", err)
	}
	if text != "" {
		t.Fatalf("expected empty text, got %q", text)
	}
}

func TestCollectResultClosedChannelPreservesPartialOutputOnEOF(t *testing.T) {
	events := make(chan core.StreamEvent, 1)
	events <- core.StreamEvent{Type: core.StreamEventTypeChunk, Content: "partial output "}
	close(events)

	text, err := collectResult(context.Background(), events, "agent", "run", nil)
	if err != nil {
		t.Fatalf("expected nil error for normal EOF, got %v", err)
	}
	if text != "partial output" {
		t.Fatalf("expected trimmed partial text, got %q", text)
	}
}

func TestCollectResultClosedChannelCancelledContext(t *testing.T) {
	// Both the event channel (closed) and ctx.Done() are ready, so select
	// would pick randomly before the fix. Loop to cover the race: every
	// iteration must report cancellation.
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		events := make(chan core.StreamEvent)
		close(events)

		_, err := collectResult(ctx, events, "agent", "run", nil)
		if err != context.Canceled {
			t.Fatalf("iteration %d: expected context.Canceled, got %v", i, err)
		}
	}
}

func TestCollectResultClosedChannelCancelledContextPreservesPartialOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan core.StreamEvent)
	type outcome struct {
		text string
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		text, err := collectResult(ctx, events, "agent", "run", nil)
		done <- outcome{text: text, err: err}
	}()
	// Ensure the chunk is consumed before cancellation closes the stream.
	events <- core.StreamEvent{Type: core.StreamEventTypeChunk, Content: "partial "}
	cancel()
	close(events)

	select {
	case res := <-done:
		if res.err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", res.err)
		}
		if res.text != "partial" {
			t.Fatalf("expected partial text, got %q", res.text)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for collectResult")
	}
}

func TestCollectResultClosedChannelDeadlineExceeded(t *testing.T) {
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		events := make(chan core.StreamEvent)
		close(events)

		_, err := collectResult(ctx, events, "agent", "run", nil)
		if err != context.DeadlineExceeded {
			t.Fatalf("iteration %d: expected context.DeadlineExceeded, got %v", i, err)
		}
	}
}
