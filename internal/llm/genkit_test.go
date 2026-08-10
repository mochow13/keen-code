package llm

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/tools"
	"google.golang.org/genai"
)

func TestGenkitClient_StreamChat_Success(t *testing.T) {
	client := &GenkitClient{
		g:        &genkit.Genkit{},
		provider: Provider(config.ProviderGoogleAI),
		model:    "googleai/gemini-pro",
	}

	expectedChunks := []string{"Hello", " world", "!"}

	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			for _, chunk := range expectedChunks {
				if !yield(&ai.ModelStreamValue{
					Chunk: &ai.ModelResponseChunk{
						Content: []*ai.Part{ai.NewTextPart(chunk)},
					},
				}, nil) {
					return
				}
			}
			yield(&ai.ModelStreamValue{Done: true}, nil)
		}
	}

	messages := []Message{
		{Role: RoleUser, Content: "Hi"},
	}

	eventCh, err := client.StreamChat(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var receivedChunks []string
	var doneReceived bool

	for event := range eventCh {
		switch event.Type {
		case StreamEventTypeChunk:
			receivedChunks = append(receivedChunks, event.Content)
		case StreamEventTypeDone:
			doneReceived = true
		case StreamEventTypeError:
			t.Fatalf("unexpected error event: %v", event.Error)
		}
	}

	if !doneReceived {
		t.Error("expected done event, but didn't receive one")
	}

	if len(receivedChunks) != len(expectedChunks) {
		t.Errorf("expected %d chunks, got %d", len(expectedChunks), len(receivedChunks))
	}

	for i, expected := range expectedChunks {
		if i >= len(receivedChunks) {
			break
		}
		if receivedChunks[i] != expected {
			t.Errorf("chunk %d: expected %q, got %q", i, expected, receivedChunks[i])
		}
	}
}

func TestGenkitClient_StreamChat_Error(t *testing.T) {
	client := &GenkitClient{
		g:          &genkit.Genkit{},
		provider:   Provider(config.ProviderGoogleAI),
		model:      "googleai/gemini-pro",
		maxRetries: 1,
	}

	expectedErr := errors.New("API error")
	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			yield(nil, expectedErr)
		}
	}

	messages := []Message{
		{Role: RoleUser, Content: "Hi"},
	}

	eventCh, err := client.StreamChat(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var errorReceived bool
	var receivedErr error

	for event := range eventCh {
		if event.Type == StreamEventTypeError {
			errorReceived = true
			receivedErr = event.Error
		}
	}

	if !errorReceived {
		t.Error("expected error event, but didn't receive one")
	}

	if receivedErr != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, receivedErr)
	}
}

func TestGenkitClient_StreamChat_RetriesOnRetryableError(t *testing.T) {
	const testMaxRetries = 2
	expectedErr := errors.New("API error")
	callCount := 0
	client := &GenkitClient{
		g:          &genkit.Genkit{},
		provider:   Provider(config.ProviderGoogleAI),
		model:      "googleai/gemini-pro",
		maxRetries: testMaxRetries,
	}

	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		callCount++
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			yield(nil, expectedErr)
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "Hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var retryEvents []StreamEvent
	var receivedErr error
	for event := range eventCh {
		switch event.Type {
		case StreamEventTypeRetry:
			retryEvents = append(retryEvents, event)
		case StreamEventTypeError:
			receivedErr = event.Error
		}
	}

	if callCount != testMaxRetries {
		t.Fatalf("expected %d stream calls, got %d", testMaxRetries, callCount)
	}
	if len(retryEvents) != testMaxRetries-1 {
		t.Fatalf("expected %d retry events, got %d", testMaxRetries-1, len(retryEvents))
	}
	if retryEvents[0].Attempt != 1 {
		t.Fatalf("expected retry attempt 1, got %d", retryEvents[0].Attempt)
	}
	if receivedErr != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, receivedErr)
	}
}

func TestGenkitClient_StreamChat_EmptyMessages(t *testing.T) {
	client := &GenkitClient{
		g:        &genkit.Genkit{},
		provider: Provider(config.ProviderGoogleAI),
		model:    "googleai/gemini-pro",
	}

	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			yield(&ai.ModelStreamValue{Done: true}, nil)
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var doneReceived bool
	for event := range eventCh {
		if event.Type == StreamEventTypeDone {
			doneReceived = true
		}
	}

	if !doneReceived {
		t.Error("expected done event for empty messages")
	}
}

