package llm

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/mochow13/keen-code/internal/auth"
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
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

const openAICodexBaseURL = "https://chatgpt.com/backend-api/codex/"

type OpenAICodexClient struct {
	model                   string
	thinkingEffort          string
	maxRetries              int
	client                  openai.Client
	responseStreamImpl      responseStreamFactory
	authManager             *auth.OAuthManager
	userAgent               string
	pendingState            []responses.ResponseInputItemUnionParam
	contextWindowTokenCount int
	headers                 map[string]string
}

func NewOpenAICodexClient(cfg *providerconfig.ClientConfig) (*OpenAICodexClient, error) {
	if cfg.Provider != providerconfig.Provider(config.ProviderOpenAICodex) {
		return nil, fmt.Errorf("unsupported Codex OAuth provider: %s. %s", cfg.Provider, config.ConfigFixHint)
	}

	c := &OpenAICodexClient{
		model:                   cfg.Model,
		thinkingEffort:          cfg.ThinkingEffort,
		maxRetries:              retry.Count(cfg.MaxRetries),
		client:                  openai.NewClient(option.WithBaseURL(openAICodexBaseURL), option.WithHTTPClient(newCodexHTTPClient())),
		contextWindowTokenCount: cfg.ContextWindowTokens,
		authManager:             auth.NewOAuthManager(nil),
		userAgent:               fmt.Sprintf("keen-code (%s; %s)", runtime.GOOS, runtime.GOARCH),
		headers:                 cfg.Headers,
	}
	c.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &sdkResponseStream{stream: c.client.Responses.NewStreaming(ctx, params, opts...)}
	}
	return c, nil
}

func newCodexHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ForceAttemptHTTP2 = false
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	} else {
		transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	}
	transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	return &http.Client{Transport: transport}
}

func (c *OpenAICodexClient) StreamChat(ctx context.Context, messages []core.Message, toolRegistry *tools.Registry, opts ...core.StreamOptions) (<-chan core.StreamEvent, error) {
	eventCh := make(chan core.StreamEvent)

	go func() {
		defer close(eventCh)

		instructions, input := codexInstructionsAndInput(messages)
		streamOpts := streamOptions(opts)
		oneShot := streamOpts.OneShot
		sessionID := streamOpts.SessionID
		compactionHistory := core.CloneMessages(messages)
		autoCompactOff := false
		hasNewToolTurns := false
		var injectedPending []responses.ResponseInputItemUnionParam
		if !oneShot {
			input, injectedPending = c.injectPendingState(input)
		}
		turnStartLen := len(input)
		responseTools := toOpenAIResponseTools(toolRegistry)

		for range maxToolTurns {
			if err := c.proactivelyCompactHistory(
				ctx, &compactionHistory, &instructions, &input, &injectedPending, &turnStartLen,
				streamOpts, toolRegistry, hasNewToolTurns, autoCompactOff, eventCh,
			); err != nil {
				autoCompactOff = true
			}

			params := responses.ResponseNewParams{
				Model:        c.model,
				Instructions: param.NewOpt(instructions),
				Store:        param.NewOpt(false),
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

			completed, streamedContent, toolCalls, err := c.collectTurnWithRetry(ctx, params, eventCh)
			if err != nil {
				c.exitIncomplete(ctx, eventCh, input, turnStartLen, injectedPending, err, oneShot)
				return
			}
			if completed == nil {
				c.exitIncomplete(ctx, eventCh, input, turnStartLen, injectedPending, nil, oneShot)
				return
			}

			if completed.Usage.InputTokens > 0 || completed.Usage.OutputTokens > 0 {
				slog.Debug(
					"OpenAI Codex usage",
					"input_tokens", completed.Usage.InputTokens,
					"output_tokens", completed.Usage.OutputTokens,
					"total_tokens", completed.Usage.TotalTokens,
					"reasoning_tokens", completed.Usage.OutputTokensDetails.ReasoningTokens,
					"cached_tokens", completed.Usage.InputTokensDetails.CachedTokens,
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
			content := completed.OutputText()
			compactionHistory = append(compactionHistory, core.Message{
				Role:       core.RoleAssistant,
				Content:    content,
				TurnMemory: &core.TurnMemory{ToolActivity: activities},
			})
			hasNewToolTurns = true
			autoCompactOff = false
		}

		c.exitIncomplete(ctx, eventCh, input, turnStartLen, injectedPending, nil, oneShot)
	}()

	return eventCh, nil
}

func (c *OpenAICodexClient) Reset() {
	c.pendingState = nil
}

func (c *OpenAICodexClient) proactivelyCompactHistory(
	ctx context.Context,
	compactionHistory *[]core.Message,
	instructions *string,
	input *[]responses.ResponseInputItemUnionParam,
	injectedPending *[]responses.ResponseInputItemUnionParam,
	turnStartLen *int,
	streamOpts core.StreamOptions,
	toolRegistry *tools.Registry,
	hasNewToolTurns bool,
	autoCompactOff bool,
	eventCh chan<- core.StreamEvent,
) error {
	if streamOpts.DisableAutoCompaction || streamOpts.OneShot || !hasNewToolTurns || autoCompactOff || len(*injectedPending) > 0 ||
		!core.ShouldAutoCompact(codexHistoryTokenCount(*compactionHistory), core.ContextInputBudget(c.contextWindowTokenCount)) {
		return nil
	}

	return c.compactHistory(ctx, compactionHistory, instructions, input, injectedPending, turnStartLen, toolRegistry, streamOpts.SessionID, eventCh)
}

func (c *OpenAICodexClient) compactHistory(
	ctx context.Context,
	compactionHistory *[]core.Message,
	instructions *string,
	input *[]responses.ResponseInputItemUnionParam,
	injectedPending *[]responses.ResponseInputItemUnionParam,
	turnStartLen *int,
	toolRegistry *tools.Registry,
	sessionID string,
	eventCh chan<- core.StreamEvent,
) error {
	compactionCtx, cancel := context.WithCancel(ctx)
	sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeAutoCompactionStarted, AutoCompaction: &core.AutoCompactionEvent{Cancel: cancel}})
	replacement, usage, err := AutoCompact(compactionCtx, c, *compactionHistory, toolRegistry, sessionID)
	cancel()
	if err != nil {
		eventType := core.StreamEventTypeAutoCompactionFailed
		if compaction.IsCancellation(err) {
			eventType = core.StreamEventTypeAutoCompactionCancelled
		}
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: eventType, AutoCompaction: &core.AutoCompactionEvent{Error: err, Usage: usage}})
		return err
	}

	*compactionHistory = replacement
	*instructions, *input = codexInstructionsAndInput(replacement)
	*injectedPending = nil
	*turnStartLen = len(*input)
	c.pendingState = nil
	sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeAutoCompactionApplied, AutoCompaction: &core.AutoCompactionEvent{Replacement: replacement, Usage: usage}})
	return nil
}

