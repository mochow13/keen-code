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
	"github.com/openai/openai-go/v3/packages/respjson"
	"github.com/openai/openai-go/v3/shared"
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
	provider                providerconfig.Provider
	model                   string
	thinkingEffort          string
	maxRetries              int
	client                  openai.Client
	streamImpl              streamFactory
	pendingState            []openai.ChatCompletionMessageParamUnion
	contextWindowTokenCount int
	headers                 map[string]string
}

type openAIThinkingParamMode uint8

const (
	openAIThinkingParamNone openAIThinkingParamMode = iota
	openAIThinkingParamReasoningEffort
	openAIThinkingParamType
	openAIThinkingParamToggleAndReasoningEffort
)

func NewOpenAICompatibleClient(cfg *providerconfig.ClientConfig) (*OpenAICompatibleClient, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		var err error
		baseURL, err = openAICompatibleBaseURL(cfg.Provider)
		if err != nil {
			return nil, err
		}
	}

	client := openai.NewClient(
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(baseURL),
	)

	c := &OpenAICompatibleClient{
		provider:                cfg.Provider,
		model:                   cfg.Model,
		thinkingEffort:          cfg.ThinkingEffort,
		maxRetries:              retry.Count(cfg.MaxRetries),
		client:                  client,
		contextWindowTokenCount: cfg.ContextWindowTokens,
		headers:                 cfg.Headers,
	}
	c.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		return &sdkChatStream{stream: c.client.Chat.Completions.NewStreaming(ctx, params, opts...)}
	}

	return c, nil
}

func openAICompatibleBaseURL(provider providerconfig.Provider) (string, error) {
	switch provider {
	case providerconfig.Provider(config.ProviderDeepSeek):
		return deepSeekBaseURL, nil
	case providerconfig.Provider(config.ProviderMoonshotAI):
		return moonshotAIBaseURL, nil
	case providerconfig.Provider(config.ProviderZAI):
		return zaiBaseURL, nil
	case providerconfig.Provider(config.ProviderOpenCodeGo):
		return openCodeGoBaseURL + "/v1/", nil
	case providerconfig.Provider(config.ProviderYoloAuto):
		return yoloAutoBaseURL, nil
	case providerconfig.Provider(config.ProviderOpenAICompatible):
		return "", fmt.Errorf("base_url must be configured for provider: %s. %s", provider, config.ConfigFixHint)
	default:
		return "", fmt.Errorf("unsupported OpenAI-compatible provider: %s. %s", provider, config.ConfigFixHint)
	}
}

func logOpenAIMessageOrder(messages []openai.ChatCompletionMessageParamUnion) {
	prettyJSON, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		slog.Debug("Failed to log OpenAI message order", "error", err)
		return
	}
	slog.Debug("OpenAI message order:\n" + string(prettyJSON))
}

func toOpenAIMessages(messages []core.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for messageIndex, m := range messages {
		content := history.FormatMessage(m)
		switch m.Role {
		case core.RoleSystem:
			result = append(result, openai.SystemMessage(content))
		case core.RoleUser:
			result = append(result, openai.UserMessage(content))
		case core.RoleAssistant:
			for _, step := range history.MessageSteps(messageIndex, m) {
				am := openai.ChatCompletionAssistantMessageParam{}
				if step.Text != "" {
					am.Content.OfString = openai.String(step.Text)
				}
				for _, invocation := range step.Activities {
					am.ToolCalls = append(am.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: invocation.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      invocation.Activity.Tool,
								Arguments: history.ToolArguments(invocation.Activity),
							},
						},
					})
				}
				if step.Text != "" || len(step.Activities) > 0 {
					result = append(result, openai.ChatCompletionMessageParamUnion{OfAssistant: &am})
				}
				for _, invocation := range step.Activities {
					result = append(result, openai.ToolMessage(history.ToolResult(invocation.Activity), invocation.ID))
				}
			}
		}
	}
	logOpenAIMessageOrder(result)
	return result
}

