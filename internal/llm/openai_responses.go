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
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/openai/openai-go/responses"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/tools"
)

type responseStream interface {
	Next() bool
	Current() responses.ResponseStreamEventUnion
	Err() error
	Close() error
}

type responseStreamFactory func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream

type sdkResponseStream struct {
	stream *ssestream.Stream[responses.ResponseStreamEventUnion]
}

func (s *sdkResponseStream) Next() bool {
	return s.stream.Next()
}

func (s *sdkResponseStream) Current() responses.ResponseStreamEventUnion {
	return s.stream.Current()
}

func (s *sdkResponseStream) Err() error {
	return s.stream.Err()
}

func (s *sdkResponseStream) Close() error {
	return s.stream.Close()
}

type OpenAIResponsesClient struct {
	provider           Provider
	model              string
	client             openai.Client
	responseStreamImpl responseStreamFactory
}

func NewOpenAIResponsesClient(cfg *ClientConfig) (*OpenAIResponsesClient, error) {
	if cfg.Provider != Provider(config.ProviderOpenAI) {
		return nil, fmt.Errorf("unsupported Responses API provider: %s", cfg.Provider)
	}

	client := openai.NewClient(
		option.WithAPIKey(cfg.APIKey),
	)

	c := &OpenAIResponsesClient{
		provider: cfg.Provider,
		model:    cfg.Model,
		client:   client,
	}
	c.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &sdkResponseStream{stream: c.client.Responses.NewStreaming(ctx, params, opts...)}
	}

	return c, nil
}

func toOpenAIResponseInput(messages []Message) []responses.ResponseInputItemUnionParam {
	result := make([]responses.ResponseInputItemUnionParam, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case RoleSystem:
			result = append(result, responses.ResponseInputItemParamOfMessage(m.Content, responses.EasyInputMessageRoleSystem))
		case RoleUser:
			result = append(result, responses.ResponseInputItemParamOfMessage(m.Content, responses.EasyInputMessageRoleUser))
		case RoleAssistant:
			result = append(result, responses.ResponseInputItemParamOfMessage(m.Content, responses.EasyInputMessageRoleAssistant))
		}
	}
	return result
}

func toOpenAIResponseTools(registry *tools.Registry) []responses.ToolUnionParam {
	if registry == nil {
		return nil
	}

	all := registry.All()
	result := make([]responses.ToolUnionParam, 0, len(all))
	for _, t := range all {
		result = append(result, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        t.Name(),
				Description: param.NewOpt(t.Description()),
				Parameters:  t.InputSchema(),
				Strict:      param.NewOpt(false),
			},
		})
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func (c *OpenAIResponsesClient) StreamChat(
	ctx context.Context,
	messages []Message,
	toolRegistry *tools.Registry,
) (<-chan StreamEvent, error) {
	eventCh := make(chan StreamEvent)

	go func() {
		defer close(eventCh)

		input := toOpenAIResponseInput(messages)
		responseTools := toOpenAIResponseTools(toolRegistry)
		previousResponseID := ""

		for range maxToolTurns {
			params := responses.ResponseNewParams{
				Model: c.model,
				Input: responses.ResponseNewParamsInputUnion{
					OfInputItemList: input,
				},
			}
			if len(responseTools) > 0 {
				params.Tools = responseTools
			}
			if previousResponseID != "" {
				params.PreviousResponseID = param.NewOpt(previousResponseID)
			}

			completed, streamedContent, toolCalls, err := c.collectTurn(ctx, params, eventCh)
			if err != nil {
				eventCh <- StreamEvent{
					Type:  StreamEventTypeError,
					Error: err,
				}
				return
			}
			if completed == nil {
				eventCh <- StreamEvent{Type: StreamEventTypeDone}
				return
			}

			previousResponseID = completed.ID
			emitMissingFinalContent(eventCh, completed.OutputText(), streamedContent)

			if len(toolCalls) == 0 {
				eventCh <- StreamEvent{Type: StreamEventTypeDone}
				return
			}

			input = c.executeTools(ctx, toolCalls, toolRegistry, eventCh)
		}

		eventCh <- StreamEvent{Type: StreamEventTypeDone}
	}()

	return eventCh, nil
}

