package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/tools"
	"google.golang.org/genai"
)

const maxToolTurns = 5000

type streamFunc func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error]

type GenkitClient struct {
	g                       *genkit.Genkit
	provider                Provider
	model                   string
	thinkingEffort          string
	maxRetries              int
	streamImpl              streamFunc
	pendingState            []*ai.Message
	contextWindowTokenCount int
	headers                 map[string]string
}

func NewGenkitClient(cfg *ClientConfig) (*GenkitClient, error) {
	ctx := context.Background()

	var g *genkit.Genkit
	var modelName string

	switch cfg.Provider {
	case config.ProviderGoogleAI:
		g = genkit.Init(ctx, genkit.WithPlugins(&googlegenai.GoogleAI{
			APIKey: cfg.APIKey,
		}))
		modelName = "googleai/" + cfg.Model
	default:
		return nil, fmt.Errorf("unsupported provider in config: %s. %s", cfg.Provider, config.ConfigFixHint)
	}

	if g == nil {
		return nil, fmt.Errorf("failed to initialize genkit")
	}

	return &GenkitClient{
		g:                       g,
		provider:                cfg.Provider,
		model:                   modelName,
		thinkingEffort:          cfg.ThinkingEffort,
		maxRetries:              retryCount(cfg.MaxRetries),
		contextWindowTokenCount: cfg.ContextWindowTokens,
		streamImpl:              genkit.GenerateStream,
		headers:                 cfg.Headers,
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
	var aiMessages []*ai.Message
	for messageIndex, m := range messages {
		if m.Role != RoleAssistant {
			aiMessages = append(aiMessages, &ai.Message{
				Role:    toGenkitRole(m.Role),
				Content: []*ai.Part{ai.NewTextPart(FormatMessageForProvider(m))},
			})
			continue
		}

		for _, step := range historicalMessageSteps(messageIndex, m) {
			parts := make([]*ai.Part, 0, len(step.Activities)+1)
			if step.Text != "" {
				parts = append(parts, ai.NewTextPart(step.Text))
			}
			for _, invocation := range step.Activities {
				parts = append(parts, ai.NewToolRequestPart(&ai.ToolRequest{
					Name:  invocation.Activity.Tool,
					Ref:   invocation.ID,
					Input: historicalToolInput(invocation.Activity),
				}))
			}
			if len(parts) > 0 {
				aiMessages = append(aiMessages, &ai.Message{Role: ai.RoleModel, Content: parts})
			}
			if len(step.Activities) > 0 {
				responses := make([]*ai.Part, 0, len(step.Activities))
				for _, invocation := range step.Activities {
					responses = append(responses, ai.NewToolResponsePart(&ai.ToolResponse{
						Name:   invocation.Activity.Tool,
						Ref:    invocation.ID,
						Output: historicalToolResult(invocation.Activity),
					}))
				}
				aiMessages = append(aiMessages, &ai.Message{Role: ai.RoleTool, Content: responses})
			}
		}
	}
	return aiMessages
}

func thinkingLevelForEffort(effort string) genai.ThinkingLevel {
	switch effort {
	case "minimal":
		return genai.ThinkingLevelMinimal
	case "low":
		return genai.ThinkingLevelLow
	case "medium":
		return genai.ThinkingLevelMedium
	case "high":
		return genai.ThinkingLevelHigh
	default:
		return genai.ThinkingLevelUnspecified
	}
}

func buildGenkitGenerateConfig(thinkingEffort string, provider Provider, headers map[string]string) *genai.GenerateContentConfig {
	var cfg *genai.GenerateContentConfig
	if thinkingEffort != "" && provider == Provider(config.ProviderGoogleAI) {
		level := thinkingLevelForEffort(thinkingEffort)
		if level != genai.ThinkingLevelUnspecified {
			cfg = &genai.GenerateContentConfig{
				ThinkingConfig: &genai.ThinkingConfig{
					IncludeThoughts: true,
					ThinkingLevel:   level,
				},
			}
		}
	}
	if len(headers) > 0 {
		if cfg == nil {
			cfg = &genai.GenerateContentConfig{}
		}
		cfg.HTTPOptions = &genai.HTTPOptions{
			Headers: make(http.Header, len(headers)),
		}
		for k, v := range headers {
			cfg.HTTPOptions.Headers.Set(k, v)
		}
	}
	return cfg
}

func (c *GenkitClient) collectTurnWithRetry(
	ctx context.Context,
	opts []ai.GenerateOption,
	eventCh chan<- StreamEvent,
) (*ai.ModelResponse, error) {
	maxRetries := retryCount(c.maxRetries)
	for attempt := 1; attempt <= maxRetries; attempt++ {
		modelResponse, err := c.collectTurn(ctx, opts, eventCh)
		if err == nil {
			return modelResponse, nil
		}
		if !isRetryableError(err) || attempt == maxRetries {
			return nil, err
		}

		backoff := time.Duration(attempt) * time.Second
		slog.Debug("LLM stream error, retrying", "attempt", attempt, "maxRetries", maxRetries, "backoff", backoff, "error", err)
		eventCh <- StreamEvent{Type: StreamEventTypeRetry, Error: err, Attempt: attempt}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return nil, nil
}

func (c *GenkitClient) collectTurn(
	ctx context.Context,
	opts []ai.GenerateOption,
	eventCh chan<- StreamEvent,
) (*ai.ModelResponse, error) {
	stream := c.streamImpl(ctx, c.g, opts...)
	var modelResponse *ai.ModelResponse

	for result, err := range stream {
		if err != nil {
			return nil, err
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

	return modelResponse, nil
}

func (c *GenkitClient) StreamChat(
	ctx context.Context,
	messages []Message,
	toolRegistry *tools.Registry,
	opts ...StreamOptions,
) (<-chan StreamEvent, error) {
	eventCh := make(chan StreamEvent)

	go func() {
		defer close(eventCh)

		streamOpts := streamOptions(opts)
		oneShot := streamOpts.OneShot
		compactionHistory := CloneMessages(messages)
		aiMessages := toGenkitMessages(compactionHistory)
		var injectedPending []*ai.Message
		if !oneShot {
			aiMessages, injectedPending = c.injectPendingState(aiMessages)
		}
		turnStartLen := len(aiMessages)
		autoCompactOff := false
		forcedRecoveryUsed := false
		hasNewToolTurns := false

		var genkitTools []ai.ToolRef
		if toolRegistry != nil && toolRegistry.Count() > 0 {
			genkitTools = ToGenkitTools(toolRegistry)
		}

		for range maxToolTurns {
			if err := c.proactivelyCompactHistory(
				ctx, &compactionHistory, &aiMessages, &injectedPending, &turnStartLen,
				streamOpts, hasNewToolTurns, autoCompactOff, eventCh,
			); err != nil {
				autoCompactOff = true
			}

			reducedMessages, compactionAttempted, err := c.reduceContextOrCompact(
				ctx, &compactionHistory, &aiMessages, &injectedPending, &turnStartLen,
				streamOpts, forcedRecoveryUsed, eventCh,
			)
			if err != nil {
				if compactionAttempted {
					c.exitIncomplete(eventCh, aiMessages, turnStartLen, injectedPending, err, oneShot)
				} else {
					c.pendingState = nil
					c.emitTerminalEvent(eventCh, aiMessages, turnStartLen, injectedPending, err)
				}
				return
			}
			if compactionAttempted {
				forcedRecoveryUsed = true
				continue
			}
			aiMessages = reducedMessages

			opts := []ai.GenerateOption{
				ai.WithModelName(c.model),
				ai.WithMessages(aiMessages...),
			}

			if genCfg := buildGenkitGenerateConfig(c.thinkingEffort, c.provider, c.headers); genCfg != nil {
				opts = append(opts, ai.WithConfig(genCfg))
			}

			if len(genkitTools) > 0 {
				opts = append(opts, ai.WithTools(genkitTools...))
				opts = append(opts, ai.WithReturnToolRequests(true))
			}

			modelResponse, err := c.collectTurnWithRetry(ctx, opts, eventCh)
			if err != nil {
				c.exitIncomplete(eventCh, aiMessages, turnStartLen, injectedPending, err, oneShot)
				return
			}

			if modelResponse == nil || modelResponse.Message == nil {
				c.exitIncomplete(eventCh, aiMessages, turnStartLen, injectedPending, nil, oneShot)
				return
			}

			if modelResponse.Usage != nil && (modelResponse.Usage.InputTokens > 0 || modelResponse.Usage.OutputTokens > 0) {
				eventCh <- StreamEvent{
					Type: StreamEventTypeUsage,
					Usage: &TokenUsage{
						InputTokens:  modelResponse.Usage.InputTokens,
						OutputTokens: modelResponse.Usage.OutputTokens,
						TotalTokens:  modelResponse.Usage.TotalTokens,
					},
				}
			}

			toolRequests := modelResponse.ToolRequests()
			if len(toolRequests) == 0 {
				eventCh <- StreamEvent{Type: StreamEventTypeDone}
				return
			}

			aiMessages = append(aiMessages, modelResponse.Message)

			toolResponseParts, activities := c.executeTools(ctx, toolRequests, toolRegistry, eventCh)
			if len(toolResponseParts) > 0 {
				toolMsg := &ai.Message{
					Role:    ai.RoleTool,
					Content: toolResponseParts,
				}
				aiMessages = append(aiMessages, toolMsg)
			}
			compactionHistory = append(compactionHistory, Message{
				Role:       RoleAssistant,
				Content:    genkitAssistantText(modelResponse.Message),
				TurnMemory: &TurnMemory{ToolActivity: activities},
			})
			hasNewToolTurns = true
			autoCompactOff = false
		}

		c.exitIncomplete(eventCh, aiMessages, turnStartLen, injectedPending, nil, oneShot)
	}()

	return eventCh, nil
}

func (c *GenkitClient) proactivelyCompactHistory(
	ctx context.Context,
	compactionHistory *[]Message,
	aiMessages *[]*ai.Message,
	injectedPending *[]*ai.Message,
	turnStartLen *int,
	streamOpts StreamOptions,
	hasNewToolTurns bool,
	autoCompactOff bool,
	eventCh chan<- StreamEvent,
) error {
	if streamOpts.DisableAutoCompaction || streamOpts.OneShot || !hasNewToolTurns || autoCompactOff || len(*injectedPending) > 0 ||
		!shouldAutoCompact(estimateGenkitMessagesTokenCount(*aiMessages), contextInputBudget(c.contextWindowTokenCount)) {
		return nil
	}
	return c.compactHistory(ctx, compactionHistory, aiMessages, injectedPending, turnStartLen, streamOpts.SessionID, eventCh)
}

func (c *GenkitClient) reduceContextOrCompact(
	ctx context.Context,
	compactionHistory *[]Message,
	aiMessages *[]*ai.Message,
	injectedPending *[]*ai.Message,
	turnStartLen *int,
	streamOpts StreamOptions,
	forcedRecoveryUsed bool,
	eventCh chan<- StreamEvent,
) ([]*ai.Message, bool, error) {
	reducedMessages, reduction := reduceGenkitContextForRequest(c.contextWindowTokenCount, *aiMessages)
	if reduction.FitsBudget {
		return reducedMessages, false, nil
	}

	slog.Debug("Genkit context still exceeds budget after reduction", "inputTokenCount", reduction.ReducedTokenCount, "removedToolResultCount", reduction.RemovedToolResults)
	if streamOpts.DisableAutoCompaction || streamOpts.OneShot || forcedRecoveryUsed || len(*injectedPending) > 0 {
		return nil, false, fmt.Errorf("%w: %s", ErrContextWindowExceeded, contextWindowExceededError)
	}
	if err := c.compactHistory(ctx, compactionHistory, aiMessages, injectedPending, turnStartLen, streamOpts.SessionID, eventCh); err != nil {
		return nil, true, fmt.Errorf("%w: automatic compaction failed: %v", ErrContextWindowExceeded, err)
	}
	return nil, true, nil
}

func (c *GenkitClient) compactHistory(
	ctx context.Context,
	compactionHistory *[]Message,
	aiMessages *[]*ai.Message,
	injectedPending *[]*ai.Message,
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

	*compactionHistory = replacement
	*aiMessages = toGenkitMessages(replacement)
	*injectedPending = nil
	*turnStartLen = len(*aiMessages)
	c.pendingState = nil
	eventCh <- StreamEvent{Type: StreamEventTypeAutoCompactionApplied, AutoCompaction: &AutoCompactionEvent{Replacement: replacement, Usage: usage}}
	return nil
}

func genkitAssistantText(response *ai.Message) string {
	var text strings.Builder
	for _, part := range response.Content {
		if part != nil && part.IsText() {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}

func (c *GenkitClient) Reset() {
	c.pendingState = nil
}

func (c *GenkitClient) injectPendingState(aiMessages []*ai.Message) ([]*ai.Message, []*ai.Message) {
	if len(c.pendingState) == 0 {
		return aiMessages, nil
	}

	injectedPending := append([]*ai.Message(nil), c.pendingState...)

	slog.Debug("Injecting pending state", "pending_messages", len(c.pendingState), "total_messages", len(aiMessages))

	if len(aiMessages) > 0 {
		last := aiMessages[len(aiMessages)-1]
		aiMessages = append(aiMessages[:len(aiMessages)-1], injectedPending...)
		aiMessages = append(aiMessages, last)
	} else {
		aiMessages = append(aiMessages, injectedPending...)
	}
	c.pendingState = nil
	return aiMessages, injectedPending
}

func (c *GenkitClient) savePendingIfAccumulated(aiMessages []*ai.Message, turnStartLen int, injectedPending []*ai.Message) {
	if len(injectedPending) == 0 && len(aiMessages) <= turnStartLen {
		return
	}

	newDelta := []*ai.Message(nil)
	if len(aiMessages) > turnStartLen {
		newDelta = aiMessages[turnStartLen:]
	}

	c.pendingState = make([]*ai.Message, 0, len(injectedPending)+len(newDelta))
	c.pendingState = append(c.pendingState, injectedPending...)
	c.pendingState = append(c.pendingState, newDelta...)
}

func (c *GenkitClient) emitTerminalEvent(eventCh chan<- StreamEvent, aiMessages []*ai.Message, turnStartLen int, injectedPending []*ai.Message, err error) {
	if len(injectedPending) > 0 || len(aiMessages) > turnStartLen {
		eventCh <- StreamEvent{Type: StreamEventTypeIncomplete, Error: err}
	} else if err != nil {
		eventCh <- StreamEvent{Type: StreamEventTypeError, Error: err}
	} else {
		eventCh <- StreamEvent{Type: StreamEventTypeDone}
	}
}

func (c *GenkitClient) exitIncomplete(eventCh chan<- StreamEvent, aiMessages []*ai.Message, turnStartLen int, injectedPending []*ai.Message, err error, oneShot bool) {
	if !oneShot {
		c.savePendingIfAccumulated(aiMessages, turnStartLen, injectedPending)
	}
	c.emitTerminalEvent(eventCh, aiMessages, turnStartLen, injectedPending, err)
}

func (c *GenkitClient) executeTools(
	ctx context.Context,
	toolRequests []*ai.ToolRequest,
	registry *tools.Registry,
	eventCh chan<- StreamEvent,
) ([]*ai.Part, []HistoricalToolActivity) {
	toolResponseParts := make([]*ai.Part, 0, len(toolRequests))
	activities := make([]HistoricalToolActivity, 0, len(toolRequests))

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

		output, execErr, toolStarted := executeValidatedTool(ctx, registry, req.Name, input, eventCh)

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
			if toolStarted {
				eventCh <- StreamEvent{
					Type:     StreamEventTypeToolEnd,
					ToolCall: toolCall,
				}
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
		activities = append(activities, historicalToolActivity(req.Name, input, output, execErr))
	}

	return toolResponseParts, activities
}
