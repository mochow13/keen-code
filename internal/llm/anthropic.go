package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm/compaction"
	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/llm/history"
	"github.com/mochow13/keen-code/internal/llm/providerconfig"
	"github.com/mochow13/keen-code/internal/llm/retry"
	"github.com/mochow13/keen-code/internal/tools"
)

const anthropicMaxTokens = 64000

type anthropicStream interface {
	Next() bool
	Current() anthropic.MessageStreamEventUnion
	Err() error
	Close() error
}

type anthropicStreamFactory func(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) anthropicStream

type anthropicContentBlockState struct {
	blockType   string
	id          string
	name        string
	text        string
	thinking    string
	signature   string
	data        string
	inputStart  []byte
	inputBuffer []byte
}

type sdkAnthropicStream struct {
	stream *ssestream.Stream[anthropic.MessageStreamEventUnion]
}

func (s *sdkAnthropicStream) Next() bool {
	return s.stream.Next()
}

func (s *sdkAnthropicStream) Current() anthropic.MessageStreamEventUnion {
	return s.stream.Current()
}

func (s *sdkAnthropicStream) Err() error {
	return s.stream.Err()
}

func (s *sdkAnthropicStream) Close() error {
	return s.stream.Close()
}

type AnthropicClient struct {
	client                  anthropic.Client
	provider                providerconfig.Provider
	model                   string
	thinkingEffort          string
	maxRetries              int
	streamImpl              anthropicStreamFactory
	pendingState            []anthropic.MessageParam
	contextWindowTokenCount int
	headers                 map[string]string
}

func NewAnthropicClient(cfg *providerconfig.ClientConfig) (*AnthropicClient, error) {
	var opts []option.RequestOption
	opts = append(opts, option.WithAPIKey(cfg.APIKey))
	if cfg.APIKeyHelper != "" {
		resolver := config.NewAPIKeyResolver(string(cfg.Provider), cfg.APIKeyHelper)
		opts = append(opts, option.WithMiddleware(apiKeyRefreshMiddleware(resolver)))
	}
	if baseURL := anthropicBaseURL(cfg.Provider, cfg.BaseURL); baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	client := anthropic.NewClient(opts...)

	c := &AnthropicClient{
		client:                  client,
		provider:                cfg.Provider,
		model:                   cfg.Model,
		thinkingEffort:          cfg.ThinkingEffort,
		maxRetries:              retry.Count(cfg.MaxRetries),
		contextWindowTokenCount: cfg.ContextWindowTokens,
		headers:                 cfg.Headers,
	}
	c.streamImpl = func(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) anthropicStream {
		return &sdkAnthropicStream{stream: c.client.Messages.NewStreaming(ctx, params, opts...)}
	}

	return c, nil
}

func anthropicBaseURL(provider providerconfig.Provider, configured string) string {
	if configured != "" {
		return configured
	}
	if provider == providerconfig.Provider(config.ProviderOpenCodeGo) {
		return openCodeGoBaseURL
	}
	if provider == providerconfig.Provider(config.ProviderMiniMax) {
		return miniMaxBaseURL
	}
	return ""
}

func formatAnthropicTurnInput(system []anthropic.TextBlockParam, messages []anthropic.MessageParam) string {
	var input strings.Builder
	for _, block := range system {
		if input.Len() > 0 {
			input.WriteString("\n\n")
		}
		input.WriteString("--- system ---\n")
		input.WriteString(block.Text)
	}
	for _, message := range messages {
		if input.Len() > 0 {
			input.WriteString("\n\n")
		}
		input.WriteString("--- ")
		input.WriteString(string(message.Role))
		input.WriteString(" ---\n")
		for i, block := range message.Content {
			if i > 0 {
				input.WriteByte('\n')
			}
			if block.OfText != nil {
				input.WriteString(block.OfText.Text)
				continue
			}
			encoded, err := json.Marshal(block)
			if err != nil {
				input.WriteString("[unprintable content block]")
				continue
			}
			input.Write(encoded)
		}
	}
	return input.String()
}

