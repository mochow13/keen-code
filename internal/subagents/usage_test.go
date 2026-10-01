package subagents

import (
	"context"
	"testing"
	"time"

	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/tools"
	"github.com/mochow13/keen-code/internal/usage"
)

func TestUsageRecordConvertsTokenCounts(t *testing.T) {
	before := time.Now().UTC()
	record, ok := usageRecord("anthropic", "child", &core.TokenUsage{
		InputTokens:      100,
		OutputTokens:     20,
		CacheReadTokens:  60,
		CacheWriteTokens: 10,
		ReasoningTokens:  5,
	})
	if !ok {
		t.Fatal("expected usage record for non-nil token usage")
	}
	if record.Provider != "anthropic" || record.Model != "child" {
		t.Fatalf("unexpected provider/model: %+v", record)
	}
	if record.Input != 100 || record.Output != 20 || record.CacheRead != 60 || record.CacheWrite != 10 || record.Reasoning != 5 {
		t.Fatalf("unexpected token counts: %+v", record)
	}
	if record.TS.IsZero() || record.TS.Location() != time.UTC || record.TS.Before(before) {
		t.Fatalf("expected UTC timestamp at or after test start, got %v", record.TS)
	}
	if record.Rollup || record.Date != "" {
		t.Fatalf("expected raw record, got %+v", record)
	}
}

func TestUsageRecordNilTokenUsage(t *testing.T) {
	if _, ok := usageRecord("anthropic", "child", nil); ok {
		t.Fatal("expected no record for nil token usage")
	}
}

func TestCollectResultEmitsOneRecordPerUsageEvent(t *testing.T) {
	events := make(chan core.StreamEvent, 5)
	events <- core.StreamEvent{Type: core.StreamEventTypeChunk, Content: "hello "}
	events <- core.StreamEvent{Type: core.StreamEventTypeUsage, Usage: &core.TokenUsage{InputTokens: 10, OutputTokens: 2}}
	events <- core.StreamEvent{Type: core.StreamEventTypeUsage, Usage: &core.TokenUsage{InputTokens: 30, OutputTokens: 4, CacheReadTokens: 12}}
	events <- core.StreamEvent{Type: core.StreamEventTypeUsage}
	events <- core.StreamEvent{Type: core.StreamEventTypeDone}
	close(events)

	activity := make(chan ToolActivity, 4)
	sink := make(chan usage.Record, 4)
	text, err := collectResult(context.Background(), events, "agent", "run", activity, "anthropic", "child", sink)
	if err != nil {
		t.Fatalf("collectResult returned error: %v", err)
	}
	if text != "hello" {
		t.Fatalf("expected accumulated text, got %q", text)
	}
	if len(activity) != 0 {
		t.Fatalf("usage events leaked into tool activity: %d events", len(activity))
	}
	if len(sink) != 2 {
		t.Fatalf("expected 2 usage records, got %d", len(sink))
	}
	first := <-sink
	second := <-sink
	if first.Input != 10 || first.Output != 2 || first.Provider != "anthropic" || first.Model != "child" {
		t.Fatalf("unexpected first record: %+v", first)
	}
	if second.Input != 30 || second.CacheRead != 12 {
		t.Fatalf("unexpected second record: %+v", second)
	}
}

func TestCollectResultNilUsageSinkDropsRecords(t *testing.T) {
	events := make(chan core.StreamEvent, 2)
	events <- core.StreamEvent{Type: core.StreamEventTypeUsage, Usage: &core.TokenUsage{InputTokens: 1}}
	events <- core.StreamEvent{Type: core.StreamEventTypeDone}
	close(events)

	if _, err := collectResult(context.Background(), events, "agent", "run", nil, "anthropic", "child", nil); err != nil {
		t.Fatalf("collectResult with nil sinks returned error: %v", err)
	}
}

func TestRunnerEmitsUsageWithResolvedProfileConfig(t *testing.T) {
	client := &recordingClient{events: []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "done"},
		{Type: core.StreamEventTypeUsage, Usage: &core.TokenUsage{InputTokens: 50, OutputTokens: 7, CacheReadTokens: 20, CacheWriteTokens: 3}},
		{Type: core.StreamEventTypeDone},
	}}
	sink := make(chan usage.Record, 4)
	runner := &Runner{
		WorkingDir: "/repo",
		Config:     &config.ResolvedConfig{Provider: config.ProviderOpenAI, Model: "parent", APIKey: "key"},
		GetProfiles: staticProfiles(Profile{
			Name: "worker", Description: "Worker", Provider: config.ProviderAnthropic, Model: "child",
		}),
		ResolveConfig: func(profile Profile) (*config.ResolvedConfig, error) {
			return &config.ResolvedConfig{Provider: profile.Provider, Model: profile.Model, APIKey: "key"}, nil
		},
		NewClient:   func(*config.ResolvedConfig) (llm.LLMClient, error) { return client, nil },
		GetRegistry: staticRegistry(tools.NewRegistry()),
		NewRegistry: namedRegistryFactory,
		Usage:       sink,
	}

	if _, err := runner.Run(context.Background(), "worker", "Implement change"); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	select {
	case record := <-sink:
		if record.Provider != config.ProviderAnthropic || record.Model != "child" {
			t.Fatalf("expected resolved profile provider/model, got %+v", record)
		}
		if record.Input != 50 || record.Output != 7 || record.CacheRead != 20 || record.CacheWrite != 3 {
			t.Fatalf("unexpected token counts: %+v", record)
		}
		if record.TS.IsZero() {
			t.Fatal("expected timestamp on usage record")
		}
	default:
		t.Fatal("expected one usage record from runner")
	}
}
