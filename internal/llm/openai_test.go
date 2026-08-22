package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/tools"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/respjson"
	"github.com/openai/openai-go/v3/shared"
)

type fakeChatStream struct {
	chunks []openai.ChatCompletionChunk
	idx    int
	err    error
}

func (s *fakeChatStream) Next() bool {
	if s.idx >= len(s.chunks) {
		return false
	}
	s.idx++
	return true
}

func (s *fakeChatStream) Current() openai.ChatCompletionChunk {
	if s.idx == 0 || s.idx > len(s.chunks) {
		return openai.ChatCompletionChunk{}
	}
	return s.chunks[s.idx-1]
}

func (s *fakeChatStream) Err() error {
	return s.err
}

func (s *fakeChatStream) Close() error { return nil }

type successToolOAI struct{}

func (t *successToolOAI) Name() string {
	return "read_file"
}

func (t *successToolOAI) Description() string {
	return "reads a file"
}

func (t *successToolOAI) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
	}
}

func (t *successToolOAI) Execute(ctx context.Context, input any) (any, error) {
	return map[string]any{"content": "module github.com/user/keen-code"}, nil
}

func makeToolCallChunk() openai.ChatCompletionChunk {
	chunk := openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Index: 0,
				Delta: openai.ChatCompletionChunkChoiceDelta{
					Role: "assistant",
					ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
						{
							Index: 0,
							ID:    "call_1",
							Type:  "function",
							Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
								Name:      "read_file",
								Arguments: `{"path":"go.mod"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}
	chunk.Choices[0].Delta.JSON.ExtraFields = map[string]respjson.Field{
		"reasoning_content": respjson.NewField(`"reasoning-step"`),
	}
	return chunk
}

func makeToolCallChunkWithReasoningField() openai.ChatCompletionChunk {
	chunk := makeToolCallChunkWithoutReasoning()
	chunk.Choices[0].Delta.JSON.ExtraFields = map[string]respjson.Field{
		"reasoning": respjson.NewField(`"kimi-reasoning"`),
	}
	return chunk
}

func makeToolCallChunkWithoutReasoning() openai.ChatCompletionChunk {
	return openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Index: 0,
				Delta: openai.ChatCompletionChunkChoiceDelta{
					Role: "assistant",
					ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{
						{
							Index: 0,
							ID:    "call_1",
							Type:  "function",
							Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
								Name:      "read_file",
								Arguments: `{"path":"go.mod"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}
}

func makeContentChunk(content string) openai.ChatCompletionChunk {
	return openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{
			{
				Index: 0,
				Delta: openai.ChatCompletionChunkChoiceDelta{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: "stop",
			},
		},
	}
}

func TestNewOpenAICompatibleClient_DeepSeek(t *testing.T) {
	client, err := NewOpenAICompatibleClient(&ClientConfig{
		Provider: Provider(config.ProviderDeepSeek),
		APIKey:   "test-key",
		Model:    "deepseek-chat",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
	if client.model != "deepseek-chat" {
		t.Fatalf("expected model deepseek-chat, got %s", client.model)
	}
}

func TestNewOpenAICompatibleClient_OpenAIProviderRejected(t *testing.T) {
	client, err := NewOpenAICompatibleClient(&ClientConfig{
		Provider: Provider(config.ProviderOpenAI),
		APIKey:   "test-key",
		Model:    "gpt-4.1-mini",
	})
	if err == nil {
		t.Fatalf("expected error, got client: %+v", client)
	}
}

func TestOpenAICompatibleClient_StreamChat_CustomHeaders(t *testing.T) {
	var gotHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := &OpenAICompatibleClient{
		provider: Provider(config.ProviderDeepSeek),
		model:    "deepseek-v4-pro",
		client:   openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test-key")),
		headers:  map[string]string{"x-custom-header": "custom-value"},
	}
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		return &sdkChatStream{stream: client.client.Chat.Completions.NewStreaming(ctx, params, opts...)}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if gotHeaders.Get("x-custom-header") != "custom-value" {
		t.Fatalf("expected x-custom-header %q, got %q", "custom-value", gotHeaders.Get("x-custom-header"))
	}
}

func TestOpenAICompatibleClient_StreamChat_OpenCodeGoSessionHeader(t *testing.T) {
	const sessionID = "f71b869f-bfbb-46ad-a7b4-99a94261f9e9"
	const expectedSessionHeader = "f71b869fbfbb46ada7b499a94261f9e9"
	var gotSessionHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSessionHeader = r.Header.Get("x-opencode-session")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := &OpenAICompatibleClient{
		provider: Provider(config.ProviderOpenCodeGo),
		model:    "glm-5.1",
		client:   openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test-key")),
	}
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		return &sdkChatStream{stream: client.client.Chat.Completions.NewStreaming(ctx, params, opts...)}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil, StreamOptions{SessionID: sessionID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if gotSessionHeader != expectedSessionHeader {
		t.Fatalf("expected x-opencode-session %q, got %q", expectedSessionHeader, gotSessionHeader)
	}
}

func TestOpenAICompatibleClient_StreamChat_InjectsReasoningContentAcrossToolTurns(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider: Provider(config.ProviderDeepSeek),
		model:    "deepseek-reasoner",
	}

	var requests []string
	callCount := 0
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		body, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("failed to marshal params: %v", err)
		}
		requests = append(requests, string(body))

		callCount++
		if callCount == 1 {
			return &fakeChatStream{
				chunks: []openai.ChatCompletionChunk{
					makeToolCallChunk(),
				},
			}
		}
		return &fakeChatStream{
			chunks: []openai.ChatCompletionChunk{
				makeContentChunk("done"),
			},
		}
	}

	registry := tools.NewRegistry()
	if err := registry.Register(&successToolOAI{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "read go.mod"},
	}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasDone bool
	var toolStartCount int
	var toolEndCount int
	var streamed strings.Builder
	var reasoning strings.Builder
	for ev := range eventCh {
		switch ev.Type {
		case StreamEventTypeDone:
			hasDone = true
		case StreamEventTypeChunk:
			streamed.WriteString(ev.Content)
		case StreamEventTypeReasoningChunk:
			reasoning.WriteString(ev.Content)
		case StreamEventTypeToolStart:
			toolStartCount++
		case StreamEventTypeToolEnd:
			toolEndCount++
		case StreamEventTypeError:
			t.Fatalf("unexpected stream error: %v", ev.Error)
		}
	}

	if !hasDone {
		t.Fatal("expected done event")
	}
	if toolStartCount != 1 || toolEndCount != 1 {
		t.Fatalf("expected 1 tool start/end, got start=%d end=%d", toolStartCount, toolEndCount)
	}
	if len(requests) != 2 {
		t.Fatalf("expected two requests, got %d", len(requests))
	}
	if !strings.Contains(requests[1], `"reasoning_content":"reasoning-step"`) {
		t.Fatalf("expected reasoning_content in second request, got: %s", requests[1])
	}
	if reasoning.String() != "reasoning-step" {
		t.Fatalf("expected reasoning stream, got: %q", reasoning.String())
	}
	if streamed.String() != "done" {
		t.Fatalf("expected assistant-only chunk stream, got: %q", streamed.String())
	}
}

func TestOpenAICompatibleClient_StreamChat_CapturesReasoningFieldAcrossToolTurns(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:       Provider(config.ProviderOpenCodeGo),
		model:          "kimi-k2.6",
		thinkingEffort: "enabled",
	}

	var requests []string
	callCount := 0
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		body, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("failed to marshal params: %v", err)
		}
		requests = append(requests, string(body))

		callCount++
		if callCount == 1 {
			return &fakeChatStream{
				chunks: []openai.ChatCompletionChunk{
					makeToolCallChunkWithReasoningField(),
				},
			}
		}
		return &fakeChatStream{
			chunks: []openai.ChatCompletionChunk{
				makeContentChunk("done"),
			},
		}
	}

	registry := tools.NewRegistry()
	if err := registry.Register(&successToolOAI{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "read go.mod"},
	}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reasoning strings.Builder
	for ev := range eventCh {
		switch ev.Type {
		case StreamEventTypeReasoningChunk:
			reasoning.WriteString(ev.Content)
		case StreamEventTypeError:
			t.Fatalf("unexpected stream error: %v", ev.Error)
		}
	}

	if len(requests) != 2 {
		t.Fatalf("expected two requests, got %d", len(requests))
	}
	if !strings.Contains(requests[1], `"reasoning_content":"kimi-reasoning"`) {
		t.Fatalf("expected reasoning field serialized as reasoning_content in second request, got: %s", requests[1])
	}
	if reasoning.String() != "kimi-reasoning" {
		t.Fatalf("expected reasoning stream, got %q", reasoning.String())
	}
}