func toAnthropicMessages(messages []core.Message) ([]anthropic.TextBlockParam, []anthropic.MessageParam) {
	var systemBlocks []anthropic.TextBlockParam
	var msgParams []anthropic.MessageParam

	for messageIndex, m := range messages {
		content := history.FormatMessage(m)
		switch m.Role {
		case core.RoleSystem:
			systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: content})
		case core.RoleUser:
			if content != "" {
				msgParams = append(msgParams, anthropic.NewUserMessage(anthropic.NewTextBlock(content)))
			}
		case core.RoleAssistant:
			for _, step := range history.MessageSteps(messageIndex, m) {
				blocks := make([]anthropic.ContentBlockParamUnion, 0, len(step.Activities)+1)
				if step.Text != "" {
					blocks = append(blocks, anthropic.NewTextBlock(step.Text))
				}
				for _, invocation := range step.Activities {
					blocks = append(blocks, anthropic.NewToolUseBlock(invocation.ID, json.RawMessage(history.ToolArguments(invocation.Activity)), invocation.Activity.Tool))
				}
				if len(blocks) > 0 {
					msgParams = append(msgParams, anthropic.NewAssistantMessage(blocks...))
				}
				if len(step.Activities) > 0 {
					results := make([]anthropic.ContentBlockParamUnion, 0, len(step.Activities))
					for _, invocation := range step.Activities {
						results = append(results, anthropic.NewToolResultBlock(invocation.ID, history.ToolResult(invocation.Activity), invocation.Activity.Status != "success"))
					}
					msgParams = append(msgParams, anthropic.NewUserMessage(results...))
				}
			}
		}
	}

	return systemBlocks, msgParams
}

func toAnthropicTools(registry *tools.Registry) []anthropic.ToolUnionParam {
	if registry == nil {
		return nil
	}

	all := registry.All()
	result := make([]anthropic.ToolUnionParam, 0, len(all))
	for _, t := range all {
		schema := t.InputSchema()
		inputSchema := anthropic.ToolInputSchemaParam{}
		if props, ok := schema["properties"]; ok {
			inputSchema.Properties = props
		}
		if req, ok := schema["required"].([]string); ok {
			inputSchema.Required = req
		}

		result = append(result, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name(),
				Description: param.NewOpt(t.Description()),
				InputSchema: inputSchema,
			},
		})
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func applyAnthropicBlockCacheControl(
	system []anthropic.TextBlockParam,
	anthropicTools []anthropic.ToolUnionParam,
	messages []anthropic.MessageParam,
	stableMessageCount int,
) ([]anthropic.TextBlockParam, []anthropic.ToolUnionParam, []anthropic.MessageParam) {
	turnSystem := append([]anthropic.TextBlockParam(nil), system...)
	turnTools := append([]anthropic.ToolUnionParam(nil), anthropicTools...)
	turnMessages := append([]anthropic.MessageParam(nil), messages...)
	clearAnthropicCacheControl(turnSystem, turnTools, turnMessages)

	cacheControl := anthropic.NewCacheControlEphemeralParam()
	if len(turnSystem) > 0 {
		turnSystem[len(turnSystem)-1].CacheControl = cacheControl
	}
	if len(turnTools) > 0 {
		if toolCacheControl := turnTools[len(turnTools)-1].GetCacheControl(); toolCacheControl != nil {
			*toolCacheControl = cacheControl
		}
	}

	stableIdx := stableMessageCount - 1
	if stableIdx >= 0 && stableIdx < len(turnMessages) {
		turnMessages = addAnthropicMessageBlockCacheControl(turnMessages, stableIdx)
	}
	lastIdx := len(turnMessages) - 1
	if lastIdx != stableIdx {
		turnMessages = addAnthropicMessageBlockCacheControl(turnMessages, lastIdx)
	}
	return turnSystem, turnTools, turnMessages
}

