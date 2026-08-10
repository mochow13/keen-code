package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/tools"
)

type compactionTestClient struct {
	request []Message
	opts    StreamOptions
	events  []StreamEvent
}

func (c *compactionTestClient) StreamChat(_ context.Context, messages []Message, _ *tools.Registry, opts ...StreamOptions) (<-chan StreamEvent, error) {
	c.request = CloneMessages(messages)
	c.opts = streamOptions(opts)
	ch := make(chan StreamEvent, len(c.events))
	for _, event := range c.events {
		ch <- event
	}
	close(ch)
	return ch, nil
}
func (*compactionTestClient) Reset() {}

func TestAutoCompactBuildsPrivateReplacement(t *testing.T) {
	client := &compactionTestClient{events: []StreamEvent{{Type: StreamEventTypeChunk, Content: "## Goal\nContinue work"}}}
	history := []Message{{Role: RoleSystem, Content: "agent prompt"}, {Role: RoleUser, Content: "implement this exactly"}}
	replacement, _, err := AutoCompact(context.Background(), client, history, "session")
	if err != nil {
		t.Fatal(err)
	}
	if !client.opts.OneShot || !client.opts.DisableAutoCompaction || client.opts.SessionID != "session" {
		t.Fatalf("unexpected nested options: %#v", client.opts)
	}
	if len(client.request) < 2 || client.request[0].Role != RoleSystem || strings.Contains(client.request[1].Content, "agent prompt") {
		t.Fatalf("system history was not excluded: %#v", client.request)
	}
	if len(replacement) != 2 || replacement[0].Role != RoleSystem || replacement[0].Content != "agent prompt" || replacement[1].Role != RoleUser || !strings.Contains(replacement[1].Content, "implement this exactly") {
		t.Fatalf("invalid replacement: %#v", replacement)
	}
	replacement[0].Content = "changed"
	if history[0].Content != "agent prompt" {
		t.Fatal("replacement system message aliases original history")
	}
}

func TestAutoCompactIsTransactional(t *testing.T) {
	history := []Message{{Role: RoleUser, Content: "task"}}
	client := &compactionTestClient{}
	_, _, err := AutoCompact(context.Background(), client, history, "")
	if err == nil || !strings.Contains(err.Error(), "empty summary") {
		t.Fatalf("expected empty summary error, got %v", err)
	}
	if history[0].Content != "task" {
		t.Fatal("history mutated")
	}
}

func TestContextBudgetAndAutoCompactionThreshold(t *testing.T) {
	if got := contextInputBudget(100000); got != 95000 {
		t.Fatalf("budget = %d, want 95000", got)
	}
	if !shouldAutoCompact(85500, 95000) || shouldAutoCompact(85499, 95000) {
		t.Fatal("unexpected 90% trigger boundary")
	}
}

func TestAutoCompactionCancellationClassification(t *testing.T) {
	if !isAutoCompactionCancellation(context.Canceled) {
		t.Fatal("context cancellation was not classified as auto-compaction cancellation")
	}
	if !isAutoCompactionCancellation(fmt.Errorf("wrapped: %w", context.Canceled)) {
		t.Fatal("wrapped context cancellation was not classified")
	}
	if isAutoCompactionCancellation(errors.New("other")) || isAutoCompactionCancellation(nil) {
		t.Fatal("unrelated error was classified as cancellation")
	}
}