func TestNewOpenAICompatibleClient_ZAI(t *testing.T) {
	client, err := NewOpenAICompatibleClient(&ClientConfig{
		Provider: Provider(config.ProviderZAI),
		APIKey:   "test-key",
		Model:    "glm-4-plus",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
	if client.model != "glm-4-plus" {
		t.Fatalf("expected model glm-4-plus, got %s", client.model)
	}
}

func TestNewOpenAICompatibleClient_OpenCodeGo(t *testing.T) {
	client, err := NewOpenAICompatibleClient(&ClientConfig{
		Provider: Provider(config.ProviderOpenCodeGo),
		APIKey:   "test-key",
		Model:    "kimi-k2.6",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
	if client.provider != Provider(config.ProviderOpenCodeGo) {
		t.Fatalf("expected provider opencode-go, got %s", client.provider)
	}
	if client.model != "kimi-k2.6" {
		t.Fatalf("expected model kimi-k2.6, got %s", client.model)
	}
}

func TestOpenAICompatibleBaseURL_OpenCodeGo(t *testing.T) {
	baseURL, err := openAICompatibleBaseURL(Provider(config.ProviderOpenCodeGo))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := openCodeGoBaseURL + "/v1/"
	if baseURL != expected {
		t.Fatalf("expected %q, got %q", expected, baseURL)
	}
}

func TestOpenAICompatibleClient_StoresThinkingEffort(t *testing.T) {
	client, err := NewOpenAICompatibleClient(&ClientConfig{
		Provider:       Provider(config.ProviderDeepSeek),
		APIKey:         "test-key",
		Model:          "deepseek-v4-pro",
		ThinkingEffort: "high",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.thinkingEffort != "high" {
		t.Errorf("expected thinkingEffort 'high' stored, got %q", client.thinkingEffort)
	}
}

func TestOpenAICompatibleClient_DeepSeek_ThinkingEffort(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:       Provider(config.ProviderDeepSeek),
		model:          "deepseek-v4-pro",
		thinkingEffort: "high",
	}

	var capturedParams openai.ChatCompletionNewParams
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		capturedParams = params
		return &fakeChatStream{
			chunks: []openai.ChatCompletionChunk{makeContentChunk("hello")},
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "test"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if string(capturedParams.ReasoningEffort) != "high" {
		t.Fatalf("expected reasoning_effort 'high', got %q", capturedParams.ReasoningEffort)
	}
	extra := capturedParams.ExtraFields()
	if extra == nil {
		t.Fatal("expected extra fields with enabled thinking config")
	}
	thinking, ok := extra["thinking"]
	if !ok {
		t.Fatal("expected 'thinking' in extra fields")
	}
	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		t.Fatalf("expected thinking to be map[string]any, got %T", thinking)
	}
	if thinkingMap["type"] != "enabled" {
		t.Fatalf("expected thinking type 'enabled', got %v", thinkingMap["type"])
	}
}

func TestOpenAICompatibleClient_DeepSeek_ThinkingDisabled(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:       Provider(config.ProviderDeepSeek),
		model:          "deepseek-v4-pro",
		thinkingEffort: "disabled",
	}

	var capturedParams openai.ChatCompletionNewParams
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		capturedParams = params
		return &fakeChatStream{
			chunks: []openai.ChatCompletionChunk{makeContentChunk("hello")},
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "test"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if capturedParams.ReasoningEffort != "" {
		t.Fatalf("expected empty reasoning_effort when thinking is disabled, got %q", capturedParams.ReasoningEffort)
	}
	extra := capturedParams.ExtraFields()
	if extra == nil {
		t.Fatal("expected extra fields with disabled thinking config")
	}
	thinking, ok := extra["thinking"]
	if !ok {
		t.Fatal("expected 'thinking' in extra fields")
	}
	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		t.Fatalf("expected thinking to be map[string]any, got %T", thinking)
	}
	if thinkingMap["type"] != "disabled" {
		t.Fatalf("expected thinking type 'disabled', got %v", thinkingMap["type"])
	}
}

func TestOpenAICompatibleClient_ZAI_ThinkingEnabled(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:       Provider(config.ProviderZAI),
		model:          "glm-5.1",
		thinkingEffort: "enabled",
	}

	var capturedParams openai.ChatCompletionNewParams
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		capturedParams = params
		return &fakeChatStream{
			chunks: []openai.ChatCompletionChunk{makeContentChunk("hello")},
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "test"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	extra := capturedParams.ExtraFields()
	if extra == nil {
		t.Fatal("expected extra fields with thinking config")
	}
	thinking, ok := extra["thinking"]
	if !ok {
		t.Fatal("expected 'thinking' in extra fields")
	}
	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		t.Fatalf("expected thinking to be map[string]any, got %T", thinking)
	}
	if thinkingMap["type"] != "enabled" {
		t.Fatalf("expected thinking type 'enabled', got %v", thinkingMap["type"])
	}
}

func TestOpenAICompatibleClient_ZAI_ThinkingDisabled(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:       Provider(config.ProviderZAI),
		model:          "glm-5.1",
		thinkingEffort: "",
	}

	var capturedParams openai.ChatCompletionNewParams
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		capturedParams = params
		return &fakeChatStream{
			chunks: []openai.ChatCompletionChunk{makeContentChunk("hello")},
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "test"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	extra := capturedParams.ExtraFields()
	if extra != nil {
		if _, ok := extra["thinking"]; ok {
			t.Fatal("expected no thinking config when thinkingEffort is empty")
		}
	}
}

func TestOpenAICompatibleClient_OpenCodeGoDeepSeekThinkingEffort(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:       Provider(config.ProviderOpenCodeGo),
		model:          "deepseek-v4-pro",
		thinkingEffort: "max",
	}

	params := client.buildChatParams(nil, nil)

	if string(params.ReasoningEffort) != "max" {
		t.Fatalf("expected reasoning_effort max, got %q", params.ReasoningEffort)
	}
	extra := params.ExtraFields()
	thinking, ok := extra["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("expected thinking map, got %#v", extra["thinking"])
	}
	if thinking["type"] != "enabled" {
		t.Fatalf("expected thinking enabled, got %#v", thinking)
	}
}

func TestOpenAICompatibleClient_OpenCodeGoDeepSeekThinkingDisabled(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:       Provider(config.ProviderOpenCodeGo),
		model:          "deepseek-v4-flash",
		thinkingEffort: "disabled",
	}

	params := client.buildChatParams(nil, nil)

	if params.ReasoningEffort != "" {
		t.Fatalf("expected empty reasoning_effort, got %q", params.ReasoningEffort)
	}
	extra := params.ExtraFields()
	thinking, ok := extra["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("expected thinking map, got %#v", extra["thinking"])
	}
	if thinking["type"] != "disabled" {
		t.Fatalf("expected thinking disabled, got %#v", thinking)
	}
}

func TestOpenAICompatibleClient_OpenCodeGoGLMThinkingEnabled(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:       Provider(config.ProviderOpenCodeGo),
		model:          "glm-5.1",
		thinkingEffort: "enabled",
	}

	params := client.buildChatParams(nil, nil)

	extra := params.ExtraFields()
	thinking, ok := extra["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("expected thinking map, got %#v", extra["thinking"])
	}
	if thinking["type"] != "enabled" {
		t.Fatalf("expected thinking enabled, got %#v", thinking)
	}
}