func clearAnthropicCacheControl(
	system []anthropic.TextBlockParam,
	anthropicTools []anthropic.ToolUnionParam,
	messages []anthropic.MessageParam,
) {
	for i := range system {
		system[i].CacheControl = anthropic.CacheControlEphemeralParam{}
	}
	for i := range anthropicTools {
		if cacheControl := anthropicTools[i].GetCacheControl(); cacheControl != nil {
			*cacheControl = anthropic.CacheControlEphemeralParam{}
		}
	}
	for msgIdx := range messages {
		messages[msgIdx].Content = append([]anthropic.ContentBlockParamUnion(nil), messages[msgIdx].Content...)
		for blockIdx := range messages[msgIdx].Content {
			if cacheControl := messages[msgIdx].Content[blockIdx].GetCacheControl(); cacheControl != nil {
				*cacheControl = anthropic.CacheControlEphemeralParam{}
			}
		}
	}
}

func addAnthropicMessageBlockCacheControl(messages []anthropic.MessageParam, idx int) []anthropic.MessageParam {
	messages[idx].Content = append([]anthropic.ContentBlockParamUnion(nil), messages[idx].Content...)
	for blockIdx := len(messages[idx].Content) - 1; blockIdx >= 0; blockIdx-- {
		cacheControl := messages[idx].Content[blockIdx].GetCacheControl()
		if cacheControl == nil {
			continue
		}
		*cacheControl = anthropic.NewCacheControlEphemeralParam()
		return messages
	}
	return messages
}

func (c *AnthropicClient) collectTurnWithRetry(ctx context.Context, params anthropic.MessageNewParams, eventCh chan<- core.StreamEvent, requestOpts ...option.RequestOption) ([]anthropic.ContentBlockParamUnion, []toolUseEntry, *core.TokenUsage, error) {
	var blocks []anthropic.ContentBlockParamUnion
	var uses []toolUseEntry
	var usage *core.TokenUsage
	err := retry.Run(ctx, c.maxRetries, func(attempt, maxRetries int, err error) {
		slog.Debug("LLM stream error, retrying", "attempt", attempt, "maxRetries", maxRetries, "backoff", time.Duration(attempt)*time.Second, "error", err)
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeRetry, Error: err, Attempt: attempt})
	}, func() error {
		var err error
		blocks, uses, usage, err = c.collectTurn(ctx, params, eventCh, requestOpts...)
		return err
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return blocks, uses, usage, nil
}