func TestGenkitClient_StreamChat_ContextCancellation(t *testing.T) {
	client := &GenkitClient{
		g:        &genkit.Genkit{},
		provider: Provider(config.ProviderGoogleAI),
		model:    "googleai/gemini-pro",
	}

	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			<-ctx.Done()
			yield(nil, ctx.Err())
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	messages := []Message{{Role: RoleUser, Content: "Hello"}}
	eventCh, _ := client.StreamChat(ctx, messages, nil)

	var errorReceived bool
	for event := range eventCh {
		if event.Type == StreamEventTypeError {
			errorReceived = true
		}
	}

	if !errorReceived {
		t.Error("expected error event for cancelled context")
	}
}

func TestGenkitClient_StreamChat_MultipleMessages(t *testing.T) {
	client := &GenkitClient{
		g:        &genkit.Genkit{},
		provider: Provider(config.ProviderGoogleAI),
		model:    "googleai/gemini-pro",
	}

	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			yield(&ai.ModelStreamValue{Done: true}, nil)
		}
	}

	messages := []Message{
		{Role: RoleSystem, Content: "You are helpful"},
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleUser, Content: "How are you?"},
	}

	eventCh, err := client.StreamChat(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var doneReceived bool
	for event := range eventCh {
		if event.Type == StreamEventTypeDone {
			doneReceived = true
		}
	}

	if !doneReceived {
		t.Error("expected done event")
	}
}

func TestToGenkitMessages_RendersTurnMemoryForAssistant(t *testing.T) {
	messages := toGenkitMessages([]Message{
		{
			Role:    RoleAssistant,
			Content: "done",
			TurnMemory: &TurnMemory{
				ToolActivity: []HistoricalToolActivity{{Tool: "read_file", Input: map[string]any{"path": "a.go"}, Status: "success"}},
			},
		},
	})

	if len(messages) != 3 {
		t.Fatalf("expected tool call, result, and final message, got %#v", messages)
	}
	input, ok := messages[0].Content[0].ToolRequest.Input.(map[string]any)
	if !ok || input["path"] != "a.go" {
		t.Fatalf("unexpected retained tool input %#v", messages[0].Content[0].ToolRequest.Input)
	}
	if got := messages[1].Content[0].ToolResponse.Output; got != `{"status":"success"}` {
		t.Fatalf("unexpected retained tool result %q", got)
	}
}

func TestGenkitClient_StreamChat_EmptyChunkContent(t *testing.T) {
	client := &GenkitClient{
		g:        &genkit.Genkit{},
		provider: Provider(config.ProviderGoogleAI),
		model:    "googleai/gemini-pro",
	}

	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			if !yield(&ai.ModelStreamValue{
				Chunk: &ai.ModelResponseChunk{
					Content: []*ai.Part{},
				},
			}, nil) {
				return
			}
			yield(&ai.ModelStreamValue{Done: true}, nil)
		}
	}

	messages := []Message{{Role: RoleUser, Content: "Hello"}}
	eventCh, _ := client.StreamChat(context.Background(), messages, nil)

	var chunkCount int
	var doneReceived bool

	for event := range eventCh {
		switch event.Type {
		case StreamEventTypeChunk:
			chunkCount++
		case StreamEventTypeDone:
			doneReceived = true
		}
	}

	if chunkCount != 0 {
		t.Errorf("expected 0 chunks for empty content, got %d", chunkCount)
	}

	if !doneReceived {
		t.Error("expected done event")
	}
}