func TestOpenAICompatibleClient_OpenCodeGoReasoningEffortModels(t *testing.T) {
	for _, model := range []string{"grok-4.5", "glm-5.2", "kimi-k3", "hy3"} {
		t.Run(model, func(t *testing.T) {
			params := (&OpenAICompatibleClient{
				provider:       Provider(config.ProviderOpenCodeGo),
				model:          model,
				thinkingEffort: "max",
			}).buildChatParams(nil, nil)

			if params.ReasoningEffort != shared.ReasoningEffort("max") {
				t.Fatalf("expected reasoning_effort max, got %q", params.ReasoningEffort)
			}
			if extra := params.ExtraFields(); extra != nil {
				t.Fatalf("expected no additional thinking fields, got %#v", extra)
			}
		})
	}
}

func TestOpenAICompatibleClient_MoonshotThinkingParameters(t *testing.T) {
	k3 := (&OpenAICompatibleClient{
		provider:       Provider(config.ProviderMoonshotAI),
		model:          "kimi-k3",
		thinkingEffort: "high",
	}).buildChatParams(nil, nil)
	if k3.ReasoningEffort != shared.ReasoningEffort("high") {
		t.Fatalf("expected Kimi K3 reasoning_effort high, got %q", k3.ReasoningEffort)
	}

	k26 := (&OpenAICompatibleClient{
		provider:       Provider(config.ProviderMoonshotAI),
		model:          "kimi-k2.6",
		thinkingEffort: "disabled",
	}).buildChatParams(nil, nil)
	thinking := k26.ExtraFields()["thinking"].(map[string]any)
	if thinking["type"] != "disabled" {
		t.Fatalf("expected Kimi K2.6 disabled thinking, got %#v", thinking)
	}
}