func (c *AnthropicClient) collectTurn(
	ctx context.Context,
	params anthropic.MessageNewParams,
	eventCh chan<- core.StreamEvent,
	requestOpts ...option.RequestOption,
) ([]anthropic.ContentBlockParamUnion, []toolUseEntry, *core.TokenUsage, error) {
	stream := c.streamImpl(ctx, params, requestOpts...)

	// Track open content blocks by index so tool continuations can replay the
	// exact assistant block sequence.
	blockStates := map[int64]*anthropicContentBlockState{}

	var assistantBlocks []anthropic.ContentBlockParamUnion
	var usage *core.TokenUsage
	var cacheReadInputTokens int64

	for stream.Next() {
		ev := stream.Current()

		switch ev.Type {
		case "message_start":
			ms := ev.AsMessageStart()
			slog.Debug(
				"Anthropic stream usage",
				"event_type", "message_start",
				"input_tokens", ms.Message.Usage.InputTokens,
				"output_tokens", ms.Message.Usage.OutputTokens,
				"cache_creation_input_tokens", ms.Message.Usage.CacheCreationInputTokens,
				"cache_read_input_tokens", ms.Message.Usage.CacheReadInputTokens,
			)
			cacheReadInputTokens = ms.Message.Usage.CacheReadInputTokens
			if ms.Message.Usage.InputTokens > 0 {
				totalInputTokens := int(ms.Message.Usage.InputTokens + ms.Message.Usage.CacheCreationInputTokens + ms.Message.Usage.CacheReadInputTokens)
				cachedTokens := int(ms.Message.Usage.CacheCreationInputTokens + ms.Message.Usage.CacheReadInputTokens)
				usage = &core.TokenUsage{
					InputTokens:  totalInputTokens,
					OutputTokens: int(ms.Message.Usage.OutputTokens),
					TotalTokens:  totalInputTokens + int(ms.Message.Usage.OutputTokens),
					CachedTokens: cachedTokens,
				}
			}

		case "message_delta":
			md := ev.AsMessageDelta()
			slog.Debug(
				"Anthropic stream usage",
				"event_type", "message_delta",
				"stop_reason", md.Delta.StopReason,
				"input_tokens", md.Usage.InputTokens,
				"output_tokens", md.Usage.OutputTokens,
				"cache_creation_input_tokens", md.Usage.CacheCreationInputTokens,
				"cache_read_input_tokens", md.Usage.CacheReadInputTokens,
			)
			cacheReadInputTokens = md.Usage.CacheReadInputTokens
			if usage == nil && md.Usage.InputTokens > 0 {
				usage = &core.TokenUsage{}
			}
			if usage != nil {
				totalInputTokens := int(md.Usage.InputTokens + md.Usage.CacheCreationInputTokens + md.Usage.CacheReadInputTokens)
				cachedTokens := int(md.Usage.CacheCreationInputTokens + md.Usage.CacheReadInputTokens)
				usage.InputTokens = totalInputTokens
				usage.OutputTokens = int(md.Usage.OutputTokens)
				usage.CachedTokens = cachedTokens
				usage.TotalTokens = totalInputTokens + usage.OutputTokens
			}

		case "content_block_start":
			cbs := ev.AsContentBlockStart()
			switch cbs.ContentBlock.Type {
			case "text":
				text := cbs.ContentBlock.AsText()
				blockStates[cbs.Index] = &anthropicContentBlockState{
					blockType: "text",
					text:      text.Text,
				}
			case "thinking":
				thinking := cbs.ContentBlock.AsThinking()
				blockStates[cbs.Index] = &anthropicContentBlockState{
					blockType: "thinking",
					thinking:  thinking.Thinking,
					signature: thinking.Signature,
				}
			case "redacted_thinking":
				redacted := cbs.ContentBlock.AsRedactedThinking()
				blockStates[cbs.Index] = &anthropicContentBlockState{
					blockType: "redacted_thinking",
					data:      redacted.Data,
				}
			case "tool_use":
				tu := cbs.ContentBlock.AsToolUse()
				var inputStart []byte
				if len(tu.Input) > 0 {
					inputStart = append(inputStart, tu.Input...)
				}
				blockStates[cbs.Index] = &anthropicContentBlockState{
					blockType:  "tool_use",
					id:         tu.ID,
					name:       tu.Name,
					inputStart: inputStart,
				}
			default:
				slog.Debug("Anthropic content block start unhandled", "block_type", cbs.ContentBlock.Type, "raw", cbs.ContentBlock.RawJSON())
			}

		case "content_block_delta":
			cbd := ev.AsContentBlockDelta()
			switch cbd.Delta.Type {
			case "text_delta":
				state, ok := blockStates[cbd.Index]
				if !ok {
					state = &anthropicContentBlockState{blockType: "text"}
					blockStates[cbd.Index] = state
				}
				state.text += cbd.Delta.Text
				if cbd.Delta.Text != "" {
					sendStreamEvent(ctx, eventCh, core.StreamEvent{
						Type:    core.StreamEventTypeChunk,
						Content: cbd.Delta.Text,
					})
				}
			case "thinking_delta":
				state, ok := blockStates[cbd.Index]
				if !ok {
					state = &anthropicContentBlockState{blockType: "thinking"}
					blockStates[cbd.Index] = state
				}
				state.thinking += cbd.Delta.Thinking
				if cbd.Delta.Thinking != "" {
					sendStreamEvent(ctx, eventCh, core.StreamEvent{
						Type:    core.StreamEventTypeReasoningChunk,
						Content: cbd.Delta.Thinking,
					})
				}
			case "signature_delta":
				state, ok := blockStates[cbd.Index]
				if !ok || state.blockType != "thinking" {
					continue
				}
				state.signature += cbd.Delta.Signature
			case "input_json_delta":
				state, ok := blockStates[cbd.Index]
				if !ok {
					state = &anthropicContentBlockState{blockType: "tool_use"}
					blockStates[cbd.Index] = state
				}
				state.inputBuffer = append(state.inputBuffer, []byte(cbd.Delta.PartialJSON)...)
			default:
				slog.Debug("Anthropic content block delta unhandled", "delta_type", cbd.Delta.Type, "raw", cbd.Delta.RawJSON())
			}

		case "content_block_stop":
			cbs := ev.AsContentBlockStop()
			if state, ok := blockStates[cbs.Index]; ok {
				switch state.blockType {
				case "text":
					if state.text != "" {
						assistantBlocks = append(assistantBlocks, anthropic.NewTextBlock(state.text))
					}
				case "thinking":
					assistantBlocks = append(assistantBlocks, anthropic.NewThinkingBlock(state.signature, state.thinking))
				case "redacted_thinking":
					assistantBlocks = append(assistantBlocks, anthropic.NewRedactedThinkingBlock(state.data))
				case "tool_use":
					if state.name == "" && state.id == "" {
						slog.Debug("Anthropic tool_use block missing name/id, ignoring", "index", cbs.Index)
						delete(blockStates, cbs.Index)
						continue
					}
					var inputRaw json.RawMessage = state.inputBuffer
					if len(inputRaw) == 0 {
						inputRaw = state.inputStart
					}
					if len(inputRaw) == 0 {
						inputRaw = json.RawMessage("{}")
					}
					assistantBlocks = append(assistantBlocks, anthropic.NewToolUseBlock(state.id, inputRaw, state.name))
				}
				delete(blockStates, cbs.Index)
			}
		case "message_stop":
			if len(blockStates) > 0 {
				slog.Debug("Anthropic stream stopped with unfinished content blocks", "count", len(blockStates))
			}
		default:
			slog.Debug("Anthropic stream event unhandled", "event_type", ev.Type, "raw", ev.RawJSON())
		}
	}
	_ = stream.Close()

	if err := stream.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("stream error: %w", err)
	}
	if len(blockStates) > 0 {
		slog.Debug("Anthropic stream ended with unfinished content blocks", "count", len(blockStates))
	}
	slog.Debug("Anthropic prompt cache hit", "cache_read_input_tokens", cacheReadInputTokens)

	var toolUses []toolUseEntry
	for _, block := range assistantBlocks {
		if block.OfToolUse == nil {
			continue
		}
		tu := block.OfToolUse
		var input map[string]any
		if err := json.Unmarshal(tu.Input.(json.RawMessage), &input); err != nil {
			slog.Debug("Anthropic tool_use input JSON malformed", "tool", tu.Name, "raw", string(tu.Input.(json.RawMessage)), "error", err)
			input = map[string]any{}
		}
		toolUses = append(toolUses, toolUseEntry{
			id:    tu.ID,
			name:  tu.Name,
			input: input,
		})
	}

	return assistantBlocks, toolUses, usage, nil
}

