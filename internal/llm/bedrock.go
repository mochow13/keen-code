package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/smithy-go/auth/bearer"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/user/keen-code/internal/tools"
)

const bedrockMaxTokens int32 = 64000

type bedrockStream interface {
	Events() <-chan brtypes.ConverseStreamOutput
	Close() error
	Err() error
}

type bedrockStreamFactory func(ctx context.Context, params *bedrockruntime.ConverseStreamInput) (bedrockStream, error)

func bedrockHeaderMiddleware(headers map[string]string) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		return stack.Build.Add(middleware.BuildMiddlewareFunc("CustomHeaders", func(ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler) (middleware.BuildOutput, middleware.Metadata, error) {
			if req, ok := in.Request.(*smithyhttp.Request); ok {
				for k, v := range headers {
					req.Header.Set(k, v)
				}
			}
			return next.HandleBuild(ctx, in)
		}), middleware.After)
	}
}

type BedrockClient struct {
	client                  *bedrockruntime.Client
	model                   string
	maxRetries              int
	streamImpl              bedrockStreamFactory
	pendingState            []brtypes.Message
	contextWindowTokenCount int
	thinkingEffort          string
	headers                 map[string]string
}

type bedrockContentBlockState struct {
	blockType   string
	id          string
	name        string
	text        string
	thinking    string
	signature   string
	data        []byte
	inputBuffer string
}

func NewBedrockClient(cfg *ClientConfig) (*BedrockClient, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	if awsCfg.Region == "" {
		awsCfg.Region = "us-east-1"
	}
	if cfg.APIKey != "" {
		awsCfg.BearerAuthTokenProvider = bearer.StaticTokenProvider{
			Token: bearer.Token{Value: cfg.APIKey},
		}
		awsCfg.AuthSchemePreference = []string{"httpBearerAuth"}
	}

	client := bedrockruntime.NewFromConfig(awsCfg, func(o *bedrockruntime.Options) {
		if cfg.BaseURL != "" {
			o.BaseEndpoint = aws.String(cfg.BaseURL)
		}
		if len(cfg.Headers) > 0 {
			o.APIOptions = append(o.APIOptions, bedrockHeaderMiddleware(cfg.Headers))
		}
	})

	c := &BedrockClient{
		client:                  client,
		model:                   cfg.Model,
		maxRetries:              retryCount(cfg.MaxRetries),
		contextWindowTokenCount: cfg.ContextWindowTokens,
		thinkingEffort:          cfg.ThinkingEffort,
		headers:                 cfg.Headers,
	}
	c.streamImpl = func(ctx context.Context, params *bedrockruntime.ConverseStreamInput) (bedrockStream, error) {
		out, err := c.client.ConverseStream(ctx, params)
		if err != nil {
			return nil, err
		}
		return out.GetStream(), nil
	}
	return c, nil
}

func toBedrockMessages(messages []Message) ([]brtypes.SystemContentBlock, []brtypes.Message) {
	var system []brtypes.SystemContentBlock
	var result []brtypes.Message

	for messageIndex, m := range messages {
		content := FormatMessageForProvider(m)
		switch m.Role {
		case RoleSystem:
			if content != "" {
				system = append(system, &brtypes.SystemContentBlockMemberText{Value: content})
			}
		case RoleUser:
			if content != "" {
				result = append(result, bedrockTextMessage(brtypes.ConversationRoleUser, content))
			}
		case RoleAssistant:
			for _, step := range historicalMessageSteps(messageIndex, m) {
				blocks := make([]brtypes.ContentBlock, 0, len(step.Activities)+1)
				if step.Text != "" {
					blocks = append(blocks, &brtypes.ContentBlockMemberText{Value: step.Text})
				}
				for _, invocation := range step.Activities {
					blocks = append(blocks, &brtypes.ContentBlockMemberToolUse{Value: brtypes.ToolUseBlock{
						ToolUseId: aws.String(invocation.ID),
						Name:      aws.String(invocation.Activity.Tool),
						Input:     document.NewLazyDocument(historicalToolInput(invocation.Activity)),
					}})
				}
				if len(blocks) > 0 {
					result = append(result, brtypes.Message{Role: brtypes.ConversationRoleAssistant, Content: blocks})
				}
				if len(step.Activities) > 0 {
					resultBlocks := make([]brtypes.ContentBlock, 0, len(step.Activities))
					for _, invocation := range step.Activities {
						status := brtypes.ToolResultStatusSuccess
						if invocation.Activity.Status != "success" {
							status = brtypes.ToolResultStatusError
						}
						resultBlocks = append(resultBlocks, &brtypes.ContentBlockMemberToolResult{Value: brtypes.ToolResultBlock{
							ToolUseId: aws.String(invocation.ID),
							Content: []brtypes.ToolResultContentBlock{
								&brtypes.ToolResultContentBlockMemberText{Value: historicalToolResult(invocation.Activity)},
							},
							Status: status,
						}})
					}
					result = append(result, brtypes.Message{Role: brtypes.ConversationRoleUser, Content: resultBlocks})
				}
			}
		}
	}

	return system, result
}