func TestOpenAICompatibleClient_ZAIGLM52ThinkingParameters(t *testing.T) {
	params := (&OpenAICompatibleClient{
		provider:       Provider(config.ProviderZAI),
		model:          "glm-5.2",
		thinkingEffort: "max",
	}).buildChatParams(nil, nil)
	thinking := params.ExtraFields()["thinking"].(map[string]any)
	if thinking["type"] != "enabled" || params.ReasoningEffort != shared.ReasoningEffort("max") {
		t.Fatalf("unexpected GLM-5.2 thinking params: thinking=%#v effort=%q", thinking, params.ReasoningEffort)
	}
}

func TestOpenAICompatibleClient_OpenCodeGoKimiThinkingDisabled(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:       Provider(config.ProviderOpenCodeGo),
		model:          "kimi-k2.6",
		thinkingEffort: "disabled",
	}

	params := client.buildChatParams(nil, nil)

	extra := params.ExtraFields()
	thinking, ok := extra["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("expected thinking map, got %#v", extra["thinking"])
	}
	if thinking["type"] != "disabled" {
		t.Fatalf("expected thinking disabled, got %#v", thinking)
	}
}

func TestOpenAICompatibleClient_OpenCodeGoMiMoOmitsThinkingConfig(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:       Provider(config.ProviderOpenCodeGo),
		model:          "mimo-v2-pro",
		thinkingEffort: "enabled",
	}

	params := client.buildChatParams(nil, nil)

	if extra := params.ExtraFields(); extra != nil {
		t.Fatalf("expected no thinking extra fields for MiMo, got %#v", extra)
	}
}

