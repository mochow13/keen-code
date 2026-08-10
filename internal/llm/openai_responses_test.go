package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/tools"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
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

func TestOpenAIResponsesClient_StreamChat_CustomHeaders(t *testing.T) {
	var gotHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`data: {"type":"response.completed","sequence_number":1,"response":{"id":"resp_1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := &OpenAIResponsesClient{
		provider: Provider(config.ProviderOpenAI),
		model:    "gpt-5.4",
		client:   openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test-key")),
		headers:  map[string]string{"x-custom-header": "custom-value"},
	}
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &sdkResponseStream{stream: client.client.Responses.NewStreaming(ctx, params, opts...)}
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

func TestOpenAIResponsesClient_OpenCodeGoSessionHeader(t *testing.T) {
	var gotSession string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSession = r.Header.Get("x-opencode-session")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"response.completed","sequence_number":1,"response":{"id":"resp_1","created_at":0,"metadata":{},"model":"gpt-5.6-luna","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := &OpenAIResponsesClient{
		provider: Provider(config.ProviderOpenCodeGo),
		model:    "gpt-5.6-luna",
		client:   openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test-key")),
	}
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &sdkResponseStream{stream: client.client.Responses.NewStreaming(ctx, params, opts...)}
	}

	sessionID := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	eventCh, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil, StreamOptions{SessionID: sessionID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if expected := opencodeSessionID(sessionID); gotSession != expected {
		t.Fatalf("expected x-opencode-session %q, got %q", expected, gotSession)
	}
}

func TestOpenAIResponsesClient_StreamChat_ToolLoop(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider:   Provider(config.ProviderOpenAI),
		model:      "gpt-5.4",
		maxRetries: 2,
	}

	callCount := 0
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		callCount++
		if callCount == 1 {
			return &fakeResponseStream{
				events: []responses.ResponseStreamEventUnion{
					mustResponseEvent(t, `{"type":"response.reasoning.delta","delta":"thinking","sequence_number":1}`),
					mustResponseEvent(t, `{"type":"response.completed","sequence_number":2,"response":{"id":"resp_1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"I'll inspect that.","annotations":[]}]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"go.mod\"}","status":"completed"}],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
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
	if streamed.String() != "I'll inspect that.done" {
		t.Fatalf("expected assistant stream, got %q", streamed.String())
	}
}

func TestOpenAIResponsesClient_StreamChat_ReplaysAssistantMessageBeforeTools(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider:   Provider(config.ProviderOpenAI),
		model:      "gpt-5.4",
		maxRetries: 1,
	}

	callCount := 0
	var capturedSecondParams responses.ResponseNewParams
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		callCount++
		if callCount == 1 {
			return &fakeResponseStream{
				events: []responses.ResponseStreamEventUnion{
					mustResponseEvent(t, `{"type":"response.completed","sequence_number":1,"response":{"id":"resp_1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"encrypted-reasoning","status":"completed"},{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"I'll inspect that.","annotations":[]}]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"go.mod\"}","status":"completed"}],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
				},
			}
		}
		capturedSecondParams = params
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":2,"response":{"id":"resp_2","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
	}

	registry := tools.NewRegistry()
	if err := registry.Register(&successToolOAI{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "read go.mod"}}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for ev := range eventCh {
		if ev.Type == StreamEventTypeError {
			t.Fatalf("unexpected stream error: %v", ev.Error)
		}
	}

	body, err := json.Marshal(capturedSecondParams.Input)
	if err != nil {
		t.Fatalf("marshal second request input: %v", err)
	}
	inputJSON := string(body)
	for _, want := range []string{"I'll inspect that.", `"type":"reasoning"`, `"id":"rs_1"`, `"encrypted_content":"encrypted-reasoning"`, `"type":"message"`, `"role":"assistant"`, `"type":"function_call"`, `"type":"function_call_output"`} {
		if !strings.Contains(inputJSON, want) {
			t.Fatalf("expected second request input to contain %q, got %s", want, inputJSON)
		}
	}
}

func TestResponseOutputInputsPreservesReasoning(t *testing.T) {
	var output []responses.ResponseOutputItemUnion
	if err := json.Unmarshal([]byte(`[{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"Inspect the file"}],"content":[{"type":"reasoning_text","text":"Need the module name"}],"encrypted_content":"encrypted-reasoning","status":"completed"}]`), &output); err != nil {
		t.Fatalf("unmarshal reasoning output: %v", err)
	}

	input := responseOutputInputs(output, nil, "")
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal response input: %v", err)
	}
	inputJSON := string(body)
	for _, want := range []string{`"type":"reasoning"`, `"id":"rs_1"`, `"encrypted_content":"encrypted-reasoning"`, "Inspect the file", "Need the module name"} {
		if !strings.Contains(inputJSON, want) {
			t.Fatalf("expected replayed reasoning to contain %q, got %s", want, inputJSON)
		}
	}
}

func TestOpenAIResponsesClient_StreamChat_TerminalResponseEvents(t *testing.T) {
	tests := []struct {
		name    string
		event   string
		wantErr []string
	}{
		{
			name:    "failed",
			event:   `{"type":"response.failed","sequence_number":1,"response":{"id":"resp_1","created_at":0,"error":{"code":"invalid_prompt","message":"prompt rejected"},"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1,"status":"failed"}}`,
			wantErr: []string{"prompt rejected", "invalid_prompt"},
		},
		{
			name:    "incomplete",
			event:   `{"type":"response.incomplete","sequence_number":1,"response":{"id":"resp_1","created_at":0,"incomplete_details":{"reason":"max_output_tokens"},"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1,"status":"incomplete"}}`,
			wantErr: []string{"response incomplete", "max_output_tokens"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &OpenAIResponsesClient{
				provider:   Provider(config.ProviderOpenAI),
				model:      "gpt-5.4",
				maxRetries: 1,
			}
			client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
				return &fakeResponseStream{events: []responses.ResponseStreamEventUnion{mustResponseEvent(t, tt.event)}}
			}

			eventCh, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hello"}}, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var terminalErr error
			for ev := range eventCh {
				switch ev.Type {
				case StreamEventTypeError:
					terminalErr = ev.Error
				case StreamEventTypeDone:
					t.Fatal("expected error event, got done")
				}
			}
			if terminalErr == nil {
				t.Fatal("expected terminal error")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(terminalErr.Error(), want) {
					t.Fatalf("expected error to contain %q, got %q", want, terminalErr)
				}
			}
		})
	}
}

func TestOpenAIResponsesClient_StreamChat_ErrorEvent(t *testing.T) {
	const testMaxRetries = 2
	client := &OpenAIResponsesClient{
		provider:   Provider(config.ProviderOpenAI),
		model:      "gpt-5.4",
		maxRetries: testMaxRetries,
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
	if streamed.String() != strings.Repeat("Hello", testMaxRetries) {
		t.Fatalf("expected streamed content from each retry before error, got %q", streamed.String())
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
		provider:   Provider(config.ProviderOpenAI),
		model:      "gpt-5.4",
		maxRetries: 1,
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

func TestOpenAIResponsesClient_PendingState_ErrorMidLoop(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider:   Provider(config.ProviderOpenAI),
		model:      "gpt-5.4",
		maxRetries: 1,
	}

	callCount := 0
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		callCount++
		if callCount == 1 {
			return &fakeResponseStream{
				events: []responses.ResponseStreamEventUnion{
					mustResponseEvent(t, `{"type":"response.completed","sequence_number":1,"response":{"id":"resp_1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"I'll inspect that.","annotations":[]}]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"go.mod\"}","status":"completed"}],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
				},
			}
		}
		return &fakeResponseStream{err: &openai.Error{StatusCode: http.StatusInternalServerError}}
	}

	registry := tools.NewRegistry()
	if err := registry.Register(&successToolOAI{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "read go.mod"}}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasIncomplete bool
	var incompleteErr error
	for ev := range eventCh {
		switch ev.Type {
		case StreamEventTypeIncomplete:
			hasIncomplete = true
			incompleteErr = ev.Error
		case StreamEventTypeError:
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
	body, err := json.Marshal(client.pendingState)
	if err != nil {
		t.Fatalf("marshal pending state: %v", err)
	}
	pendingJSON := string(body)
	for _, want := range []string{"I'll inspect that.", `"type":"message"`, `"role":"assistant"`, `"type":"function_call"`, `"type":"function_call_output"`, `"call_id":"call_1"`, "module github.com/user/keen-code"} {
		if !strings.Contains(pendingJSON, want) {
			t.Fatalf("expected pending state to contain %q, got %s", want, pendingJSON)
		}
	}
}

func TestOpenAIResponsesClient_PendingState_InjectedOnNextCall(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider:   Provider(config.ProviderOpenAI),
		model:      "gpt-5.4",
		maxRetries: 1,
	}

	callCount := 0
	var capturedRecoveryParams responses.ResponseNewParams
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		callCount++
		switch callCount {
		case 1:
			return &fakeResponseStream{
				events: []responses.ResponseStreamEventUnion{
					mustResponseEvent(t, `{"type":"response.completed","sequence_number":1,"response":{"id":"resp_1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"I'll inspect that.","annotations":[]}]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"go.mod\"}","status":"completed"}],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
				},
			}
		case 2:
			return &fakeResponseStream{err: &openai.Error{StatusCode: http.StatusInternalServerError}}
		default:
			capturedRecoveryParams = params
			return &fakeResponseStream{
				events: []responses.ResponseStreamEventUnion{
					mustResponseEvent(t, `{"type":"response.output_text.delta","delta":"recovered","sequence_number":2}`),
					mustResponseEvent(t, `{"type":"response.completed","sequence_number":3,"response":{"id":"resp_2","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
				},
			}
		}
	}

	registry := tools.NewRegistry()
	if err := registry.Register(&successToolOAI{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "read go.mod"}}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if len(client.pendingState) == 0 {
		t.Fatal("expected pending state after failed turn")
	}
	savedLen := len(client.pendingState)

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
			t.Fatalf("unexpected stream error: %v", ev.Error)
		}
	}

	if !hasDone {
		t.Fatal("expected done event")
	}
	if streamed.String() != "recovered" {
		t.Fatalf("expected recovered stream, got %q", streamed.String())
	}
	if len(client.pendingState) != 0 {
		t.Fatal("expected pending state to be cleared after injection")
	}
	if capturedRecoveryParams.PreviousResponseID.Valid() {
		t.Fatalf("expected no previous_response_id, got %#v", capturedRecoveryParams.PreviousResponseID)
	}

	body, err := json.Marshal(capturedRecoveryParams.Input)
	if err != nil {
		t.Fatalf("marshal recovery input: %v", err)
	}
	inputJSON := string(body)
	for _, want := range []string{"read go.mod", "I'll inspect that.", `"type":"message"`, `"role":"assistant"`, `"type":"function_call"`, `"type":"function_call_output"`, "module github.com/user/keen-code", "continue"} {
		if !strings.Contains(inputJSON, want) {
			t.Fatalf("expected recovery input to contain %q, got %s", want, inputJSON)
		}
	}
	if len(capturedRecoveryParams.Input.OfInputItemList) != 1+savedLen+1 {
		t.Fatalf("expected pending state inserted before new user message, got %d input items", len(capturedRecoveryParams.Input.OfInputItemList))
	}
}

func TestOpenAIResponsesClient_PendingState_PreservedWhenRecoveryFailsBeforeProgress(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider:   Provider(config.ProviderOpenAI),
		model:      "gpt-5.4",
		maxRetries: 1,
		pendingState: []responses.ResponseInputItemUnionParam{
			responses.ResponseInputItemParamOfFunctionCall("{}", "call_old", "read_file"),
			responses.ResponseInputItemParamOfFunctionCallOutput("call_old", `{"content":"old"}`),
		},
	}

	wantPending, err := json.Marshal(client.pendingState)
	if err != nil {
		t.Fatalf("marshal pending state: %v", err)
	}

	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &fakeResponseStream{err: &openai.Error{StatusCode: http.StatusInternalServerError}}
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

func TestOpenAIResponsesClient_PendingState_NoAccumulation_EmitsError(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider:   Provider(config.ProviderOpenAI),
		model:      "gpt-5.4",
		maxRetries: 1,
	}
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &fakeResponseStream{err: &openai.Error{StatusCode: http.StatusUnauthorized}}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "test"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasError bool
	for ev := range eventCh {
		switch ev.Type {
		case StreamEventTypeError:
			hasError = true
		case StreamEventTypeIncomplete:
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

func TestOpenAIResponsesClient_PendingState_EmptyResponseMidLoop(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider:   Provider(config.ProviderOpenAI),
		model:      "gpt-5.4",
		maxRetries: 1,
	}

	callCount := 0
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		callCount++
		if callCount == 1 {
			return &fakeResponseStream{
				events: []responses.ResponseStreamEventUnion{
					mustResponseEvent(t, `{"type":"response.completed","sequence_number":1,"response":{"id":"resp_1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"I'll inspect that.","annotations":[]}]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"go.mod\"}","status":"completed"}],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
				},
			}
		}
		return &fakeResponseStream{}
	}

	registry := tools.NewRegistry()
	if err := registry.Register(&successToolOAI{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "read go.mod"}}, registry)
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

func TestOpenAIResponsesClient_PendingState_ClearedOnSuccess(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider: Provider(config.ProviderOpenAI),
		model:    "gpt-5.4",
		pendingState: []responses.ResponseInputItemUnionParam{
			responses.ResponseInputItemParamOfFunctionCall("{}", "call_old", "read_file"),
			responses.ResponseInputItemParamOfFunctionCallOutput("call_old", "old result"),
		},
	}

	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.output_text.delta","delta":"hello","sequence_number":1}`),
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":2,"response":{"id":"resp_1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
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

func TestOpenAIResponsesClient_ThinkingEffort_SetsReasoning(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider:       Provider(config.ProviderOpenAI),
		model:          "gpt-5.4",
		thinkingEffort: "high",
	}

	var capturedParams responses.ResponseNewParams
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		capturedParams = params
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":1,"response":{"id":"r1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if capturedParams.Reasoning.Effort != shared.ReasoningEffortHigh {
		t.Errorf("expected Reasoning.Effort 'high', got %q", capturedParams.Reasoning.Effort)
	}
}

func TestOpenAIResponsesClient_NoThinkingEffort_OmitsReasoning(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider:       Provider(config.ProviderOpenAI),
		model:          "gpt-5.4",
		thinkingEffort: "",
	}

	var capturedParams responses.ResponseNewParams
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		capturedParams = params
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":1,"response":{"id":"r1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if capturedParams.Reasoning.Effort != "" {
		t.Errorf("expected empty Reasoning.Effort when thinkingEffort is empty, got %q", capturedParams.Reasoning.Effort)
	}
}

func TestToOpenAIResponseInput_RendersTurnMemoryForAssistant(t *testing.T) {
	exitCode := 1
	input := toOpenAIResponseInput([]Message{
		{
			Role:    RoleAssistant,
			Content: "done",
			TurnMemory: &TurnMemory{
				ToolActivity: []HistoricalToolActivity{{Tool: "bash", Input: map[string]any{"command": "go test ./..."}, Status: "success", ExitCode: &exitCode}},
			},
		},
	})

	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	if !strings.Contains(string(body), `\"command\":\"go test ./...\"`) || !strings.Contains(string(body), `\"exit_code\":1`) || strings.Contains(string(body), "failed_command") || strings.Contains(string(body), "Tool memory:") {
		t.Fatalf("expected command input and compact Responses result, got %s", string(body))
	}
}

func TestOpenAIResponsesClient_PromptCacheKey_SetFromSessionID(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider:   Provider(config.ProviderOpenAI),
		model:      "gpt-5.4",
		maxRetries: 1,
	}

	var capturedParams responses.ResponseNewParams
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		capturedParams = params
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":1,"response":{"id":"r1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "hi"},
	}, nil, StreamOptions{SessionID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if !capturedParams.PromptCacheKey.Valid() {
		t.Fatal("expected PromptCacheKey to be set")
	}
	want := "a1b2c3d4e5f67890abcdef1234567890"
	if capturedParams.PromptCacheKey.Value != want {
		t.Fatalf("expected PromptCacheKey %q, got %q", want, capturedParams.PromptCacheKey.Value)
	}
}

func TestOpenAIResponsesClient_PromptCacheKey_OmittedWithoutSessionID(t *testing.T) {
	client := &OpenAIResponsesClient{
		provider:   Provider(config.ProviderOpenAI),
		model:      "gpt-5.4",
		maxRetries: 1,
	}

	var capturedParams responses.ResponseNewParams
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		capturedParams = params
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":1,"response":{"id":"r1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if capturedParams.PromptCacheKey.Valid() {
		t.Fatalf("expected PromptCacheKey to be omitted, got %q", capturedParams.PromptCacheKey.Value)
	}
}