func bedrockTextMessage(role brtypes.ConversationRole, text string) brtypes.Message {
	return brtypes.Message{
		Role: role,
		Content: []brtypes.ContentBlock{
			&brtypes.ContentBlockMemberText{Value: text},
		},
	}
}

func bedrockThinkingFields(effort string) document.Interface {
	switch effort {
	case "low", "medium", "high", "xhigh", "max":
		return document.NewLazyDocument(map[string]any{
			"thinking":      map[string]any{"type": "adaptive"},
			"output_config": map[string]any{"effort": effort},
		})
	default:
		return nil
	}
}

func bedrockCachePoint() brtypes.CachePointBlock {
	return brtypes.CachePointBlock{Type: brtypes.CachePointTypeDefault}
}

func applyBedrockPromptCaching(system []brtypes.SystemContentBlock, toolConfig *brtypes.ToolConfiguration, oneShot bool) []brtypes.SystemContentBlock {
	if oneShot {
		return system
	}
	if len(system) > 0 {
		system = append(system, &brtypes.SystemContentBlockMemberCachePoint{Value: bedrockCachePoint()})
	}
	if toolConfig != nil && len(toolConfig.Tools) > 0 {
		toolConfig.Tools = append(toolConfig.Tools, &brtypes.ToolMemberCachePoint{Value: bedrockCachePoint()})
	}
	return system
}

func applyBedrockMessageCaching(messages []brtypes.Message, stableMessageCount int, oneShot bool) []brtypes.Message {
	result := append([]brtypes.Message(nil), messages...)
	if oneShot || len(result) == 0 {
		return result
	}

	stableIdx := stableMessageCount - 1
	if stableIdx >= 0 && stableIdx < len(result) {
		result = addBedrockMessageCachePoint(result, stableIdx)
	}
	lastIdx := len(result) - 1
	if lastIdx != stableIdx {
		result = addBedrockMessageCachePoint(result, lastIdx)
	}
	return result
}

func addBedrockMessageCachePoint(messages []brtypes.Message, idx int) []brtypes.Message {
	messages[idx].Content = append([]brtypes.ContentBlock(nil), messages[idx].Content...)
	messages[idx].Content = append(messages[idx].Content, &brtypes.ContentBlockMemberCachePoint{Value: bedrockCachePoint()})
	return messages
}

func toBedrockTools(registry *tools.Registry) *brtypes.ToolConfiguration {
	if registry == nil {
		return nil
	}

	all := registry.All()
	if len(all) == 0 {
		return nil
	}

	result := make([]brtypes.Tool, 0, len(all))
	for _, t := range all {
		description := t.Description()
		name := t.Name()
		result = append(result, &brtypes.ToolMemberToolSpec{
			Value: brtypes.ToolSpecification{
				Name:        aws.String(name),
				Description: aws.String(description),
				InputSchema: &brtypes.ToolInputSchemaMemberJson{Value: document.NewLazyDocument(t.InputSchema())},
			},
		})
	}

	return &brtypes.ToolConfiguration{Tools: result}
}

