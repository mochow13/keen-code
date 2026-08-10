package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/tools"
)

type mockBedrockStream struct {
	events []brtypes.ConverseStreamOutput
	err    error
}

func (m *mockBedrockStream) Events() <-chan brtypes.ConverseStreamOutput {
	ch := make(chan brtypes.ConverseStreamOutput, len(m.events))
	for _, ev := range m.events {
		ch <- ev
	}
	close(ch)
	return ch
}

func (m *mockBedrockStream) Close() error { return nil }
func (m *mockBedrockStream) Err() error   { return m.err }

func makeBedrockTextDelta(index int32, text string) brtypes.ConverseStreamOutput {
	return &brtypes.ConverseStreamOutputMemberContentBlockDelta{
		Value: brtypes.ContentBlockDeltaEvent{
			ContentBlockIndex: aws.Int32(index),
			Delta:             &brtypes.ContentBlockDeltaMemberText{Value: text},
		},
	}
}

func makeBedrockReasoningDelta(index int32, text string) brtypes.ConverseStreamOutput {
	return &brtypes.ConverseStreamOutputMemberContentBlockDelta{
		Value: brtypes.ContentBlockDeltaEvent{
			ContentBlockIndex: aws.Int32(index),
			Delta: &brtypes.ContentBlockDeltaMemberReasoningContent{
				Value: &brtypes.ReasoningContentBlockDeltaMemberText{Value: text},
			},
		},
	}
}

func makeBedrockReasoningSignatureDelta(index int32, signature string) brtypes.ConverseStreamOutput {
	return &brtypes.ConverseStreamOutputMemberContentBlockDelta{
		Value: brtypes.ContentBlockDeltaEvent{
			ContentBlockIndex: aws.Int32(index),
			Delta: &brtypes.ContentBlockDeltaMemberReasoningContent{
				Value: &brtypes.ReasoningContentBlockDeltaMemberSignature{Value: signature},
			},
		},
	}
}

func makeBedrockToolUseStart(index int32, id, name string) brtypes.ConverseStreamOutput {
	return &brtypes.ConverseStreamOutputMemberContentBlockStart{
		Value: brtypes.ContentBlockStartEvent{
			ContentBlockIndex: aws.Int32(index),
			Start: &brtypes.ContentBlockStartMemberToolUse{
				Value: brtypes.ToolUseBlockStart{
					ToolUseId: aws.String(id),
					Name:      aws.String(name),
				},
			},
		},
	}
}

func makeBedrockToolUseDelta(index int32, input string) brtypes.ConverseStreamOutput {
	return &brtypes.ConverseStreamOutputMemberContentBlockDelta{
		Value: brtypes.ContentBlockDeltaEvent{
			ContentBlockIndex: aws.Int32(index),
			Delta: &brtypes.ContentBlockDeltaMemberToolUse{
				Value: brtypes.ToolUseBlockDelta{Input: aws.String(input)},
			},
		},
	}
}

func makeBedrockContentBlockStop(index int32) brtypes.ConverseStreamOutput {
	return &brtypes.ConverseStreamOutputMemberContentBlockStop{
		Value: brtypes.ContentBlockStopEvent{ContentBlockIndex: aws.Int32(index)},
	}
}

func makeBedrockMetadata(input, output, read, write int32) brtypes.ConverseStreamOutput {
	total := input + output
	return &brtypes.ConverseStreamOutputMemberMetadata{
		Value: brtypes.ConverseStreamMetadataEvent{
			Usage: &brtypes.TokenUsage{
				InputTokens:           aws.Int32(input),
				OutputTokens:          aws.Int32(output),
				TotalTokens:           aws.Int32(total),
				CacheReadInputTokens:  aws.Int32(read),
				CacheWriteInputTokens: aws.Int32(write),
			},
		},
	}
}