func codexHistoryTokenCount(messages []core.Message) int {
	tokenCount := 0
	for _, message := range messages {
		tokenCount += core.EstimateContextTokenCount(history.FormatMessage(message))
	}
	return tokenCount
}

func codexInstructionsAndInput(messages []core.Message) (string, []responses.ResponseInputItemUnionParam) {
	instructions := make([]string, 0, 1)
	inputMessages := make([]core.Message, 0, len(messages))
	for _, m := range messages {
		if m.Role == core.RoleSystem {
			content := strings.TrimSpace(history.FormatMessage(m))
			if content != "" {
				instructions = append(instructions, content)
			}
			continue
		}
		inputMessages = append(inputMessages, m)
	}
	return strings.Join(instructions, "\n\n"), toOpenAIResponseInput(inputMessages)
}

func (c *OpenAICodexClient) injectPendingState(input []responses.ResponseInputItemUnionParam) ([]responses.ResponseInputItemUnionParam, []responses.ResponseInputItemUnionParam) {
	if len(c.pendingState) == 0 {
		return input, nil
	}

	injectedPending := append([]responses.ResponseInputItemUnionParam(nil), c.pendingState...)

	slog.Debug("Injecting pending state", "pending_messages", len(c.pendingState), "total_messages", len(input))
	if prettyJSON, err := json.MarshalIndent(c.pendingState, "", "  "); err == nil {
		slog.Debug("Pending state contents:\n" + string(prettyJSON))
	}

	if len(input) > 0 {
		last := input[len(input)-1]
		input = append(input[:len(input)-1], injectedPending...)
		input = append(input, last)
	} else {
		input = append(input, injectedPending...)
	}
	c.pendingState = nil
	return input, injectedPending
}