func (c *BedrockClient) StreamChat(
	ctx context.Context,
	messages []Message,
	toolRegistry *tools.Registry,
	opts ...StreamOptions,
) (<-chan StreamEvent, error) {
	eventCh := make(chan StreamEvent)
	streamOpts := streamOptions(opts)

	go func() {
		defer close(eventCh)

		system, msgParams := toBedrockMessages(messages)
		oneShot := streamOpts.OneShot
		var injectedPending []brtypes.Message
		if !oneShot {
			msgParams, injectedPending = c.injectPendingState(msgParams)
		}
		turnStartLen := len(msgParams)
		toolConfig := toBedrockTools(toolRegistry)
		compactionHistory := CloneMessages(messages)
		autoCompactOff := false
		forcedRecoveryUsed := false
		hasNewToolTurns := false

		for range maxToolTurns {
			if err := c.proactivelyCompactHistory(
				ctx, &compactionHistory, &msgParams, &injectedPending, &turnStartLen,
				streamOpts, hasNewToolTurns, autoCompactOff, eventCh,
			); err != nil {
				autoCompactOff = true
			}

			reducedMessages, compactionAttempted, err := c.reduceContextOrCompact(
				ctx, &compactionHistory, &msgParams, &injectedPending, &turnStartLen,
				streamOpts, forcedRecoveryUsed, eventCh,
			)
			if err != nil {
				if compactionAttempted {
					c.exitIncomplete(eventCh, msgParams, turnStartLen, injectedPending, err, oneShot)
				} else {
					c.pendingState = nil
					c.emitTerminalEvent(eventCh, msgParams, turnStartLen, injectedPending, err)
				}
				return
			}
			if compactionAttempted {
				forcedRecoveryUsed = true
				continue
			}
			msgParams = reducedMessages

			turnToolConfig := cloneBedrockToolConfig(toolConfig)
			turnSystem := append([]brtypes.SystemContentBlock(nil), system...)
			turnSystem = applyBedrockPromptCaching(turnSystem, turnToolConfig, oneShot)
			turnMessages := applyBedrockMessageCaching(msgParams, turnStartLen, oneShot)

			params := &bedrockruntime.ConverseStreamInput{
				ModelId:                      aws.String(c.model),
				Messages:                     turnMessages,
				AdditionalModelRequestFields: bedrockThinkingFields(c.thinkingEffort),
				InferenceConfig: &brtypes.InferenceConfiguration{
					MaxTokens: aws.Int32(bedrockMaxTokens),
				},
			}
			if len(turnSystem) > 0 {
				params.System = turnSystem
			}
			if turnToolConfig != nil && len(turnToolConfig.Tools) > 0 {
				params.ToolConfig = turnToolConfig
			}

			assistantBlocks, toolUses, usage, err := c.collectTurnWithRetry(ctx, params, eventCh)
			if err != nil {
				c.exitIncomplete(eventCh, msgParams, turnStartLen, injectedPending, err, oneShot)
				return
			}

			if usage != nil {
				slog.Debug(
					"Bedrock usage emitted",
					"input_tokens", usage.InputTokens,
					"output_tokens", usage.OutputTokens,
					"total_tokens", usage.TotalTokens,
					"cached_tokens", usage.CachedTokens,
				)
				eventCh <- StreamEvent{Type: StreamEventTypeUsage, Usage: usage}
			}

			if len(toolUses) == 0 {
				eventCh <- StreamEvent{Type: StreamEventTypeDone}
				return
			}

			msgParams = append(msgParams, brtypes.Message{
				Role:    brtypes.ConversationRoleAssistant,
				Content: assistantBlocks,
			})
			toolResults, activities := c.executeTools(ctx, toolUses, toolRegistry, eventCh)
			msgParams = append(msgParams, brtypes.Message{Role: brtypes.ConversationRoleUser, Content: toolResults})
			compactionHistory = append(compactionHistory, Message{
				Role:       RoleAssistant,
				Content:    bedrockAssistantText(assistantBlocks),
				TurnMemory: &TurnMemory{ToolActivity: activities},
			})
			hasNewToolTurns = true
			autoCompactOff = false
		}

		c.exitIncomplete(eventCh, msgParams, turnStartLen, injectedPending, nil, oneShot)
	}()

	return eventCh, nil
}

