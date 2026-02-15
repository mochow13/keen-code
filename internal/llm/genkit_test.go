package llm

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

func TestGenkitClient_StreamChat_Success(t *testing.T) {
	client := &GenkitClient{
		g:        &genkit.Genkit{},
		provider: ProviderAnthropic,
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

	eventCh, err := client.StreamChat(context.Background(), messages)
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
		provider: ProviderAnthropic,
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

	eventCh, err := client.StreamChat(context.Background(), messages)
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
		provider: ProviderAnthropic,
		model:    "anthropic/claude-3-haiku",
	}

	client.streamImpl = func(ctx context.Context, g *genkit.Genkit, opts ...ai.GenerateOption) iter.Seq2[*ai.ModelStreamValue, error] {
		return func(yield func(*ai.ModelStreamValue, error) bool) {
			yield(&ai.ModelStreamValue{Done: true}, nil)
		}
	}

	eventCh, err := client.StreamChat(context.Background(), []Message{})
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
		provider: ProviderGemini,
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
	eventCh, _ := client.StreamChat(ctx, messages)

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
		provider: ProviderOpenAI,
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

	eventCh, err := client.StreamChat(context.Background(), messages)
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
		provider: ProviderAnthropic,
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
	eventCh, _ := client.StreamChat(context.Background(), messages)

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
