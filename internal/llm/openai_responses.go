package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm/compaction"
	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/llm/history"
	"github.com/mochow13/keen-code/internal/llm/providerconfig"
	"github.com/mochow13/keen-code/internal/llm/retry"
	"github.com/mochow13/keen-code/internal/tools"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
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
	provider                providerconfig.Provider
	model                   string
	thinkingEffort          string
	maxRetries              int
	client                  openai.Client
	responseStreamImpl      responseStreamFactory
	pendingState            []responses.ResponseInputItemUnionParam
	contextWindowTokenCount int
	headers                 map[string]string
}

func NewOpenAIResponsesClient(cfg *providerconfig.ClientConfig) (*OpenAIResponsesClient, error) {
	if cfg.Provider != providerconfig.Provider(config.ProviderOpenAI) && cfg.Provider != providerconfig.Provider(config.ProviderOpenCodeGo) {
		return nil, fmt.Errorf("unsupported Responses API provider: %s. %s", cfg.Provider, config.ConfigFixHint)
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	client := openai.NewClient(opts...)

	c := &OpenAIResponsesClient{
		provider:                cfg.Provider,
		model:                   cfg.Model,
		thinkingEffort:          cfg.ThinkingEffort,
		maxRetries:              retry.Count(cfg.MaxRetries),
		client:                  client,
		contextWindowTokenCount: cfg.ContextWindowTokens,
		headers:                 cfg.Headers,
	}
	c.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &sdkResponseStream{stream: c.client.Responses.NewStreaming(ctx, params, opts...)}
	}

	return c, nil
}

func toOpenAIResponseInput(messages []core.Message) []responses.ResponseInputItemUnionParam {
	result := make([]responses.ResponseInputItemUnionParam, 0, len(messages))
	for messageIndex, m := range messages {
		content := history.FormatMessage(m)
		switch m.Role {
		case core.RoleSystem:
			result = append(result, responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleSystem))
		case core.RoleUser:
			result = append(result, responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser))
		case core.RoleAssistant:
			for _, step := range history.MessageSteps(messageIndex, m) {
				if step.Text != "" {
					result = append(result, responses.ResponseInputItemParamOfMessage(step.Text, responses.EasyInputMessageRoleAssistant))
				}
				for _, invocation := range step.Activities {
					result = append(result, responses.ResponseInputItemParamOfFunctionCall(history.ToolArguments(invocation.Activity), invocation.ID, invocation.Activity.Tool))
				}
				for _, invocation := range step.Activities {
					result = append(result, responses.ResponseInputItemParamOfFunctionCallOutput(invocation.ID, history.ToolResult(invocation.Activity)))
				}
			}
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

func (c *OpenAIResponsesClient) compactHistory(
	ctx context.Context,
	compactionHistory *[]core.Message,
	input *[]responses.ResponseInputItemUnionParam,
	replayedPendingInput *[]responses.ResponseInputItemUnionParam,
	turnStartLen *int,
	streamOpts core.StreamOptions,
	toolRegistry *tools.Registry,
	eventCh chan<- core.StreamEvent,
) error {
	if streamOpts.OneShot || streamOpts.DisableAutoCompaction || len(*replayedPendingInput) > 0 {
		return fmt.Errorf("automatic compaction unavailable")
	}

	childCtx, cancel := context.WithCancel(ctx)
	sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeAutoCompactionStarted, AutoCompaction: &core.AutoCompactionEvent{Cancel: cancel}})
	replacement, usage, err := AutoCompact(childCtx, c, *compactionHistory, toolRegistry, streamOpts.SessionID)
	cancel()
	if err != nil {
		eventType := core.StreamEventTypeAutoCompactionFailed
		if compaction.IsCancellation(err) {
			eventType = core.StreamEventTypeAutoCompactionCancelled
		}
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: eventType, AutoCompaction: &core.AutoCompactionEvent{Usage: usage, Error: err}})
		return err
	}

	*compactionHistory = replacement
	*input = toOpenAIResponseInput(replacement)
	*turnStartLen = len(*input)
	*replayedPendingInput = nil
	c.pendingState = nil
	sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeAutoCompactionApplied, AutoCompaction: &core.AutoCompactionEvent{Replacement: replacement, Usage: usage}})
	return nil
}