func toOpenAITools(registry *tools.Registry) []openai.ChatCompletionToolUnionParam {
	if registry == nil {
		return nil
	}

	all := registry.All()
	result := make([]openai.ChatCompletionToolUnionParam, 0, len(all))
	for _, t := range all {
		result = append(result, openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: shared.FunctionDefinitionParam{
					Name:        t.Name(),
					Description: openai.String(t.Description()),
					Parameters:  openai.FunctionParameters(t.InputSchema()),
					Strict:      openai.Bool(false),
				},
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

func extractReasoningDelta(extra map[string]respjson.Field) string {
	for _, key := range []string{"reasoning_content", "reasoning", "reasoning_text"} {
		if value := extractJSONStringField(extra, key); value != "" {
			return value
		}
	}
	return ""
}

func emitChunk(ctx context.Context, eventCh chan<- core.StreamEvent, content string) {
	if content == "" {
		return
	}
	sendStreamEvent(ctx, eventCh, core.StreamEvent{
		Type:    core.StreamEventTypeChunk,
		Content: content,
	})
}

func functionToolCalls(toolCalls []openai.ChatCompletionMessageToolCallUnion) []openai.ChatCompletionMessageFunctionToolCall {
	result := make([]openai.ChatCompletionMessageFunctionToolCall, 0, len(toolCalls))
	for _, toolCall := range toolCalls {
		if strings.TrimSpace(toolCall.Function.Name) == "" {
			continue
		}
		result = append(result, openai.ChatCompletionMessageFunctionToolCall{
			ID:       toolCall.ID,
			Function: toolCall.Function,
		})
	}
	return result
}

func (c *OpenAICompatibleClient) buildAssistantMessage(message openai.ChatCompletionMessage, reasoningContent string, toolCalls []openai.ChatCompletionMessageFunctionToolCall) openai.ChatCompletionAssistantMessageParam {
	assistant := openai.ChatCompletionAssistantMessageParam{}
	if message.Content != "" {
		assistant.Content.OfString = openai.String(message.Content)
	}
	if len(toolCalls) > 0 {
		assistant.ToolCalls = make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(toolCalls))
		for _, toolCall := range toolCalls {
			assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: toolCall.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      toolCall.Function.Name,
						Arguments: toolCall.Function.Arguments,
					},
				},
			})
		}
	}
	if reasoningContent != "" {
		assistant.SetExtraFields(map[string]any{
			"reasoning_content": reasoningContent,
		})
	}
	return assistant
}

func emitMissingFinalContent(
	ctx context.Context,
	eventCh chan<- core.StreamEvent,
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
			emitChunk(ctx, eventCh, tail)
		}
		return
	}

	if streamedContent == "" {
		emitChunk(ctx, eventCh, fullContent)
	}
}

func (c *OpenAICompatibleClient) shouldLogRawChunks() bool {
	return c.provider == providerconfig.Provider(config.ProviderOpenCodeGo) && isOpenCodeGoKimiModel(c.model)
}

func (c *OpenAICompatibleClient) collectTurn(
	ctx context.Context,
	params openai.ChatCompletionNewParams,
	eventCh chan<- core.StreamEvent,
	requestOpts ...option.RequestOption,
) (openai.ChatCompletionMessage, string, string, bool, openai.CompletionUsage, error) {
	stream := c.streamImpl(ctx, params, requestOpts...)
	var acc openai.ChatCompletionAccumulator
	var reasoningContent strings.Builder
	var streamedContent strings.Builder

	for stream.Next() {
		chunk := stream.Current()
		if c.shouldLogRawChunks() {
			slog.Debug("OpenCode Go Kimi stream chunk", "chunk", chunk.RawJSON())
		}
		acc.AddChunk(chunk)

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			streamedContent.WriteString(delta.Content)
			emitChunk(ctx, eventCh, delta.Content)
		}

		// reasoning_content/reasoning are OpenAI-compatible extensions not modeled by openai-go.
		// Capture it during streaming because the SDK accumulator does not retain JSON metadata.
		reasoningDelta := extractReasoningDelta(delta.JSON.ExtraFields)
		reasoningContent.WriteString(reasoningDelta)
		if reasoningDelta != "" {
			sendStreamEvent(ctx, eventCh, core.StreamEvent{
				Type:    core.StreamEventTypeReasoningChunk,
				Content: reasoningDelta,
			})
		}

		if len(delta.ToolCalls) > 0 {
			slog.Debug("OpenAI chat delta tool_calls", "tool_calls", delta.ToolCalls)
		}
		if delta.FunctionCall.Name != "" || delta.FunctionCall.Arguments != "" {
			slog.Debug("OpenAI chat delta function_call", "function_call", delta.FunctionCall)
		}
	}
	_ = stream.Close()

	if err := stream.Err(); err != nil {
		return openai.ChatCompletionMessage{}, "", "", false, openai.CompletionUsage{}, fmt.Errorf("stream error: %w", err)
	}
	if len(acc.ChatCompletion.Choices) == 0 {
		return openai.ChatCompletionMessage{}, "", "", false, openai.CompletionUsage{}, nil
	}

	return acc.ChatCompletion.Choices[0].Message, reasoningContent.String(), streamedContent.String(), true, acc.ChatCompletion.Usage, nil
}

