package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/respjson"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/tools"
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

func TestOpenAICompatibleClient_StreamChat_InjectsReasoningContentForReasoner(t *testing.T) {
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