type toolUseEntry struct {
	id    string
	name  string
	input map[string]any
}

func anthropicThinkingParams(effort string) (anthropic.ThinkingConfigParamUnion, anthropic.OutputConfigParam, int64) {
	switch effort {
	case "low", "medium", "high", "xhigh", "max":
		return anthropic.ThinkingConfigParamUnion{
				OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
			}, anthropic.OutputConfigParam{
				Effort: anthropic.OutputConfigEffort(effort),
			}, anthropicMaxTokens
	case "adaptive":
		return anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		}, anthropic.OutputConfigParam{}, anthropicMaxTokens
	case "enabled":
		thinking := param.Override[anthropic.ThinkingConfigParamUnion](json.RawMessage(`{"type":"enabled"}`))
		return thinking, anthropic.OutputConfigParam{}, anthropicMaxTokens
	case "disabled":
		return anthropic.ThinkingConfigParamUnion{
			OfDisabled: &anthropic.ThinkingConfigDisabledParam{},
		}, anthropic.OutputConfigParam{}, anthropicMaxTokens
	default:
		return anthropic.ThinkingConfigParamUnion{}, anthropic.OutputConfigParam{}, anthropicMaxTokens
	}
}

func anthropicThinkingParamsForModel(provider providerconfig.Provider, model, effort string) (anthropic.ThinkingConfigParamUnion, anthropic.OutputConfigParam, int64) {
	if provider == providerconfig.Provider(config.ProviderMiniMax) {
		if model == "MiniMax-M3" {
			return anthropicThinkingParams(effort)
		}
		return anthropic.ThinkingConfigParamUnion{}, anthropic.OutputConfigParam{}, anthropicMaxTokens
	}
	if provider == providerconfig.Provider(config.ProviderOpenCodeGo) {
		if model == "minimax-m3" {
			return anthropicThinkingParams(effort)
		}
		if isOpenCodeGoMiniMaxModel(model) {
			return anthropic.ThinkingConfigParamUnion{}, anthropic.OutputConfigParam{}, anthropicMaxTokens
		}
	}

	return anthropicThinkingParams(effort)
}