func TestGenkitClient_executeTools_Success(t *testing.T) {
	client := &GenkitClient{}

	registry := tools.NewRegistry()
	if err := registry.Register(&successTool{}); err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	toolRequests := []*ai.ToolRequest{
		{
			Name:  "success_tool",
			Input: map[string]any{"message": "hello"},
			Ref:   "ref-success",
		},
	}

	eventCh := make(chan StreamEvent, 4)
	parts, activities := client.executeTools(context.Background(), toolRequests, registry, eventCh)

	if len(parts) != 1 {
		t.Fatalf("expected 1 tool response part, got %d", len(parts))
	}
	if len(activities) != 1 || activities[0].Status != "success" || !activities[0].HasRawOutput {
		t.Fatalf("activities = %#v", activities)
	}

	startEvent := <-eventCh
	if startEvent.Type != StreamEventTypeToolStart {
		t.Fatalf("expected first event %q, got %q", StreamEventTypeToolStart, startEvent.Type)
	}
	if startEvent.ToolCall == nil || startEvent.ToolCall.Name != "success_tool" {
		t.Fatalf("unexpected tool_start event payload: %+v", startEvent.ToolCall)
	}

	endEvent := <-eventCh
	if endEvent.Type != StreamEventTypeToolEnd {
		t.Fatalf("expected second event %q, got %q", StreamEventTypeToolEnd, endEvent.Type)
	}
	if endEvent.ToolCall == nil {
		t.Fatal("expected tool_end ToolCall")
	}
	if endEvent.ToolCall.Error != "" {
		t.Fatalf("expected empty tool error, got %q", endEvent.ToolCall.Error)
	}

	if parts[0].ToolResponse == nil {
		t.Fatal("expected ToolResponse in part")
	}
	if parts[0].ToolResponse.Name != "success_tool" {
		t.Fatalf("expected tool response name success_tool, got %q", parts[0].ToolResponse.Name)
	}
	if parts[0].ToolResponse.Ref != "ref-success" {
		t.Fatalf("expected tool response ref ref-success, got %q", parts[0].ToolResponse.Ref)
	}

	outputMap, ok := parts[0].ToolResponse.Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %T", parts[0].ToolResponse.Output)
	}
	if outputMap["result"] != "processed: hello" {
		t.Fatalf("expected result output 'processed: hello', got %v", outputMap["result"])
	}
}

func TestGenkitClient_StreamChat_ToolInvocation(t *testing.T) {
	client := &GenkitClient{
		g:        &genkit.Genkit{},
		provider: Provider(config.ProviderGoogleAI),
		model:    "googleai/gemini-pro",
	}

	callCount := 0
	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			callCount++
			if callCount == 1 {
				if !yield(&ai.ModelStreamValue{
					Chunk: &ai.ModelResponseChunk{
						Content: []*ai.Part{ai.NewTextPart("I'll use the tool")},
					},
				}, nil) {
					return
				}
				yield(&ai.ModelStreamValue{
					Done: true,
					Response: &ai.ModelResponse{
						Message: &ai.Message{
							Role: ai.RoleModel,
							Content: []*ai.Part{
								ai.NewToolRequestPart(&ai.ToolRequest{
									Name:  "success_tool",
									Input: map[string]any{"message": "hello"},
									Ref:   "ref-123",
								}),
							},
						},
					},
				}, nil)
			} else {
				if !yield(&ai.ModelStreamValue{
					Chunk: &ai.ModelResponseChunk{
						Content: []*ai.Part{ai.NewTextPart("Tool result: processed: hello")},
					},
				}, nil) {
					return
				}
				yield(&ai.ModelStreamValue{
					Done: true,
					Response: &ai.ModelResponse{
						Message: &ai.Message{Role: ai.RoleModel},
					},
				}, nil)
			}
		}
	}

	registry := tools.NewRegistry()
	if err := registry.Register(&successTool{}); err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	messages := []Message{
		{Role: RoleUser, Content: "Call the tool"},
	}

	eventCh, err := client.StreamChat(context.Background(), messages, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var chunks []string
	var toolStartReceived bool
	var toolEndReceived bool
	var doneReceived bool

	for event := range eventCh {
		switch event.Type {
		case StreamEventTypeChunk:
			chunks = append(chunks, event.Content)
		case StreamEventTypeToolStart:
			toolStartReceived = true
			if event.ToolCall == nil {
				t.Error("expected ToolCall in tool_start event")
			} else if event.ToolCall.Name != "success_tool" {
				t.Errorf("expected tool name 'success_tool', got %q", event.ToolCall.Name)
			}
		case StreamEventTypeToolEnd:
			toolEndReceived = true
			if event.ToolCall == nil {
				t.Error("expected ToolCall in tool_end event")
			} else if event.ToolCall.Name != "success_tool" {
				t.Errorf("expected tool name 'success_tool', got %q", event.ToolCall.Name)
			}
			if event.ToolCall.Output == nil {
				t.Error("expected tool output in tool_end event")
			}
		case StreamEventTypeDone:
			doneReceived = true
		case StreamEventTypeError:
			t.Fatalf("unexpected error event: %v", event.Error)
		}
	}

	if !toolStartReceived {
		t.Error("expected tool_start event")
	}
	if !toolEndReceived {
		t.Error("expected tool_end event")
	}
	if !doneReceived {
		t.Error("expected done event")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls to GenerateStream (1 for tool request, 1 for final response), got %d", callCount)
	}
	if len(chunks) != 2 {
		t.Errorf("expected 2 text chunks, got %d", len(chunks))
	}
}