func TestToOpenAIMessages_RendersTurnMemoryForAssistant(t *testing.T) {
	messages := toOpenAIMessages([]Message{
		{
			Role:    RoleAssistant,
			Content: "done",
			TurnMemory: &TurnMemory{
				ToolActivity: []HistoricalToolActivity{{Tool: "read_file", Input: map[string]any{"path": "a.go"}, Status: "success"}},
			},
		},
	})

	body, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	if !strings.Contains(string(body), `\"path\":\"a.go\"`) || strings.Contains(string(body), "Tool memory:") {
		t.Fatalf("expected input in OpenAI tool call, got %s", string(body))
	}
}

func TestToOpenAIMessagesRetainsAskUserInputAndOutput(t *testing.T) {
	messages := toOpenAIMessages([]Message{{
		Role: RoleAssistant,
		TurnMemory: &TurnMemory{ToolActivity: []HistoricalToolActivity{{
			Tool:           "ask_user",
			Input:          map[string]any{"questions": []any{map[string]any{"question": "Database?", "options": []any{"PostgreSQL", "SQLite"}}}},
			Status:         "success",
			RetainedOutput: map[string]any{"answers": []string{"PostgreSQL"}, "cancelled": false},
		}}},
	}})

	body, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	encoded := string(body)
	for _, want := range []string{"ask_user", "Database?", "PostgreSQL", "cancelled"} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("expected %q in OpenAI history, got %s", want, encoded)
		}
	}
	if strings.Contains(encoded, `\"tool\":\"ask_user\"`) {
		t.Fatalf("ask_user result leaked an internal marker: %s", encoded)
	}
}

