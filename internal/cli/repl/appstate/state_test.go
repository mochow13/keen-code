package appstate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/skills"
	"github.com/mochow13/keen-code/internal/tools"
)

type mockLLMClient struct {
	streamChatFunc func(ctx context.Context, messages []llm.Message, toolRegistry *tools.Registry) (<-chan llm.StreamEvent, error)
	resetCount     int
}

type dummyTool struct {
	name string
}

func (d dummyTool) Name() string { return d.name }

func (d dummyTool) Description() string { return "dummy" }

func (d dummyTool) InputSchema() map[string]any { return nil }

func (d dummyTool) Execute(ctx context.Context, input any) (any, error) { return nil, nil }

func (m *mockLLMClient) StreamChat(ctx context.Context, messages []llm.Message, toolRegistry *tools.Registry, opts ...llm.StreamOptions) (<-chan llm.StreamEvent, error) {
	if m.streamChatFunc != nil {
		return m.streamChatFunc(ctx, messages, toolRegistry)
	}
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch, nil
}

func (m *mockLLMClient) Reset() {
	m.resetCount++
}

func TestNewAppState(t *testing.T) {
	client := &mockLLMClient{}
	state := New(client, t.TempDir())

	if state == nil {
		t.Fatal("expected non-nil AppState")
	}
	if state.llmClient != client {
		t.Error("expected llmClient to be set")
	}
	if len(state.messages) != 0 {
		t.Errorf("expected empty messages, got %d", len(state.messages))
	}
}

func TestAppState_AddMessage(t *testing.T) {
	state := New(nil, t.TempDir())

	state.AddMessage(llm.RoleUser, "Hello")
	if len(state.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(state.messages))
	}
	if state.messages[0].Role != llm.RoleUser {
		t.Errorf("expected role %s, got %s", llm.RoleUser, state.messages[0].Role)
	}
	if state.messages[0].Content != "Hello" {
		t.Errorf("expected content %q, got %q", "Hello", state.messages[0].Content)
	}

	state.AddMessage(llm.RoleAssistant, "Hi there")
	if len(state.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(state.messages))
	}
	if state.messages[1].Role != llm.RoleAssistant {
		t.Errorf("expected role %s, got %s", llm.RoleAssistant, state.messages[1].Role)
	}
}

func TestAppState_GetMessages(t *testing.T) {
	state := New(nil, t.TempDir())

	messages := state.GetMessages()
	if len(messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(messages))
	}

	state.AddMessage(llm.RoleUser, "Test")
	messages = state.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Content != "Test" {
		t.Errorf("expected content %q, got %q", "Test", messages[0].Content)
	}
}

func TestAppState_GetMessages_ReturnsCopy(t *testing.T) {
	state := New(nil, t.TempDir())
	state.AppendMessage(llm.Message{
		Role:    llm.RoleAssistant,
		Content: "Original",
		TurnMemory: &llm.TurnMemory{
			ToolActivity: []llm.HistoricalToolActivity{{Tool: "write_file", Input: map[string]any{"path": "a.go", "content": "content"}, Status: "success"}},
		},
	})

	messages := state.GetMessages()
	messages[0].Content = "Modified"
	messages[0].TurnMemory.ToolActivity[0].Input["path"] = "b.go"

	original := state.GetMessages()
	if original[0].Content != "Original" {
		t.Error("GetMessages should return a copy, but original was modified")
	}
	if original[0].TurnMemory.ToolActivity[0].Input["path"] != "a.go" {
		t.Error("GetMessages should deep-clone turn memory, but original was modified")
	}
}

func TestAppState_ClearMessages(t *testing.T) {
	state := New(nil, t.TempDir())

	state.AddMessage(llm.RoleUser, "Hello")
	state.AddMessage(llm.RoleAssistant, "Hi")
	if len(state.messages) != 2 {
		t.Fatalf("expected 2 messages before clear, got %d", len(state.messages))
	}

	state.ClearMessages()
	if len(state.messages) != 0 {
		t.Errorf("expected 0 messages after clear, got %d", len(state.messages))
	}
}