func (c *OpenAICompatibleClient) collectTurnWithRetry(ctx context.Context, params openai.ChatCompletionNewParams, eventCh chan<- core.StreamEvent, requestOpts ...option.RequestOption) (openai.ChatCompletionMessage, string, string, bool, openai.CompletionUsage, error) {
	var message openai.ChatCompletionMessage
	var reasoning, content string
	var hasChoice bool
	var usage openai.CompletionUsage
	err := retry.Run(ctx, c.maxRetries, func(attempt, maxRetries int, err error) {
		slog.Debug("LLM stream error, retrying", "attempt", attempt, "maxRetries", maxRetries, "backoff", time.Duration(attempt)*time.Second, "error", err)
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeRetry, Error: err, Attempt: attempt})
	}, func() error {
		var err error
		message, reasoning, content, hasChoice, usage, err = c.collectTurn(ctx, params, eventCh, requestOpts...)
		return err
	})
	if err != nil {
		return openai.ChatCompletionMessage{}, "", "", false, openai.CompletionUsage{}, err
	}
	return message, reasoning, content, hasChoice, usage, nil
}

func (c *OpenAICompatibleClient) injectPendingState(oaiMessages []openai.ChatCompletionMessageParamUnion) ([]openai.ChatCompletionMessageParamUnion, []openai.ChatCompletionMessageParamUnion) {
	if len(c.pendingState) == 0 {
		return oaiMessages, nil
	}

	injectedPending := append([]openai.ChatCompletionMessageParamUnion(nil), c.pendingState...)

	slog.Debug("Injecting pending state", "pending_messages", len(c.pendingState), "total_messages", len(oaiMessages))
	if prettyJSON, err := json.MarshalIndent(c.pendingState, "", "  "); err == nil {
		slog.Debug("Pending state contents:\n" + string(prettyJSON))
	}

	if len(oaiMessages) > 0 {
		last := oaiMessages[len(oaiMessages)-1]
		oaiMessages = append(oaiMessages[:len(oaiMessages)-1], injectedPending...)
		oaiMessages = append(oaiMessages, last)
	} else {
		oaiMessages = append(oaiMessages, injectedPending...)
	}
	c.pendingState = nil
	return oaiMessages, injectedPending
}

func (c *OpenAICompatibleClient) buildChatParams(
	oaiMessages []openai.ChatCompletionMessageParamUnion,
	oaiTools []openai.ChatCompletionToolUnionParam) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Model:    c.model,
		Messages: oaiMessages,
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		},
	}
	if len(oaiTools) > 0 {
		params.Tools = oaiTools
	}
	if c.thinkingEffort != "" {
		applyOpenAIThinking(&params, openAIThinkingMode(c.provider, c.model), c.thinkingEffort)
	}
	return params
}

func setThinkingType(params *openai.ChatCompletionNewParams, thinkingType string) {
	params.SetExtraFields(map[string]any{"thinking": map[string]any{"type": thinkingType}})
}

func applyOpenAIThinking(params *openai.ChatCompletionNewParams, mode openAIThinkingParamMode, option string) {
	switch mode {
	case openAIThinkingParamReasoningEffort:
		params.ReasoningEffort = shared.ReasoningEffort(option)
	case openAIThinkingParamType:
		setThinkingType(params, option)
	case openAIThinkingParamToggleAndReasoningEffort:
		if option == "disabled" {
			setThinkingType(params, option)
			return
		}
		params.ReasoningEffort = shared.ReasoningEffort(option)
		setThinkingType(params, "enabled")
	}
}

