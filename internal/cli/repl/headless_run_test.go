package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/cli/repl/appstate"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/session"
	"github.com/mochow13/keen-code/internal/tools"
)

type recordingHeadlessClient struct {
	events   []llm.StreamEvent
	messages [][]llm.Message
	opts     [][]llm.StreamOptions
}

func (c *recordingHeadlessClient) StreamChat(ctx context.Context, messages []llm.Message, toolRegistry *tools.Registry, opts ...llm.StreamOptions) (<-chan llm.StreamEvent, error) {
	c.messages = append(c.messages, llm.CloneMessages(messages))
	c.opts = append(c.opts, append([]llm.StreamOptions(nil), opts...))
	ch := make(chan llm.StreamEvent, len(c.events))
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
	client := &recordingHeadlessClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeChunk, Content: "Let me check."},
		{Type: llm.StreamEventTypeToolStart, ToolCall: &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "foo.go"}}},
		{Type: llm.StreamEventTypeToolEnd, ToolCall: &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": "foo.go"}}},
		{Type: llm.StreamEventTypeChunk, Content: " Done."},
		{Type: llm.StreamEventTypeDone},
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
		toolCall  *llm.ToolCall
		forbidden string
	}{
		{
			name: "missing read file",
			toolCall: &llm.ToolCall{
				Name:  "read_file",
				Error: `not found: file "missing.go" does not exist`,
			},
			forbidden: "missing.go",
		},
		{
			name: "stale edit anchor",
			toolCall: &llm.ToolCall{
				Name:  "edit_file",
				Error: `op 1: anchor "2:fff" does not exist in the current file snapshot; re-read the file and retry`,
			},
			forbidden: "current file snapshot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &recordingHeadlessClient{events: []llm.StreamEvent{
				{Type: llm.StreamEventTypeChunk, Content: "Trying."},
				{Type: llm.StreamEventTypeToolEnd, ToolCall: tt.toolCall},
				{Type: llm.StreamEventTypeChunk, Content: " Retrying."},
				{Type: llm.StreamEventTypeDone},
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
	client := &recordingHeadlessClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeChunk, Content: "json response"},
		{Type: llm.StreamEventTypeDone},
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
	client := &recordingHeadlessClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeChunk, Content: "hello"},
		{Type: llm.StreamEventTypeUsage, Usage: &llm.TokenUsage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}},
		{Type: llm.StreamEventTypeDone},
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
	firstClient := &recordingHeadlessClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeChunk, Content: "first response"},
		{Type: llm.StreamEventTypeDone},
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

	secondClient := &recordingHeadlessClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeChunk, Content: "second response"},
		{Type: llm.StreamEventTypeDone},
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
	want := []string{"first prompt", "first response", "second prompt"}
	if !containsOrderedSuffix(got, want) {
		t.Fatalf("expected conversation suffix %#v, got %#v", want, got)
	}
	if len(secondClient.opts) != 1 || len(secondClient.opts[0]) != 1 || secondClient.opts[0][0].SessionID != first.SessionID {
		t.Fatalf("expected session stream option %q, got %#v", first.SessionID, secondClient.opts)
	}
}

func TestRunHeadless_PersistsHistoricalToolActivity(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeChunk, Content: "Let me inspect."},
		{Type: llm.StreamEventTypeToolStart, ToolCall: &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": filepath.Join(workingDir, "a.go")}}},
		{Type: llm.StreamEventTypeToolEnd, ToolCall: &llm.ToolCall{Name: "read_file", Input: map[string]any{"path": filepath.Join(workingDir, "a.go")}}},
		{Type: llm.StreamEventTypeChunk, Content: " Found it."},
		{Type: llm.StreamEventTypeDone},
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
	client := &recordingHeadlessClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeChunk, Content: "done <promise>COMPLETE</promise>"},
		{Type: llm.StreamEventTypeDone},
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
	client := &recordingHeadlessClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeChunk, Content: "done but not complete"},
		{Type: llm.StreamEventTypeDone},
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
	client := &recordingHeadlessClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeChunk, Content: "json response"},
		{Type: llm.StreamEventTypeDone},
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
	handler.Start(make(chan llm.StreamEvent), "")
	appState := appstate.New(nil, "/tmp")
	completedText := &strings.Builder{}
	turnMemory := newTurnMemoryAccumulator(false)

	err := checkpointHeadlessAutoCompaction(nil, appState, handler, turnMemory, completedText, &llm.AutoCompactionEvent{})
	if err == nil {
		t.Fatal("expected empty replacement error")
	}
	if !strings.Contains(err.Error(), "without replacement history") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunHeadless_AutoCompactionFailureReturnsPartialOutput(t *testing.T) {
	workingDir := setupHeadlessTestHome(t)
	client := &recordingHeadlessClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeChunk, Content: "before checkpoint"},
		{Type: llm.StreamEventTypeAutoCompactionApplied, AutoCompaction: &llm.AutoCompactionEvent{
			Replacement: []llm.Message{
				{Role: llm.RoleSystem, Content: "provider system prompt"},
				{Role: llm.RoleUser, Content: "compacted context"},
			},
		}},
		{Type: llm.StreamEventTypeChunk, Content: " after checkpoint"},
		{Type: llm.StreamEventTypeError, Error: errors.New("provider failed")},
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
	client := &recordingHeadlessClient{events: []llm.StreamEvent{
		{Type: llm.StreamEventTypeChunk, Content: "before checkpoint"},
		{Type: llm.StreamEventTypeReasoningChunk, Content: "private reasoning"},
		{Type: llm.StreamEventTypeAutoCompactionApplied, AutoCompaction: &llm.AutoCompactionEvent{
			Replacement: []llm.Message{{Role: llm.RoleUser, Content: "compacted context"}},
		}},
		{Type: llm.StreamEventTypeChunk, Content: " after checkpoint"},
		{Type: llm.StreamEventTypeDone},
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
	if got := session.BuildConversation(events); len(got) != 2 || got[0].Role != llm.RoleUser || got[0].Content != "compacted context" || got[1].Content != " after checkpoint" {
		t.Fatalf("unexpected projected conversation: %#v", got)
	}
}

func messageContents(messages []llm.Message) []string {
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