func (c *OpenAIResponsesClient) proactivelyCompactHistory(
	ctx context.Context,
	compactionHistory *[]core.Message,
	input *[]responses.ResponseInputItemUnionParam,
	replayedPendingInput *[]responses.ResponseInputItemUnionParam,
	turnStartLen *int,
	streamOpts core.StreamOptions,
	toolRegistry *tools.Registry,
	hasNewToolTurns bool,
	autoCompactOff bool,
	eventCh chan<- core.StreamEvent,
) error {
	if streamOpts.DisableAutoCompaction || streamOpts.OneShot || !hasNewToolTurns || autoCompactOff ||
		!core.ShouldAutoCompact(estimateResponsesInput(*input), core.ContextInputBudget(c.contextWindowTokenCount)) {
		return nil
	}

	return c.compactHistory(ctx, compactionHistory, input, replayedPendingInput, turnStartLen, streamOpts, toolRegistry, eventCh)
}

func estimateResponsesInput(input []responses.ResponseInputItemUnionParam) int {
	tokens := 0
	for _, item := range input {
		b, err := json.Marshal(item)
		if err != nil {
			continue
		}
		tokens += core.EstimateContextTokenCount(string(b))
	}
	return tokens
}

func (c *OpenAIResponsesClient) StreamChat(
	ctx context.Context,
	messages []core.Message,
	toolRegistry *tools.Registry,
	opts ...core.StreamOptions,
) (<-chan core.StreamEvent, error) {
	eventCh := make(chan core.StreamEvent)
	streamOpts := streamOptions(opts)

	go func() {
		defer close(eventCh)

		input := toOpenAIResponseInput(messages)
		compactionHistory := core.CloneMessages(messages)
		oneShot := streamOpts.OneShot
		sessionID := streamOpts.SessionID
		autoCompactOff := false
		hasNewToolTurns := false
		var replayedPendingInput []responses.ResponseInputItemUnionParam
		if !oneShot {
			input, replayedPendingInput = c.injectPendingState(input)
		}
		turnStartLen := len(input)
		responseTools := toOpenAIResponseTools(toolRegistry)

		for range maxToolTurns {
			if err := c.proactivelyCompactHistory(
				ctx, &compactionHistory, &input, &replayedPendingInput, &turnStartLen,
				streamOpts, toolRegistry, hasNewToolTurns, autoCompactOff, eventCh,
			); err != nil {
				autoCompactOff = true
			}

			params := responses.ResponseNewParams{
				Model: c.model,
				Store: param.NewOpt(false),
				Input: responses.ResponseNewParamsInputUnion{
					OfInputItemList: input,
				},
			}
			if key := promptCacheKey(sessionID); key != "" {
				params.PromptCacheKey = param.NewOpt(key)
			}
			if c.thinkingEffort != "" {
				params.Reasoning = shared.ReasoningParam{
					Effort: shared.ReasoningEffort(c.thinkingEffort),
				}
			}
			if len(responseTools) > 0 {
				params.Tools = responseTools
			}

			completed, streamedContent, toolCalls, err := c.collectTurnWithRetry(ctx, params, eventCh, c.requestOptions(sessionID)...)
			if err != nil {
				c.exitIncomplete(ctx, eventCh, input, turnStartLen, replayedPendingInput, err, oneShot)
				return
			}
			if completed == nil {
				c.exitIncomplete(ctx, eventCh, input, turnStartLen, replayedPendingInput, nil, oneShot)
				return
			}

			if completed.Usage.InputTokens > 0 || completed.Usage.OutputTokens > 0 {
				slog.Debug(
					"OpenAI Responses usage",
					"inputTokens", completed.Usage.InputTokens,
					"outputTokens", completed.Usage.OutputTokens,
					"totalTokens", completed.Usage.TotalTokens,
					"reasoningTokens", completed.Usage.OutputTokensDetails.ReasoningTokens,
					"cachedTokens", completed.Usage.InputTokensDetails.CachedTokens,
				)
				sendStreamEvent(ctx, eventCh, core.StreamEvent{
					Type: core.StreamEventTypeUsage,
					Usage: &core.TokenUsage{
						InputTokens:     int(completed.Usage.InputTokens),
						OutputTokens:    int(completed.Usage.OutputTokens),
						TotalTokens:     int(completed.Usage.TotalTokens),
						ReasoningTokens: int(completed.Usage.OutputTokensDetails.ReasoningTokens),
						CachedTokens:    int(completed.Usage.InputTokensDetails.CachedTokens),
					},
				})
			}
			emitMissingFinalContent(ctx, eventCh, completed.OutputText(), streamedContent)

			if len(toolCalls) == 0 {
				sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeDone})
				return
			}

			input = append(input, responseOutputInputs(completed.Output, toolCalls, streamedContent)...)
			execRegistry := toolRegistry
			if streamOpts.DisableToolCalls {
				execRegistry = denyToolRegistry(toolRegistry)
			} else if streamOpts.DisableWriteToolCalls {
				execRegistry = denyWriteToolRegistry(toolRegistry)
			}
			toolResults, activities := c.executeTools(ctx, toolCalls, execRegistry, eventCh)
			input = append(input, toolResults...)
			compactionHistory = append(compactionHistory, core.Message{
				Role:       core.RoleAssistant,
				Content:    completed.OutputText(),
				TurnMemory: &core.TurnMemory{ToolActivity: activities},
			})
			hasNewToolTurns = true
			autoCompactOff = false
		}

		c.exitIncomplete(ctx, eventCh, input, turnStartLen, replayedPendingInput, nil, oneShot)
	}()

	return eventCh, nil
}