func (c *AnthropicClient) proactivelyCompactHistory(
	ctx context.Context,
	compactionHistory *[]core.Message,
	msgParams *[]anthropic.MessageParam,
	injectedPending *[]anthropic.MessageParam,
	turnStartLen *int,
	streamOpts core.StreamOptions,
	toolRegistry *tools.Registry,
	hasNewToolTurns bool,
	autoCompactOff bool,
	eventCh chan<- core.StreamEvent,
) error {
	if streamOpts.DisableAutoCompaction || streamOpts.OneShot || !hasNewToolTurns || autoCompactOff || len(*injectedPending) > 0 ||
		!core.ShouldAutoCompact(
			estimateAnthropicInput(*msgParams),
			core.ContextInputBudget(c.contextWindowTokenCount),
		) {
		return nil
	}

	return c.compactHistory(ctx, compactionHistory, msgParams, injectedPending, turnStartLen, toolRegistry, streamOpts.SessionID, eventCh)
}

func estimateAnthropicInput(messages []anthropic.MessageParam) int {
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

func (c *AnthropicClient) compactHistory(
	ctx context.Context,
	compactionHistory *[]core.Message,
	msgParams *[]anthropic.MessageParam,
	injectedPending *[]anthropic.MessageParam,
	turnStartLen *int,
	toolRegistry *tools.Registry,
	sessionID string,
	eventCh chan<- core.StreamEvent,
) error {
	compactionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeAutoCompactionStarted, AutoCompaction: &core.AutoCompactionEvent{Cancel: cancel}})
	replacement, usage, err := AutoCompact(compactionCtx, c, *compactionHistory, toolRegistry, sessionID)
	if err != nil {
		eventType := core.StreamEventTypeAutoCompactionFailed
		if compaction.IsCancellation(err) {
			eventType = core.StreamEventTypeAutoCompactionCancelled
		}
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: eventType, AutoCompaction: &core.AutoCompactionEvent{Error: err, Usage: usage}})
		return err
	}

	_, *msgParams = toAnthropicMessages(replacement)
	*compactionHistory = replacement
	c.pendingState = nil
	*injectedPending = nil
	*turnStartLen = len(*msgParams)
	sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeAutoCompactionApplied, AutoCompaction: &core.AutoCompactionEvent{Replacement: replacement, Usage: usage}})
	return nil
}