func TestBedrockClient_PromptCachingUsesCachePoint(t *testing.T) {
	var captured *bedrockruntime.ConverseStreamInput
	registry := tools.NewRegistry()
	if err := registry.Register(&successTool{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	c := &BedrockClient{model: "global.anthropic.claude-sonnet-4-6"}
	c.streamImpl = func(ctx context.Context, params *bedrockruntime.ConverseStreamInput) (bedrockStream, error) {
		captured = params
		return &mockBedrockStream{events: []brtypes.ConverseStreamOutput{
			makeBedrockTextDelta(0, "ok"),
			makeBedrockContentBlockStop(0),
		}}, nil
	}

	eventCh, err := c.StreamChat(context.Background(), []Message{
		{Role: RoleSystem, Content: "system prompt"},
		{Role: RoleUser, Content: "hi"},
	}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if captured == nil {
		t.Fatal("expected captured Bedrock request")
	}
	if len(captured.System) != 2 {
		t.Fatalf("expected system text and cachePoint, got %d blocks", len(captured.System))
	}
	cacheBlock, ok := captured.System[1].(*brtypes.SystemContentBlockMemberCachePoint)
	if !ok {
		t.Fatalf("expected system cachePoint block, got %T", captured.System[1])
	}
	if cacheBlock.Value.Type != brtypes.CachePointTypeDefault {
		t.Fatalf("expected default cachePoint, got %q", cacheBlock.Value.Type)
	}
	if captured.ToolConfig == nil || len(captured.ToolConfig.Tools) != 2 {
		t.Fatalf("expected tool spec and cachePoint, got %#v", captured.ToolConfig)
	}
	if _, ok := captured.ToolConfig.Tools[1].(*brtypes.ToolMemberCachePoint); !ok {
		t.Fatalf("expected tool cachePoint block, got %T", captured.ToolConfig.Tools[1])
	}
	if len(captured.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(captured.Messages))
	}
	if len(captured.Messages[0].Content) != 2 {
		t.Fatalf("expected user text and cachePoint, got %d blocks", len(captured.Messages[0].Content))
	}
	messageCacheBlock, ok := captured.Messages[0].Content[1].(*brtypes.ContentBlockMemberCachePoint)
	if !ok {
		t.Fatalf("expected message cachePoint block, got %T", captured.Messages[0].Content[1])
	}
	if messageCacheBlock.Value.Type != brtypes.CachePointTypeDefault {
		t.Fatalf("expected default message cachePoint, got %q", messageCacheBlock.Value.Type)
	}
}

func TestBedrockClient_OneShotSkipsPromptCaching(t *testing.T) {
	var captured *bedrockruntime.ConverseStreamInput

	c := &BedrockClient{model: "global.anthropic.claude-sonnet-4-6"}
	c.streamImpl = func(ctx context.Context, params *bedrockruntime.ConverseStreamInput) (bedrockStream, error) {
		captured = params
		return &mockBedrockStream{events: []brtypes.ConverseStreamOutput{
			makeBedrockTextDelta(0, "ok"),
			makeBedrockContentBlockStop(0),
		}}, nil
	}

	eventCh, err := c.StreamChat(context.Background(), []Message{
		{Role: RoleSystem, Content: "system prompt"},
		{Role: RoleUser, Content: "hi"},
	}, nil, StreamOptions{OneShot: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if captured == nil {
		t.Fatal("expected captured Bedrock request")
	}
	if len(captured.System) != 1 {
		t.Fatalf("expected only system text for oneshot, got %d blocks", len(captured.System))
	}
	if _, ok := captured.System[0].(*brtypes.SystemContentBlockMemberText); !ok {
		t.Fatalf("expected system text block, got %T", captured.System[0])
	}
	if len(captured.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(captured.Messages))
	}
	if len(captured.Messages[0].Content) != 1 {
		t.Fatalf("expected only user text for oneshot, got %d blocks", len(captured.Messages[0].Content))
	}
	if _, ok := captured.Messages[0].Content[0].(*brtypes.ContentBlockMemberText); !ok {
		t.Fatalf("expected user text block, got %T", captured.Messages[0].Content[0])
	}
}

func TestBedrockClient_StreamChat_TextReasoningUsage(t *testing.T) {
	c := &BedrockClient{model: "global.anthropic.claude-sonnet-4-6"}
	c.streamImpl = func(ctx context.Context, params *bedrockruntime.ConverseStreamInput) (bedrockStream, error) {
		return &mockBedrockStream{events: []brtypes.ConverseStreamOutput{
			makeBedrockReasoningDelta(0, "think"),
			makeBedrockContentBlockStop(0),
			makeBedrockTextDelta(1, "hello"),
			makeBedrockTextDelta(1, " world"),
			makeBedrockContentBlockStop(1),
			makeBedrockMetadata(100, 12, 70, 20),
		}}, nil
	}

	eventCh, err := c.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reasoning string
	var text string
	var usage *TokenUsage
	var done bool
	for event := range eventCh {
		switch event.Type {
		case StreamEventTypeReasoningChunk:
			reasoning += event.Content
		case StreamEventTypeChunk:
			text += event.Content
		case StreamEventTypeUsage:
			usage = event.Usage
		case StreamEventTypeDone:
			done = true
		case StreamEventTypeError:
			t.Fatalf("unexpected error: %v", event.Error)
		}
	}

	if reasoning != "think" {
		t.Fatalf("expected reasoning chunk, got %q", reasoning)
	}
	if text != "hello world" {
		t.Fatalf("expected text chunks, got %q", text)
	}
	if usage == nil {
		t.Fatal("expected usage event")
	}
	if usage.InputTokens != 190 || usage.OutputTokens != 12 || usage.TotalTokens != 202 || usage.CachedTokens != 90 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
	if !done {
		t.Fatal("expected done event")
	}
}

func TestBedrockUsage_IncludesCacheTokensInInputFootprint(t *testing.T) {
	usage := bedrockUsage(&brtypes.TokenUsage{
		InputTokens:           aws.Int32(0),
		OutputTokens:          aws.Int32(12),
		TotalTokens:           aws.Int32(12),
		CacheReadInputTokens:  aws.Int32(2000),
		CacheWriteInputTokens: aws.Int32(3000),
	})

	if usage == nil {
		t.Fatal("expected usage")
	}
	if usage.InputTokens != 5000 {
		t.Fatalf("expected cached input footprint 5000, got %d", usage.InputTokens)
	}
	if usage.TotalTokens != 5012 {
		t.Fatalf("expected total tokens 5012, got %d", usage.TotalTokens)
	}
	if usage.CachedTokens != 5000 {
		t.Fatalf("expected cached tokens 5000, got %d", usage.CachedTokens)
	}
}

func TestBedrockClient_StreamChat_LogsPromptCacheHits(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previousLogger)

	c := &BedrockClient{model: "global.anthropic.claude-sonnet-4-6"}
	c.streamImpl = func(ctx context.Context, params *bedrockruntime.ConverseStreamInput) (bedrockStream, error) {
		return &mockBedrockStream{events: []brtypes.ConverseStreamOutput{
			makeBedrockTextDelta(0, "ok"),
			makeBedrockContentBlockStop(0),
			makeBedrockMetadata(8, 12, 900, 100),
		}}, nil
	}

	eventCh, err := c.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for event := range eventCh {
		if event.Type == StreamEventTypeError {
			t.Fatalf("unexpected error: %v", event.Error)
		}
	}

	got := logs.String()
	if !strings.Contains(got, "Bedrock prompt cache hit") {
		t.Fatalf("expected prompt cache hit log, got:\n%s", got)
	}
	if !strings.Contains(got, "cache_read_input_tokens=900") {
		t.Fatalf("expected cache read token count in log, got:\n%s", got)
	}
}

func TestBedrockClient_StreamChat_ToolLoop(t *testing.T) {
	registry := tools.NewRegistry()
	if err := registry.Register(&successTool{}); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	var captured []*bedrockruntime.ConverseStreamInput
	callCount := 0
	c := &BedrockClient{model: "global.anthropic.claude-sonnet-4-6"}
	c.streamImpl = func(ctx context.Context, params *bedrockruntime.ConverseStreamInput) (bedrockStream, error) {
		captured = append(captured, params)
		callCount++
		if callCount == 1 {
			return &mockBedrockStream{events: []brtypes.ConverseStreamOutput{
				makeBedrockReasoningDelta(0, "think before tool"),
				makeBedrockReasoningSignatureDelta(0, "sig_01"),
				makeBedrockContentBlockStop(0),
				makeBedrockToolUseStart(1, "toolu_01", "success_tool"),
				makeBedrockToolUseDelta(1, `{"message":"hi"}`),
				makeBedrockContentBlockStop(1),
			}}, nil
		}
		return &mockBedrockStream{events: []brtypes.ConverseStreamOutput{
			makeBedrockTextDelta(0, "done"),
			makeBedrockContentBlockStop(0),
		}}, nil
	}

	eventCh, err := c.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var toolStart bool
	var toolEnd bool
	var finalText string
	var done bool
	for event := range eventCh {
		switch event.Type {
		case StreamEventTypeToolStart:
			toolStart = true
			if event.ToolCall.Name != "success_tool" {
				t.Fatalf("expected success_tool start, got %q", event.ToolCall.Name)
			}
		case StreamEventTypeToolEnd:
			toolEnd = true
			if event.ToolCall.Error != "" {
				t.Fatalf("unexpected tool error: %s", event.ToolCall.Error)
			}
		case StreamEventTypeChunk:
			finalText += event.Content
		case StreamEventTypeDone:
			done = true
		case StreamEventTypeError:
			t.Fatalf("unexpected error: %v", event.Error)
		}
	}

	if !toolStart || !toolEnd {
		t.Fatalf("expected tool start/end events, start=%v end=%v", toolStart, toolEnd)
	}
	if finalText != "done" {
		t.Fatalf("expected final text, got %q", finalText)
	}
	if !done {
		t.Fatal("expected done event")
	}
	if len(captured) != 2 {
		t.Fatalf("expected two Bedrock requests, got %d", len(captured))
	}
	second := captured[1]
	if len(second.Messages) != 3 {
		t.Fatalf("expected original user, assistant tool use, and tool result messages, got %d", len(second.Messages))
	}
	if len(second.Messages[0].Content) != 2 {
		t.Fatalf("expected stable user text and cachePoint, got %d blocks", len(second.Messages[0].Content))
	}
	if _, ok := second.Messages[0].Content[1].(*brtypes.ContentBlockMemberCachePoint); !ok {
		t.Fatalf("expected stable user cachePoint block, got %T", second.Messages[0].Content[1])
	}
	if second.Messages[1].Role != brtypes.ConversationRoleAssistant {
		t.Fatalf("expected assistant tool use replay, got role %q", second.Messages[1].Role)
	}
	reasoningBlock, ok := second.Messages[1].Content[0].(*brtypes.ContentBlockMemberReasoningContent)
	if !ok {
		t.Fatalf("expected reasoning replay content, got %T", second.Messages[1].Content[0])
	}
	reasoningText, ok := reasoningBlock.Value.(*brtypes.ReasoningContentBlockMemberReasoningText)
	if !ok {
		t.Fatalf("expected reasoning text block, got %T", reasoningBlock.Value)
	}
	if aws.ToString(reasoningText.Value.Text) != "think before tool" {
		t.Fatalf("expected reasoning text preserved, got %q", aws.ToString(reasoningText.Value.Text))
	}
	if aws.ToString(reasoningText.Value.Signature) != "sig_01" {
		t.Fatalf("expected reasoning signature preserved, got %q", aws.ToString(reasoningText.Value.Signature))
	}
	if _, ok := second.Messages[1].Content[1].(*brtypes.ContentBlockMemberToolUse); !ok {
		t.Fatalf("expected tool use content, got %T", second.Messages[1].Content[1])
	}
	if second.Messages[2].Role != brtypes.ConversationRoleUser {
		t.Fatalf("expected user tool result message, got role %q", second.Messages[2].Role)
	}
	if _, ok := second.Messages[2].Content[0].(*brtypes.ContentBlockMemberToolResult); !ok {
		t.Fatalf("expected tool result content, got %T", second.Messages[2].Content[0])
	}
	if len(second.Messages[2].Content) != 2 {
		t.Fatalf("expected tool result and cachePoint, got %d blocks", len(second.Messages[2].Content))
	}
	if _, ok := second.Messages[2].Content[1].(*brtypes.ContentBlockMemberCachePoint); !ok {
		t.Fatalf("expected transient tool result cachePoint block, got %T", second.Messages[2].Content[1])
	}
}

func TestBedrockClient_StreamError(t *testing.T) {
	c := &BedrockClient{model: "global.anthropic.claude-sonnet-4-6", maxRetries: 1}
	c.streamImpl = func(ctx context.Context, params *bedrockruntime.ConverseStreamInput) (bedrockStream, error) {
		return &mockBedrockStream{err: errors.New("boom")}, nil
	}

	eventCh, err := c.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var gotErr error
	for event := range eventCh {
		if event.Type == StreamEventTypeError {
			gotErr = event.Error
		}
	}
	if gotErr == nil || gotErr.Error() != "stream error: boom" {
		t.Fatalf("expected stream error, got %v", gotErr)
	}
}

func TestNewClient_Bedrock(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider: config.ProviderBedrock,
		Model:    "global.anthropic.claude-sonnet-4-6",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bedrockClient, ok := client.(*BedrockClient)
	if !ok {
		t.Fatalf("expected *BedrockClient, got %T", client)
	}
	if bedrockClient.model != "global.anthropic.claude-sonnet-4-6" {
		t.Fatalf("expected model to be set, got %q", bedrockClient.model)
	}
	if bedrockClient.contextWindowTokenCount != 1000000 {
		t.Fatalf("expected Bedrock context window 1000000, got %d", bedrockClient.contextWindowTokenCount)
	}
}

func TestBedrockThinkingFields(t *testing.T) {
	fields := bedrockThinkingFields("xhigh")
	if fields == nil {
		t.Fatal("expected Bedrock thinking fields")
	}
	body, err := fields.MarshalSmithyDocument()
	if err != nil {
		t.Fatalf("marshal Bedrock thinking fields: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal Bedrock thinking fields: %v", err)
	}
	thinking := got["thinking"].(map[string]any)
	if thinking["type"] != "adaptive" {
		t.Fatalf("expected adaptive thinking, got %#v", thinking)
	}
	outputConfig := got["output_config"].(map[string]any)
	if outputConfig["effort"] != "xhigh" {
		t.Fatalf("expected xhigh effort, got %#v", outputConfig)
	}
	if bedrockThinkingFields("") != nil {
		t.Fatal("expected empty effort to omit additional fields")
	}
}

type staticCredentials struct {
	Value aws.Credentials
}

func (s staticCredentials) Retrieve(ctx context.Context) (aws.Credentials, error) {
	return s.Value, nil
}

func TestBedrockClient_CustomHeaders(t *testing.T) {
	var captured http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"intentional failure"}`))
	}))
	defer srv.Close()

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(staticCredentials{Value: aws.Credentials{
			AccessKeyID:     "AKID",
			SecretAccessKey: "SECRET",
		}}),
	)
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}

	client := bedrockruntime.NewFromConfig(awsCfg, func(o *bedrockruntime.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
		o.APIOptions = append(o.APIOptions, bedrockHeaderMiddleware(map[string]string{
			"x-custom-header": "custom-value",
		}))
	})

	c := &BedrockClient{client: client, model: "global.anthropic.claude-sonnet-4-6", maxRetries: 1}
	c.streamImpl = func(ctx context.Context, params *bedrockruntime.ConverseStreamInput) (bedrockStream, error) {
		out, err := c.client.ConverseStream(ctx, params)
		if err != nil {
			return nil, err
		}
		return out.GetStream(), nil
	}

	eventCh, err := c.StreamChat(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range eventCh {
	}

	if captured.Get("x-custom-header") != "custom-value" {
		t.Fatalf("expected x-custom-header %q, got %q", "custom-value", captured.Get("x-custom-header"))
	}
}