func (c *BedrockClient) proactivelyCompactHistory(
	ctx context.Context,
	compactionHistory *[]Message,
	msgParams *[]brtypes.Message,
	injectedPending *[]brtypes.Message,
	turnStartLen *int,
	streamOpts StreamOptions,
	hasNewToolTurns bool,
	autoCompactOff bool,
	eventCh chan<- StreamEvent,
) error {
	if streamOpts.DisableAutoCompaction || streamOpts.OneShot || !hasNewToolTurns || autoCompactOff || len(*injectedPending) > 0 ||
		!shouldAutoCompact(
			estimateBedrockMessagesTokenCount(*msgParams),
			contextInputBudget(c.contextWindowTokenCount),
		) {
		return nil
	}
	return c.compactHistory(ctx, compactionHistory, msgParams, injectedPending, turnStartLen, streamOpts.SessionID, eventCh)
}

func (c *BedrockClient) reduceContextOrCompact(
	ctx context.Context,
	compactionHistory *[]Message,
	msgParams *[]brtypes.Message,
	injectedPending *[]brtypes.Message,
	turnStartLen *int,
	streamOpts StreamOptions,
	forcedRecoveryUsed bool,
	eventCh chan<- StreamEvent,
) ([]brtypes.Message, bool, error) {
	reducedMessages, reduction := reduceBedrockContextForRequest(c.contextWindowTokenCount, *msgParams)
	if reduction.FitsBudget {
		return reducedMessages, false, nil
	}

	slog.Debug("Bedrock context still exceeds budget after reduction", "inputTokenCount", reduction.ReducedTokenCount, "removedToolResultCount", reduction.RemovedToolResults)
	if streamOpts.DisableAutoCompaction || streamOpts.OneShot || forcedRecoveryUsed || len(*injectedPending) > 0 {
		return nil, false, fmt.Errorf("%w: %s", ErrContextWindowExceeded, contextWindowExceededError)
	}
	if err := c.compactHistory(ctx, compactionHistory, msgParams, injectedPending, turnStartLen, streamOpts.SessionID, eventCh); err != nil {
		return nil, true, fmt.Errorf("%w: automatic compaction failed: %v", ErrContextWindowExceeded, err)
	}
	return nil, true, nil
}

func (c *BedrockClient) compactHistory(
	ctx context.Context,
	compactionHistory *[]Message,
	msgParams *[]brtypes.Message,
	injectedPending *[]brtypes.Message,
	turnStartLen *int,
	sessionID string,
	eventCh chan<- StreamEvent,
) error {
	compactionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	eventCh <- StreamEvent{Type: StreamEventTypeAutoCompactionStarted, AutoCompaction: &AutoCompactionEvent{Cancel: cancel}}
	replacement, usage, err := AutoCompact(compactionCtx, c, *compactionHistory, sessionID)
	if err != nil {
		eventType := StreamEventTypeAutoCompactionFailed
		if isAutoCompactionCancellation(err) {
			eventType = StreamEventTypeAutoCompactionCancelled
		}
		eventCh <- StreamEvent{Type: eventType, AutoCompaction: &AutoCompactionEvent{Error: err, Usage: usage}}
		return err
	}

	_, *msgParams = toBedrockMessages(replacement)
	*compactionHistory = replacement
	*injectedPending = nil
	*turnStartLen = len(*msgParams)
	c.pendingState = nil
	eventCh <- StreamEvent{Type: StreamEventTypeAutoCompactionApplied, AutoCompaction: &AutoCompactionEvent{Replacement: replacement, Usage: usage}}
	return nil
}

func bedrockAssistantText(assistant []brtypes.ContentBlock) string {
	var text strings.Builder
	for _, block := range assistant {
		if block, ok := block.(*brtypes.ContentBlockMemberText); ok {
			text.WriteString(block.Value)
		}
	}
	return text.String()
}