func (c *OpenAIResponsesClient) Reset() {
	c.pendingState = nil
}

func (c *OpenAIResponsesClient) requestOptions(sessionID string) []option.RequestOption {
	var requestOpts []option.RequestOption
	for k, v := range c.headers {
		requestOpts = append(requestOpts, option.WithHeader(k, v))
	}
	if c.provider == providerconfig.Provider(config.ProviderOpenCodeGo) && sessionID != "" {
		requestOpts = append(requestOpts, option.WithHeader("x-opencode-session", opencodeSessionID(sessionID)))
	}
	return requestOpts
}

func (c *OpenAIResponsesClient) injectPendingState(input []responses.ResponseInputItemUnionParam) ([]responses.ResponseInputItemUnionParam, []responses.ResponseInputItemUnionParam) {
	if len(c.pendingState) == 0 {
		return input, nil
	}

	replayedPendingInput := append([]responses.ResponseInputItemUnionParam(nil), c.pendingState...)

	slog.Debug("Injecting pending state", "pending_messages", len(c.pendingState), "total_messages", len(input))
	if prettyJSON, err := json.MarshalIndent(c.pendingState, "", "  "); err == nil {
		slog.Debug("Pending state contents:\n" + string(prettyJSON))
	}

	if len(input) > 0 {
		last := input[len(input)-1]
		input = append(input[:len(input)-1], replayedPendingInput...)
		input = append(input, last)
	} else {
		input = append(input, replayedPendingInput...)
	}
	c.pendingState = nil
	return input, replayedPendingInput
}

func (c *OpenAIResponsesClient) exitIncomplete(
	ctx context.Context,
	eventCh chan<- core.StreamEvent,
	input []responses.ResponseInputItemUnionParam,
	turnStartLen int,
	replayedPendingInput []responses.ResponseInputItemUnionParam,
	err error,
	oneShot bool,
) {
	if !oneShot {
		c.savePendingIfAccumulated(input, turnStartLen, replayedPendingInput)
	}
	c.emitTerminalEvent(ctx, eventCh, input, turnStartLen, replayedPendingInput, err)
}