func TestGenkitClient_executeTools_Error(t *testing.T) {
	client := &GenkitClient{}

	registry := tools.NewRegistry()
	if err := registry.Register(&failingTool{}); err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	toolRequests := []*ai.ToolRequest{
		{
			Name:  "failing_tool",
			Input: map[string]any{"message": "hello"},
			Ref:   "ref-error",
		},
	}

	eventCh := make(chan StreamEvent, 4)
	parts, activities := client.executeTools(context.Background(), toolRequests, registry, eventCh)

	if len(parts) != 1 {
		t.Fatalf("expected 1 tool response part, got %d", len(parts))
	}
	if len(activities) != 1 || activities[0].Status != "error" || !activities[0].HasRawOutput {
		t.Fatalf("activities = %#v", activities)
	}

	startEvent := <-eventCh
	if startEvent.Type != StreamEventTypeToolStart {
		t.Fatalf("expected first event %q, got %q", StreamEventTypeToolStart, startEvent.Type)
	}

	endEvent := <-eventCh
	if endEvent.Type != StreamEventTypeToolEnd {
		t.Fatalf("expected second event %q, got %q", StreamEventTypeToolEnd, endEvent.Type)
	}
	if endEvent.ToolCall == nil {
		t.Fatal("expected tool_end ToolCall")
	}
	if endEvent.ToolCall.Error != "tool failed" {
		t.Fatalf("expected tool error 'tool failed', got %q", endEvent.ToolCall.Error)
	}

	if parts[0].ToolResponse == nil {
		t.Fatal("expected ToolResponse in part")
	}
	if parts[0].ToolResponse.Name != "failing_tool" {
		t.Fatalf("expected tool response name failing_tool, got %q", parts[0].ToolResponse.Name)
	}
	if parts[0].ToolResponse.Ref != "ref-error" {
		t.Fatalf("expected tool response ref ref-error, got %q", parts[0].ToolResponse.Ref)
	}

	outputMap, ok := parts[0].ToolResponse.Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %T", parts[0].ToolResponse.Output)
	}
	if outputMap["error"] != "tool failed" {
		t.Fatalf("expected error output 'tool failed', got %v", outputMap["error"])
	}
}

func TestGenkitClient_PendingState_ErrorMidLoop(t *testing.T) {
	callCount := 0
	client := &GenkitClient{
		g:          &genkit.Genkit{},
		provider:   Provider(config.ProviderGoogleAI),
		model:      "googleai/gemini-pro",
		maxRetries: 1,
	}

	expectedErr := errors.New("API error")
	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			callCount++
			if callCount == 1 {
				yield(&ai.ModelStreamValue{
					Done: true,
					Response: &ai.ModelResponse{
						Message: &ai.Message{
							Role: ai.RoleModel,
							Content: []*ai.Part{
								ai.NewToolRequestPart(&ai.ToolRequest{
									Name:  "success_tool",
									Input: map[string]any{"message": "hi"},
									Ref:   "ref-1",
								}),
							},
						},
					},
				}, nil)
			} else {
				yield(nil, expectedErr)
			}
		}
	}

	registry := tools.NewRegistry()
	if err := registry.Register(&successTool{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "go"}}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	if len(client.pendingState) != 2 {
		t.Fatalf("expected 2 pending messages (model + tool), got %d", len(client.pendingState))
	}
	if client.pendingState[0].Role != ai.RoleModel {
		t.Fatalf("expected first pending message to be model role, got %q", client.pendingState[0].Role)
	}
	if client.pendingState[1].Role != ai.RoleTool {
		t.Fatalf("expected second pending message to be tool role, got %q", client.pendingState[1].Role)
	}
}

