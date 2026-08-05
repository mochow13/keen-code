package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"github.com/user/keen-code/internal/auth"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/tools"
)

func TestNewClient_OpenAICodexAllowsMissingAPIKey(t *testing.T) {
	client, err := NewClient(&config.ResolvedConfig{
		Provider:       config.ProviderOpenAICodex,
		Model:          "gpt-5.4",
		ThinkingEffort: "medium",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := client.(*OpenAICodexClient); !ok {
		t.Fatalf("expected *OpenAICodexClient, got %T", client)
	}
}

func TestNewCodexHTTPClientDisablesHTTP2(t *testing.T) {
	client := newCodexHTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if transport.ForceAttemptHTTP2 {
		t.Fatal("expected Codex HTTP client to disable automatic HTTP/2")
	}
	if transport.TLSNextProto == nil {
		t.Fatal("expected Codex HTTP client to override TLSNextProto")
	}
	if got := transport.TLSClientConfig.NextProtos; len(got) != 1 || got[0] != "http/1.1" {
		t.Fatalf("expected Codex HTTP client to force HTTP/1.1 ALPN, got %#v", got)
	}
}

func TestOpenAICodexClientRequestTargetsCodexEndpointAndOAuthHeaders(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-api-key")

	store := auth.NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	if err := store.Set(auth.OpenAICodexProviderID, auth.OAuthCredential{
		Type:         "oauth",
		AccessToken:  "oauth-access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		AccountID:    "acct_123",
	}); err != nil {
		t.Fatalf("seed auth store: %v", err)
	}

	var gotPath string
	var gotAuth string
	var gotAccountID string
	var gotOriginator string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAccountID = r.Header.Get("ChatGPT-Account-Id")
		gotOriginator = r.Header.Get("originator")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"response.completed","sequence_number":1,"response":{"id":"r1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := &OpenAICodexClient{
		model:       "gpt-5.4",
		client:      openai.NewClient(option.WithBaseURL(server.URL + "/backend-api/codex/")),
		authManager: auth.NewOAuthManager(store),
		userAgent:   "keen-code-test",
	}
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &sdkResponseStream{stream: client.client.Responses.NewStreaming(ctx, params, opts...)}
	}

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}
	for ev := range ch {
		if ev.Type == StreamEventTypeError {
			t.Fatalf("unexpected stream error: %v", ev.Error)
		}
	}

	if gotPath != "/backend-api/codex/responses" {
		t.Fatalf("expected Codex responses path, got %q", gotPath)
	}
	if gotAuth != "Bearer oauth-access" {
		t.Fatalf("expected OAuth authorization override, got %q", gotAuth)
	}
	if gotAccountID != "acct_123" {
		t.Fatalf("expected account header, got %q", gotAccountID)
	}
	if gotOriginator != "keen-code" {
		t.Fatalf("expected originator header, got %q", gotOriginator)
	}
}

func TestOpenAICodexClientRequestCustomHeaders(t *testing.T) {
	store := auth.NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	if err := store.Set(auth.OpenAICodexProviderID, auth.OAuthCredential{
		Type:         "oauth",
		AccessToken:  "oauth-access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("seed auth store: %v", err)
	}

	var gotCustom string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCustom = r.Header.Get("x-custom-header")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"response.completed","sequence_number":1,"response":{"id":"r1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client := &OpenAICodexClient{
		model:       "gpt-5.4",
		client:      openai.NewClient(option.WithBaseURL(server.URL + "/backend-api/codex/")),
		authManager: auth.NewOAuthManager(store),
		userAgent:   "keen-code-test",
		headers:     map[string]string{"x-custom-header": "custom-value"},
	}
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &sdkResponseStream{stream: client.client.Responses.NewStreaming(ctx, params, opts...)}
	}

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}
	for ev := range ch {
		if ev.Type == StreamEventTypeError {
			t.Fatalf("unexpected stream error: %v", ev.Error)
		}
	}

	if gotCustom != "custom-value" {
		t.Fatalf("expected x-custom-header %q, got %q", "custom-value", gotCustom)
	}
}