func openAIThinkingMode(provider providerconfig.Provider, model string) openAIThinkingParamMode {
	switch provider {
	case providerconfig.Provider(config.ProviderDeepSeek):
		return openAIThinkingParamToggleAndReasoningEffort
	case providerconfig.Provider(config.ProviderMoonshotAI):
		switch model {
		case "kimi-k3":
			return openAIThinkingParamReasoningEffort
		case "kimi-k2.6":
			return openAIThinkingParamType
		}
	case providerconfig.Provider(config.ProviderZAI):
		if model == "glm-5.2" || model == "glm-5.3" {
			return openAIThinkingParamToggleAndReasoningEffort
		}
		if model == "glm-5.1" {
			return openAIThinkingParamType
		}
	case providerconfig.Provider(config.ProviderOpenCodeGo):
		if isOpenCodeGoDeepSeekModel(model) {
			return openAIThinkingParamToggleAndReasoningEffort
		}
		if isOpenCodeGoReasoningEffortModel(model) {
			return openAIThinkingParamReasoningEffort
		}
		if isOpenCodeGoGLMModel(model) || isOpenCodeGoKimiModel(model) || isOpenCodeGoLongCatModel(model) {
			return openAIThinkingParamType
		}
	}
	return openAIThinkingParamNone
}

func (c *OpenAICompatibleClient) exitIncomplete(ctx context.Context, eventCh chan<- core.StreamEvent, oaiMessages []openai.ChatCompletionMessageParamUnion, turnStartLen int, injectedPending []openai.ChatCompletionMessageParamUnion, err error, oneShot bool) {
	if !oneShot {
		c.savePendingIfAccumulated(oaiMessages, turnStartLen, injectedPending)
	}
	c.emitTerminalEvent(ctx, eventCh, oaiMessages, turnStartLen, injectedPending, err)
}

func (c *OpenAICompatibleClient) StreamChat(
	ctx context.Context,
	messages []core.Message,
	toolRegistry *tools.Registry,
	opts ...core.StreamOptions,
) (<-chan core.StreamEvent, error) {
	eventCh := make(chan core.StreamEvent)
	streamOpts := streamOptions(opts)

	go func() {
		defer close(eventCh)

		oaiMessages := toOpenAIMessages(messages)
		oneShot := streamOpts.OneShot
		var injectedPending []openai.ChatCompletionMessageParamUnion
		if !oneShot {
			oaiMessages, injectedPending = c.injectPendingState(oaiMessages)
		}

		turnStartLen := len(oaiMessages)

		oaiTools := toOpenAITools(toolRegistry)
		requestOpts := c.requestOptions(streamOpts)

		compactionHistory := core.CloneMessages(messages)
		autoCompactOff := false
		hasNewToolTurns := false

		for range maxToolTurns {
			if err := c.proactivelyCompactHistory(
				ctx, &compactionHistory, &oaiMessages, &injectedPending, &turnStartLen,
				streamOpts, toolRegistry, hasNewToolTurns, autoCompactOff, eventCh,
			); err != nil {
				autoCompactOff = true
			}

			params := c.buildChatParams(oaiMessages, oaiTools)

			message, reasoningContent, streamedContent, hasChoice, usage, err := c.collectTurnWithRetry(ctx, params, eventCh, requestOpts...)
			if err != nil {
				c.exitIncomplete(ctx, eventCh, oaiMessages, turnStartLen, injectedPending, err, oneShot)
				return
			}

			if !hasChoice {
				c.exitIncomplete(ctx, eventCh, oaiMessages, turnStartLen, injectedPending, nil, oneShot)
				return
			}
			if usage.PromptTokens > 0 || usage.CompletionTokens > 0 {
				slog.Debug(
					"OpenAI usage",
					"prompt_tokens", usage.PromptTokens,
					"completion_tokens", usage.CompletionTokens,
					"total_tokens", usage.TotalTokens,
					"cached_tokens", usage.PromptTokensDetails.CachedTokens,
					"reasoning_tokens", usage.CompletionTokensDetails.ReasoningTokens,
				)
				sendStreamEvent(ctx, eventCh, core.StreamEvent{
					Type: core.StreamEventTypeUsage,
					Usage: &core.TokenUsage{
						InputTokens:     int(usage.PromptTokens),
						OutputTokens:    int(usage.CompletionTokens),
						TotalTokens:     int(usage.TotalTokens),
						CachedTokens:    int(usage.PromptTokensDetails.CachedTokens),
						ReasoningTokens: int(usage.CompletionTokensDetails.ReasoningTokens),
					},
				})
			}
			emitMissingFinalContent(ctx, eventCh, message.Content, streamedContent)
			toolCalls := functionToolCalls(message.ToolCalls)
			assistant := c.buildAssistantMessage(message, reasoningContent, toolCalls)

			if len(toolCalls) == 0 {
				sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeDone})
				return
			}

			oaiMessages = append(oaiMessages, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &assistant,
			})

			execRegistry := toolRegistry
			if streamOpts.DisableToolCalls {
				execRegistry = denyToolRegistry(toolRegistry)
			} else if streamOpts.DisableWriteToolCalls {
				execRegistry = denyWriteToolRegistry(toolRegistry)
			}
			toolMsgs, activities := c.executeTools(ctx, toolCalls, execRegistry, eventCh)
			if len(toolMsgs) > 0 {
				oaiMessages = append(oaiMessages, toolMsgs...)
			}
			compactionHistory = append(compactionHistory, core.Message{
				Role: core.RoleAssistant, Content: message.Content,
				TurnMemory: &core.TurnMemory{ToolActivity: activities},
			})
			autoCompactOff = false
			hasNewToolTurns = true
		}

		c.exitIncomplete(ctx, eventCh, oaiMessages, turnStartLen, injectedPending, nil, oneShot)
	}()

	return eventCh, nil
}