func (c *OpenAICodexClient) exitIncomplete(
	ctx context.Context,
	eventCh chan<- core.StreamEvent,
	input []responses.ResponseInputItemUnionParam,
	turnStartLen int,
	injectedPending []responses.ResponseInputItemUnionParam,
	err error,
	oneShot bool,
) {
	if !oneShot {
		c.savePendingIfAccumulated(input, turnStartLen, injectedPending)
	}
	c.emitTerminalEvent(ctx, eventCh, input, turnStartLen, injectedPending, err)
}

func (c *OpenAICodexClient) savePendingIfAccumulated(input []responses.ResponseInputItemUnionParam, turnStartLen int, injectedPending []responses.ResponseInputItemUnionParam) {
	if len(injectedPending) == 0 && len(input) <= turnStartLen {
		return
	}

	newDelta := []responses.ResponseInputItemUnionParam(nil)
	if len(input) > turnStartLen {
		newDelta = input[turnStartLen:]
	}

	c.pendingState = make([]responses.ResponseInputItemUnionParam, 0, len(injectedPending)+len(newDelta))
	c.pendingState = append(c.pendingState, injectedPending...)
	c.pendingState = append(c.pendingState, newDelta...)
}

func (c *OpenAICodexClient) emitTerminalEvent(
	ctx context.Context,
	eventCh chan<- core.StreamEvent,
	input []responses.ResponseInputItemUnionParam,
	turnStartLen int,
	injectedPending []responses.ResponseInputItemUnionParam,
	err error,
) {
	if len(injectedPending) > 0 || len(input) > turnStartLen {
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeIncomplete, Error: err})
	} else if err != nil {
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeError, Error: err})
	} else {
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeDone})
	}
}

func (c *OpenAICodexClient) collectTurnWithRetry(ctx context.Context, params responses.ResponseNewParams, eventCh chan<- core.StreamEvent) (*responses.Response, string, []responses.ResponseFunctionToolCall, error) {
	var completed *responses.Response
	var content string
	var calls []responses.ResponseFunctionToolCall
	err := retry.Run(ctx, c.maxRetries, func(attempt, maxRetries int, err error) {
		slog.Debug("LLM stream error, retrying", "attempt", attempt, "maxRetries", maxRetries, "backoff", time.Duration(attempt)*time.Second, "error", err)
		sendStreamEvent(ctx, eventCh, core.StreamEvent{Type: core.StreamEventTypeRetry, Error: err, Attempt: attempt})
	}, func() error {
		var err error
		completed, content, calls, err = c.collectTurn(ctx, params, eventCh)
		return err
	})
	if err != nil {
		return nil, "", nil, err
	}
	return completed, content, calls, nil
}

