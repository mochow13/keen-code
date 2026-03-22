package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"time"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/anthropic"
	"github.com/firebase/genkit/go/plugins/compat_oai"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"github.com/joho/godotenv"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/tools"
)

const maxToolTurns = 2000

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
	case config.ProviderAnthropic:
		env, _ := godotenv.Read(".env")
		baseURL := env["ANTHROPIC_BASE_URL"]

		if baseURL == "" {
			g = genkit.Init(ctx, genkit.WithPlugins(&anthropic.Anthropic{
				APIKey: cfg.APIKey,
			}))
		} else {
			g = genkit.Init(ctx, genkit.WithPlugins(&anthropic.Anthropic{
				APIKey:  cfg.APIKey,
				BaseURL: baseURL,
			}))
		}
		modelName = "anthropic/" + cfg.Model
	case config.ProviderOpenAI:
		g = genkit.Init(ctx, genkit.WithPlugins(&compat_oai.OpenAICompatible{
			APIKey:   cfg.APIKey,
			Provider: "openai",
		}))
		modelName = "openai/" + cfg.Model
	case config.ProviderGoogleAI:
		g = genkit.Init(ctx, genkit.WithPlugins(&googlegenai.GoogleAI{
			APIKey: cfg.APIKey,
		}))
		modelName = "googleai/" + cfg.Model
	default:
		return nil, fmt.Errorf("unsupported provider in config: %s", cfg.Provider)
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

func toGenkitRole(role Role) ai.Role {
	switch role {
	case RoleUser:
		return ai.RoleUser
	case RoleAssistant:
		return ai.RoleModel
	case RoleSystem:
		return ai.RoleSystem
	default:
		return ai.Role(role)
	}
}

func toGenkitMessages(messages []Message) []*ai.Message {
	aiMessages := make([]*ai.Message, len(messages))
	for i, m := range messages {
		aiMessages[i] = &ai.Message{
			Role: toGenkitRole(m.Role),
			Content: []*ai.Part{
				ai.NewTextPart(m.Content),
			},
		}
	}
	return aiMessages
}

func (c *GenkitClient) StreamChat(
	ctx context.Context,
	messages []Message,
	toolRegistry *tools.Registry,
) (<-chan StreamEvent, error) {
	eventCh := make(chan StreamEvent)

	go func() {
		defer close(eventCh)

		aiMessages := toGenkitMessages(messages)

		var genkitTools []ai.ToolRef
		if toolRegistry != nil && toolRegistry.Count() > 0 {
			genkitTools = ToGenkitTools(toolRegistry)
		}

		for range maxToolTurns {
			opts := []ai.GenerateOption{
				ai.WithModelName(c.model),
				ai.WithMessages(aiMessages...),
			}

			if c.provider == config.ProviderAnthropic {
				opts = append(opts, ai.WithConfig(&anthropicsdk.MessageNewParams{
					MaxTokens: 16192,
				}))
			}

			if len(genkitTools) > 0 {
				opts = append(opts, ai.WithTools(genkitTools...))
				opts = append(opts, ai.WithReturnToolRequests(true))
			}

			stream := c.streamImpl(ctx, c.g, opts...)
			var modelResponse *ai.ModelResponse

			for result, err := range stream {
				if err != nil {
					eventCh <- StreamEvent{
						Type:  StreamEventTypeError,
						Error: err,
					}
					return
				}

				if result.Done {
					modelResponse = result.Response
					break
				}

				if result.Chunk != nil && len(result.Chunk.Content) > 0 {
					for _, part := range result.Chunk.Content {
						if part.IsReasoning() && part.Text != "" {
							eventCh <- StreamEvent{
								Type:    StreamEventTypeReasoningChunk,
								Content: part.Text,
							}
						} else if (part.IsText() || part.IsData()) && part.Text != "" {
							eventCh <- StreamEvent{
								Type:    StreamEventTypeChunk,
								Content: part.Text,
							}
						}
					}
				}
			}

			if modelResponse == nil || modelResponse.Message == nil {
				eventCh <- StreamEvent{Type: StreamEventTypeDone}
				return
			}

			toolRequests := modelResponse.ToolRequests()
			if len(toolRequests) == 0 {
				eventCh <- StreamEvent{Type: StreamEventTypeDone}
				return
			}

			aiMessages = append(aiMessages, modelResponse.Message)

			toolResponseParts := c.executeTools(ctx, toolRequests, toolRegistry, eventCh)
			if len(toolResponseParts) > 0 {
				toolMsg := &ai.Message{
					Role:    ai.RoleTool,
					Content: toolResponseParts,
				}
				aiMessages = append(aiMessages, toolMsg)
			}
		}

		eventCh <- StreamEvent{Type: StreamEventTypeDone}
	}()

	return eventCh, nil
}

func (c *GenkitClient) executeTools(
	ctx context.Context,
	toolRequests []*ai.ToolRequest,
	registry *tools.Registry,
	eventCh chan<- StreamEvent,
) []*ai.Part {
	var toolResponseParts []*ai.Part

	for _, req := range toolRequests {
		start := time.Now()

		input, _ := req.Input.(map[string]any)
		if input == nil {
			if raw, ok := req.Input.(json.RawMessage); ok {
				if err := json.Unmarshal(raw, &input); err != nil {
					input = nil
				}
			}
		}
		slog.Debug("Tool request", "tool", req.Name, "input", input)
		eventCh <- StreamEvent{
			Type: StreamEventTypeToolStart,
			ToolCall: &ToolCall{
				Name:  req.Name,
				Input: input,
			},
		}

		var output any
		var execErr error

		if registry == nil {
			execErr = fmt.Errorf("tool registry not available")
		} else if tool, exists := registry.Get(req.Name); !exists {
			execErr = fmt.Errorf("tool %q not found", req.Name)
		} else {
			output, execErr = tool.Execute(ctx, input)
		}

		duration := time.Since(start)

		toolCall := &ToolCall{
			Name:     req.Name,
			Input:    input,
			Output:   output,
			Duration: duration,
		}

		if execErr != nil {
			toolCall.Error = execErr.Error()
			slog.Debug("Tool response", "tool", req.Name, "error", execErr.Error(), "duration", duration)
			eventCh <- StreamEvent{
				Type:     StreamEventTypeToolEnd,
				ToolCall: toolCall,
			}
			toolResponseParts = append(toolResponseParts, ai.NewToolResponsePart(&ai.ToolResponse{
				Name:   req.Name,
				Ref:    req.Ref,
				Output: map[string]any{"error": execErr.Error()},
			}))
		} else {
			slog.Debug("Tool response", "tool", req.Name, "duration", duration)
			eventCh <- StreamEvent{
				Type:     StreamEventTypeToolEnd,
				ToolCall: toolCall,
			}
			if output == nil {
				output = map[string]any{}
			}
			toolResponseParts = append(toolResponseParts, ai.NewToolResponsePart(&ai.ToolResponse{
				Name:   req.Name,
				Ref:    req.Ref,
				Output: output,
			}))
		}
	}

	return toolResponseParts
}
