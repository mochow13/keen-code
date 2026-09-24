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
	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/skills"
	"github.com/mochow13/keen-code/internal/tools"
)

type mockLLMClient struct {
	streamChatFunc func(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry, opts []core.StreamOptions) (<-chan core.StreamEvent, error)
	resetCount     int
}

type dummyTool struct {
	name string
}

func (d dummyTool) Name() string { return d.name }

func (d dummyTool) Description() string { return "dummy" }

func (d dummyTool) InputSchema() map[string]any { return nil }

func (d dummyTool) Execute(ctx context.Context, input any) (any, error) { return nil, nil }

func (m *mockLLMClient) StreamChat(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry, opts ...core.StreamOptions) (<-chan core.StreamEvent, error) {
	if m.streamChatFunc != nil {
		return m.streamChatFunc(ctx, messages, toolRegistry, opts)
	}
	ch := make(chan core.StreamEvent)
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

	state.AddMessage(core.RoleUser, "Hello")
	if len(state.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(state.messages))
	}
	if state.messages[0].Role != core.RoleUser {
		t.Errorf("expected role %s, got %s", core.RoleUser, state.messages[0].Role)
	}
	if state.messages[0].Content != "Hello" {
		t.Errorf("expected content %q, got %q", "Hello", state.messages[0].Content)
	}

	state.AddMessage(core.RoleAssistant, "Hi there")
	if len(state.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(state.messages))
	}
	if state.messages[1].Role != core.RoleAssistant {
		t.Errorf("expected role %s, got %s", core.RoleAssistant, state.messages[1].Role)
	}
}