func (c *OpenAIResponsesClient) savePendingIfAccumulated(input []responses.ResponseInputItemUnionParam, turnStartLen int, replayedPendingInput []responses.ResponseInputItemUnionParam) {
	if len(replayedPendingInput) == 0 && len(input) <= turnStartLen {
		return
	}

	newDelta := []responses.ResponseInputItemUnionParam(nil)
	if len(input) > turnStartLen {
		newDelta = input[turnStartLen:]
	}

	c.pendingState = make([]responses.ResponseInputItemUnionParam, 0, len(replayedPendingInput)+len(newDelta))
	c.pendingState = append(c.pendingState, replayedPendingInput...)
	c.pendingState = append(c.pendingState, newDelta...)
}

func (c *OpenAIResponsesClient) emitTerminalEvent(
	ctx context.Context,
	eventCh chan<- core.StreamEvent,
	input []responses.ResponseInputItemUnionParam,
	turnStartLen int,
	replayedPendingInput []responses.ResponseInputItemUnionParam,
	err error,
) {
	if len(replayedPendingInput) > 0 || len(input) > turnStartLen {
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeIncomplete, Error: err})
	} else if err != nil {
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeError, Error: err})
	} else {
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeDone})
	}
}

func (c *OpenAIResponsesClient) collectTurnWithRetry(ctx context.Context, params responses.ResponseNewParams, eventCh chan<- core.StreamEvent, opts ...option.RequestOption) (*responses.Response, string, []responses.ResponseFunctionToolCall, error) {
	var completed *responses.Response
	var content string
	var calls []responses.ResponseFunctionToolCall
	err := retry.Run(ctx, c.maxRetries, func(attempt, maxRetries int, err error) {
		slog.Debug("LLM stream error, retrying", "attempt", attempt, "maxRetries", maxRetries, "backoff", time.Duration(attempt)*time.Second, "error", err)
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeRetry, Error: err, Attempt: attempt})
	}, func() error {
		var err error
		completed, content, calls, err = c.collectTurn(ctx, params, eventCh, opts...)
		return err
	})
	if err != nil {
		return nil, "", nil, err
	}
	return completed, content, calls, nil
}

