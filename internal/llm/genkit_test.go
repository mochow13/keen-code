package llm

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/tools"
)

func TestGenkitClient_StreamChat_Success(t *testing.T) {
	client := &GenkitClient{
		g:        &genkit.Genkit{},
		provider: Provider(config.ProviderAnthropic),
		model:    "anthropic/claude-3-haiku",
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
		g:        &genkit.Genkit{},
		provider: Provider(config.ProviderAnthropic),
		model:    "anthropic/claude-3-haiku",
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

func TestGenkitClient_StreamChat_EmptyMessages(t *testing.T) {
	client := &GenkitClient{
		g:        &genkit.Genkit{},
		provider: Provider(config.ProviderAnthropic),
		model:    "anthropic/claude-3-haiku",
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
		provider: Provider(config.ProviderOpenAI),
		model:    "openai/gpt-4",
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

func TestGenkitClient_StreamChat_EmptyChunkContent(t *testing.T) {
	client := &GenkitClient{
		g:        &genkit.Genkit{},
		provider: Provider(config.ProviderAnthropic),
		model:    "anthropic/claude-3-haiku",
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

func TestGenkitClient_StreamChat_ReasoningChunks(t *testing.T) {
	client := &GenkitClient{
		g:        &genkit.Genkit{},
		provider: Provider(config.ProviderAnthropic),
		model:    "anthropic/claude-opus-4",
	}

	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			if !yield(&ai.ModelStreamValue{
				Chunk: &ai.ModelResponseChunk{
					Content: []*ai.Part{ai.NewReasoningPart("thinking step 1", nil)},
				},
			}, nil) {
				return
			}
			if !yield(&ai.ModelStreamValue{
				Chunk: &ai.ModelResponseChunk{
					Content: []*ai.Part{ai.NewReasoningPart("thinking step 2", nil)},
				},
			}, nil) {
				return
			}
			if !yield(&ai.ModelStreamValue{
				Chunk: &ai.ModelResponseChunk{
					Content: []*ai.Part{ai.NewTextPart("final answer")},
				},
			}, nil) {
				return
			}
			yield(&ai.ModelStreamValue{Done: true}, nil)
		}
	}

	messages := []Message{{Role: RoleUser, Content: "Think about this"}}
	eventCh, err := client.StreamChat(context.Background(), messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reasoningChunks []string
	var textChunks []string
	var doneReceived bool

	for event := range eventCh {
		switch event.Type {
		case StreamEventTypeReasoningChunk:
			reasoningChunks = append(reasoningChunks, event.Content)
		case StreamEventTypeChunk:
			textChunks = append(textChunks, event.Content)
		case StreamEventTypeDone:
			doneReceived = true
		case StreamEventTypeError:
			t.Fatalf("unexpected error event: %v", event.Error)
		}
	}

	if !doneReceived {
		t.Error("expected done event")
	}
	if len(reasoningChunks) != 2 {
		t.Errorf("expected 2 reasoning chunks, got %d", len(reasoningChunks))
	}
	if len(textChunks) != 1 {
		t.Errorf("expected 1 text chunk, got %d", len(textChunks))
	}
	if len(reasoningChunks) >= 1 && reasoningChunks[0] != "thinking step 1" {
		t.Errorf("reasoning chunk 0: expected %q, got %q", "thinking step 1", reasoningChunks[0])
	}
	if len(reasoningChunks) >= 2 && reasoningChunks[1] != "thinking step 2" {
		t.Errorf("reasoning chunk 1: expected %q, got %q", "thinking step 2", reasoningChunks[1])
	}
	if len(textChunks) >= 1 && textChunks[0] != "final answer" {
		t.Errorf("text chunk 0: expected %q, got %q", "final answer", textChunks[0])
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
	parts := client.executeTools(context.Background(), toolRequests, registry, eventCh)

	if len(parts) != 1 {
		t.Fatalf("expected 1 tool response part, got %d", len(parts))
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
		provider: Provider(config.ProviderAnthropic),
		model:    "anthropic/claude-3-haiku",
	}

	callCount := 0
	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			callCount++
			if callCount == 1 {
				// First call: LLM requests a tool
				yield(&ai.ModelStreamValue{
					Chunk: &ai.ModelResponseChunk{
						Content: []*ai.Part{ai.NewTextPart("I'll use the tool")},
					},
				}, nil)
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
				// Second call: LLM responds with final answer
				yield(&ai.ModelStreamValue{
					Chunk: &ai.ModelResponseChunk{
						Content: []*ai.Part{ai.NewTextPart("Tool result: processed: hello")},
					},
				}, nil)
				yield(&ai.ModelStreamValue{Done: true}, nil)
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
	parts := client.executeTools(context.Background(), toolRequests, registry, eventCh)

	if len(parts) != 1 {
		t.Fatalf("expected 1 tool response part, got %d", len(parts))
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

type successTool struct{}

func (t *successTool) Name() string { return "success_tool" }

func (t *successTool) Description() string { return "always succeeds" }

func (t *successTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "The message to process",
			},
		},
		"required": []string{"message"},
	}
}

func (t *successTool) Execute(ctx context.Context, input any) (any, error) {
	params, ok := input.(map[string]any)
	if !ok {
		return nil, errors.New("invalid input type")
	}
	message, _ := params["message"].(string)
	return map[string]any{"result": "processed: " + message}, nil
}

type failingTool struct{}

func (t *failingTool) Name() string { return "failing_tool" }

func (t *failingTool) Description() string { return "always fails" }

func (t *failingTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
	}
}

func (t *failingTool) Execute(ctx context.Context, input any) (any, error) {
	return nil, errors.New("tool failed")
}
