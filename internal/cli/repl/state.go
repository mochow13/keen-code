package repl

import (
	"context"

	"github.com/user/keen-cli/internal/config"
	"github.com/user/keen-cli/internal/llm"
	"github.com/user/keen-cli/internal/tools"
)

type AppState struct {
	messages     []llm.Message
	llmClient    llm.LLMClient
	toolRegistry *tools.Registry
}

func NewAppState(client llm.LLMClient) *AppState {
	return &AppState{
		messages:     []llm.Message{},
		llmClient:    client,
		toolRegistry: tools.NewRegistry(),
	}
}

func (s *AppState) AddMessage(role llm.Role, content string) {
	s.messages = append(s.messages, llm.Message{
		Role:    role,
		Content: content,
	})
}

func (s *AppState) GetMessages() []llm.Message {
	result := make([]llm.Message, len(s.messages))
	copy(result, s.messages)
	return result
}

func (s *AppState) ClearMessages() {
	s.messages = []llm.Message{}
}

func (s *AppState) StreamChat(ctx context.Context, cfg *config.ResolvedConfig) (<-chan llm.StreamEvent, error) {
	if s.llmClient == nil {
		return nil, nil
	}
	return s.llmClient.StreamChat(ctx, s.messages, s.toolRegistry)
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

func (s *AppState) GetToolRegistry() *tools.Registry {
	return s.toolRegistry
}

func (s *AppState) RegisterTool(tool tools.Tool) error {
	return s.toolRegistry.Register(tool)
}