func (c *AnthropicClient) StreamChat(
	ctx context.Context,
	messages []core.Message,
	toolRegistry *tools.Registry,
	opts ...core.StreamOptions,
) (<-chan core.StreamEvent, error) {
	eventCh := make(chan core.StreamEvent)
	streamOpts := streamOptions(opts)

	go func() {
		defer close(eventCh)

		systemBlocks, msgParams := toAnthropicMessages(messages)
		oneShot := streamOpts.OneShot
		var injectedPending []anthropic.MessageParam
		if !oneShot {
			msgParams, injectedPending = c.injectPendingState(msgParams)
		}
		turnStartLen := len(msgParams)
		anthropicTools := toAnthropicTools(toolRegistry)
		requestOpts := c.requestOptions(streamOpts)
		compactionHistory := core.CloneMessages(messages)
		autoCompactOff := false
		hasNewToolTurns := false

		for range maxToolTurns {
			if err := c.proactivelyCompactHistory(
				ctx, &compactionHistory, &msgParams, &injectedPending, &turnStartLen,
				streamOpts, toolRegistry, hasNewToolTurns, autoCompactOff, eventCh,
			); err != nil {
				autoCompactOff = true
			}

			turnSystem, turnTools, turnMessages := applyAnthropicBlockCacheControl(systemBlocks, anthropicTools, msgParams, turnStartLen)

			params := anthropic.MessageNewParams{
				Model:     c.model,
				MaxTokens: anthropicMaxTokens,
				Messages:  turnMessages,
			}
			if c.thinkingEffort != "" {
				params.Thinking, params.OutputConfig, params.MaxTokens = anthropicThinkingParamsForModel(c.provider, c.model, c.thinkingEffort)
			}
			if len(turnSystem) > 0 {
				params.System = turnSystem
			}
			if len(turnTools) > 0 {
				params.Tools = turnTools
			}

			slog.Debug("Anthropic turn input", "messages", formatAnthropicTurnInput(turnSystem, turnMessages))

			assistantBlocks, toolUses, usage, err := c.collectTurnWithRetry(ctx, params, eventCh, requestOpts...)
			if err != nil {
				c.exitIncomplete(ctx, eventCh, msgParams, turnStartLen, injectedPending, err, oneShot)
				return
			}

			if usage != nil {
				slog.Debug(
					"Anthropic usage emitted",
					"input_tokens", usage.InputTokens,
					"output_tokens", usage.OutputTokens,
					"total_tokens", usage.TotalTokens,
					"cached_tokens", usage.CachedTokens,
				)
				sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeUsage, Usage: usage})
			} else {
				slog.Debug("Anthropic usage unavailable for turn")
			}

			if len(toolUses) == 0 {
				sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeDone})
				return
			}

			msgParams = append(msgParams, anthropic.NewAssistantMessage(assistantBlocks...))

			execRegistry := toolRegistry
			if streamOpts.DisableToolCalls {
				execRegistry = denyToolRegistry(toolRegistry)
			} else if streamOpts.DisableWriteToolCalls {
				execRegistry = denyWriteToolRegistry(toolRegistry)
			}
			toolResultBlocks, activities := c.executeTools(ctx, toolUses, execRegistry, eventCh)
			msgParams = append(msgParams, anthropic.NewUserMessage(toolResultBlocks...))
			compactionHistory = append(compactionHistory, core.Message{
				Role:       core.RoleAssistant,
				Content:    anthropicAssistantText(assistantBlocks),
				TurnMemory: &core.TurnMemory{ToolActivity: activities},
			})
			hasNewToolTurns = true
			autoCompactOff = false
		}

		c.exitIncomplete(ctx, eventCh, msgParams, turnStartLen, injectedPending, nil, oneShot)
	}()

	return eventCh, nil
}

func apiKeyRefreshMiddleware(resolver *config.APIKeyResolver) option.Middleware {
	return func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		key, err := resolver.Get()
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Api-Key", key)

		res, err := next(req)
		if err != nil || res.StatusCode != http.StatusUnauthorized {
			return res, err
		}

		res.Body.Close()
		key, err = resolver.Refresh()
		if err != nil {
			return nil, err
		}
		clone := req.Clone(req.Context())
		if req.GetBody != nil {
			clone.Body, err = req.GetBody()
			if err != nil {
				return nil, err
			}
		}
		clone.Header.Set("X-Api-Key", key)
		return next(clone)
	}
}

func (c *AnthropicClient) requestOptions(opts core.StreamOptions) []option.RequestOption {
	var requestOpts []option.RequestOption
	for k, v := range c.headers {
		requestOpts = append(requestOpts, option.WithHeader(k, v))
	}
	if c.provider == providerconfig.Provider(config.ProviderOpenCodeGo) && opts.SessionID != "" {
		requestOpts = append(requestOpts, option.WithHeader("x-opencode-session", opencodeSessionID(opts.SessionID)))
	}
	return requestOpts
}