func (c *OpenAICodexClient) collectTurn(
	ctx context.Context,
	params responses.ResponseNewParams,
	eventCh chan<- core.StreamEvent,
) (*responses.Response, string, []responses.ResponseFunctionToolCall, error) {
	opts, err := c.requestOptions(ctx)
	if err != nil {
		return nil, "", nil, err
	}
	stream := c.responseStreamImpl(ctx, params, opts...)
	var completed *responses.Response
	var streamedContent strings.Builder
	textProgress := make(map[string]int)
	toolCalls := make([]responses.ResponseFunctionToolCall, 0)
	seenToolCalls := make(map[string]bool)

	for stream.Next() {
		ev := stream.Current()

		switch ev.Type {
		case "response.output_text.delta":
			text := ev.Delta
			emitCodexText(ctx, eventCh, &streamedContent, textProgress, codexTextProgressKey(ev), text)
		case "response.output_text.done":
			text := ev.Text
			if text == "" {
				text = ev.AsResponseOutputTextDone().Text
			}
			emitCodexTextDone(ctx, eventCh, &streamedContent, textProgress, codexTextProgressKey(ev), text)
		case "response.content_part.done":
			if ev.Part.Type == "output_text" {
				emitCodexTextDone(ctx, eventCh, &streamedContent, textProgress, codexTextProgressKey(ev), ev.Part.Text)
			}
		case "response.output_item.done":
			emitCodexOutputItemDone(ctx, eventCh, &streamedContent, textProgress, ev.Item)
			if ev.Item.Type == "function_call" {
				toolCalls = appendCodexToolCall(toolCalls, seenToolCalls, ev.Item.AsFunctionCall())
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
				msg = msg + " (" + ev.Code + ")"
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
		return nil, streamedContent.String(), nil, formatCodexStreamError(err)
	}
	if completed == nil {
		return nil, streamedContent.String(), nil, nil
	}

	for _, item := range completed.Output {
		if item.Type != "function_call" {
			continue
		}
		toolCalls = appendCodexToolCall(toolCalls, seenToolCalls, item.AsFunctionCall())
	}

	return completed, streamedContent.String(), toolCalls, nil
}

func formatCodexStreamError(err error) error {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("stream error: %w", err)
	}

	message := strings.TrimSpace(apiErr.Message)
	if message == "" {
		message = strings.TrimSpace(apiErr.RawJSON())
	}
	if message == "" {
		message = readCodexAPIErrorResponseBody(apiErr)
	}
	if message == "" {
		message = http.StatusText(apiErr.StatusCode)
	}

	parts := []string{fmt.Sprintf("HTTP %d", apiErr.StatusCode), message}
	if apiErr.Code != "" {
		parts = append(parts, "code="+apiErr.Code)
	}
	if apiErr.Param != "" {
		parts = append(parts, "param="+apiErr.Param)
	}
	return codexStreamError{
		message: fmt.Sprintf("OpenAI Codex API error: %s", strings.Join(parts, ": ")),
		err:     err,
	}
}

type codexStreamError struct {
	message string
	err     error
}

func (e codexStreamError) Error() string {
	return e.message
}

func (e codexStreamError) Unwrap() error {
	return e.err
}

func readCodexAPIErrorResponseBody(apiErr *openai.Error) string {
	if apiErr == nil || apiErr.Response == nil || apiErr.Response.Body == nil {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(apiErr.Response.Body, 4096))
	if err != nil {
		return ""
	}
	apiErr.Response.Body = io.NopCloser(bytes.NewBuffer(body))
	return strings.TrimSpace(string(body))
}

func codexTextProgressKey(ev responses.ResponseStreamEventUnion) string {
	return ev.ItemID + ":" + fmt.Sprint(ev.ContentIndex)
}

func emitCodexText(ctx context.Context, eventCh chan<- core.StreamEvent, streamedContent *strings.Builder, progress map[string]int, key string, text string) {
	if text == "" {
		return
	}
	streamedContent.WriteString(text)
	progress[key] += len(text)
	emitChunk(ctx, eventCh, text)
}

func emitCodexTextDone(ctx context.Context, eventCh chan<- core.StreamEvent, streamedContent *strings.Builder, progress map[string]int, key string, text string) {
	if text == "" {
		return
	}
	already := progress[key]
	if already >= len(text) {
		return
	}
	emitCodexText(ctx, eventCh, streamedContent, progress, key, text[already:])
}

func emitCodexOutputItemDone(ctx context.Context, eventCh chan<- core.StreamEvent, streamedContent *strings.Builder, progress map[string]int, item responses.ResponseOutputItemUnion) {
	if item.Type != "message" {
		return
	}
	for i, content := range item.Content {
		if content.Type != "output_text" {
			continue
		}
		emitCodexTextDone(ctx, eventCh, streamedContent, progress, fmt.Sprintf("%s:%d", item.ID, i), content.Text)
	}
}

func appendCodexToolCall(toolCalls []responses.ResponseFunctionToolCall, seen map[string]bool, toolCall responses.ResponseFunctionToolCall) []responses.ResponseFunctionToolCall {
	key := toolCall.CallID
	if key == "" {
		key = toolCall.ID
	}
	if key != "" {
		if seen[key] {
			return toolCalls
		}
		seen[key] = true
	}
	return append(toolCalls, toolCall)
}

func (c *OpenAICodexClient) executeTools(
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

func (c *OpenAICodexClient) requestOptions(ctx context.Context) ([]option.RequestOption, error) {
	cred, err := c.authManager.ValidAccessToken(ctx, auth.OpenAICodexProviderID)
	if err != nil {
		return nil, err
	}
	opts := make([]option.RequestOption, 0, len(c.headers)+5)
	for k, v := range c.headers {
		opts = append(opts, option.WithHeader(k, v))
	}
	opts = append(opts,
		option.WithHeader("Authorization", "Bearer "+cred.AccessToken),
		option.WithHeader("originator", "keen-code"),
		option.WithHeader("User-Agent", c.userAgent),
		option.WithHeaderDel("OpenAI-Organization"),
		option.WithHeaderDel("OpenAI-Project"),
	)
	if cred.AccountID != "" {
		opts = append(opts, option.WithHeader("ChatGPT-Account-Id", cred.AccountID))
	}
	return opts, nil
}