func cloneBedrockToolConfig(toolConfig *brtypes.ToolConfiguration) *brtypes.ToolConfiguration {
	if toolConfig == nil {
		return nil
	}
	cloned := &brtypes.ToolConfiguration{
		ToolChoice: toolConfig.ToolChoice,
	}
	if len(toolConfig.Tools) > 0 {
		cloned.Tools = append([]brtypes.Tool(nil), toolConfig.Tools...)
	}
	return cloned
}

func (c *BedrockClient) collectTurnWithRetry(
	ctx context.Context,
	params *bedrockruntime.ConverseStreamInput,
	eventCh chan<- StreamEvent,
) ([]brtypes.ContentBlock, []toolUseEntry, *TokenUsage, error) {
	maxRetries := retryCount(c.maxRetries)
	for attempt := 1; attempt <= maxRetries; attempt++ {
		assistantBlocks, toolUses, usage, err := c.collectTurn(ctx, params, eventCh)
		if err == nil {
			return assistantBlocks, toolUses, usage, nil
		}
		if !isRetryableError(err) || attempt == maxRetries {
			return nil, nil, nil, err
		}

		backoff := time.Duration(attempt) * time.Second
		slog.Debug("Bedrock stream error, retrying", "attempt", attempt, "maxRetries", maxRetries, "backoff", backoff, "error", err)
		eventCh <- StreamEvent{Type: StreamEventTypeRetry, Error: err, Attempt: attempt}

		select {
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return nil, nil, nil, nil
}

func (c *BedrockClient) collectTurn(
	ctx context.Context,
	params *bedrockruntime.ConverseStreamInput,
	eventCh chan<- StreamEvent,
) ([]brtypes.ContentBlock, []toolUseEntry, *TokenUsage, error) {
	stream, err := c.streamImpl(ctx, params)
	if err != nil {
		return nil, nil, nil, err
	}
	defer stream.Close()

	blockStates := map[int32]*bedrockContentBlockState{}
	var assistantBlocks []brtypes.ContentBlock
	var toolUses []toolUseEntry
	var usage *TokenUsage
	var cacheReadInputTokens int32

	for ev := range stream.Events() {
		switch e := ev.(type) {
		case *brtypes.ConverseStreamOutputMemberContentBlockStart:
			idx := aws.ToInt32(e.Value.ContentBlockIndex)
			state := blockStates[idx]
			if state == nil {
				state = &bedrockContentBlockState{}
				blockStates[idx] = state
			}
			switch start := e.Value.Start.(type) {
			case *brtypes.ContentBlockStartMemberToolUse:
				state.blockType = "tool_use"
				state.id = aws.ToString(start.Value.ToolUseId)
				state.name = aws.ToString(start.Value.Name)
			}

		case *brtypes.ConverseStreamOutputMemberContentBlockDelta:
			idx := aws.ToInt32(e.Value.ContentBlockIndex)
			state := blockStates[idx]
			if state == nil {
				state = &bedrockContentBlockState{}
				blockStates[idx] = state
			}
			switch delta := e.Value.Delta.(type) {
			case *brtypes.ContentBlockDeltaMemberText:
				state.blockType = "text"
				state.text += delta.Value
				eventCh <- StreamEvent{Type: StreamEventTypeChunk, Content: delta.Value}
			case *brtypes.ContentBlockDeltaMemberReasoningContent:
				switch reasoning := delta.Value.(type) {
				case *brtypes.ReasoningContentBlockDeltaMemberText:
					state.blockType = "reasoning"
					state.thinking += reasoning.Value
					eventCh <- StreamEvent{Type: StreamEventTypeReasoningChunk, Content: reasoning.Value}
				case *brtypes.ReasoningContentBlockDeltaMemberSignature:
					state.blockType = "reasoning"
					state.signature = reasoning.Value
				case *brtypes.ReasoningContentBlockDeltaMemberRedactedContent:
					state.blockType = "redacted_reasoning"
					state.data = append(state.data, reasoning.Value...)
				}
			case *brtypes.ContentBlockDeltaMemberToolUse:
				state.blockType = "tool_use"
				state.inputBuffer += aws.ToString(delta.Value.Input)
			}

		case *brtypes.ConverseStreamOutputMemberContentBlockStop:
			idx := aws.ToInt32(e.Value.ContentBlockIndex)
			state := blockStates[idx]
			if state == nil {
				continue
			}
			switch state.blockType {
			case "text":
				if state.text != "" {
					assistantBlocks = append(assistantBlocks, &brtypes.ContentBlockMemberText{Value: state.text})
				}
			case "reasoning":
				if state.thinking != "" {
					var signature *string
					if state.signature != "" {
						signature = aws.String(state.signature)
					}
					assistantBlocks = append(assistantBlocks, &brtypes.ContentBlockMemberReasoningContent{
						Value: &brtypes.ReasoningContentBlockMemberReasoningText{
							Value: brtypes.ReasoningTextBlock{
								Text:      aws.String(state.thinking),
								Signature: signature,
							},
						},
					})
				}
			case "redacted_reasoning":
				if len(state.data) > 0 {
					assistantBlocks = append(assistantBlocks, &brtypes.ContentBlockMemberReasoningContent{
						Value: &brtypes.ReasoningContentBlockMemberRedactedContent{Value: append([]byte(nil), state.data...)},
					})
				}
			case "tool_use":
				input := bedrockToolInput(state.inputBuffer)
				assistantBlocks = append(assistantBlocks, &brtypes.ContentBlockMemberToolUse{
					Value: brtypes.ToolUseBlock{
						ToolUseId: aws.String(state.id),
						Name:      aws.String(state.name),
						Input:     document.NewLazyDocument(input),
					},
				})
				toolUses = append(toolUses, toolUseEntry{
					id:    state.id,
					name:  state.name,
					input: input,
				})
			}
			delete(blockStates, idx)

		case *brtypes.ConverseStreamOutputMemberMetadata:
			if e.Value.Usage != nil {
				slog.Debug(
					"Bedrock stream usage",
					"event_type", "metadata",
					"input_tokens", aws.ToInt32(e.Value.Usage.InputTokens),
					"output_tokens", aws.ToInt32(e.Value.Usage.OutputTokens),
					"total_tokens", aws.ToInt32(e.Value.Usage.TotalTokens),
					"cache_read_input_tokens", aws.ToInt32(e.Value.Usage.CacheReadInputTokens),
					"cache_write_input_tokens", aws.ToInt32(e.Value.Usage.CacheWriteInputTokens),
				)
				cacheReadInputTokens = aws.ToInt32(e.Value.Usage.CacheReadInputTokens)
			}
			usage = bedrockUsage(e.Value.Usage)
		}
	}

	if err := stream.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("stream error: %w", err)
	}
	slog.Debug("Bedrock prompt cache hit", "cache_read_input_tokens", cacheReadInputTokens)

	return assistantBlocks, toolUses, usage, nil
}

func bedrockToolInput(raw string) map[string]any {
	if raw == "" {
		return map[string]any{}
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return map[string]any{}
	}
	return input
}

func bedrockUsage(usage *brtypes.TokenUsage) *TokenUsage {
	if usage == nil {
		return nil
	}
	inputTokens := int(aws.ToInt32(usage.InputTokens))
	outputTokens := int(aws.ToInt32(usage.OutputTokens))
	cachedTokens := int(aws.ToInt32(usage.CacheReadInputTokens) + aws.ToInt32(usage.CacheWriteInputTokens))
	totalInputTokens := inputTokens + cachedTokens
	return &TokenUsage{
		InputTokens:  totalInputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  totalInputTokens + outputTokens,
		CachedTokens: cachedTokens,
	}
}

func (c *BedrockClient) Reset() {
	c.pendingState = nil
}

func (c *BedrockClient) injectPendingState(msgParams []brtypes.Message) ([]brtypes.Message, []brtypes.Message) {
	if len(c.pendingState) == 0 {
		return msgParams, nil
	}

	injectedPending := append([]brtypes.Message(nil), c.pendingState...)
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

func (c *BedrockClient) savePendingIfAccumulated(msgParams []brtypes.Message, turnStartLen int, injectedPending []brtypes.Message) {
	if len(injectedPending) == 0 && len(msgParams) <= turnStartLen {
		return
	}

	newDelta := []brtypes.Message(nil)
	if len(msgParams) > turnStartLen {
		newDelta = msgParams[turnStartLen:]
	}

	c.pendingState = make([]brtypes.Message, 0, len(injectedPending)+len(newDelta))
	c.pendingState = append(c.pendingState, injectedPending...)
	c.pendingState = append(c.pendingState, newDelta...)
}

func (c *BedrockClient) emitTerminalEvent(eventCh chan<- StreamEvent, msgParams []brtypes.Message, turnStartLen int, injectedPending []brtypes.Message, err error) {
	if len(injectedPending) > 0 || len(msgParams) > turnStartLen {
		eventCh <- StreamEvent{Type: StreamEventTypeIncomplete, Error: err}
	} else if err != nil {
		eventCh <- StreamEvent{Type: StreamEventTypeError, Error: err}
	} else {
		eventCh <- StreamEvent{Type: StreamEventTypeDone}
	}
}

func (c *BedrockClient) exitIncomplete(eventCh chan<- StreamEvent, msgParams []brtypes.Message, turnStartLen int, injectedPending []brtypes.Message, err error, oneShot bool) {
	if !oneShot {
		c.savePendingIfAccumulated(msgParams, turnStartLen, injectedPending)
	}
	c.emitTerminalEvent(eventCh, msgParams, turnStartLen, injectedPending, err)
}

func (c *BedrockClient) executeTools(
	ctx context.Context,
	toolUses []toolUseEntry,
	registry *tools.Registry,
	eventCh chan<- StreamEvent,
) ([]brtypes.ContentBlock, []HistoricalToolActivity) {
	resultBlocks := make([]brtypes.ContentBlock, 0, len(toolUses))
	activities := make([]HistoricalToolActivity, 0, len(toolUses))

	for _, tu := range toolUses {
		start := time.Now()

		slog.Debug("Tool request", "tool", tu.name, "input", tu.input)

		output, execErr, toolStarted := executeValidatedTool(ctx, registry, tu.name, tu.input, eventCh)

		duration := time.Since(start)
		toolCall := &ToolCall{
			Name:     tu.name,
			Input:    tu.input,
			Output:   output,
			Duration: duration,
		}

		status := brtypes.ToolResultStatusSuccess
		content := []brtypes.ToolResultContentBlock{}
		if execErr != nil {
			toolCall.Error = execErr.Error()
			status = brtypes.ToolResultStatusError
			slog.Debug("Tool response", "tool", tu.name, "error", execErr.Error(), "duration", duration)
			if toolStarted {
				eventCh <- StreamEvent{
					Type:     StreamEventTypeToolEnd,
					ToolCall: toolCall,
				}
			}
			content = append(content, &brtypes.ToolResultContentBlockMemberJson{Value: document.NewLazyDocument(map[string]any{"error": execErr.Error()})})
		} else {
			slog.Debug("Tool response", "tool", tu.name, "duration", duration)
			eventCh <- StreamEvent{
				Type:     StreamEventTypeToolEnd,
				ToolCall: toolCall,
			}
			if output == nil {
				output = map[string]any{}
			}
			content = append(content, &brtypes.ToolResultContentBlockMemberJson{Value: document.NewLazyDocument(output)})
		}

		resultBlocks = append(resultBlocks, &brtypes.ContentBlockMemberToolResult{
			Value: brtypes.ToolResultBlock{
				ToolUseId: aws.String(tu.id),
				Content:   content,
				Status:    status,
			},
		})
		activities = append(activities, historicalToolActivity(tu.name, tu.input, output, execErr))
	}

	return resultBlocks, activities
}