func TestFunctionToolCalls(t *testing.T) {
	toolCalls := []openai.ChatCompletionMessageToolCallUnion{
		{
			ID:   "call_1",
			Type: "function",
			Function: openai.ChatCompletionMessageFunctionToolCallFunction{
				Name:      "read_file",
				Arguments: `{"path":"go.mod"}`,
			},
		},
		{
			ID:   "call_2",
			Type: "function",
		},
		{
			ID:   "call_3",
			Type: "custom",
		},
		{
			ID: "call_4",
			Function: openai.ChatCompletionMessageFunctionToolCallFunction{
				Name:      "grep",
				Arguments: `{"pattern":"test"}`,
			},
		},
	}

	got := functionToolCalls(toolCalls)
	if len(got) != 2 {
		t.Fatalf("expected two named function calls, got %#v", got)
	}
	if got[0].ID != "call_1" || got[0].Function.Name != "read_file" {
		t.Fatalf("unexpected first function call: %#v", got[0])
	}
	if got[1].ID != "call_4" || got[1].Function.Name != "grep" {
		t.Fatalf("unexpected second function call: %#v", got[1])
	}
}

func TestOpenAICompatibleClient_buildAssistantMessage_AttachesReasoningWithoutToolCalls(t *testing.T) {
	client := &OpenAICompatibleClient{}

	msg := openai.ChatCompletionMessage{
		Role:    "assistant",
		Content: "answer",
	}

	assistant := client.buildAssistantMessage(msg, "reasoning-step", nil)
	extra := assistant.ExtraFields()
	if extra == nil {
		t.Fatal("expected extra fields to be set")
	}
	v, ok := extra["reasoning_content"]
	if !ok {
		t.Fatalf("expected reasoning_content in extra fields, got: %#v", extra)
	}
	if got, _ := v.(string); got != "reasoning-step" {
		t.Fatalf("expected reasoning_content=reasoning-step, got %#v", v)
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"nil", nil, false},
		{"context canceled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, false},
		{"generic error", fmt.Errorf("connection reset"), true},
		{"429 rate limit", &openai.Error{StatusCode: 429}, true},
		{"500 server error", &openai.Error{StatusCode: 500}, true},
		{"502 bad gateway", &openai.Error{StatusCode: 502}, true},
		{"503 service unavailable", &openai.Error{StatusCode: 503}, true},
		{"504 gateway timeout", &openai.Error{StatusCode: 504}, true},
		{"400 bad request", &openai.Error{StatusCode: 400}, false},
		{"401 unauthorized", &openai.Error{StatusCode: 401}, false},
		{"403 forbidden", &openai.Error{StatusCode: 403}, false},
		{"404 not found", &openai.Error{StatusCode: 404}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableError(tt.err); got != tt.retryable {
				t.Errorf("isRetryableError(%v) = %v, want %v", tt.err, got, tt.retryable)
			}
		})
	}
}

func TestOpenAICompatibleClient_StreamChat_RetriesOnRetryableError(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:   Provider(config.ProviderDeepSeek),
		model:      "deepseek-chat",
		maxRetries: 3,
	}

	callCount := 0
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		callCount++
		if callCount <= 2 {
			return &fakeChatStream{err: fmt.Errorf("connection reset")}
		}
		return &fakeChatStream{
			chunks: []openai.ChatCompletionChunk{makeContentChunk("recovered")},
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "test"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var retryEvents []StreamEvent
	var streamed strings.Builder
	var hasDone bool
	for ev := range eventCh {
		switch ev.Type {
		case StreamEventTypeRetry:
			retryEvents = append(retryEvents, ev)
		case StreamEventTypeChunk:
			streamed.WriteString(ev.Content)
		case StreamEventTypeDone:
			hasDone = true
		case StreamEventTypeError:
			t.Fatalf("unexpected error event: %v", ev.Error)
		}
	}

	if !hasDone {
		t.Fatal("expected done event")
	}
	if callCount != 3 {
		t.Fatalf("expected 3 calls (2 retries + 1 success), got %d", callCount)
	}
	if len(retryEvents) != 2 {
		t.Fatalf("expected 2 retry events, got %d", len(retryEvents))
	}
	if retryEvents[0].Attempt != 1 || retryEvents[1].Attempt != 2 {
		t.Fatalf("expected retry attempts 1,2; got %d,%d", retryEvents[0].Attempt, retryEvents[1].Attempt)
	}
	if streamed.String() != "recovered" {
		t.Fatalf("expected 'recovered', got %q", streamed.String())
	}
}

