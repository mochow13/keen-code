package repl

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/mochow13/keen-code/internal/llm"
	"github.com/mochow13/keen-code/internal/tools"
)

type mockLLMClient struct {
	streamChatFunc func(ctx context.Context, messages []llm.Message, toolRegistry *tools.Registry) (<-chan llm.StreamEvent, error)
	resetCount     int
}

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

func processCmd(m replModel, cmd tea.Cmd) (replModel, tea.Cmd) {
	if cmd == nil {
		return m, nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				m, _ = processCmd(m, c)
			}
		}
		return m, nil
	}
	return m.updateNormalMode(msg)
}
