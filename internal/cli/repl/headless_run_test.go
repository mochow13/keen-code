package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/mochow13/keen-code/internal/cli/repl/agentcore"
	"github.com/mochow13/keen-code/internal/llm/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/session"
	"github.com/mochow13/keen-code/internal/tools"
)

type recordingHeadlessClient struct {
	events   []core.StreamEvent
	messages [][]core.Message
	opts     [][]core.StreamOptions
}

func (c *recordingHeadlessClient) StreamChat(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry, opts ...core.StreamOptions) (<-chan core.StreamEvent, error) {
	c.messages = append(c.messages, core.CloneMessages(messages))
	c.opts = append(c.opts, append([]core.StreamOptions(nil), opts...))
	ch := make(chan core.StreamEvent, len(c.events))
	go func() {
		defer close(ch)
		for _, event := range c.events {
			select {
			case ch <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

func (c *recordingHeadlessClient) Reset() {}

func TestRunHeadless_StreamsProgress(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{events: []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "Let me check."},
		{Type: core.StreamEventTypeToolStart, ToolCall: &core.ToolCall{Name: "read_file", Input: map[string]any{"path": "foo.go"}}},
		{Type: core.StreamEventTypeToolEnd, ToolCall: &core.ToolCall{Name: "read_file", Input: map[string]any{"path": "foo.go"}}},
		{Type: core.StreamEventTypeChunk, Content: " Done."},
		{Type: core.StreamEventTypeDone},
	}}
	var out bytes.Buffer
	var progress bytes.Buffer

	result, err := RunHeadless(context.Background(), HeadlessRunOptions{
		WorkingDir: workingDir,
		Config:     headlessTestConfig(),
		Client:     client,
		Prompt:     "inspect",
		Out:        &out,
		Progress:   &progress,
	})
	if err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}
	if result.Text != "Let me check. Done." {
		t.Fatalf("result text = %q", result.Text)
	}
	if out.String() != "Let me check. Done.\n" {
		t.Fatalf("out = %q", out.String())
	}

	progressText := progress.String()
	if !strings.Contains(progressText, "Let me check.") {
		t.Fatalf("progress missing text: %q", progressText)
	}
	if !strings.Contains(progressText, "Read") {
		t.Fatalf("progress missing tool end: %q", progressText)
	}
	if !strings.Contains(progressText, " Done.") {
		t.Fatalf("progress missing trailing text: %q", progressText)
	}
}

func TestRunHeadless_ProgressHidesExpectedToolFailures(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	tests := []struct {
		name      string
		toolCall  *core.ToolCall
		forbidden string
	}{
		{
			name: "missing read file",
			toolCall: &core.ToolCall{
				Name:  "read_file",
				Error: `not found: file "missing.go" does not exist`,
			},
			forbidden: "missing.go",
		},
		{
			name: "stale edit anchor",
			toolCall: &core.ToolCall{
				Name:  "edit_file",
				Error: `op 1: anchor "2:fff" does not exist in the current file snapshot; re-read the file and retry`,
			},
			forbidden: "current file snapshot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &recordingHeadlessClient{events: []core.StreamEvent{
				{Type: core.StreamEventTypeChunk, Content: "Trying."},
				{Type: core.StreamEventTypeToolEnd, ToolCall: tt.toolCall},
				{Type: core.StreamEventTypeChunk, Content: " Retrying."},
				{Type: core.StreamEventTypeDone},
			}}
			var progress bytes.Buffer

			if _, err := RunHeadless(context.Background(), HeadlessRunOptions{
				WorkingDir: workingDir,
				Config:     headlessTestConfig(),
				Client:     client,
				Prompt:     "inspect",
				Progress:   &progress,
			}); err != nil {
				t.Fatalf("RunHeadless() error = %v", err)
			}
			if got := progress.String(); strings.Contains(got, tt.forbidden) {
				t.Fatalf("progress exposed hidden tool failure: %q", got)
			}
			if got := progress.String(); got != "Trying. Retrying.\n" {
				t.Fatalf("progress = %q", got)
			}
		})
	}
}

func TestRunHeadless_ProgressDisabledForJSON(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{events: []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "json response"},
		{Type: core.StreamEventTypeDone},
	}}
	var out bytes.Buffer
	var progress bytes.Buffer

	_, err := RunHeadless(context.Background(), HeadlessRunOptions{
		WorkingDir: workingDir,
		Config:     headlessTestConfig(),
		Client:     client,
		Prompt:     "prompt",
		Format:     HeadlessFormatJSON,
		Out:        &out,
		Progress:   &progress,
	})
	if err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}
	if progress.String() != "" {
		t.Fatalf("progress emitted for JSON format: %q", progress.String())
	}
}