func TestOpenAICompatibleClient_StreamChat_NoRetryOnNonRetryableError(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider: Provider(config.ProviderDeepSeek),
		model:    "deepseek-chat",
	}

	callCount := 0
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		callCount++
		return &fakeChatStream{err: &openai.Error{StatusCode: 401}}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "test"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasError bool
	var retryCount int
	for ev := range eventCh {
		switch ev.Type {
		case StreamEventTypeRetry:
			retryCount++
		case StreamEventTypeError:
			hasError = true
		}
	}

	if !hasError {
		t.Fatal("expected error event")
	}
	if callCount != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", callCount)
	}
	if retryCount != 0 {
		t.Fatalf("expected 0 retry events, got %d", retryCount)
	}
}

func TestOpenAICompatibleClient_PendingState_ErrorMidLoop(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:   Provider(config.ProviderDeepSeek),
		model:      "deepseek-chat",
		maxRetries: 1,
	}

	callCount := 0
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		callCount++
		if callCount == 1 {
			return &fakeChatStream{chunks: []openai.ChatCompletionChunk{makeToolCallChunk()}}
		}
		return &fakeChatStream{err: &openai.Error{StatusCode: 500}}
	}

	registry := tools.NewRegistry()
	if err := registry.Register(&successToolOAI{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "read go.mod"},
	}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasIncomplete bool
	var incompleteErr error
	for ev := range eventCh {
		if ev.Type == StreamEventTypeIncomplete {
			hasIncomplete = true
			incompleteErr = ev.Error
		}
		if ev.Type == StreamEventTypeError {
			t.Fatalf("expected incomplete, got error: %v", ev.Error)
		}
	}

	if !hasIncomplete {
		t.Fatal("expected incomplete event")
	}
	if incompleteErr == nil {
		t.Fatal("expected error on incomplete event")
	}
	if len(client.pendingState) == 0 {
		t.Fatal("expected pending state to be saved")
	}
}

func TestOpenAICompatibleClient_PendingState_InjectedOnNextCall(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:   Provider(config.ProviderDeepSeek),
		model:      "deepseek-chat",
		maxRetries: 1,
	}

	callCount := 0
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		callCount++
		if callCount == 1 {
			return &fakeChatStream{chunks: []openai.ChatCompletionChunk{makeToolCallChunk()}}
		}
		if callCount == 2 {
			return &fakeChatStream{err: &openai.Error{StatusCode: 500}}
		}
		return &fakeChatStream{chunks: []openai.ChatCompletionChunk{makeContentChunk("recovered")}}
	}

	registry := tools.NewRegistry()
	if err := registry.Register(&successToolOAI{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	// First call: tool call succeeds, then API error on second iteration
	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "read go.mod"},
	}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if len(client.pendingState) == 0 {
		t.Fatal("expected pending state after failed turn")
	}
	savedLen := len(client.pendingState)

	// Second call: pending state should be injected, model recovers
	var capturedParams openai.ChatCompletionNewParams
	origStreamImpl := client.streamImpl
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		capturedParams = params
		return origStreamImpl(ctx, params, opts...)
	}

	eventCh, err = client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "read go.mod"},
		{Role: RoleUser, Content: "continue"},
	}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasDone bool
	var streamed strings.Builder
	for ev := range eventCh {
		switch ev.Type {
		case StreamEventTypeDone:
			hasDone = true
		case StreamEventTypeChunk:
			streamed.WriteString(ev.Content)
		case StreamEventTypeError:
			t.Fatalf("unexpected error: %v", ev.Error)
		}
	}

	if !hasDone {
		t.Fatal("expected done event on recovery")
	}
	if streamed.String() != "recovered" {
		t.Fatalf("expected 'recovered', got %q", streamed.String())
	}
	if len(client.pendingState) != 0 {
		t.Fatal("expected pending state to be cleared after injection")
	}
	// The injected messages should include the original user message + pending state + "continue"
	// Original: [user:"read go.mod"] + pending (assistant+tool = savedLen) + [user:"continue"]
	expectedMsgCount := 1 + savedLen + 1
	if len(capturedParams.Messages) != expectedMsgCount {
		t.Fatalf("expected %d messages after injection, got %d", expectedMsgCount, len(capturedParams.Messages))
	}
}

