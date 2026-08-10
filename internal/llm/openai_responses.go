package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mochow13/keen-code/internal/config"
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
	provider                Provider
	model                   string
	thinkingEffort          string
	maxRetries              int
	client                  openai.Client
	responseStreamImpl      responseStreamFactory
	pendingState            []responses.ResponseInputItemUnionParam
	contextWindowTokenCount int
	headers                 map[string]string
}

func NewOpenAIResponsesClient(cfg *ClientConfig) (*OpenAIResponsesClient, error) {
	if cfg.Provider != Provider(config.ProviderOpenAI) && cfg.Provider != Provider(config.ProviderOpenCodeGo) {
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
		maxRetries:              retryCount(cfg.MaxRetries),
		client:                  client,
		contextWindowTokenCount: cfg.ContextWindowTokens,
		headers:                 cfg.Headers,
	}
	c.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &sdkResponseStream{stream: c.client.Responses.NewStreaming(ctx, params, opts...)}
	}

	return c, nil
}

func toOpenAIResponseInput(messages []Message) []responses.ResponseInputItemUnionParam {
	result := make([]responses.ResponseInputItemUnionParam, 0, len(messages))
	for messageIndex, m := range messages {
		content := FormatMessageForProvider(m)
		switch m.Role {
		case RoleSystem:
			result = append(result, responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleSystem))
		case RoleUser:
			result = append(result, responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser))
		case RoleAssistant:
			for _, step := range historicalMessageSteps(messageIndex, m) {
				if step.Text != "" {
					result = append(result, responses.ResponseInputItemParamOfMessage(step.Text, responses.EasyInputMessageRoleAssistant))
				}
				for _, invocation := range step.Activities {
					result = append(result, responses.ResponseInputItemParamOfFunctionCall(historicalToolArguments(invocation.Activity), invocation.ID, invocation.Activity.Tool))
				}
				for _, invocation := range step.Activities {
					result = append(result, responses.ResponseInputItemParamOfFunctionCallOutput(invocation.ID, historicalToolResult(invocation.Activity)))
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
	compactionHistory *[]Message,
	input *[]responses.ResponseInputItemUnionParam,
	replayedPendingInput *[]responses.ResponseInputItemUnionParam,
	turnStartLen *int,
	streamOpts StreamOptions,
	eventCh chan<- StreamEvent,
) error {
	if streamOpts.OneShot || streamOpts.DisableAutoCompaction || len(*replayedPendingInput) > 0 {
		return fmt.Errorf("automatic compaction unavailable")
	}

	childCtx, cancel := context.WithCancel(ctx)
	eventCh <- StreamEvent{Type: StreamEventTypeAutoCompactionStarted, AutoCompaction: &AutoCompactionEvent{Cancel: cancel}}
	replacement, usage, err := AutoCompact(childCtx, c, *compactionHistory, streamOpts.SessionID)
	cancel()
	if err != nil {
		eventType := StreamEventTypeAutoCompactionFailed
		if isAutoCompactionCancellation(err) {
			eventType = StreamEventTypeAutoCompactionCancelled
		}
		eventCh <- StreamEvent{Type: eventType, AutoCompaction: &AutoCompactionEvent{Usage: usage, Error: err}}
		return err
	}

	*compactionHistory = replacement
	*input = toOpenAIResponseInput(replacement)
	*turnStartLen = len(*input)
	*replayedPendingInput = nil
	c.pendingState = nil
	eventCh <- StreamEvent{Type: StreamEventTypeAutoCompactionApplied, AutoCompaction: &AutoCompactionEvent{Replacement: replacement, Usage: usage}}
	return nil
}

func (c *OpenAIResponsesClient) proactivelyCompactHistory(
	ctx context.Context,
	compactionHistory *[]Message,
	input *[]responses.ResponseInputItemUnionParam,
	replayedPendingInput *[]responses.ResponseInputItemUnionParam,
	turnStartLen *int,
	streamOpts StreamOptions,
	hasNewToolTurns bool,
	autoCompactOff bool,
	eventCh chan<- StreamEvent,
) error {
	if streamOpts.DisableAutoCompaction || streamOpts.OneShot || !hasNewToolTurns || autoCompactOff ||
		!shouldAutoCompact(estimateResponsesInputTokenCount(*input), contextInputBudget(c.contextWindowTokenCount)) {
		return nil
	}

	return c.compactHistory(ctx, compactionHistory, input, replayedPendingInput, turnStartLen, streamOpts, eventCh)
}

func (c *OpenAIResponsesClient) reduceContextOrCompact(
	ctx context.Context,
	compactionHistory *[]Message,
	input *[]responses.ResponseInputItemUnionParam,
	replayedPendingInput *[]responses.ResponseInputItemUnionParam,
	turnStartLen *int,
	streamOpts StreamOptions,
	forcedRecoveryUsed bool,
	eventCh chan<- StreamEvent,
) ([]responses.ResponseInputItemUnionParam, bool, error) {
	reducedInput, reduction := reduceResponsesContextForRequest(c.contextWindowTokenCount, *input)
	if reduction.FitsBudget {
		return reducedInput, false, nil
	}

	slog.Debug("OpenAI Responses context still exceeds budget after reduction", "inputTokenCount", reduction.ReducedTokenCount, "removedToolResultCount", reduction.RemovedToolResults)
	if streamOpts.DisableAutoCompaction || streamOpts.OneShot || forcedRecoveryUsed {
		return nil, false, fmt.Errorf("%w: %s", ErrContextWindowExceeded, contextWindowExceededError)
	}
	if err := c.compactHistory(ctx, compactionHistory, input, replayedPendingInput, turnStartLen, streamOpts, eventCh); err != nil {
		return nil, true, fmt.Errorf("%w: automatic compaction failed: %v", ErrContextWindowExceeded, err)
	}
	return nil, true, nil
}

func (c *OpenAIResponsesClient) StreamChat(
	ctx context.Context,
	messages []Message,
	toolRegistry *tools.Registry,
	opts ...StreamOptions,
) (<-chan StreamEvent, error) {
	eventCh := make(chan StreamEvent)
	streamOpts := streamOptions(opts)

	go func() {
		defer close(eventCh)

		input := toOpenAIResponseInput(messages)
		compactionHistory := CloneMessages(messages)
		oneShot := streamOpts.OneShot
		sessionID := streamOpts.SessionID
		autoCompactOff := false
		forcedRecoveryUsed := false
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
				streamOpts, hasNewToolTurns, autoCompactOff, eventCh,
			); err != nil {
				autoCompactOff = true
			}

			reducedInput, compactionAttempted, err := c.reduceContextOrCompact(
				ctx, &compactionHistory, &input, &replayedPendingInput, &turnStartLen,
				streamOpts, forcedRecoveryUsed, eventCh,
			)
			if err != nil {
				if compactionAttempted {
					c.exitIncomplete(eventCh, input, turnStartLen, replayedPendingInput, err, oneShot)
				} else {
					c.pendingState = nil
					c.emitTerminalEvent(eventCh, input, turnStartLen, replayedPendingInput, err)
				}
				return
			}
			if compactionAttempted {
				forcedRecoveryUsed = true
				continue
			}
			input = reducedInput

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
				c.exitIncomplete(eventCh, input, turnStartLen, replayedPendingInput, err, oneShot)
				return
			}
			if completed == nil {
				c.exitIncomplete(eventCh, input, turnStartLen, replayedPendingInput, nil, oneShot)
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
				eventCh <- StreamEvent{
					Type: StreamEventTypeUsage,
					Usage: &TokenUsage{
						InputTokens:     int(completed.Usage.InputTokens),
						OutputTokens:    int(completed.Usage.OutputTokens),
						TotalTokens:     int(completed.Usage.TotalTokens),
						ReasoningTokens: int(completed.Usage.OutputTokensDetails.ReasoningTokens),
						CachedTokens:    int(completed.Usage.InputTokensDetails.CachedTokens),
					},
				}
			}
			emitMissingFinalContent(eventCh, completed.OutputText(), streamedContent)

			if len(toolCalls) == 0 {
				eventCh <- StreamEvent{Type: StreamEventTypeDone}
				return
			}

			input = append(input, responseOutputInputs(completed.Output, toolCalls, streamedContent)...)
			toolResults, activities := c.executeTools(ctx, toolCalls, toolRegistry, eventCh)
			input = append(input, toolResults...)
			compactionHistory = append(compactionHistory, Message{
				Role:       RoleAssistant,
				Content:    completed.OutputText(),
				TurnMemory: &TurnMemory{ToolActivity: activities},
			})
			hasNewToolTurns = true
			autoCompactOff = false
		}

		c.exitIncomplete(eventCh, input, turnStartLen, replayedPendingInput, nil, oneShot)
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
	if c.provider == Provider(config.ProviderOpenCodeGo) && sessionID != "" {
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

func (c *OpenAIResponsesClient) exitIncomplete(eventCh chan<- StreamEvent, input []responses.ResponseInputItemUnionParam, turnStartLen int, replayedPendingInput []responses.ResponseInputItemUnionParam, err error, oneShot bool) {
	if !oneShot {
		c.savePendingIfAccumulated(input, turnStartLen, replayedPendingInput)
	}
	c.emitTerminalEvent(eventCh, input, turnStartLen, replayedPendingInput, err)
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

func (c *OpenAIResponsesClient) emitTerminalEvent(eventCh chan<- StreamEvent, input []responses.ResponseInputItemUnionParam, turnStartLen int, replayedPendingInput []responses.ResponseInputItemUnionParam, err error) {
	if len(replayedPendingInput) > 0 || len(input) > turnStartLen {
		eventCh <- StreamEvent{Type: StreamEventTypeIncomplete, Error: err}
	} else if err != nil {
		eventCh <- StreamEvent{Type: StreamEventTypeError, Error: err}
	} else {
		eventCh <- StreamEvent{Type: StreamEventTypeDone}
	}
}

func (c *OpenAIResponsesClient) collectTurnWithRetry(
	ctx context.Context,
	params responses.ResponseNewParams,
	eventCh chan<- StreamEvent,
	opts ...option.RequestOption,
) (*responses.Response, string, []responses.ResponseFunctionToolCall, error) {
	maxRetries := retryCount(c.maxRetries)
	for attempt := 1; attempt <= maxRetries; attempt++ {
		completed, streamedContent, toolCalls, err := c.collectTurn(ctx, params, eventCh, opts...)
		if err == nil {
			return completed, streamedContent, toolCalls, nil
		}
		if !isRetryableError(err) || attempt == maxRetries {
			return nil, "", nil, err
		}

		backoff := time.Duration(attempt) * time.Second
		slog.Debug("LLM stream error, retrying", "attempt", attempt, "maxRetries", maxRetries, "backoff", backoff, "error", err)
		eventCh <- StreamEvent{Type: StreamEventTypeRetry, Error: err, Attempt: attempt}

		select {
		case <-ctx.Done():
			return nil, "", nil, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return nil, "", nil, nil
}

func (c *OpenAIResponsesClient) collectTurn(
	ctx context.Context,
	params responses.ResponseNewParams,
	eventCh chan<- StreamEvent,
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
				emitChunk(eventCh, ev.Delta)
			}
		case "response.reasoning.delta", "response.reasoning_summary.delta", "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			reasoning := ev.Delta
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
	eventCh chan<- StreamEvent,
) ([]responses.ResponseInputItemUnionParam, []HistoricalToolActivity) {
	toolMessages := make([]responses.ResponseInputItemUnionParam, 0, len(toolCalls))
	activities := make([]HistoricalToolActivity, 0, len(toolCalls))

	for _, tc := range toolCalls {
		start := time.Now()
		input := map[string]any{}
		if tc.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Arguments), &input); err != nil {
				input = map[string]any{}
			}
		}
		slog.Debug("Tool request", "tool", tc.Name, "input", input)

		output, execErr, toolStarted := executeValidatedTool(ctx, registry, tc.Name, input, eventCh)

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
			if toolStarted {
				eventCh <- StreamEvent{
					Type:     StreamEventTypeToolEnd,
					ToolCall: toolCall,
				}
			}
			toolOutput = serializeJSON(map[string]any{"error": execErr.Error()})
		} else {
			slog.Debug("Tool response", "tool", tc.Name, "duration", duration)
			eventCh <- StreamEvent{
				Type:     StreamEventTypeToolEnd,
				ToolCall: toolCall,
			}
			toolOutput = serializeJSON(output)
		}

		toolMessages = append(toolMessages, responses.ResponseInputItemParamOfFunctionCallOutput(tc.CallID, toolOutput))
		activities = append(activities, historicalToolActivity(tc.Name, input, output, execErr))
	}

	return toolMessages, activities
}