func TestRunHeadless_CreatesSessionAndWritesText(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{events: []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "hello"},
		{Type: core.StreamEventTypeUsage, Usage: &core.TokenUsage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}},
		{Type: core.StreamEventTypeDone},
	}}
	var out bytes.Buffer

	result, err := RunHeadless(context.Background(), HeadlessRunOptions{
		WorkingDir: workingDir,
		Config:     headlessTestConfig(),
		Client:     client,
		Prompt:     "say hi",
		Out:        &out,
	})
	if err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}
	if result.SessionID == "" {
		t.Fatal("expected session id")
	}
	if result.OpenCodeSessionID == "" || result.OpenCodeSessionID == result.SessionID {
		t.Fatalf("expected hyphen-stripped OpenCode session id, got %q from %q", result.OpenCodeSessionID, result.SessionID)
	}
	if result.Text != "hello" || out.String() != "hello\n" {
		t.Fatalf("unexpected output result=%q out=%q", result.Text, out.String())
	}
	if result.Usage == nil || result.Usage.InputTokens != 3 || result.Usage.OutputTokens != 2 || result.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage: %#v", result.Usage)
	}

	events := loadOnlyHeadlessSessionEvents(t, workingDir)
	if len(events) != 3 {
		t.Fatalf("expected session started, user, assistant events; got %d", len(events))
	}
	if events[1].UserMessage == nil || events[1].UserMessage.Content != "say hi" {
		t.Fatalf("unexpected user event: %#v", events[1].UserMessage)
	}
	if events[2].AssistantTurn == nil || events[2].AssistantTurn.Message != "hello" {
		t.Fatalf("unexpected assistant event: %#v", events[2].AssistantTurn)
	}
}

func TestRunHeadless_ResumesSessionConversation(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	firstClient := &recordingHeadlessClient{events: []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "first response"},
		{Type: core.StreamEventTypeDone},
	}}

	first, err := RunHeadless(context.Background(), HeadlessRunOptions{
		WorkingDir: workingDir,
		Config:     headlessTestConfig(),
		Client:     firstClient,
		Prompt:     "first prompt",
	})
	if err != nil {
		t.Fatalf("first RunHeadless() error = %v", err)
	}

	secondClient := &recordingHeadlessClient{events: []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "second response"},
		{Type: core.StreamEventTypeDone},
	}}
	_, err = RunHeadless(context.Background(), HeadlessRunOptions{
		WorkingDir: workingDir,
		Config:     headlessTestConfig(),
		Client:     secondClient,
		SessionID:  first.SessionID,
		Prompt:     "second prompt",
	})
	if err != nil {
		t.Fatalf("second RunHeadless() error = %v", err)
	}
	if len(secondClient.messages) != 1 {
		t.Fatalf("expected one StreamChat call, got %d", len(secondClient.messages))
	}
	got := messageContents(secondClient.messages[0])
	if len(got) < 4 || got[len(got)-3] != "first prompt" || got[len(got)-2] != "first response" {
		t.Fatalf("expected conversation suffix ending with first prompt/response, got %#v", got)
	}
	if last := got[len(got)-1]; last != "second prompt" {
		t.Fatalf("expected last message to be second prompt with build mode suffix, got %#v", got)
	}
	if len(secondClient.opts) != 1 || len(secondClient.opts[0]) != 1 || secondClient.opts[0][0].SessionID != first.SessionID {
		t.Fatalf("expected session stream option %q, got %#v", first.SessionID, secondClient.opts)
	}
}

func TestRunHeadless_PersistsHistoricalToolActivity(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{events: []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "Let me inspect."},
		{Type: core.StreamEventTypeToolStart, ToolCall: &core.ToolCall{Name: "read_file", Input: map[string]any{"path": filepath.Join(workingDir, "a.go")}}},
		{Type: core.StreamEventTypeToolEnd, ToolCall: &core.ToolCall{Name: "read_file", Input: map[string]any{"path": filepath.Join(workingDir, "a.go")}}},
		{Type: core.StreamEventTypeChunk, Content: " Found it."},
		{Type: core.StreamEventTypeDone},
	}}

	if _, err := RunHeadless(context.Background(), HeadlessRunOptions{
		WorkingDir: workingDir,
		Config:     headlessTestConfig(),
		Client:     client,
		Prompt:     "inspect",
	}); err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}

	events := loadOnlyHeadlessSessionEvents(t, workingDir)
	memory := events[len(events)-1].AssistantTurn.TurnMemory
	if memory == nil || len(memory.ToolActivity) != 1 {
		t.Fatalf("expected historical tool activity, got %#v", memory)
	}
	activity := memory.ToolActivity[0]
	if activity.Tool != "read_file" || activity.Input["path"] != "a.go" || activity.TextOffset != len("Let me inspect.") {
		t.Fatalf("unexpected historical tool activity %#v", activity)
	}
}