func TestOpenAICompatibleClient_PendingState_PreservedWhenRecoveryFailsBeforeProgress(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:   Provider(config.ProviderDeepSeek),
		model:      "deepseek-chat",
		maxRetries: 1,
		pendingState: []openai.ChatCompletionMessageParamUnion{
			openai.ToolMessage("old result 1", "call_old_1"),
			openai.ToolMessage("old result 2", "call_old_2"),
		},
	}

	wantPending, err := json.Marshal(client.pendingState)
	if err != nil {
		t.Fatalf("marshal pending state: %v", err)
	}

	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		return &fakeChatStream{err: &openai.Error{StatusCode: 500}}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "read go.mod"},
		{Role: RoleUser, Content: "continue"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasIncomplete bool
	for ev := range eventCh {
		switch ev.Type {
		case StreamEventTypeIncomplete:
			hasIncomplete = true
		case StreamEventTypeError:
			t.Fatalf("expected incomplete, got error: %v", ev.Error)
		}
	}

	if !hasIncomplete {
		t.Fatal("expected incomplete event")
	}

	gotPending, err := json.Marshal(client.pendingState)
	if err != nil {
		t.Fatalf("marshal saved pending state: %v", err)
	}
	if string(gotPending) != string(wantPending) {
		t.Fatalf("expected pending state to be preserved\nwant: %s\n got: %s", wantPending, gotPending)
	}
}

func TestOpenAICompatibleClient_PendingState_NoAccumulation_EmitsError(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider: Provider(config.ProviderDeepSeek),
		model:    "deepseek-chat",
	}

	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		return &fakeChatStream{err: &openai.Error{StatusCode: 401}}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "test"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasError bool
	for ev := range eventCh {
		if ev.Type == StreamEventTypeError {
			hasError = true
		}
		if ev.Type == StreamEventTypeIncomplete {
			t.Fatal("expected error, not incomplete")
		}
	}

	if !hasError {
		t.Fatal("expected error event")
	}
	if len(client.pendingState) != 0 {
		t.Fatal("expected no pending state when nothing accumulated")
	}
}

func TestOpenAICompatibleClient_PendingState_EmptyResponseMidLoop(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider:   Provider(config.ProviderDeepSeek),
		model:      "deepseek-chat",
		maxRetries: 1,
	}

	callCount := 0
	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		callCount++
		if callCount == 1 {
			return &fakeChatStream{chunks: []openai.ChatCompletionChunk{makeToolCallChunk()}}
		}
		return &fakeChatStream{chunks: nil}
	}

	registry := tools.NewRegistry()
	if err := registry.Register(&successToolOAI{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "read go.mod"},
	}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasIncomplete bool
	for ev := range eventCh {
		if ev.Type == StreamEventTypeIncomplete {
			hasIncomplete = true
		}
	}

	if !hasIncomplete {
		t.Fatal("expected incomplete event for empty response mid-loop")
	}
	if len(client.pendingState) == 0 {
		t.Fatal("expected pending state to be saved")
	}
}

func TestOpenAICompatibleClient_PendingState_ClearedOnSuccess(t *testing.T) {
	client := &OpenAICompatibleClient{
		provider: Provider(config.ProviderDeepSeek),
		model:    "deepseek-chat",
		pendingState: []openai.ChatCompletionMessageParamUnion{
			openai.ToolMessage("old result", "call_old"),
		},
	}

	client.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
		return &fakeChatStream{chunks: []openai.ChatCompletionChunk{makeContentChunk("hello")}}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "original"},
		{Role: RoleUser, Content: "continue"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasDone bool
	for ev := range eventCh {
		if ev.Type == StreamEventTypeDone {
			hasDone = true
		}
	}

	if !hasDone {
		t.Fatal("expected done event")
	}
	if len(client.pendingState) != 0 {
		t.Fatal("expected pending state to be cleared after successful completion")
	}
}
