package repl

import (
	"context"

	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/llm"
	"github.com/user/keen-code/internal/tools"
)

type AppState struct {
	messages     []llm.Message
	llmClient    llm.LLMClient
	toolRegistry *tools.Registry
	workingDir   string
}

func NewAppState(client llm.LLMClient, workingDir string) *AppState {
	return &AppState{
		messages:     []llm.Message{},
		llmClient:    client,
		toolRegistry: tools.NewRegistry(),
		workingDir:   workingDir,
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
	systemMsg := llm.Message{
		Role:    llm.RoleSystem,
		Content: llm.Build(s.workingDir),
	}
	messages := append([]llm.Message{systemMsg}, s.GetMessages()...)
	return s.llmClient.StreamChat(ctx, messages, s.toolRegistry)
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