func TestRunHeadless_CompletionSignalPresent(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{events: []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "done <promise>COMPLETE</promise>"},
		{Type: core.StreamEventTypeDone},
	}}
	var out bytes.Buffer

	result, err := RunHeadless(context.Background(), HeadlessRunOptions{
		WorkingDir:       workingDir,
		Config:           headlessTestConfig(),
		Client:           client,
		Prompt:           "task",
		CompletionSignal: "<promise>COMPLETE</promise>",
		Out:              &out,
	})
	if err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}
	if !strings.Contains(result.Text, "<promise>COMPLETE</promise>") {
		t.Fatalf("result text missing signal: %q", result.Text)
	}
}

func TestRunHeadless_CompletionSignalMissing(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{events: []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "done but not complete"},
		{Type: core.StreamEventTypeDone},
	}}
	var out bytes.Buffer

	result, err := RunHeadless(context.Background(), HeadlessRunOptions{
		WorkingDir:       workingDir,
		Config:           headlessTestConfig(),
		Client:           client,
		Prompt:           "task",
		CompletionSignal: "<promise>COMPLETE</promise>",
		Out:              &out,
	})
	if !errors.Is(err, ErrCompletionSignalMissing) {
		t.Fatalf("RunHeadless() error = %v, want ErrCompletionSignalMissing", err)
	}
	if result == nil || result.Text == "" {
		t.Fatalf("expected result with partial text, got %#v", result)
	}
	if !strings.Contains(out.String(), "done but not complete") {
		t.Fatalf("expected output to be written despite missing signal: %q", out.String())
	}
}

func TestRunHeadless_WritesJSON(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{events: []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "json response"},
		{Type: core.StreamEventTypeDone},
	}}
	var out bytes.Buffer

	_, err := RunHeadless(context.Background(), HeadlessRunOptions{
		WorkingDir: workingDir,
		Config:     headlessTestConfig(),
		Client:     client,
		Prompt:     "prompt",
		Format:     HeadlessFormatJSON,
		Out:        &out,
	})
	if err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}

	var decoded HeadlessRunResult
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json output: %v", err)
	}
	if decoded.SessionID == "" || decoded.Text != "json response" {
		t.Fatalf("unexpected json result: %#v", decoded)
	}
}

func setupHeadlessTestHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	workingDir := filepath.Join(tmp, "project")
	if err := os.MkdirAll(workingDir, 0755); err != nil {
		t.Fatalf("create working dir: %v", err)
	}
	return workingDir
}

func headlessTestConfig() *config.ResolvedConfig {
	return &config.ResolvedConfig{
		Provider: config.ProviderOpenAI,
		APIKey:   "test-key",
		Model:    "test-model",
	}
}

func loadOnlyHeadlessSessionEvents(t *testing.T, workingDir string) []session.Event {
	t.Helper()
	store, err := session.NewStore(workingDir)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	summaries, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one session, got %d", len(summaries))
	}
	loaded, err := store.Load(summaries[0])
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return loaded.Events
}

