package repl

import (
	"context"
	"errors"
	"testing"

	"github.com/user/keen-cli/internal/config"
	"github.com/user/keen-cli/internal/llm"
)

type mockLLMClient struct {
	streamChatFunc func(ctx context.Context, messages []llm.Message) (<-chan llm.StreamEvent, error)
}

func (m *mockLLMClient) StreamChat(ctx context.Context, messages []llm.Message) (<-chan llm.StreamEvent, error) {
	if m.streamChatFunc != nil {
		return m.streamChatFunc(ctx, messages)
	}
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch, nil
}

func TestNewAppState(t *testing.T) {
	client := &mockLLMClient{}
	state := NewAppState(client)

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

func TestNewAppState_NilClient(t *testing.T) {
	state := NewAppState(nil)

	if state == nil {
		t.Fatal("expected non-nil AppState")
	}
	if state.llmClient != nil {
		t.Error("expected nil llmClient")
	}
}

func TestAppState_AddMessage(t *testing.T) {
	state := NewAppState(nil)

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
	state := NewAppState(nil)

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
	state := NewAppState(nil)
	state.AddMessage(llm.RoleUser, "Original")

	messages := state.GetMessages()
	messages[0].Content = "Modified"

	original := state.GetMessages()
	if original[0].Content != "Original" {
		t.Error("GetMessages should return a copy, but original was modified")
	}
}

func TestAppState_ClearMessages(t *testing.T) {
	state := NewAppState(nil)

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

func TestAppState_ClearMessages_EmptyState(t *testing.T) {
	state := NewAppState(nil)

	state.ClearMessages()
	if len(state.messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(state.messages))
	}
}

func TestAppState_StreamChat_WithClient(t *testing.T) {
	expectedEvents := []llm.StreamEvent{
		{Type: llm.StreamEventTypeChunk, Content: "Hello"},
		{Type: llm.StreamEventTypeDone},
	}

	client := &mockLLMClient{
		streamChatFunc: func(ctx context.Context, messages []llm.Message) (<-chan llm.StreamEvent, error) {
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

	state := NewAppState(client)
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
}

func TestAppState_StreamChat_NilClient(t *testing.T) {
	state := NewAppState(nil)
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

func TestAppState_StreamChat_ClientError(t *testing.T) {
	expectedErr := errors.New("stream error")
	client := &mockLLMClient{
		streamChatFunc: func(ctx context.Context, messages []llm.Message) (<-chan llm.StreamEvent, error) {
			return nil, expectedErr
		},
	}

	state := NewAppState(client)
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
			state := NewAppState(tt.client)
			got := state.IsClientReady(tt.cfg)
			if got != tt.expected {
				t.Errorf("IsClientReady() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestAppState_UpdateClient(t *testing.T) {
	oldClient := &mockLLMClient{}
	state := NewAppState(oldClient)

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
	state := NewAppState(client)

	state.UpdateClient(nil)

	if state.llmClient != nil {
		t.Error("expected client to be nil after update")
	}
}

func TestAppState_GetClient(t *testing.T) {
	client := &mockLLMClient{}
	state := NewAppState(client)

	got := state.GetClient()
	if got != client {
		t.Error("GetClient() returned unexpected client")
	}
}

func TestAppState_GetClient_Nil(t *testing.T) {
	state := NewAppState(nil)

	got := state.GetClient()
	if got != nil {
		t.Error("GetClient() expected nil, got non-nil")
	}
}