func TestOpenAICodexClientSetsInstructionsStoreAndReasoning(t *testing.T) {
	client := newTestCodexClient(t)
	client.thinkingEffort = "high"

	var capturedParams responses.ResponseNewParams
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		capturedParams = params
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":1,"response":{"id":"r1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
	}

	ch, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleSystem, Content: "system prompt"},
		{Role: RoleUser, Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}
	for ev := range ch {
		if ev.Type == StreamEventTypeError {
			t.Fatalf("unexpected stream error: %v", ev.Error)
		}
	}

	if !capturedParams.Instructions.Valid() || capturedParams.Instructions.Value != "system prompt" {
		t.Fatalf("expected system prompt in instructions, got %#v", capturedParams.Instructions)
	}
	if !capturedParams.Store.Valid() || capturedParams.Store.Value {
		t.Fatalf("expected store=false, got %#v", capturedParams.Store)
	}
	if capturedParams.Reasoning.Effort != shared.ReasoningEffortHigh {
		t.Fatalf("expected high reasoning, got %q", capturedParams.Reasoning.Effort)
	}
	body, err := json.Marshal(capturedParams.Input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	if strings.Contains(string(body), "system prompt") {
		t.Fatalf("did not expect system prompt in input items, got %s", string(body))
	}
	if !strings.Contains(string(body), "hi") {
		t.Fatalf("expected user input item, got %s", string(body))
	}
}

func TestOpenAICodexClientOutputTextDoneEmitsFinalText(t *testing.T) {
	client := newTestCodexClient(t)
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.output_text.done","item_id":"msg_1","output_index":0,"content_index":0,"text":"final text","sequence_number":1}`),
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":2,"response":{"id":"r1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
	}

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}
	var streamed strings.Builder
	for ev := range ch {
		if ev.Type == StreamEventTypeChunk {
			streamed.WriteString(ev.Content)
		}
	}
	if streamed.String() != "final text" {
		t.Fatalf("expected finalized text, got %q", streamed.String())
	}
}

func TestOpenAICodexClientOutputTextDoneDoesNotDuplicateDeltas(t *testing.T) {
	client := newTestCodexClient(t)
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"final ","sequence_number":1}`),
				mustResponseEvent(t, `{"type":"response.output_text.done","item_id":"msg_1","output_index":0,"content_index":0,"text":"final text","sequence_number":2}`),
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":3,"response":{"id":"r1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
	}

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}
	var streamed strings.Builder
	for ev := range ch {
		if ev.Type == StreamEventTypeChunk {
			streamed.WriteString(ev.Content)
		}
	}
	if streamed.String() != "final text" {
		t.Fatalf("expected non-duplicated finalized text, got %q", streamed.String())
	}
}

func TestOpenAICodexClientOutputItemDoneEmitsMessageText(t *testing.T) {
	client := newTestCodexClient(t)
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"review text","annotations":[]}]},"sequence_number":1}`),
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":2,"response":{"id":"r1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
	}

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "review this"}}, nil)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}
	var streamed strings.Builder
	for ev := range ch {
		if ev.Type == StreamEventTypeChunk {
			streamed.WriteString(ev.Content)
		}
	}
	if streamed.String() != "review text" {
		t.Fatalf("expected output item text, got %q", streamed.String())
	}
}