func (c *OpenAICompatibleClient) proactivelyCompactHistory(
	ctx context.Context,
	compactionHistory *[]core.Message,
	oaiMessages *[]openai.ChatCompletionMessageParamUnion,
	injectedPending *[]openai.ChatCompletionMessageParamUnion,
	turnStartLen *int,
	streamOpts core.StreamOptions,
	toolRegistry *tools.Registry,
	hasNewToolTurns bool,
	autoCompactOff bool,
	eventCh chan<- core.StreamEvent,
) error {
	if streamOpts.DisableAutoCompaction || !hasNewToolTurns || autoCompactOff ||
		!core.ShouldAutoCompact(
			estimateOpenAIInput(*oaiMessages),
			core.ContextInputBudget(c.contextWindowTokenCount),
		) {
		return nil
	}

	return c.compactHistory(
		ctx, compactionHistory, oaiMessages, injectedPending, turnStartLen,
		toolRegistry, streamOpts.SessionID, eventCh,
	)
}

func estimateOpenAIInput(messages []openai.ChatCompletionMessageParamUnion) int {
	tokens := 0
	for _, message := range messages {
		b, err := json.Marshal(message)
		if err != nil {
			continue
		}
		tokens += core.EstimateContextTokenCount(string(b))
	}
	return tokens
}

func (c *OpenAICompatibleClient) compactHistory(
	ctx context.Context,
	compactionHistory *[]core.Message,
	oaiMessages *[]openai.ChatCompletionMessageParamUnion,
	injectedPending *[]openai.ChatCompletionMessageParamUnion,
	turnStartLen *int,
	toolRegistry *tools.Registry,
	sessionID string,
	eventCh chan<- core.StreamEvent,
) error {
	replacement, _, err := c.autoCompact(ctx, *compactionHistory, toolRegistry, sessionID, eventCh)
	if err != nil {
		return err
	}

	*oaiMessages = toOpenAIMessages(replacement)
	*compactionHistory = replacement
	c.pendingState = nil
	*injectedPending = nil
	*turnStartLen = len(*oaiMessages)
	return nil
}