func TestAppState_GetMessages(t *testing.T) {
	state := New(nil, t.TempDir())

	messages := state.GetMessages()
	if len(messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(messages))
	}

	state.AddMessage(core.RoleUser, "Test")
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
	state.AppendMessage(core.Message{
		Role:    core.RoleAssistant,
		Content: "Original",
		TurnMemory: &core.TurnMemory{
			ToolActivity: []core.HistoricalToolActivity{{Tool: "write_file", Input: map[string]any{"path": "a.go", "content": "content"}, Status: "success"}},
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

	state.AddMessage(core.RoleUser, "Hello")
	state.AddMessage(core.RoleAssistant, "Hi")
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
	expectedEvents := []core.StreamEvent{
		{Type: core.StreamEventTypeChunk, Content: "Hello"},
		{Type: core.StreamEventTypeDone},
	}
	var capturedMessages []core.Message

	client := &mockLLMClient{
		streamChatFunc: func(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry, _ []core.StreamOptions) (<-chan core.StreamEvent, error) {
			capturedMessages = append([]core.Message(nil), messages...)
			ch := make(chan core.StreamEvent)
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
	state.AddMessage(core.RoleUser, "Hi")

	cfg := &config.ResolvedConfig{APIKey: "key", Model: "model"}
	eventCh, err := state.StreamChat(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var received []core.StreamEvent
	for e := range eventCh {
		received = append(received, e)
	}

	if len(received) != len(expectedEvents) {
		t.Errorf("expected %d events, got %d", len(expectedEvents), len(received))
	}
	if len(capturedMessages) == 0 || capturedMessages[0].Role != core.RoleSystem {
		t.Fatalf("expected system message, got %#v", capturedMessages)
	}
	if !strings.Contains(capturedMessages[0].Content, "- demo: Demo skill") {
		t.Fatalf("expected cached skills catalog in system prompt, got %q", capturedMessages[0].Content)
	}
}

func TestAppState_StreamChat_NilClient(t *testing.T) {
	state := New(nil, t.TempDir())
	state.AddMessage(core.RoleUser, "Hi")

	cfg := &config.ResolvedConfig{APIKey: "key", Model: "model"}
	eventCh, err := state.StreamChat(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventCh != nil {
		t.Error("expected nil event channel when client is nil")
	}
}

func TestAppState_StreamChatPlanModeKeepsPromptStableAndDeniesWritesAtExecution(t *testing.T) {
	var capturedMessages []core.Message
	var capturedRegistry *tools.Registry
	var capturedOpts []core.StreamOptions
	client := &mockLLMClient{
		streamChatFunc: func(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry, opts []core.StreamOptions) (<-chan core.StreamEvent, error) {
			capturedMessages = append([]core.Message(nil), messages...)
			capturedRegistry = toolRegistry
			capturedOpts = append([]core.StreamOptions(nil), opts...)
			ch := make(chan core.StreamEvent)
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
	state.AddUserMessage("make a plan")

	if _, err := state.StreamChat(context.Background(), &config.ResolvedConfig{APIKey: "key", Model: "model"}); err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}

	if len(capturedMessages) == 0 || strings.Contains(capturedMessages[0].Content, "Active mode:") {
		t.Fatalf("expected stable system prompt without mode suffix, got %#v", capturedMessages)
	}
	if last := capturedMessages[len(capturedMessages)-1]; last.Role != core.RoleUser || !strings.Contains(last.Content, "Active mode: plan") {
		t.Fatalf("expected plan suffix on last user message, got %#v", last)
	}
	if got := state.GetMessages(); len(got) != 1 || got[0].Content != "make a plan"+llm.ModeUserSuffix(llm.ModePlan) {
		t.Fatalf("expected stored history to persist plan suffix, got %#v", got)
	}
	for _, name := range []string{"read_file", "bash", "write_file", "edit_file"} {
		if _, ok := capturedRegistry.Get(name); !ok {
			t.Fatalf("expected %s to remain in the request registry for cache stability", name)
		}
	}
	if len(capturedOpts) != 1 || !capturedOpts[0].DisableWriteToolCalls {
		t.Fatalf("expected DisableWriteToolCalls in plan mode, got %#v", capturedOpts)
	}
	for _, name := range []string{"write_file", "edit_file"} {
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

	state.SetMode(llm.ModeBuild)
	state.AddUserMessage("build it")
	if _, err := state.StreamChat(context.Background(), &config.ResolvedConfig{APIKey: "key", Model: "model"}); err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	if last := capturedMessages[len(capturedMessages)-1]; strings.Contains(last.Content, "Active mode:") {
		t.Fatalf("expected no mode suffix on build user message, got %#v", last)
	}
	if last := capturedMessages[len(capturedMessages)-1]; last.Content != "build it" {
		t.Fatalf("expected stored build message unchanged, got %#v", last)
	}
	if first := capturedMessages[1]; !strings.Contains(first.Content, "Active mode: plan") {
		t.Fatalf("expected first user message to keep plan suffix, got %#v", first)
	}
	if _, err := state.StreamChat(context.Background(), &config.ResolvedConfig{APIKey: "key", Model: "model"}); err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	if last := capturedMessages[len(capturedMessages)-1]; strings.Contains(last.Content, "Active mode:") {
		t.Fatalf("expected no mode suffix on build user message, got %#v", last)
	}
	if len(capturedOpts) != 1 || capturedOpts[0].DisableWriteToolCalls {
		t.Fatalf("expected DisableWriteToolCalls=false in build mode, got %#v", capturedOpts)
	}
}

func TestAppState_StreamChat_ClientError(t *testing.T) {
	expectedErr := errors.New("stream error")
	client := &mockLLMClient{
		streamChatFunc: func(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry, _ []core.StreamOptions) (<-chan core.StreamEvent, error) {
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
	var capturedMessages []core.Message
	var capturedRegistry *tools.Registry
	var capturedOpts []core.StreamOptions

	client := &mockLLMClient{
		streamChatFunc: func(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry, opts []core.StreamOptions) (<-chan core.StreamEvent, error) {
			capturedMessages = append([]core.Message(nil), messages...)
			capturedRegistry = toolRegistry
			capturedOpts = append([]core.StreamOptions(nil), opts...)

			ch := make(chan core.StreamEvent, 2)
			ch <- core.StreamEvent{Type: core.StreamEventTypeChunk, Content: "compacted summary"}
			ch <- core.StreamEvent{Type: core.StreamEventTypeDone}
			close(ch)
			return ch, nil
		},
	}

	state := New(client, t.TempDir())
	original := make([]core.Message, 0, 25)
	for i := 0; i < 25; i++ {
		role := core.RoleUser
		if i%2 == 1 {
			role = core.RoleAssistant
		}
		msg := core.Message{Role: role, Content: "message " + strings.Repeat("x", i+1)}
		original = append(original, msg)
		state.AddMessage(role, msg.Content)
	}

	eventCh, err := state.StreamCompact(context.Background(), &config.ResolvedConfig{
		APIKey: "key",
		Model:  "model",
	}, "Keep business logic details", core.StreamOptions{SessionID: "compact-session"})
	if err != nil {
		t.Fatalf("StreamCompact() returned error: %v", err)
	}
	if eventCh == nil {
		t.Fatal("expected compaction stream")
	}

	if capturedRegistry != state.GetToolRegistry() {
		t.Fatal("expected compaction to reuse the normal tool registry for prompt-cache parity")
	}
	if len(capturedOpts) != 1 || !capturedOpts[0].DisableAutoCompaction || !capturedOpts[0].DisableToolCalls || capturedOpts[0].SessionID != "compact-session" {
		t.Fatalf("expected compaction options to preserve the session ID, disable tool calls, and block nested automatic compaction, got %#v", capturedOpts)
	}
	if len(capturedMessages) != len(original)+2 {
		t.Fatalf("expected %d compaction request messages, got %d", len(original)+2, len(capturedMessages))
	}
	if capturedMessages[0].Role != core.RoleSystem {
		t.Fatalf("expected first compaction message to be system, got %s", capturedMessages[0].Role)
	}
	wantSystem := llm.Build(state.WorkingDir(), state.SkillsCatalog(), state.SubagentsCatalog())
	if capturedMessages[0].Content != wantSystem {
		t.Fatalf("expected the normal agent system prompt, got %q", capturedMessages[0].Content)
	}
	for i, msg := range original {
		got := capturedMessages[i+1]
		if got != msg {
			t.Fatalf("expected compaction request message %d to match original history", i)
		}
	}
	last := capturedMessages[len(capturedMessages)-1]
	if last.Role != core.RoleUser {
		t.Fatalf("expected final compaction message to be user, got %s", last.Role)
	}
	if last.Content != llm.BuildCompactionPrompt("Keep business logic details") {
		t.Fatalf("unexpected final compaction instruction: %q", last.Content)
	}
}

func TestAppState_ApplyCompactionReplacesHistoryWithSingleSummaryMessage(t *testing.T) {
	state := New(&mockLLMClient{}, t.TempDir())
	state.AddMessage(core.RoleUser, "hello")
	state.AddMessage(core.RoleAssistant, "world")

	if err := state.ApplyCompaction("  compacted summary  "); err != nil {
		t.Fatalf("ApplyCompaction() returned error: %v", err)
	}

	compacted := state.GetMessages()
	if len(compacted) != 1 {
		t.Fatalf("expected compacted history to contain one summary message, got %d", len(compacted))
	}
	if compacted[0].Role != core.RoleUser || compacted[0].Content != "compacted summary" {
		t.Fatalf("unexpected summary message: %#v", compacted[0])
	}
}

func TestAppState_StreamBtwBuildsCorrectMessages(t *testing.T) {
	var capturedMessages []core.Message
	var capturedRegistry *tools.Registry

	client := &mockLLMClient{
		streamChatFunc: func(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry, _ []core.StreamOptions) (<-chan core.StreamEvent, error) {
			capturedMessages = append([]core.Message(nil), messages...)
			capturedRegistry = toolRegistry

			ch := make(chan core.StreamEvent, 2)
			ch <- core.StreamEvent{Type: core.StreamEventTypeChunk, Content: "answer"}
			ch <- core.StreamEvent{Type: core.StreamEventTypeDone}
			close(ch)
			return ch, nil
		},
	}

	state := New(client, t.TempDir())
	state.AddMessage(core.RoleUser, "fix the bug")
	state.AddMessage(core.RoleAssistant, "done")

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
	if capturedMessages[0].Role != core.RoleSystem {
		t.Fatalf("expected first message to be system, got %s", capturedMessages[0].Role)
	}
	if !strings.Contains(capturedMessages[0].Content, "btw") {
		t.Fatalf("expected btw system prompt, got %q", capturedMessages[0].Content)
	}
	if capturedMessages[1].Role != core.RoleUser || capturedMessages[1].Content != "fix the bug" {
		t.Fatalf("expected first history message to be user, got %#v", capturedMessages[1])
	}
	if capturedMessages[2].Role != core.RoleAssistant || capturedMessages[2].Content != "done" {
		t.Fatalf("expected second history message to be assistant, got %#v", capturedMessages[2])
	}
	last := capturedMessages[len(capturedMessages)-1]
	if last.Role != core.RoleUser || last.Content != "what is the bug about?" {
		t.Fatalf("expected user question as last message, got %#v", last)
	}
}

func TestBtwContext(t *testing.T) {
	makeMsg := func(role core.Role, content string) core.Message {
		return core.Message{Role: role, Content: content}
	}

	tests := []struct {
		name     string
		messages []core.Message
		max      int
		wantLen  int
		wantLast core.Role
	}{
		{
			name:     "empty history",
			messages: nil,
			max:      10,
			wantLen:  0,
		},
		{
			name:     "only unanswered user message",
			messages: []core.Message{makeMsg(core.RoleUser, "hi")},
			max:      10,
			wantLen:  0,
		},
		{
			name: "trailing unanswered user message excluded",
			messages: []core.Message{
				makeMsg(core.RoleUser, "q1"),
				makeMsg(core.RoleAssistant, "a1"),
				makeMsg(core.RoleUser, "unanswered"),
			},
			max:      10,
			wantLen:  2,
			wantLast: core.RoleAssistant,
		},
		{
			name: "capped at max",
			messages: func() []core.Message {
				msgs := make([]core.Message, 14)
				for i := range msgs {
					if i%2 == 0 {
						msgs[i] = makeMsg(core.RoleUser, "u")
					} else {
						msgs[i] = makeMsg(core.RoleAssistant, "a")
					}
				}
				return msgs
			}(),
			max:      10,
			wantLen:  10,
			wantLast: core.RoleAssistant,
		},
		{
			name: "fewer messages than max returned as-is",
			messages: []core.Message{
				makeMsg(core.RoleUser, "q"),
				makeMsg(core.RoleAssistant, "a"),
			},
			max:      10,
			wantLen:  2,
			wantLast: core.RoleAssistant,
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
	state.AddMessage(core.RoleUser, "hello")

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
	state.AddMessage(core.RoleUser, "original")

	_, _ = state.StreamBtw(context.Background(), "side question")

	messages := state.GetMessages()
	if len(messages) != 1 || messages[0].Content != "original" {
		t.Fatalf("expected btw not to modify appstate messages, got %#v", messages)
	}
}

func TestAppState_ApplyCompactionLeavesMessagesUntouchedOnError(t *testing.T) {
	state := New(&mockLLMClient{}, t.TempDir())
	state.AddMessage(core.RoleUser, "hello")
	state.AddMessage(core.RoleAssistant, "world")
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
	state.AddMessage(core.RoleUser, "hello")
	original := state.GetMessages()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	eventCh, err := state.StreamCompact(ctx, &config.ResolvedConfig{
		APIKey: "key",
		Model:  "model",
	}, "", core.StreamOptions{})
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

func TestAppState_MessageReplacementFiltersSystemMessagesAndClones(t *testing.T) {
	state := &AppState{}
	messages := []core.Message{
		{Role: core.RoleSystem, Content: "system"},
		{Role: core.RoleUser, Content: "user", TurnMemory: &core.TurnMemory{ToolActivity: []core.HistoricalToolActivity{{Tool: "read_file"}}}},
		{Role: core.RoleAssistant, Content: "assistant"},
	}

	filtered := WithoutSystemMessages(messages)
	if len(filtered) != 2 || filtered[0].Role != core.RoleUser || filtered[1].Role != core.RoleAssistant {
		t.Fatalf("WithoutSystemMessages() = %#v", filtered)
	}
	filtered[0].Content = "changed"
	if messages[1].Content != "user" {
		t.Fatal("WithoutSystemMessages returned aliases to input messages")
	}

	state.ReplaceMessages(filtered)
	filtered[0].Content = "changed again"
	if got := state.GetMessages()[0].Content; got != "changed" {
		t.Fatalf("ReplaceMessages did not clone input: %q", got)
	}
}

func TestAppState_ClientModeAndUsageAccessors(t *testing.T) {
	client := &mockLLMClient{}
	adversary := &mockLLMClient{}
	state := &AppState{llmClient: client, workingDir: "/tmp/project"}

	if state.GetClient() != client || state.WorkingDir() != "/tmp/project" {
		t.Fatal("client or working directory accessor returned unexpected value")
	}
	if state.IsAdversaryClientReady() {
		t.Fatal("adversary client unexpectedly ready")
	}
	state.SetAdversaryClient(adversary)
	if !state.IsAdversaryClientReady() {
		t.Fatal("adversary client was not marked ready")
	}

	if state.Mode() != llm.ModeBuild {
		t.Fatalf("zero mode = %q, want build", state.Mode())
	}
	state.SetMode(llm.AgentMode("invalid"))
	if state.Mode() != llm.ModeBuild {
		t.Fatalf("invalid mode = %q, want build", state.Mode())
	}

	usage := &core.TokenUsage{InputTokens: 42}
	state.SetLastUsage(usage)
	usage.InputTokens = 99
	if got := state.GetLastUsage(); got == nil || got.InputTokens != 42 {
		t.Fatalf("stored usage = %#v", got)
	}
	state.ClearContextMetrics()
	if state.GetLastUsage() != nil {
		t.Fatal("ClearContextMetrics did not clear usage")
	}
	state.SetLastUsage(nil)
}

func TestAppState_SkillConfigAndSuggestionsAreIndependent(t *testing.T) {
	state := &AppState{
		skills: skills.Discovery{Skills: []skills.Skill{
			{Name: "enabled"},
			{Name: "disabled"},
		}},
		skillsConfig: skills.Config{IsEnabled: map[string]bool{"enabled": true, "disabled": false}},
	}

	cfg := state.GetSkillsConfig()
	cfg.IsEnabled["enabled"] = false
	if !state.GetSkillsConfig().Enabled("enabled") {
		t.Fatal("GetSkillsConfig returned mutable state")
	}

	suggestions := state.SkillSuggestions()
	if len(suggestions) != 1 || suggestions[0].Name != "enabled" {
		t.Fatalf("SkillSuggestions() = %#v", suggestions)
	}
}

func TestAppState_SetModeYoloRoundTrips(t *testing.T) {
	state := New(nil, t.TempDir())
	state.SetMode(llm.ModeYolo)
	if state.Mode() != llm.ModeYolo {
		t.Fatalf("yolo mode = %q, want yolo", state.Mode())
	}
	state.SetMode(llm.ModePlan)
	if state.Mode() != llm.ModePlan {
		t.Fatalf("plan mode = %q, want plan", state.Mode())
	}
	state.SetMode(llm.ModeBuild)
	if state.Mode() != llm.ModeBuild {
		t.Fatalf("build mode = %q, want build", state.Mode())
	}
	state.SetMode(llm.AgentMode("invalid"))
	if state.Mode() != llm.ModeBuild {
		t.Fatalf("invalid mode = %q, want build", state.Mode())
	}
}

func TestAppState_EffectiveToolRegistryYoloIncludesWriteEditBash(t *testing.T) {
	state := New(nil, t.TempDir())
	for _, name := range []string{"read_file", "write_file", "edit_file", "bash"} {
		if err := state.RegisterTool(dummyTool{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	state.SetMode(llm.ModeYolo)
	for _, name := range []string{"read_file", "write_file", "edit_file", "bash"} {
		if _, ok := state.EffectiveToolRegistry().Get(name); !ok {
			t.Fatalf("expected %s in yolo registry", name)
		}
	}
	state.SetMode(llm.ModePlan)
	for _, name := range []string{"write_file", "edit_file"} {
		if _, ok := state.EffectiveToolRegistry().Get(name); ok {
			t.Fatalf("expected %s excluded from plan registry", name)
		}
	}
}