func (c *AnthropicClient) Reset() {
	c.pendingState = nil
}

func (c *AnthropicClient) injectPendingState(msgParams []anthropic.MessageParam) ([]anthropic.MessageParam, []anthropic.MessageParam) {
	if len(c.pendingState) == 0 {
		return msgParams, nil
	}

	injectedPending := append([]anthropic.MessageParam(nil), c.pendingState...)

	slog.Debug("Injecting pending state", "pending_messages", len(c.pendingState), "total_messages", len(msgParams))

	if len(msgParams) > 0 {
		last := msgParams[len(msgParams)-1]
		msgParams = append(msgParams[:len(msgParams)-1], injectedPending...)
		msgParams = append(msgParams, last)
	} else {
		msgParams = append(msgParams, injectedPending...)
	}
	c.pendingState = nil
	return msgParams, injectedPending
}

func (c *AnthropicClient) savePendingIfAccumulated(msgParams []anthropic.MessageParam, turnStartLen int, injectedPending []anthropic.MessageParam) {
	if len(injectedPending) == 0 && len(msgParams) <= turnStartLen {
		return
	}

	newDelta := []anthropic.MessageParam(nil)
	if len(msgParams) > turnStartLen {
		newDelta = msgParams[turnStartLen:]
	}

	c.pendingState = make([]anthropic.MessageParam, 0, len(injectedPending)+len(newDelta))
	c.pendingState = append(c.pendingState, injectedPending...)
	c.pendingState = append(c.pendingState, newDelta...)
}

func (c *AnthropicClient) emitTerminalEvent(
	ctx context.Context,
	eventCh chan<- core.StreamEvent,
	msgParams []anthropic.MessageParam,
	turnStartLen int,
	injectedPending []anthropic.MessageParam,
	err error,
) {
	if len(injectedPending) > 0 || len(msgParams) > turnStartLen {
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeIncomplete, Error: err})
	} else if err != nil {
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeError, Error: err})
	} else {
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeDone})
	}
}

func (c *AnthropicClient) exitIncomplete(
	ctx context.Context,
	eventCh chan<- core.StreamEvent,
	msgParams []anthropic.MessageParam,
	turnStartLen int,
	injectedPending []anthropic.MessageParam,
	err error,
	oneShot bool,
) {
	if !oneShot {
		c.savePendingIfAccumulated(msgParams, turnStartLen, injectedPending)
	}
	c.emitTerminalEvent(ctx, eventCh, msgParams, turnStartLen, injectedPending, err)
}

func anthropicAssistantText(blocks []anthropic.ContentBlockParamUnion) string {
	var text strings.Builder
	for _, block := range blocks {
		if block.OfText != nil {
			text.WriteString(block.OfText.Text)
		}
	}
	return text.String()
}

func (c *AnthropicClient) executeTools(
	ctx context.Context,
	toolUses []toolUseEntry,
	registry *tools.Registry,
	eventCh chan<- core.StreamEvent,
) ([]anthropic.ContentBlockParamUnion, []core.HistoricalToolActivity) {
	resultBlocks := make([]anthropic.ContentBlockParamUnion, 0, len(toolUses))
	activities := make([]core.HistoricalToolActivity, 0, len(toolUses))

	for _, tu := range toolUses {
		slog.Debug("Tool request", "tool", tu.name, "input", tu.input)
		execution := executeTool(ctx, registry, tu.name, tu.input, eventCh)
		resultContent := history.SerializeJSON(execution.LLMOutput)
		if execution.Err != nil {
			resultContent = history.SerializeJSON(map[string]any{"error": execution.Err.Error()})
		}
		resultBlocks = append(resultBlocks, anthropic.NewToolResultBlock(tu.id, resultContent, execution.Err != nil))
		activities = append(activities, execution.Activity)
	}

	return resultBlocks, activities
}
