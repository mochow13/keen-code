package llm

import (
	"context"
	"fmt"
	"iter"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/anthropic"
	"github.com/firebase/genkit/go/plugins/compat_oai"
	"github.com/firebase/genkit/go/plugins/googlegenai"
)

type streamFunc func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error]

type GenkitClient struct {
	g          *genkit.Genkit
	provider   Provider
	model      string
	streamImpl streamFunc
}

func NewGenkitClient(cfg *ClientConfig) (*GenkitClient, error) {
	ctx := context.Background()

	var g *genkit.Genkit
	var modelName string

	switch cfg.Provider {
	case ProviderAnthropic:
		g = genkit.Init(ctx, genkit.WithPlugins(&anthropic.Anthropic{
			APIKey: cfg.APIKey,
		}))
		modelName = "anthropic/" + cfg.Model
	case ProviderOpenAI:
		g = genkit.Init(ctx, genkit.WithPlugins(&compat_oai.OpenAICompatible{
			APIKey:   cfg.APIKey,
			Provider: "openai",
		}))
		modelName = "openai/" + cfg.Model
	case ProviderGemini:
		g = genkit.Init(ctx, genkit.WithPlugins(&googlegenai.GoogleAI{
			APIKey: cfg.APIKey,
		}))
		modelName = "googleai/" + cfg.Model
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}

	if g == nil {
		return nil, fmt.Errorf("failed to initialize genkit")
	}

	return &GenkitClient{
		g:          g,
		provider:   cfg.Provider,
		model:      modelName,
		streamImpl: genkit.GenerateStream,
	}, nil
}

func (c *GenkitClient) StreamChat(ctx context.Context, messages []Message) (<-chan StreamEvent, error) {
	aiMessages := make([]*ai.Message, len(messages))
	for i, m := range messages {
		aiMessages[i] = &ai.Message{
			Role: ai.Role(m.Role),
			Content: []*ai.Part{
				ai.NewTextPart(m.Content),
			},
		}
	}

	eventCh := make(chan StreamEvent)

	go func() {
		defer close(eventCh)

		stream := c.streamImpl(ctx, c.g,
			ai.WithModelName(c.model),
			ai.WithMessages(aiMessages...),
		)

		for result, err := range stream {
			if err != nil {
				eventCh <- StreamEvent{
					Type:  StreamEventTypeError,
					Error: err,
				}
				return
			}

			if result.Done {
				eventCh <- StreamEvent{
					Type: StreamEventTypeDone,
				}
				return
			}

			if result.Chunk != nil && len(result.Chunk.Content) > 0 {
				text := result.Chunk.Text()
				if text != "" {
					eventCh <- StreamEvent{
						Type:    StreamEventTypeChunk,
						Content: text,
					}
				}
			}
		}
	}()

	return eventCh, nil
}