func TestAppState_ReloadSkillsCachesMetadataOnly(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(work, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	body := "---\nname: demo\ndescription: Demo skill\n---\nSecret instruction body"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	state := New(nil, work)
	discovery := state.GetSkills()
	var demo *skills.Skill
	for i := range discovery.Skills {
		if discovery.Skills[i].Name == "demo" {
			demo = &discovery.Skills[i]
			break
		}
	}
	if demo == nil {
		t.Fatalf("expected demo skill in cache, got %#v", discovery.Skills)
	}
	if demo.Description != "Demo skill" {
		t.Fatalf("expected metadata description, got %#v", *demo)
	}
	if strings.Contains(state.SkillsCatalog(), "Secret instruction body") {
		t.Fatal("expected cached catalog to exclude instruction body")
	}
}

func TestAppState_SkillsReloadUpdatesMetadata(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	state := New(nil, work)
	if _, ok := state.FindEnabledSkill("demo"); ok {
		t.Fatal("did not expect demo skill before it is written")
	}

	skillDir := filepath.Join(work, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo skill\n---\nBody"), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	state.ReloadSkills()
	if _, ok := state.FindEnabledSkill("demo"); !ok {
		t.Fatal("expected reloaded skill")
	}
}

func TestAppState_SetSkillStatusUpdatesCachedConfig(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(work, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo skill\n---\nBody"), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	state := New(nil, work)
	if err := state.SetSkillStatus("demo", skills.StatusDisabled); err != nil {
		t.Fatalf("SetSkillStatus() error = %v", err)
	}
	if _, ok := state.FindEnabledSkill("demo"); ok {
		t.Fatal("expected disabled skill to be hidden")
	}
}

func TestAppState_ResetClientState(t *testing.T) {
	client := &mockLLMClient{}
	state := New(client, t.TempDir())

	state.ResetClientState()

	if client.resetCount != 1 {
		t.Fatalf("expected reset once, got %d", client.resetCount)
	}
}

func TestAppState_ResetClientState_NilClient(t *testing.T) {
	state := New(nil, t.TempDir())

	state.ResetClientState()
}

func TestAppState_StreamChat_WithClient(t *testing.T) {
	expectedEvents := []llm.StreamEvent{
		{Type: llm.StreamEventTypeChunk, Content: "Hello"},
		{Type: llm.StreamEventTypeDone},
	}
	var capturedMessages []llm.Message

	client := &mockLLMClient{
		streamChatFunc: func(ctx context.Context, messages []llm.Message, toolRegistry *tools.Registry) (<-chan llm.StreamEvent, error) {
			capturedMessages = append([]llm.Message(nil), messages...)
			ch := make(chan llm.StreamEvent)
			go func() {
				defer close(ch)
				for _, e := range expectedEvents {
					ch <- e
				}
			}()
			return ch, nil
		},
	}

	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	skillDir := filepath.Join(work, ".agents", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: demo\ndescription: Demo skill\n---\nBody"), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	state := New(client, work)
	state.AddMessage(llm.RoleUser, "Hi")

	cfg := &config.ResolvedConfig{APIKey: "key", Model: "model"}
	eventCh, err := state.StreamChat(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var received []llm.StreamEvent
	for e := range eventCh {
		received = append(received, e)
	}

	if len(received) != len(expectedEvents) {
		t.Errorf("expected %d events, got %d", len(expectedEvents), len(received))
	}
	if len(capturedMessages) == 0 || capturedMessages[0].Role != llm.RoleSystem {
		t.Fatalf("expected system message, got %#v", capturedMessages)
	}
	if !strings.Contains(capturedMessages[0].Content, "- demo: Demo skill") {
		t.Fatalf("expected cached skills catalog in system prompt, got %q", capturedMessages[0].Content)
	}
}

func TestAppState_StreamChat_NilClient(t *testing.T) {
	state := New(nil, t.TempDir())
	state.AddMessage(llm.RoleUser, "Hi")

	cfg := &config.ResolvedConfig{APIKey: "key", Model: "model"}
	eventCh, err := state.StreamChat(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventCh != nil {
		t.Error("expected nil event channel when client is nil")
	}
}

func TestAppState_StreamChatPlanModeUsesPlanPromptAndRemovesWriteTools(t *testing.T) {
	var capturedMessages []llm.Message
	var capturedRegistry *tools.Registry
	client := &mockLLMClient{
		streamChatFunc: func(ctx context.Context, messages []llm.Message, toolRegistry *tools.Registry) (<-chan llm.StreamEvent, error) {
			capturedMessages = append([]llm.Message(nil), messages...)
			capturedRegistry = toolRegistry
			ch := make(chan llm.StreamEvent)
			close(ch)
			return ch, nil
		},
	}
	state := New(client, t.TempDir())
	if err := state.RegisterTool(dummyTool{name: "read_file"}); err != nil {
		t.Fatalf("register read_file: %v", err)
	}
	if err := state.RegisterTool(dummyTool{name: "write_file"}); err != nil {
		t.Fatalf("register write_file: %v", err)
	}
	if err := state.RegisterTool(dummyTool{name: "edit_file"}); err != nil {
		t.Fatalf("register edit_file: %v", err)
	}
	if err := state.RegisterTool(dummyTool{name: "bash"}); err != nil {
		t.Fatalf("register bash: %v", err)
	}
	state.SetMode(llm.ModePlan)

	if _, err := state.StreamChat(context.Background(), &config.ResolvedConfig{APIKey: "key", Model: "model"}); err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}

	if len(capturedMessages) == 0 || !strings.Contains(capturedMessages[0].Content, "# Active mode: plan") {
		t.Fatalf("expected plan system prompt, got %#v", capturedMessages)
	}
	for _, name := range []string{"read_file", "bash"} {
		if _, ok := capturedRegistry.Get(name); !ok {
			t.Fatalf("expected %s to remain in the plan mode registry", name)
		}
	}
	for _, name := range []string{"write_file", "edit_file"} {
		if _, ok := capturedRegistry.Get(name); ok {
			t.Fatalf("expected %s to be removed from the plan mode registry", name)
		}
		if _, ok := state.EffectiveToolRegistry().Get(name); ok {
			t.Fatalf("expected effective plan registry to exclude %s", name)
		}
	}
	if _, ok := state.GetToolRegistry().Get("write_file"); !ok {
		t.Fatal("expected original registry to keep write_file")
	}
	if _, ok := state.GetToolRegistry().Get("edit_file"); !ok {
		t.Fatal("expected original registry to keep edit_file")
	}
}

func TestAppState_StreamChat_ClientError(t *testing.T) {
	expectedErr := errors.New("stream error")
	client := &mockLLMClient{
		streamChatFunc: func(ctx context.Context, messages []llm.Message, toolRegistry *tools.Registry) (<-chan llm.StreamEvent, error) {
			return nil, expectedErr
		},
	}

	state := New(client, t.TempDir())
	cfg := &config.ResolvedConfig{APIKey: "key", Model: "model"}

	_, err := state.StreamChat(context.Background(), cfg)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestAppState_IsClientReady(t *testing.T) {
	client := &mockLLMClient{}

	tests := []struct {
		name     string
		client   llm.LLMClient
		cfg      *config.ResolvedConfig
		expected bool
	}{
		{
			name:     "ready with all fields",
			client:   client,
			cfg:      &config.ResolvedConfig{APIKey: "key", Model: "model"},
			expected: true,
		},
		{
			name:     "not ready with nil client",
			client:   nil,
			cfg:      &config.ResolvedConfig{APIKey: "key", Model: "model"},
			expected: false,
		},
		{
			name:     "not ready with empty API key",
			client:   client,
			cfg:      &config.ResolvedConfig{APIKey: "", Model: "model"},
			expected: false,
		},
		{
			name:     "not ready with empty model",
			client:   client,
			cfg:      &config.ResolvedConfig{APIKey: "key", Model: ""},
			expected: false,
		},
		{
			name:     "not ready with all empty",
			client:   nil,
			cfg:      &config.ResolvedConfig{APIKey: "", Model: ""},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := New(tt.client, t.TempDir())
			got := state.IsClientReady(tt.cfg)
			if got != tt.expected {
				t.Errorf("IsClientReady() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestAppState_UpdateClient(t *testing.T) {
	oldClient := &mockLLMClient{}
	state := New(oldClient, t.TempDir())

	if state.llmClient != oldClient {
		t.Error("expected old client to be set initially")
	}

	newClient := &mockLLMClient{}
	state.UpdateClient(newClient)

	if state.llmClient != newClient {
		t.Error("expected new client to be set after update")
	}
}

func TestAppState_UpdateClient_ToNil(t *testing.T) {
	client := &mockLLMClient{}
	state := New(client, t.TempDir())

	state.UpdateClient(nil)

	if state.llmClient != nil {
		t.Error("expected client to be nil after update")
	}
}

func TestAppState_StreamCompactBuildsCompactionRequest(t *testing.T) {
	var capturedMessages []llm.Message
	var capturedRegistry *tools.Registry

	client := &mockLLMClient{
		streamChatFunc: func(ctx context.Context, messages []llm.Message, toolRegistry *tools.Registry) (<-chan llm.StreamEvent, error) {
			capturedMessages = append([]llm.Message(nil), messages...)
			capturedRegistry = toolRegistry

			ch := make(chan llm.StreamEvent, 2)
			ch <- llm.StreamEvent{Type: llm.StreamEventTypeChunk, Content: "compacted summary"}
			ch <- llm.StreamEvent{Type: llm.StreamEventTypeDone}
			close(ch)
			return ch, nil
		},
	}

	state := New(client, t.TempDir())
	original := make([]llm.Message, 0, 25)
	for i := 0; i < 25; i++ {
		role := llm.RoleUser
		if i%2 == 1 {
			role = llm.RoleAssistant
		}
		msg := llm.Message{Role: role, Content: "message " + strings.Repeat("x", i+1)}
		original = append(original, msg)
		state.AddMessage(role, msg.Content)
	}

	eventCh, err := state.StreamCompact(context.Background(), &config.ResolvedConfig{
		APIKey: "key",
		Model:  "model",
	}, "Keep business logic details")
	if err != nil {
		t.Fatalf("StreamCompact() returned error: %v", err)
	}
	if eventCh == nil {
		t.Fatal("expected compaction stream")
	}

	if capturedRegistry != nil {
		t.Fatal("expected compaction to disable tools")
	}
	if len(capturedMessages) != len(original)+2 {
		t.Fatalf("expected %d compaction request messages, got %d", len(original)+2, len(capturedMessages))
	}
	if capturedMessages[0].Role != llm.RoleSystem {
		t.Fatalf("expected first compaction message to be system, got %s", capturedMessages[0].Role)
	}
	if !strings.Contains(capturedMessages[0].Content, "Keep business logic details") {
		t.Fatalf("expected extra prompt in system prompt, got %q", capturedMessages[0].Content)
	}
	for i, msg := range original {
		got := capturedMessages[i+1]
		if got != msg {
			t.Fatalf("expected compaction request message %d to match original history", i)
		}
	}
	last := capturedMessages[len(capturedMessages)-1]
	if last.Role != llm.RoleUser {
		t.Fatalf("expected final compaction message to be user, got %s", last.Role)
	}
	if last.Content != compactionUserInstruction {
		t.Fatalf("unexpected final compaction instruction: %q", last.Content)
	}
}

func TestAppState_ApplyCompactionReplacesHistoryWithSingleSummaryMessage(t *testing.T) {
	state := New(&mockLLMClient{}, t.TempDir())
	state.AddMessage(llm.RoleUser, "hello")
	state.AddMessage(llm.RoleAssistant, "world")

	if err := state.ApplyCompaction("  compacted summary  "); err != nil {
		t.Fatalf("ApplyCompaction() returned error: %v", err)
	}

	compacted := state.GetMessages()
	if len(compacted) != 1 {
		t.Fatalf("expected compacted history to contain one summary message, got %d", len(compacted))
	}
	if compacted[0].Role != llm.RoleUser || compacted[0].Content != "compacted summary" {
		t.Fatalf("unexpected summary message: %#v", compacted[0])
	}
}

func TestAppState_StreamBtwBuildsCorrectMessages(t *testing.T) {
	var capturedMessages []llm.Message
	var capturedRegistry *tools.Registry

	client := &mockLLMClient{
		streamChatFunc: func(ctx context.Context, messages []llm.Message, toolRegistry *tools.Registry) (<-chan llm.StreamEvent, error) {
			capturedMessages = append([]llm.Message(nil), messages...)
			capturedRegistry = toolRegistry

			ch := make(chan llm.StreamEvent, 2)
			ch <- llm.StreamEvent{Type: llm.StreamEventTypeChunk, Content: "answer"}
			ch <- llm.StreamEvent{Type: llm.StreamEventTypeDone}
			close(ch)
			return ch, nil
		},
	}

	state := New(client, t.TempDir())
	state.AddMessage(llm.RoleUser, "fix the bug")
	state.AddMessage(llm.RoleAssistant, "done")

	eventCh, err := state.StreamBtw(context.Background(), "what is the bug about?")
	if err != nil {
		t.Fatalf("StreamBtw() returned error: %v", err)
	}
	if eventCh == nil {
		t.Fatal("expected btw stream")
	}

	for range eventCh {
	}

	if capturedRegistry != nil {
		t.Fatal("expected btw to pass nil tool registry")
	}
	// system + 2 history + user question
	if len(capturedMessages) != 4 {
		t.Fatalf("expected 4 messages (system + 2 history + question), got %d", len(capturedMessages))
	}
	if capturedMessages[0].Role != llm.RoleSystem {
		t.Fatalf("expected first message to be system, got %s", capturedMessages[0].Role)
	}
	if !strings.Contains(capturedMessages[0].Content, "btw") {
		t.Fatalf("expected btw system prompt, got %q", capturedMessages[0].Content)
	}
	if capturedMessages[1].Role != llm.RoleUser || capturedMessages[1].Content != "fix the bug" {
		t.Fatalf("expected first history message to be user, got %#v", capturedMessages[1])
	}
	if capturedMessages[2].Role != llm.RoleAssistant || capturedMessages[2].Content != "done" {
		t.Fatalf("expected second history message to be assistant, got %#v", capturedMessages[2])
	}
	last := capturedMessages[len(capturedMessages)-1]
	if last.Role != llm.RoleUser || last.Content != "what is the bug about?" {
		t.Fatalf("expected user question as last message, got %#v", last)
	}
}

func TestBtwContext(t *testing.T) {
	makeMsg := func(role llm.Role, content string) llm.Message {
		return llm.Message{Role: role, Content: content}
	}

	tests := []struct {
		name     string
		messages []llm.Message
		max      int
		wantLen  int
		wantLast llm.Role
	}{
		{
			name:     "empty history",
			messages: nil,
			max:      10,
			wantLen:  0,
		},
		{
			name:     "only unanswered user message",
			messages: []llm.Message{makeMsg(llm.RoleUser, "hi")},
			max:      10,
			wantLen:  0,
		},
		{
			name: "trailing unanswered user message excluded",
			messages: []llm.Message{
				makeMsg(llm.RoleUser, "q1"),
				makeMsg(llm.RoleAssistant, "a1"),
				makeMsg(llm.RoleUser, "unanswered"),
			},
			max:      10,
			wantLen:  2,
			wantLast: llm.RoleAssistant,
		},
		{
			name: "capped at max",
			messages: func() []llm.Message {
				msgs := make([]llm.Message, 14)
				for i := range msgs {
					if i%2 == 0 {
						msgs[i] = makeMsg(llm.RoleUser, "u")
					} else {
						msgs[i] = makeMsg(llm.RoleAssistant, "a")
					}
				}
				return msgs
			}(),
			max:      10,
			wantLen:  10,
			wantLast: llm.RoleAssistant,
		},
		{
			name: "fewer messages than max returned as-is",
			messages: []llm.Message{
				makeMsg(llm.RoleUser, "q"),
				makeMsg(llm.RoleAssistant, "a"),
			},
			max:      10,
			wantLen:  2,
			wantLast: llm.RoleAssistant,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := btwContext(tt.messages, tt.max)
			if len(got) != tt.wantLen {
				t.Fatalf("btwContext() len = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 && got[len(got)-1].Role != tt.wantLast {
				t.Fatalf("btwContext() last role = %s, want %s", got[len(got)-1].Role, tt.wantLast)
			}
		})
	}
}

func TestAppState_StreamBtwNilClient(t *testing.T) {
	state := New(nil, t.TempDir())
	state.AddMessage(llm.RoleUser, "hello")

	eventCh, err := state.StreamBtw(context.Background(), "question")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventCh != nil {
		t.Error("expected nil event channel when client is nil")
	}
}

func TestAppState_StreamBtwDoesNotModifyMessages(t *testing.T) {
	client := &mockLLMClient{}
	state := New(client, t.TempDir())
	state.AddMessage(llm.RoleUser, "original")

	_, _ = state.StreamBtw(context.Background(), "side question")

	messages := state.GetMessages()
	if len(messages) != 1 || messages[0].Content != "original" {
		t.Fatalf("expected btw not to modify appstate messages, got %#v", messages)
	}
}

func TestAppState_ApplyCompactionLeavesMessagesUntouchedOnError(t *testing.T) {
	state := New(&mockLLMClient{}, t.TempDir())
	state.AddMessage(llm.RoleUser, "hello")
	state.AddMessage(llm.RoleAssistant, "world")
	original := state.GetMessages()

	err := state.ApplyCompaction(" \n\t ")
	if err == nil || err.Error() != "compaction returned empty summary" {
		t.Fatalf("expected empty summary error, got %v", err)
	}

	if got := state.GetMessages(); len(got) != len(original) || got[0] != original[0] || got[1] != original[1] {
		t.Fatalf("expected messages to remain unchanged, got %#v", got)
	}
}

func TestAppState_StreamCompactLeavesMessagesUntouchedOnCancel(t *testing.T) {
	state := New(&mockLLMClient{}, t.TempDir())
	state.AddMessage(llm.RoleUser, "hello")
	original := state.GetMessages()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	eventCh, err := state.StreamCompact(ctx, &config.ResolvedConfig{
		APIKey: "key",
		Model:  "model",
	}, "")
	if err != nil {
		t.Fatalf("expected nil error from StreamCompact, got %v", err)
	}
	if eventCh == nil {
		t.Fatal("expected compaction stream")
	}

	if got := state.GetMessages(); len(got) != len(original) || got[0] != original[0] {
		t.Fatalf("expected messages to remain unchanged, got %#v", got)
	}
}