func TestGenkitClient_PendingState_InjectedOnNextCall(t *testing.T) {
	callCount := 0
	var capturedOpts [][]ai.GenerateOption
	client := &GenkitClient{
		g:          &genkit.Genkit{},
		provider:   Provider(config.ProviderGoogleAI),
		model:      "googleai/gemini-pro",
		maxRetries: 1,
	}

	expectedErr := errors.New("API error")
	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			callCount++
			capturedOpts = append(capturedOpts, opts)
			switch callCount {
			case 1:
				yield(&ai.ModelStreamValue{
					Done: true,
					Response: &ai.ModelResponse{
						Message: &ai.Message{
							Role: ai.RoleModel,
							Content: []*ai.Part{
								ai.NewToolRequestPart(&ai.ToolRequest{
									Name:  "success_tool",
									Input: map[string]any{"message": "hi"},
									Ref:   "ref-1",
								}),
							},
						},
					},
				}, nil)
			case 2:
				yield(nil, expectedErr)
			default:
				if !yield(&ai.ModelStreamValue{
					Chunk: &ai.ModelResponseChunk{
						Content: []*ai.Part{ai.NewTextPart("recovered")},
					},
				}, nil) {
					return
				}
				yield(&ai.ModelStreamValue{
					Done: true,
					Response: &ai.ModelResponse{
						Message: &ai.Message{Role: ai.RoleModel},
					},
				}, nil)
			}
		}
	}

	registry := tools.NewRegistry()
	if err := registry.Register(&successTool{}); err != nil {
		t.Fatalf("register: %v", err)
	}

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "go"}}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range ch {
	}

	if len(client.pendingState) == 0 {
		t.Fatal("expected pending state after failed turn")
	}
	savedLen := len(client.pendingState)

	ch, err = client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "go"},
		{Role: RoleUser, Content: "continue"},
	}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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

	// Recovery call injects pending state (savedLen messages) before "continue"
	// so total = initial messages (2) + savedLen pending + last "continue" already in initial
	// Actually injection inserts before the last element:
	// initial: [system?+user "go", user "continue"] = 2 messages
	// after injection: [user "go", pending..., user "continue"] = 1 + savedLen + 1
	if len(capturedOpts) < 3 {
		t.Fatalf("expected at least 3 stream calls, got %d", len(capturedOpts))
	}
	_ = savedLen
}

func TestGenkitClient_PendingState_PreservedWhenRecoveryFailsBeforeProgress(t *testing.T) {
	client := &GenkitClient{
		g:          &genkit.Genkit{},
		provider:   Provider(config.ProviderGoogleAI),
		model:      "googleai/gemini-pro",
		maxRetries: 1,
		pendingState: []*ai.Message{
			{Role: ai.RoleModel, Content: []*ai.Part{ai.NewTextPart("prior tool use")}},
			{Role: ai.RoleTool, Content: []*ai.Part{ai.NewTextPart("prior tool result")}},
		},
	}

	wantLen := len(client.pendingState)
	expectedErr := errors.New("API error")
	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			yield(nil, expectedErr)
		}
	}

	ch, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "go"},
		{Role: RoleUser, Content: "continue"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	if len(client.pendingState) != wantLen {
		t.Fatalf("expected pending state length %d preserved, got %d", wantLen, len(client.pendingState))
	}
}

