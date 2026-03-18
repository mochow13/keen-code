package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/tools"
)

type fakeResponseStream struct {
	events []responses.ResponseStreamEventUnion
	idx    int
	err    error
}

func (s *fakeResponseStream) Next() bool {
	if s.idx >= len(s.events) {
		return false
	}
	s.idx++
	return true
}

func (s *fakeResponseStream) Current() responses.ResponseStreamEventUnion {
	if s.idx == 0 || s.idx > len(s.events) {
		return responses.ResponseStreamEventUnion{}
	}
	return s.events[s.idx-1]
}

func (s *fakeResponseStream) Err() error {
	return s.err
}

func (s *fakeResponseStream) Close() error { return nil }

func mustResponseEvent(t *testing.T, raw string) responses.ResponseStreamEventUnion {
	t.Helper()
	var ev responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		t.Fatalf("unmarshal response event: %v", err)
	}
	return ev
}

func TestNewOpenAIResponsesClient_OpenAI(t *testing.T) {
	client, err := NewOpenAIResponsesClient(&ClientConfig{
		Provider: Provider(config.ProviderOpenAI),
		APIKey:   "test-key",
		Model:    "gpt-5.4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
	if client.model != "gpt-5.4" {
		t.Fatalf("expected model gpt-5.4, got %s", client.model)
	}
}

func TestOpenAIResponsesClient_StreamChat_ToolLoop(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider: Provider(config.ProviderOpenAI),
		model:    "gpt-5.4",
	}

	callCount := 0
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		callCount++
		if callCount == 1 {
			return &fakeResponseStream{
				events: []responses.ResponseStreamEventUnion{
					mustResponseEvent(t, `{"type":"response.reasoning.delta","delta":"thinking","sequence_number":1}`),
					mustResponseEvent(t, `{"type":"response.completed","sequence_number":2,"response":{"id":"resp_1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"go.mod\"}","status":"completed"}],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
				},
			}
		}
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.output_text.delta","delta":"done","sequence_number":3}`),
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":4,"response":{"id":"resp_2","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
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
	if callCount != 2 {
		t.Fatalf("expected two response turns, got %d", callCount)
	}
	if toolStartCount != 1 || toolEndCount != 1 {
		t.Fatalf("expected 1 tool start/end, got start=%d end=%d", toolStartCount, toolEndCount)
	}
	if reasoning.String() != "thinking" {
		t.Fatalf("expected reasoning stream, got %q", reasoning.String())
	}
	if streamed.String() != "done" {
		t.Fatalf("expected assistant stream, got %q", streamed.String())
	}
}

func TestOpenAIResponsesClient_StreamChat_ErrorEvent(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider: Provider(config.ProviderOpenAI),
		model:    "gpt-5.4",
	}

	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.output_text.delta","delta":"Hello","sequence_number":1}`),
				mustResponseEvent(t, `{"type":"error","message":"Rate limit exceeded","code":"rate_limit_exceeded","sequence_number":2}`),
			},
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "hello"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasError bool
	var errorMsg string
	var streamed strings.Builder
	for ev := range eventCh {
		switch ev.Type {
		case StreamEventTypeChunk:
			streamed.WriteString(ev.Content)
		case StreamEventTypeError:
			hasError = true
			errorMsg = ev.Error.Error()
		case StreamEventTypeDone:
			t.Fatal("expected error event before done")
		}
	}

	if !hasError {
		t.Fatal("expected error event")
	}
	if streamed.String() != "Hello" {
		t.Fatalf("expected streamed content before error, got %q", streamed.String())
	}
	if !strings.Contains(errorMsg, "Rate limit exceeded") {
		t.Fatalf("expected error message to contain 'Rate limit exceeded', got %q", errorMsg)
	}
	if !strings.Contains(errorMsg, "rate_limit_exceeded") {
		t.Fatalf("expected error message to contain error code, got %q", errorMsg)
	}
}

func TestOpenAIResponsesClient_StreamChat_ErrorEventEmptyMessage(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider: Provider(config.ProviderOpenAI),
		model:    "gpt-5.4",
	}

	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"error","message":"","sequence_number":1}`),
			},
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "hello"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasError bool
	var errorMsg string
	for ev := range eventCh {
		switch ev.Type {
		case StreamEventTypeError:
			hasError = true
			errorMsg = ev.Error.Error()
		}
	}

	if !hasError {
		t.Fatal("expected error event")
	}
	if !strings.Contains(errorMsg, "responses stream error") {
		t.Fatalf("expected default error message, got %q", errorMsg)
	}
}
