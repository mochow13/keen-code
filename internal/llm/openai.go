package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/respjson"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/tools"
)

type chatStream interface {
	Next() bool
	Current() openai.ChatCompletionChunk
	Err() error
	Close() error
}

type streamFactory func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream

type sdkChatStream struct {
	stream interface {
		Next() bool
		Current() openai.ChatCompletionChunk
		Err() error
		Close() error
	}
}

func (s *sdkChatStream) Next() bool {
	return s.stream.Next()
}

func (s *sdkChatStream) Current() openai.ChatCompletionChunk {
	return s.stream.Current()
}

func (s *sdkChatStream) Err() error {
	return s.stream.Err()
}

func (s *sdkChatStream) Close() error {
	return s.stream.Close()
}

type OpenAICompatibleClient struct {
	provider   Provider
	model      string
	client     openai.Client
	streamImpl streamFactory
}

const deepSeekReasonerModel = "deepseek-reasoner"

func NewOpenAICompatibleClient(cfg *ClientConfig) (*OpenAICompatibleClient, error) {
	baseURL, err := openAICompatibleBaseURL(cfg.Provider)
	if err != nil {
		return nil, err
	}

	client := openai.NewClient(
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(baseURL),
	)

	c := &OpenAICompatibleClient{
		provider: cfg.Provider,
		model:    cfg.Model,
		client:   client,
	}
	c.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		return &sdkChatStream{stream: c.client.Chat.Completions.NewStreaming(ctx, params, opts...)}
	}

	return c, nil
}

func openAICompatibleBaseURL(provider Provider) (string, error) {
	switch provider {
	case Provider(config.ProviderDeepSeek):
		return "https://api.deepseek.com/", nil
	case Provider(config.ProviderMoonshotAI):
		return "https://api.moonshot.ai/v1/", nil
	default:
		return "", fmt.Errorf("unsupported OpenAI-compatible provider: %s", provider)
	}
}

func toOpenAIMessages(messages []Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case RoleSystem:
			result = append(result, openai.SystemMessage(m.Content))
		case RoleUser:
			result = append(result, openai.UserMessage(m.Content))
		case RoleAssistant:
			am := openai.ChatCompletionAssistantMessageParam{}
			am.Content.OfString = openai.String(m.Content)
			result = append(result, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &am,
			})
		}
	}
	return result
}

func toOpenAITools(registry *tools.Registry) []openai.ChatCompletionToolParam {
	if registry == nil {
		return nil
	}

	all := registry.All()
	result := make([]openai.ChatCompletionToolParam, 0, len(all))
	for _, t := range all {
		result = append(result, openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        t.Name(),
				Description: openai.String(t.Description()),
				Parameters:  openai.FunctionParameters(t.InputSchema()),
				Strict:      openai.Bool(false),
			},
		})
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func extractJSONStringField(extra map[string]respjson.Field, key string) string {
	if len(extra) == 0 {
		return ""
	}
	field, ok := extra[key]
	if !ok {
		return ""
	}
	raw := field.Raw()
	if raw == "" || raw == respjson.Null {
		return ""
	}

	var value string
	if err := json.Unmarshal([]byte(raw), &value); err == nil {
		return value
	}
	return ""
}

func (c *OpenAICompatibleClient) isReasonerModel() bool {
	return c.model == deepSeekReasonerModel
}

func emitChunk(eventCh chan<- StreamEvent, content string) {
	if content == "" {
		return
	}
	eventCh <- StreamEvent{
		Type:    StreamEventTypeChunk,
		Content: content,
	}
}