func TestGenkitClient_PendingState_NoAccumulation_EmitsError(t *testing.T) {
	expectedErr := errors.New("API error")
	client := &GenkitClient{
		g:          &genkit.Genkit{},
		provider:   Provider(config.ProviderGoogleAI),
		model:      "googleai/gemini-pro",
		maxRetries: 1,
	}
	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			yield(nil, expectedErr)
		}
	}

	ch, err := client.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func TestGenkitClient_PendingState_ClearedOnSuccess(t *testing.T) {
	client := &GenkitClient{
		g:        &genkit.Genkit{},
		provider: Provider(config.ProviderGoogleAI),
		model:    "googleai/gemini-pro",
		pendingState: []*ai.Message{
			{Role: ai.RoleModel, Content: []*ai.Part{ai.NewTextPart("prior tool use")}},
			{Role: ai.RoleTool, Content: []*ai.Part{ai.NewTextPart("prior tool result")}},
		},
	}

	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			if !yield(&ai.ModelStreamValue{
				Chunk: &ai.ModelResponseChunk{
					Content: []*ai.Part{ai.NewTextPart("done")},
				},
			}, nil) {
				return
			}
			yield(&ai.ModelStreamValue{
				Done: true,
				Response: &ai.ModelResponse{
					Message: &ai.Message{Role: ai.RoleModel},
				},
			}, nil)
		}
	}

	ch, err := client.StreamChat(context.Background(), []Message{
		{Role: RoleUser, Content: "original"},
		{Role: RoleUser, Content: "continue"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func TestBuildGenkitGenerateConfig_CustomHeaders(t *testing.T) {
	cfg := buildGenkitGenerateConfig("", Provider(config.ProviderGoogleAI), map[string]string{
		"x-custom-header": "custom-value",
		"x-another":       "another-value",
	})
	if cfg == nil {
		t.Fatal("expected config")
	}
	if cfg.HTTPOptions == nil {
		t.Fatal("expected HTTPOptions")
	}
	if got := cfg.HTTPOptions.Headers.Get("x-custom-header"); got != "custom-value" {
		t.Fatalf("expected x-custom-header %q, got %q", "custom-value", got)
	}
	if got := cfg.HTTPOptions.Headers.Get("x-another"); got != "another-value" {
		t.Fatalf("expected x-another %q, got %q", "another-value", got)
	}
}

func TestBuildGenkitGenerateConfig_HeadersWithThinking(t *testing.T) {
	cfg := buildGenkitGenerateConfig("low", Provider(config.ProviderGoogleAI), map[string]string{
		"x-custom-header": "custom-value",
	})
	if cfg == nil {
		t.Fatal("expected config")
	}
	if cfg.ThinkingConfig == nil || cfg.ThinkingConfig.ThinkingLevel != genai.ThinkingLevelLow {
		t.Fatal("expected thinking config")
	}
	if cfg.HTTPOptions == nil {
		t.Fatal("expected HTTPOptions")
	}
	if got := cfg.HTTPOptions.Headers.Get("x-custom-header"); got != "custom-value" {
		t.Fatalf("expected x-custom-header %q, got %q", "custom-value", got)
	}
}

func TestBuildGenkitGenerateConfig_NoHeaders(t *testing.T) {
	cfg := buildGenkitGenerateConfig("", Provider(config.ProviderGoogleAI), nil)
	if cfg != nil {
		t.Fatalf("expected nil config without thinking or headers, got %+v", cfg)
	}
}

func TestGenkitGenerateConfig_CustomHeadersReachWire(t *testing.T) {
	ctx := context.Background()

	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"candidates": [{
				"content": {
					"parts": [{"text": "hello"}],
					"role": "model"
				}
			}]
		}`))
	}))
	defer server.Close()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: "test-api-key",
		HTTPOptions: genai.HTTPOptions{
			BaseURL: server.URL,
		},
	})
	if err != nil {
		t.Fatalf("failed to create genai client: %v", err)
	}

	cfg := buildGenkitGenerateConfig("", Provider(config.ProviderGoogleAI), map[string]string{
		"x-custom-header": "custom-value",
		"x-another":       "another-value",
	})

	_, err = client.Models.GenerateContent(ctx, "gemini-test", genai.Text("hi"), cfg)
	if err != nil {
		t.Fatalf("GenerateContent failed: %v", err)
	}

	if got := capturedHeaders.Get("x-custom-header"); got != "custom-value" {
		t.Fatalf("expected x-custom-header %q, got %q", "custom-value", got)
	}
	if got := capturedHeaders.Get("x-another"); got != "another-value" {
		t.Fatalf("expected x-another %q, got %q", "another-value", got)
	}
}