func (c *OpenAIResponsesClient) collectTurn(
	ctx context.Context,
	params responses.ResponseNewParams,
	eventCh chan<- StreamEvent,
) (*responses.Response, string, []responses.ResponseFunctionToolCall, error) {
	stream := c.responseStreamImpl(ctx, params)
	var completed *responses.Response
	var streamedContent strings.Builder

	for stream.Next() {
		ev := stream.Current()

		switch ev.Type {
		case "response.output_text.delta":
			if ev.Delta.OfString != "" {
				streamedContent.WriteString(ev.Delta.OfString)
				emitChunk(eventCh, ev.Delta.OfString)
			}
		case "response.reasoning.delta", "response.reasoning_summary.delta", "response.reasoning_summary_text.delta":
			reasoning := ev.Delta.OfString
			if reasoning == "" {
				reasoning = ev.Text
			}
			if reasoning != "" {
				eventCh <- StreamEvent{
					Type:    StreamEventTypeReasoningChunk,
					Content: reasoning,
				}
			}
		case "error":
			msg := strings.TrimSpace(ev.Message)
			if msg == "" {
				msg = "responses stream error"
			}
			if ev.Code != "" {
				msg = msg + " (" + ev.Code + ")"
			}
			return nil, streamedContent.String(), nil, fmt.Errorf("%s", msg)
		case "response.completed":
			v := ev.AsResponseCompleted()
			completed = &v.Response
		}
	}
	_ = stream.Close()

	if err := stream.Err(); err != nil {
		return nil, streamedContent.String(), nil, fmt.Errorf("stream error: %w", err)
	}
	if completed == nil {
		return nil, streamedContent.String(), nil, nil
	}

	toolCalls := make([]responses.ResponseFunctionToolCall, 0)
	for _, item := range completed.Output {
		if item.Type != "function_call" {
			continue
		}
		toolCalls = append(toolCalls, item.AsFunctionCall())
	}

	return completed, streamedContent.String(), toolCalls, nil
}

func (c *OpenAIResponsesClient) executeTools(
	ctx context.Context,
	toolCalls []responses.ResponseFunctionToolCall,
	registry *tools.Registry,
	eventCh chan<- StreamEvent,
) []responses.ResponseInputItemUnionParam {
	toolMessages := make([]responses.ResponseInputItemUnionParam, 0, len(toolCalls))

	for _, tc := range toolCalls {
		start := time.Now()
		input := map[string]any{}
		if tc.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil {
				input = map[string]any{}
			}
		}
		slog.Debug("Tool request", "tool", tc.Name, "input", input)
		eventCh <- StreamEvent{
			Type: StreamEventTypeToolStart,
			ToolCall: &ToolCall{
				Name:  tc.Name,
				Input: input,
			},
		}

		var output any
		var execErr error

		if registry == nil {
			execErr = fmt.Errorf("tool registry not available")
		} else if tool, exists := registry.Get(tc.Name); !exists {
			execErr = fmt.Errorf("tool %q not found", tc.Name)
		} else {
			output, execErr = tool.Execute(ctx, input)
		}

		duration := time.Since(start)
		toolCall := &ToolCall{
			Name:     tc.Name,
			Input:    input,
			Output:   output,
			Duration: duration,
		}

		var toolOutput string
		if execErr != nil {
			toolCall.Error = execErr.Error()
			slog.Debug("Tool response", "tool", tc.Name, "error", execErr.Error(), "duration", duration)
			eventCh <- StreamEvent{
				Type:     StreamEventTypeToolEnd,
				ToolCall: toolCall,
			}
			toolOutput = fmt.Sprintf(`{"error":%q}`, execErr.Error())
		} else {
			slog.Debug("Tool response", "tool", tc.Name, "duration", duration)
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

		toolMessages = append(toolMessages, responses.ResponseInputItemParamOfFunctionCallOutput(tc.CallID, toolOutput))
	}

	return toolMessages
}