func TestOpenAICodexClientOutputItemDoneDoesNotDuplicateDeltas(t *testing.T) {
	client := newTestCodexClient(t)
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"review ","sequence_number":1}`),
				mustResponseEvent(t, `{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"review text","annotations":[]}]},"sequence_number":2}`),
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":3,"response":{"id":"r1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
	}

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "review this"}}, nil)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}
	var streamed strings.Builder
	for ev := range ch {
		if ev.Type == StreamEventTypeChunk {
			streamed.WriteString(ev.Content)
		}
	}
	if streamed.String() != "review text" {
		t.Fatalf("expected non-duplicated output item text, got %q", streamed.String())
	}
}

func TestOpenAICodexClientTerminalResponseEvents(t *testing.T) {
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
			event:   `{"type":"response.incomplete","sequence_number":1,"response":{"id":"resp_1","created_at":0,"incomplete_details":{"reason":"content_filter"},"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1,"status":"incomplete"}}`,
			wantErr: []string{"response incomplete", "content_filter"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestCodexClient(t)
			client.maxRetries = 1
			client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
				return &fakeResponseStream{events: []responses.ResponseStreamEventUnion{mustResponseEvent(t, tt.event)}}
			}

			ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hello"}}, nil)
			if err != nil {
				t.Fatalf("StreamChat() failed: %v", err)
			}

			var terminalErr error
			for ev := range ch {
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

func TestOpenAICodexClientToolCallsReplayItemsWithoutPreviousResponseID(t *testing.T) {
	client := newTestCodexClient(t)

	var capturedParams []responses.ResponseNewParams
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		capturedParams = append(capturedParams, params)
		if len(capturedParams) == 1 {
			return &fakeResponseStream{
				events: []responses.ResponseStreamEventUnion{
					mustResponseEvent(t, `{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"I'll inspect that.","annotations":[]}]},"sequence_number":1}`),
					mustResponseEvent(t, `{"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"go.mod\"}","status":"completed"},"sequence_number":2}`),
					mustResponseEvent(t, `{"type":"response.completed","sequence_number":3,"response":{"id":"resp_1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"encrypted-reasoning","status":"completed"},{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"I'll inspect that.","annotations":[]}]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"go.mod\"}","status":"completed"}],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
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

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "read go.mod"}}, registry)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}

	var streamed strings.Builder
	var toolStartCount int
	var toolEndCount int
	for ev := range ch {
		switch ev.Type {
		case StreamEventTypeChunk:
			streamed.WriteString(ev.Content)
		case StreamEventTypeToolStart:
			toolStartCount++
		case StreamEventTypeToolEnd:
			toolEndCount++
		case StreamEventTypeError:
			t.Fatalf("unexpected stream error: %v", ev.Error)
		}
	}

	if len(capturedParams) != 2 {
		t.Fatalf("expected two Codex response turns, got %d", len(capturedParams))
	}
	if capturedParams[0].PreviousResponseID.Valid() || capturedParams[1].PreviousResponseID.Valid() {
		t.Fatalf("did not expect previous_response_id with store=false")
	}
	if toolStartCount != 1 || toolEndCount != 1 {
		t.Fatalf("expected one tool start/end, got start=%d end=%d", toolStartCount, toolEndCount)
	}
	if streamed.String() != "I'll inspect that.done" {
		t.Fatalf("expected final assistant stream, got %q", streamed.String())
	}

	body, err := json.Marshal(capturedParams[1].Input)
	if err != nil {
		t.Fatalf("marshal second request input: %v", err)
	}
	inputJSON := string(body)
	for _, want := range []string{
		"read go.mod",
		"I'll inspect that.",
		`"type":"reasoning"`,
		`"id":"rs_1"`,
		`"encrypted_content":"encrypted-reasoning"`,
		`"type":"message"`,
		`"role":"assistant"`,
		`"type":"function_call"`,
		`"call_id":"call_1"`,
		`"name":"read_file"`,
		`"type":"function_call_output"`,
		"module github.com/user/keen-code",
	} {
		if !strings.Contains(inputJSON, want) {
			t.Fatalf("expected second request input to contain %q, got %s", want, inputJSON)
		}
	}
}

func TestOpenAICodexClient_PendingState_ErrorMidLoop(t *testing.T) {
	client := newTestCodexClient(t)
	client.maxRetries = 1

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

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "read go.mod"}}, registry)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}

	var hasIncomplete bool
	var incompleteErr error
	for ev := range ch {
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
	for _, want := range []string{"I'll inspect that.", `"type":"message"`, `"role":"assistant"`, `"type":"function_call"`, `"call_id":"call_1"`, `"type":"function_call_output"`, "module github.com/user/keen-code"} {
		if !strings.Contains(pendingJSON, want) {
			t.Fatalf("expected pending state to contain %q, got %s", want, pendingJSON)
		}
	}
}

func TestOpenAICodexClient_PendingState_InjectedOnNextCall(t *testing.T) {
	client := newTestCodexClient(t)
	client.maxRetries = 1

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

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "read go.mod"}}, registry)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}
	for range ch {
	}

	if len(client.pendingState) == 0 {
		t.Fatal("expected pending state after failed turn")
	}
	savedLen := len(client.pendingState)

	ch, err = client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "read go.mod"},
		{Role: RoleUser, Content: "continue"},
	}, registry)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}

	var hasDone bool
	var streamed strings.Builder
	for ev := range ch {
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

	body, err := json.Marshal(capturedRecoveryParams.Input)
	if err != nil {
		t.Fatalf("marshal recovery input: %v", err)
	}
	inputJSON := string(body)
	for _, want := range []string{"read go.mod", "I'll inspect that.", `"type":"message"`, `"role":"assistant"`, `"type":"function_call"`, `"type":"function_call_output"`, "continue"} {
		if !strings.Contains(inputJSON, want) {
			t.Fatalf("expected recovery input to contain %q, got %s", want, inputJSON)
		}
	}
	if len(capturedRecoveryParams.Input.OfInputItemList) != 1+savedLen+1 {
		t.Fatalf("expected pending state inserted before new user message, got %d input items", len(capturedRecoveryParams.Input.OfInputItemList))
	}
}

func TestOpenAICodexClient_PendingState_PreservedWhenRecoveryFailsBeforeProgress(t *testing.T) {
	client := newTestCodexClient(t)
	client.maxRetries = 1
	client.pendingState = []responses.ResponseInputItemUnionParam{
		responses.ResponseInputItemParamOfFunctionCall("{}", "call_old", "read_file"),
		responses.ResponseInputItemParamOfFunctionCallOutput("call_old", `{"content":"old"}`),
	}

	wantPending, err := json.Marshal(client.pendingState)
	if err != nil {
		t.Fatalf("marshal pending state: %v", err)
	}

	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &fakeResponseStream{err: &openai.Error{StatusCode: http.StatusInternalServerError}}
	}

	ch, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "read go.mod"},
		{Role: RoleUser, Content: "continue"},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}

	var hasIncomplete bool
	for ev := range ch {
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

func TestOpenAICodexClient_PendingState_NoAccumulation_EmitsError(t *testing.T) {
	client := newTestCodexClient(t)
	client.maxRetries = 1
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &fakeResponseStream{err: &openai.Error{StatusCode: http.StatusUnauthorized}}
	}

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "test"}}, nil)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}

	var hasError bool
	for ev := range ch {
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

func TestOpenAICodexClient_PendingState_EmptyResponseMidLoop(t *testing.T) {
	client := newTestCodexClient(t)
	client.maxRetries = 1

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

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "read go.mod"}}, registry)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}

	var hasIncomplete bool
	for ev := range ch {
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

func TestOpenAICodexClient_PendingState_ClearedOnSuccess(t *testing.T) {
	client := newTestCodexClient(t)
	client.pendingState = []responses.ResponseInputItemUnionParam{
		responses.ResponseInputItemParamOfFunctionCallOutput("call_old", "old result"),
	}

	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.output_text.delta","delta":"hello","sequence_number":1}`),
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":2,"response":{"id":"resp_1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
	}

	ch, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "original"},
		{Role: RoleUser, Content: "continue"},
	}, nil)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}

	var hasDone bool
	for ev := range ch {
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

func TestFormatCodexStreamErrorIncludesAPIErrorBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Status:     "400 Bad Request",
		Body:       io.NopCloser(bytes.NewBufferString(`{"detail":"bad codex request"}`)),
	}
	apiErr := &openai.Error{
		StatusCode: http.StatusBadRequest,
		Request:    req,
		Response:   resp,
	}

	got := formatCodexStreamError(apiErr).Error()
	if !strings.Contains(got, "HTTP 400") || !strings.Contains(got, "bad codex request") {
		t.Fatalf("expected formatted API error body, got %q", got)
	}
}

func TestFormatCodexStreamErrorWrapsNonAPIError(t *testing.T) {
	err := errors.New("network failed")
	got := formatCodexStreamError(err)
	if !strings.Contains(got.Error(), "stream error: network failed") {
		t.Fatalf("unexpected error: %v", got)
	}
}

func newTestCodexClient(t *testing.T) *OpenAICodexClient {
	t.Helper()
	store := auth.NewStoreAt(filepath.Join(t.TempDir(), "auth.json"))
	if err := store.Set(auth.OpenAICodexProviderID, auth.OAuthCredential{
		Type:         "oauth",
		AccessToken:  "oauth-access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
		AccountID:    "acct_123",
	}); err != nil {
		t.Fatalf("seed auth store: %v", err)
	}
	return &OpenAICodexClient{
		model:       "gpt-5.4",
		authManager: auth.NewOAuthManager(store),
		userAgent:   "keen-code-test",
	}
}

func TestOpenAICodexClient_PromptCacheKey_SetFromSessionID(t *testing.T) {
	client := newTestCodexClient(t)

	var capturedParams responses.ResponseNewParams
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		capturedParams = params
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":1,"response":{"id":"r1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
	}

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil, StreamOptions{SessionID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"})
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}
	for ev := range ch {
		if ev.Type == StreamEventTypeError {
			t.Fatalf("unexpected stream error: %v", ev.Error)
		}
	}

	if !capturedParams.PromptCacheKey.Valid() {
		t.Fatal("expected PromptCacheKey to be set")
	}
	want := "a1b2c3d4e5f67890abcdef1234567890"
	if capturedParams.PromptCacheKey.Value != want {
		t.Fatalf("expected PromptCacheKey %q, got %q", want, capturedParams.PromptCacheKey.Value)
	}
}

func TestOpenAICodexClient_PromptCacheKey_OmittedWithoutSessionID(t *testing.T) {
	client := newTestCodexClient(t)

	var capturedParams responses.ResponseNewParams
	client.responseStreamImpl = func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
		capturedParams = params
		return &fakeResponseStream{
			events: []responses.ResponseStreamEventUnion{
				mustResponseEvent(t, `{"type":"response.completed","sequence_number":1,"response":{"id":"r1","created_at":0,"metadata":{},"model":"gpt-5.4","object":"response","output":[],"parallel_tool_calls":false,"temperature":1,"tool_choice":"auto","tools":[],"top_p":1}}`),
			},
		}
	}

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("StreamChat() failed: %v", err)
	}
	for ev := range ch {
		if ev.Type == StreamEventTypeError {
			t.Fatalf("unexpected stream error: %v", ev.Error)
		}
	}

	if capturedParams.PromptCacheKey.Valid() {
		t.Fatalf("expected PromptCacheKey to be omitted, got %q", capturedParams.PromptCacheKey.Value)
	}
}