func TestCheckpointHeadlessAutoCompactionRejectsEmptyReplacement(t *testing.T) {
	handler := NewStreamHandler(nil)
	handler.Start(make(chan agentcore.StreamEvent), "")
	completedText := &strings.Builder{}
	turnMemory := newTurnMemoryAccumulator(false)

	err := checkpointHeadlessAutoCompaction(nil, agentcore.New(nil, "/tmp", &config.ResolvedConfig{}, nil), handler, turnMemory, completedText, &agentcore.AutoCompactionEvent{})
	if err == nil {
		t.Fatal("expected empty replacement error")
	}
	if !strings.Contains(err.Error(), "without replacement history") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunHeadless_AutoCompactionFailureReturnsPartialOutput(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{events: []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "before checkpoint"},
		{Type: core.StreamEventTypeAutoCompactionApplied, AutoCompaction: &core.AutoCompactionEvent{
			Replacement: []core.Message{
				{Role: core.RoleSystem, Content: "provider system prompt"},
				{Role: core.RoleUser, Content: "compacted context"},
			},
		}},
		{Type: core.StreamEventTypeChunk, Content: " after checkpoint"},
		{Type: core.StreamEventTypeError, Error: errors.New("provider failed")},
	}}
	var out bytes.Buffer

	result, err := RunHeadless(context.Background(), HeadlessRunOptions{
		WorkingDir: workingDir,
		Config:     headlessTestConfig(),
		Client:     client,
		Prompt:     "task",
		Out:        &out,
	})
	if err == nil || !strings.Contains(err.Error(), "provider failed") {
		t.Fatalf("error = %v, want provider failure", err)
	}
	if result == nil || result.Text != "before checkpoint after checkpoint" {
		t.Fatalf("result = %#v", result)
	}
	if got := out.String(); got != "before checkpoint after checkpoint\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestRunHeadless_AutoCompactionCheckpointsOutputAndSession(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{events: []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "before checkpoint"},
		{Type: core.StreamEventTypeReasoningChunk, Content: "private reasoning"},
		{Type: core.StreamEventTypeAutoCompactionApplied, AutoCompaction: &core.AutoCompactionEvent{
			Replacement: []core.Message{{Role: core.RoleUser, Content: "compacted context"}},
		}},
		{Type: core.StreamEventTypeChunk, Content: " after checkpoint"},
		{Type: core.StreamEventTypeDone},
	}}
	var out bytes.Buffer

	result, err := RunHeadless(context.Background(), HeadlessRunOptions{
		WorkingDir: workingDir,
		Config:     headlessTestConfig(),
		Client:     client,
		Prompt:     "task",
		Format:     HeadlessFormatJSON,
		Out:        &out,
	})
	if err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}
	if result.Text != "before checkpoint after checkpoint" {
		t.Fatalf("result text = %q", result.Text)
	}
	if bytes.Contains(out.Bytes(), []byte("private")) {
		t.Fatalf("headless output exposed private compaction content: %s", out.String())
	}

	events := loadOnlyHeadlessSessionEvents(t, workingDir)
	if len(events) != 5 {
		t.Fatalf("expected session, user, checkpoint, compaction, final events; got %d", len(events))
	}
	if got := events[2].AssistantTurn; got == nil || got.Message != "before checkpoint" {
		t.Fatalf("unexpected checkpoint event: %#v", got)
	}
	compaction := events[3].CompactionApplied
	if compaction == nil || compaction.Status != "" || len(compaction.Transcript) != 0 {
		t.Fatalf("unexpected compaction event: %#v", compaction)
	}
	if got := session.BuildConversation(events); len(got) != 2 || got[0].Role != core.RoleUser || got[0].Content != "compacted context" || got[1].Content != " after checkpoint" {
		t.Fatalf("unexpected projected conversation: %#v", got)
	}
}

func TestRunHeadless_ClosedStreamWithoutDoneFinishesSuccessfully(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{events: []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "partial"},
	}}
	var out bytes.Buffer

	result, err := RunHeadless(context.Background(), HeadlessRunOptions{
		WorkingDir: workingDir,
		Config:     headlessTestConfig(),
		Client:     client,
		Prompt:     "task",
		Out:        &out,
	})
	if err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}
	if result.Text != "partial" {
		t.Fatalf("result text = %q, want %q", result.Text, "partial")
	}
	if !strings.Contains(out.String(), "partial") {
		t.Fatalf("expected output to contain partial response: %q", out.String())
	}
}

func TestRunHeadless_ClosedStreamWithCancelledContextFails(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{}
	var out bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := RunHeadless(ctx, HeadlessRunOptions{
		WorkingDir: workingDir,
		Config:     headlessTestConfig(),
		Client:     client,
		Prompt:     "task",
		Out:        &out,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunHeadless() error = %v, want context.Canceled", err)
	}
	if result == nil {
		t.Fatal("expected partial result on cancellation")
	}
}

func TestRunHeadless_ClosedStreamWithDeadlineExceededFails(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{}
	var out bytes.Buffer

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
	defer cancel()

	result, err := RunHeadless(ctx, HeadlessRunOptions{
		WorkingDir: workingDir,
		Config:     headlessTestConfig(),
		Client:     client,
		Prompt:     "task",
		Out:        &out,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunHeadless() error = %v, want context.DeadlineExceeded", err)
	}
	if result == nil {
		t.Fatal("expected partial result on deadline exceeded")
	}
}

func messageContents(messages []core.Message) []string {
	contents := make([]string, 0, len(messages))
	for _, message := range messages {
		contents = append(contents, message.Content)
	}
	return contents
}

func containsOrderedSuffix(got []string, want []string) bool {
	if len(got) < len(want) {
		return false
	}
	got = got[len(got)-len(want):]
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
