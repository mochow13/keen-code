package cli

import (
	"context"

	"github.com/user/keen-cli/internal/config"
	"github.com/user/keen-cli/internal/llm"
)

type AppState struct {
	messages  []llm.Message
	llmClient llm.LLMClient
}

func NewAppState(client llm.LLMClient) *AppState {
	return &AppState{
		messages:  []llm.Message{},
		llmClient: client,
	}
}

func (s *AppState) AddMessage(role llm.Role, content string) {
	s.messages = append(s.messages, llm.Message{
		Role:    role,
		Content: content,
	})
}

func (s *AppState) GetMessages() []llm.Message {
	return s.messages
}

func (s *AppState) ClearMessages() {
	s.messages = []llm.Message{}
}

func (s *AppState) StreamChat(ctx context.Context, cfg *config.ResolvedConfig) (<-chan llm.StreamEvent, error) {
	if s.llmClient == nil {
		return nil, nil
	}
	return s.llmClient.StreamChat(ctx, s.messages)
}

func (s *AppState) IsClientReady(cfg *config.ResolvedConfig) bool {
	return s.llmClient != nil && cfg.APIKey != "" && cfg.Model != ""
}

func (s *AppState) UpdateClient(client llm.LLMClient) {
	s.llmClient = client
}

func (s *AppState) GetClient() llm.LLMClient {
	return s.llmClient
}