func (c *OpenAICompatibleClient) autoCompact(ctx context.Context, history []core.Message, toolRegistry *tools.Registry, sessionID string, eventCh chan<- core.StreamEvent) ([]core.Message, bool, error) {
	compactionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sendStreamEvent(ctx, eventCh, core.StreamEvent{
		Type:           core.StreamEventTypeAutoCompactionStarted,
		AutoCompaction: &core.AutoCompactionEvent{Cancel: cancel},
	})
	replacement, usage, err := AutoCompact(compactionCtx, c, history, toolRegistry, sessionID)
	if err != nil {
		eventType := core.StreamEventTypeAutoCompactionFailed
		if compaction.IsCancellation(err) {
			eventType = core.StreamEventTypeAutoCompactionCancelled
		}
		sendStreamEvent(ctx, eventCh, core.StreamEvent{
			Type:           eventType,
			AutoCompaction: &core.AutoCompactionEvent{Usage: usage, Error: err},
		})
		return nil, false, err
	}
	sendStreamEvent(ctx, eventCh, core.StreamEvent{
		Type:           core.StreamEventTypeAutoCompactionApplied,
		AutoCompaction: &core.AutoCompactionEvent{Replacement: replacement, Usage: usage},
	})
	return replacement, true, nil
}

func (c *OpenAICompatibleClient) requestOptions(opts core.StreamOptions) []option.RequestOption {
	var requestOpts []option.RequestOption
	for k, v := range c.headers {
		requestOpts = append(requestOpts, option.WithHeader(k, v))
	}
	if c.provider == providerconfig.Provider(config.ProviderOpenCodeGo) && opts.SessionID != "" {
		requestOpts = append(requestOpts, option.WithHeader("x-opencode-session", opencodeSessionID(opts.SessionID)))
	}
	return requestOpts
}

func (c *OpenAICompatibleClient) Reset() {
	c.pendingState = nil
}

func (c *OpenAICompatibleClient) savePendingIfAccumulated(
	oaiMessages []openai.ChatCompletionMessageParamUnion,
	turnStartLen int,
	injectedPending []openai.ChatCompletionMessageParamUnion,
) {
	if len(injectedPending) == 0 && len(oaiMessages) <= turnStartLen {
		return
	}

	newDelta := []openai.ChatCompletionMessageParamUnion(nil)
	if len(oaiMessages) > turnStartLen {
		newDelta = oaiMessages[turnStartLen:]
	}

	c.pendingState = make([]openai.ChatCompletionMessageParamUnion, 0, len(injectedPending)+len(newDelta))
	c.pendingState = append(c.pendingState, injectedPending...)
	c.pendingState = append(c.pendingState, newDelta...)
}

func (c *OpenAICompatibleClient) emitTerminalEvent(
	ctx context.Context,
	eventCh chan<- core.StreamEvent,
	oaiMessages []openai.ChatCompletionMessageParamUnion,
	turnStartLen int,
	injectedPending []openai.ChatCompletionMessageParamUnion,
	err error,
) {
	if len(injectedPending) > 0 || len(oaiMessages) > turnStartLen {
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeIncomplete, Error: err})
	} else if err != nil {
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeError, Error: err})
	} else {
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeDone})
	}
}

func (c *OpenAICompatibleClient) executeTools(
	ctx context.Context,
	toolCalls []openai.ChatCompletionMessageFunctionToolCall,
	registry *tools.Registry,
	eventCh chan<- core.StreamEvent,
) ([]openai.ChatCompletionMessageParamUnion, []core.HistoricalToolActivity) {
	toolMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(toolCalls))
	activities := make([]core.HistoricalToolActivity, 0, len(toolCalls))

	for _, tc := range toolCalls {
		input := map[string]any{}
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
				input = map[string]any{}
			}
		}
		slog.Debug("Tool request", "tool", tc.Function.Name, "input", input)
		execution := executeTool(ctx, registry, tc.Function.Name, input, eventCh)
		toolOutput := history.SerializeJSON(execution.LLMOutput)
		if execution.Err != nil {
			toolOutput = history.SerializeJSON(map[string]any{"error": execution.Err.Error()})
		}
		toolMessages = append(toolMessages, openai.ToolMessage(toolOutput, tc.ID))
		activities = append(activities, execution.Activity)
	}

	return toolMessages, activities
}