func (c *OpenAIResponsesClient) collectTurn(
	ctx context.Context,
	params responses.ResponseNewParams,
	eventCh chan<- core.StreamEvent,
	opts ...option.RequestOption,
) (*responses.Response, string, []responses.ResponseFunctionToolCall, error) {
	stream := c.responseStreamImpl(ctx, params, opts...)
	var completed *responses.Response
	var streamedContent strings.Builder

	for stream.Next() {
		ev := stream.Current()

		switch ev.Type {
		case "response.output_text.delta":
			if ev.Delta != "" {
				streamedContent.WriteString(ev.Delta)
				emitChunk(ctx, eventCh, ev.Delta)
			}
		case "response.reasoning.delta", "response.reasoning_summary.delta", "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			reasoning := ev.Delta
			if reasoning == "" {
				reasoning = ev.Text
			}
			if reasoning != "" {
				sendStreamEvent(ctx, eventCh, core.StreamEvent{
					Type:    core.StreamEventTypeReasoningChunk,
					Content: reasoning,
				})
			}
		case "error":
			msg := strings.TrimSpace(ev.Message)
			if msg == "" {
				msg = "responses stream error"
			}
			if ev.Code != "" {
				msg += " (" + ev.Code + ")"
			}
			return nil, streamedContent.String(), nil, fmt.Errorf("%s", msg)
		case "response.failed":
			return nil, streamedContent.String(), nil, responseFailedError(ev.AsResponseFailed().Response)
		case "response.incomplete":
			return nil, streamedContent.String(), nil, responseIncompleteError(ev.AsResponseIncomplete().Response)
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

func responseOutputInputs(output []responses.ResponseOutputItemUnion, fallbackToolCalls []responses.ResponseFunctionToolCall, fallbackText string) []responses.ResponseInputItemUnionParam {
	result := make([]responses.ResponseInputItemUnionParam, 0, len(output)+len(fallbackToolCalls)+1)
	seenMessage := false
	seenToolCalls := make(map[string]bool)
	for _, item := range output {
		switch item.Type {
		case "message":
			message := item.AsMessage()
			if len(message.Content) == 0 {
				continue
			}
			messageParam := message.ToParam()
			result = append(result, responses.ResponseInputItemUnionParam{OfOutputMessage: &messageParam})
			seenMessage = true
		case "reasoning":
			reasoningParam := item.AsReasoning().ToParam()
			result = append(result, responses.ResponseInputItemUnionParam{OfReasoning: &reasoningParam})
		case "function_call":
			toolCall := item.AsFunctionCall()
			result = append(result, responseFunctionCallInput(toolCall))
			markResponseToolCallSeen(seenToolCalls, toolCall)
		}
	}
	if !seenMessage && fallbackText != "" {
		result = append(result, responses.ResponseInputItemParamOfMessage(fallbackText, responses.EasyInputMessageRoleAssistant))
	}
	for _, tc := range fallbackToolCalls {
		if responseToolCallSeen(seenToolCalls, tc) {
			continue
		}
		result = append(result, responseFunctionCallInput(tc))
	}
	return result
}

func responseFailedError(response responses.Response) error {
	message := strings.TrimSpace(response.Error.Message)
	if message == "" {
		message = "response failed"
	}
	if response.Error.Code != "" {
		message += " (" + string(response.Error.Code) + ")"
	}
	return fmt.Errorf("%s", message)
}

func responseIncompleteError(response responses.Response) error {
	message := "response incomplete"
	if reason := strings.TrimSpace(response.IncompleteDetails.Reason); reason != "" {
		message += " (" + reason + ")"
	}
	return fmt.Errorf("%s", message)
}

func responseFunctionCallInput(tc responses.ResponseFunctionToolCall) responses.ResponseInputItemUnionParam {
	item := responses.ResponseInputItemParamOfFunctionCall(tc.Arguments, tc.CallID, tc.Name)
	if tc.ID != "" {
		item.OfFunctionCall.ID = param.NewOpt(tc.ID)
	}
	if tc.Status != "" {
		item.OfFunctionCall.Status = tc.Status
	}
	return item
}

func markResponseToolCallSeen(seen map[string]bool, tc responses.ResponseFunctionToolCall) {
	key := responseToolCallKey(tc)
	if key != "" {
		seen[key] = true
	}
}

func responseToolCallSeen(seen map[string]bool, tc responses.ResponseFunctionToolCall) bool {
	key := responseToolCallKey(tc)
	return key != "" && seen[key]
}

func responseToolCallKey(tc responses.ResponseFunctionToolCall) string {
	if tc.CallID != "" {
		return tc.CallID
	}
	return tc.ID
}

func (c *OpenAIResponsesClient) executeTools(
	ctx context.Context,
	toolCalls []responses.ResponseFunctionToolCall,
	registry *tools.Registry,
	eventCh chan<- core.StreamEvent,
) ([]responses.ResponseInputItemUnionParam, []core.HistoricalToolActivity) {
	toolMessages := make([]responses.ResponseInputItemUnionParam, 0, len(toolCalls))
	activities := make([]core.HistoricalToolActivity, 0, len(toolCalls))

	for _, tc := range toolCalls {
		input := map[string]any{}
		if tc.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil {
				input = map[string]any{}
			}
		}
		slog.Debug("Tool request", "tool", tc.Name, "input", input)
		execution := executeTool(ctx, registry, tc.Name, input, eventCh)
		toolOutput := history.SerializeJSON(execution.LLMOutput)
		if execution.Err != nil {
			toolOutput = history.SerializeJSON(map[string]any{"error": execution.Err.Error()})
		}
		toolMessages = append(toolMessages, responses.ResponseInputItemParamOfFunctionCallOutput(tc.CallID, toolOutput))
		activities = append(activities, execution.Activity)
	}

	return toolMessages, activities
}