func (c *OpenAICompatibleClient) buildAssistantMessage(message openai.ChatCompletionMessage, reasoningContent string) openai.ChatCompletionAssistantMessageParam {
	assistant := openai.ChatCompletionAssistantMessageParam{}
	if message.Content != "" {
		assistant.Content.OfString = openai.String(message.Content)
	}
	if len(message.ToolCalls) > 0 {
		assistant.ToolCalls = make([]openai.ChatCompletionMessageToolCallParam, len(message.ToolCalls))
		for i, tc := range message.ToolCalls {
			assistant.ToolCalls[i] = openai.ChatCompletionMessageToolCallParam{
				ID: tc.ID,
				Function: openai.ChatCompletionMessageToolCallFunctionParam{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}
	if len(message.ToolCalls) > 0 {
		assistant.SetExtraFields(map[string]any{
			"reasoning_content": reasoningContent,
		})
	}
	return assistant
}

func (c *OpenAICompatibleClient) emitMissingFinalContent(
	eventCh chan<- StreamEvent,
	fullContent string,
	streamedContent string,
) {
	if fullContent == "" {
		return
	}

	// We stream delta content live as it arrives. At stream end, the accumulator
	// also exposes the full final content. Emit only the missing tail to avoid
	// duplicate UI text while still handling providers that send little/no deltas.
	if strings.HasPrefix(fullContent, streamedContent) {
		if tail := fullContent[len(streamedContent):]; tail != "" {
			emitChunk(eventCh, tail)
		}
		return
	}

	if streamedContent == "" {
		emitChunk(eventCh, fullContent)
	}
}

func (c *OpenAICompatibleClient) collectTurn(
	ctx context.Context,
	params openai.ChatCompletionNewParams,
	eventCh chan<- StreamEvent,
) (openai.ChatCompletionMessage, string, string, bool, error) {
	stream := c.streamImpl(ctx, params)
	var acc openai.ChatCompletionAccumulator
	var reasoningContent strings.Builder
	var streamedContent strings.Builder

	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			streamedContent.WriteString(delta.Content)
			emitChunk(eventCh, delta.Content)
		}

		// reasoning_content is a DeepSeek extension not modeled by openai-go.
		// Capture it during streaming because the SDK accumulator does not retain JSON metadata.
		reasoningDelta := extractJSONStringField(delta.JSON.ExtraFields, "reasoning_content")
		reasoningContent.WriteString(reasoningDelta)
		if reasoningDelta != "" {
			eventCh <- StreamEvent{
				Type:    StreamEventTypeReasoningChunk,
				Content: reasoningDelta,
			}
		}
	}
	_ = stream.Close()

	if err := stream.Err(); err != nil {
		return openai.ChatCompletionMessage{}, "", "", false, fmt.Errorf("stream error: %w", err)
	}
	if len(acc.ChatCompletion.Choices) == 0 {
		return openai.ChatCompletionMessage{}, "", "", false, nil
	}

	return acc.ChatCompletion.Choices[0].Message, reasoningContent.String(), streamedContent.String(), true, nil
}

func (c *OpenAICompatibleClient) StreamChat(
	ctx context.Context,
	messages []Message,
	toolRegistry *tools.Registry,
) (<-chan StreamEvent, error) {
	eventCh := make(chan StreamEvent)

	go func() {
		defer close(eventCh)

		oaiMessages := toOpenAIMessages(messages)
		oaiTools := toOpenAITools(toolRegistry)

		for range maxToolTurns {
			params := openai.ChatCompletionNewParams{
				Model:    c.model,
				Messages: oaiMessages,
			}
			if len(oaiTools) > 0 {
				params.Tools = oaiTools
			}
			message, reasoningContent, streamedContent, hasChoice, err := c.collectTurn(ctx, params, eventCh)
			if err != nil {
				eventCh <- StreamEvent{
					Type:  StreamEventTypeError,
					Error: err,
				}
				return
			}
			if !hasChoice {
				eventCh <- StreamEvent{Type: StreamEventTypeDone}
				return
			}
			c.emitMissingFinalContent(eventCh, message.Content, streamedContent)
			assistant := c.buildAssistantMessage(message, reasoningContent)

			if len(message.ToolCalls) == 0 {
				eventCh <- StreamEvent{Type: StreamEventTypeDone}
				return
			}

			oaiMessages = append(oaiMessages, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &assistant,
			})

			toolMsgs := c.executeTools(ctx, message.ToolCalls, toolRegistry, eventCh)
			if len(toolMsgs) > 0 {
				oaiMessages = append(oaiMessages, toolMsgs...)
			}
		}

		eventCh <- StreamEvent{Type: StreamEventTypeDone}
	}()

	return eventCh, nil
}

func (c *OpenAICompatibleClient) executeTools(
	ctx context.Context,
	toolCalls []openai.ChatCompletionMessageToolCall,
	registry *tools.Registry,
	eventCh chan<- StreamEvent,
) []openai.ChatCompletionMessageParamUnion {
	toolMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(toolCalls))

	for _, tc := range toolCalls {
		start := time.Now()
		input := map[string]any{}
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
				input = map[string]any{}
			}
		}
		slog.Debug("Tool request", "tool", tc.Function.Name, "input", input)
		eventCh <- StreamEvent{
			Type: StreamEventTypeToolStart,
			ToolCall: &ToolCall{
				Name:  tc.Function.Name,
				Input: input,
			},
		}

		var output any
		var execErr error

		if registry == nil {
			execErr = fmt.Errorf("tool registry not available")
		} else if tool, exists := registry.Get(tc.Function.Name); !exists {
			execErr = fmt.Errorf("tool %q not found", tc.Function.Name)
		} else {
			output, execErr = tool.Execute(ctx, input)
		}

		duration := time.Since(start)
		toolCall := &ToolCall{
			Name:     tc.Function.Name,
			Input:    input,
			Output:   output,
			Duration: duration,
		}

		var toolOutput string
		if execErr != nil {
			toolCall.Error = execErr.Error()
			slog.Debug("Tool response", "tool", tc.Function.Name, "error", execErr.Error(), "duration", duration)
			eventCh <- StreamEvent{
				Type:     StreamEventTypeToolEnd,
				ToolCall: toolCall,
			}
			toolOutput = fmt.Sprintf(`{"error":%q}`, execErr.Error())
		} else {
			slog.Debug("Tool response", "tool", tc.Function.Name, "duration", duration)
			eventCh <- StreamEvent{
				Type:     StreamEventTypeToolEnd,
				ToolCall: toolCall,
			}
			if output == nil {
				output = map[string]any{}
			}
			b, err := json.Marshal(output)
			if err != nil {
				toolOutput = "{}"
			} else {
				toolOutput = string(b)
			}
		}

		toolMessages = append(toolMessages, openai.ToolMessage(toolOutput, tc.ID))
	}

	return toolMessages
}
