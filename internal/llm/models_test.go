package llm

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/mochow13/keen-code/internal/config"
	"github.com/mochow13/keen-code/internal/llm/core"
	"github.com/mochow13/keen-code/internal/llm/providerconfig"
	"github.com/mochow13/keen-code/internal/tools"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

func TestNewClient_MissingAPIKey(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider: "anthropic",
		Model:    "claude-3-haiku",
		APIKey:   "",
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("expected error for missing API key")
	}

	if err.Error() != "API key is required. "+config.ConfigFixHint {
		t.Errorf("expected 'API key is required', got %q", err.Error())
	}
}

func TestNewClient_MissingModel(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider: "anthropic",
		Model:    "",
		APIKey:   "test-api-key",
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("expected error for missing model")
	}

	if err.Error() != "model is required. "+config.ConfigFixHint {
		t.Errorf("expected 'model is required', got %q", err.Error())
	}
}

func TestNewClient_UnsupportedProvider(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider: "unknown-provider",
		Model:    "some-model",
		APIKey:   "test-api-key",
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("expected error for unsupported provider")
	}

	expectedMsg := "unsupported provider: unknown-provider. " + config.ConfigFixHint
	if err.Error() != expectedMsg {
		t.Errorf("expected %q, got %q", expectedMsg, err.Error())
	}
}

func TestNewClient_Anthropic(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider: "anthropic",
		Model:    "claude-haiku-4-5",
		APIKey:   "test-api-key",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client == nil {
		t.Error("expected non-nil client")
	}

	anthropicClient, ok := client.(*AnthropicClient)
	if !ok {
		t.Fatalf("expected *AnthropicClient, got %T", client)
	}

	if anthropicClient.model != "claude-haiku-4-5" {
		t.Errorf("expected model claude-haiku-4-5, got %s", anthropicClient.model)
	}
	if anthropicClient.contextWindowTokenCount != 200000 {
		t.Errorf("expected context window 200000, got %d", anthropicClient.contextWindowTokenCount)
	}
}

func TestNewClient_OpenAI(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider: "openai",
		Model:    "gpt-5.4-mini",
		APIKey:   "test-api-key",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client == nil {
		t.Error("expected non-nil client")
	}

	responsesClient, ok := client.(*OpenAIResponsesClient)
	if !ok {
		t.Fatalf("expected *OpenAIResponsesClient, got %T", client)
	}

	if responsesClient.provider != providerconfig.Provider(config.ProviderOpenAI) {
		t.Errorf("expected provider openai, got %s", responsesClient.provider)
	}

	if responsesClient.model != "gpt-5.4-mini" {
		t.Errorf("expected model gpt-5.4-mini, got %s", responsesClient.model)
	}
	if responsesClient.contextWindowTokenCount != core.DefaultContextWindowTokenCount {
		t.Errorf("expected fallback context window %d, got %d", core.DefaultContextWindowTokenCount, responsesClient.contextWindowTokenCount)
	}
}

func TestNewClient_Gemini(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider: "googleai",
		Model:    "gemini-3.6-flash",
		APIKey:   "test-api-key",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client == nil {
		t.Error("expected non-nil client")
	}

	genkitClient, ok := client.(*GenkitClient)
	if !ok {
		t.Error("expected *GenkitClient type")
	}

	if genkitClient.provider != providerconfig.Provider(config.ProviderGoogleAI) {
		t.Errorf("expected provider googleai, got %s", genkitClient.provider)
	}

	if genkitClient.model != "googleai/gemini-3.6-flash" {
		t.Errorf("expected model googleai/gemini-3.6-flash, got %s", genkitClient.model)
	}
	if genkitClient.contextWindowTokenCount != 1048576 {
		t.Errorf("expected context window 1048576, got %d", genkitClient.contextWindowTokenCount)
	}
}

func TestNewClient_OpenCodeGoOpenAICompatibleModel(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider:       config.ProviderOpenCodeGo,
		Model:          "kimi-k2.6",
		APIKey:         "test-api-key",
		ThinkingEffort: "enabled",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	oaiClient, ok := client.(*OpenAICompatibleClient)
	if !ok {
		t.Fatalf("expected *OpenAICompatibleClient, got %T", client)
	}
	if oaiClient.provider != providerconfig.Provider(config.ProviderOpenCodeGo) {
		t.Fatalf("expected provider opencode-go, got %s", oaiClient.provider)
	}
	if oaiClient.model != "kimi-k2.6" {
		t.Fatalf("expected model kimi-k2.6, got %s", oaiClient.model)
	}
	if oaiClient.thinkingEffort != "enabled" {
		t.Fatalf("expected thinking effort enabled, got %q", oaiClient.thinkingEffort)
	}
	if oaiClient.contextWindowTokenCount != 262144 {
		t.Fatalf("expected context window 262144, got %d", oaiClient.contextWindowTokenCount)
	}
}

func TestNewClient_OpenCodeGoAnthropicModel(t *testing.T) {
	tests := []struct {
		name           string
		model          string
		thinkingEffort string
	}{
		{name: "minimax", model: "minimax-m2.7"},
		{name: "minimax m3", model: "minimax-m3", thinkingEffort: "adaptive"},
		{name: "qwen3.8 flash", model: "qwen3.8-flash", thinkingEffort: "enabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ResolvedConfig{
				Provider:       config.ProviderOpenCodeGo,
				Model:          tt.model,
				APIKey:         "test-api-key",
				ThinkingEffort: tt.thinkingEffort,
			}

			client, err := NewClient(cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			anthropicClient, ok := client.(*AnthropicClient)
			if !ok {
				t.Fatalf("expected *AnthropicClient, got %T", client)
			}
			if anthropicClient.model != tt.model {
				t.Fatalf("expected model %s, got %s", tt.model, anthropicClient.model)
			}
			if anthropicClient.thinkingEffort != tt.thinkingEffort {
				t.Fatalf("expected Anthropic thinking effort %q for OpenCode Go %s, got %q", tt.thinkingEffort, tt.model, anthropicClient.thinkingEffort)
			}
			if anthropicClient.contextWindowTokenCount <= 0 {
				t.Fatalf("expected context window to be populated")
			}
		})
	}
}

func TestNewClient_RejectsUnsupportedThinkingEffort(t *testing.T) {
	_, err := NewClient(&config.ResolvedConfig{
		Provider:       config.ProviderDeepSeek,
		Model:          "deepseek-v4-pro",
		APIKey:         "test-api-key",
		ThinkingEffort: "medium",
	})
	if err == nil {
		t.Fatal("expected unsupported thinking effort error")
	}
}

func TestNewClient_OpenCodeGoResponsesModel(t *testing.T) {
	tests := []struct {
		name           string
		model          string
		thinkingEffort string
	}{
		{name: "gpt-6-luna", model: "gpt-6-luna", thinkingEffort: "max"},
		{name: "gpt-5.6-luna", model: "gpt-5.6-luna", thinkingEffort: "max"},
		{name: "grok-4.7", model: "grok-4.7", thinkingEffort: "high"},
		{name: "grok-4.6", model: "grok-4.6", thinkingEffort: "high"},
		{name: "muse spark 1.2", model: "muse-spark-1.2-contributor", thinkingEffort: "high"},
		{name: "muse spark 1.3", model: "muse-spark-1.3-contributor", thinkingEffort: "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(&config.ResolvedConfig{
				Provider:       config.ProviderOpenCodeGo,
				Model:          tt.model,
				APIKey:         "test-api-key",
				ThinkingEffort: tt.thinkingEffort,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			responsesClient, ok := client.(*OpenAIResponsesClient)
			if !ok {
				t.Fatalf("expected *OpenAIResponsesClient, got %T", client)
			}
			if responsesClient.provider != providerconfig.Provider(config.ProviderOpenCodeGo) {
				t.Fatalf("expected provider opencode-go, got %s", responsesClient.provider)
			}
			if responsesClient.thinkingEffort != tt.thinkingEffort {
				t.Fatalf("expected thinking effort %q, got %q", tt.thinkingEffort, responsesClient.thinkingEffort)
			}
		})
	}
}

func TestNewClient_MiniMax(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider:       config.ProviderMiniMax,
		Model:          "MiniMax-M3",
		APIKey:         "test-api-key",
		ThinkingEffort: "adaptive",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	anthropicClient, ok := client.(*AnthropicClient)
	if !ok {
		t.Fatalf("expected *AnthropicClient, got %T", client)
	}
	if anthropicClient.provider != providerconfig.Provider(config.ProviderMiniMax) {
		t.Fatalf("expected provider minimax, got %s", anthropicClient.provider)
	}
	if anthropicClient.model != "MiniMax-M3" {
		t.Fatalf("expected model MiniMax-M3, got %s", anthropicClient.model)
	}
	if anthropicClient.thinkingEffort != "adaptive" {
		t.Fatalf("expected adaptive MiniMax thinking effort, got %q", anthropicClient.thinkingEffort)
	}
	if anthropicClient.contextWindowTokenCount != 1000000 {
		t.Fatalf("expected context window 1000000, got %d", anthropicClient.contextWindowTokenCount)
	}
}

func TestNewClient_OpenCodeGoMissingAPIKey(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider: config.ProviderOpenCodeGo,
		Model:    "glm-5.1",
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Fatal("expected missing API key error")
	}
	if err.Error() != "API key is required. "+config.ConfigFixHint {
		t.Fatalf("expected API key error, got %q", err.Error())
	}
}

func TestNewClient_ZAI(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider: "zai",
		Model:    "glm-4-plus",
		APIKey:   "test-api-key",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	oaiClient, ok := client.(*OpenAICompatibleClient)
	if !ok {
		t.Fatalf("expected *OpenAICompatibleClient, got %T", client)
	}

	if oaiClient.provider != providerconfig.Provider(config.ProviderZAI) {
		t.Errorf("expected provider zai, got %s", oaiClient.provider)
	}
	if oaiClient.model != "glm-4-plus" {
		t.Errorf("expected model glm-4-plus, got %s", oaiClient.model)
	}
	if oaiClient.contextWindowTokenCount != core.DefaultContextWindowTokenCount {
		t.Errorf("expected fallback context window %d, got %d", core.DefaultContextWindowTokenCount, oaiClient.contextWindowTokenCount)
	}
}

func TestNewClient_ZAIGLM53(t *testing.T) {
	client, err := NewClient(&config.ResolvedConfig{
		Provider:       config.ProviderZAI,
		Model:          "glm-5.3",
		APIKey:         "test-api-key",
		ThinkingEffort: "max",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	oaiClient, ok := client.(*OpenAICompatibleClient)
	if !ok {
		t.Fatalf("expected *OpenAICompatibleClient, got %T", client)
	}
	if oaiClient.contextWindowTokenCount != 1000000 {
		t.Fatalf("expected context window 1000000, got %d", oaiClient.contextWindowTokenCount)
	}
	if oaiClient.thinkingEffort != "max" {
		t.Fatalf("expected thinking effort max, got %q", oaiClient.thinkingEffort)
	}
}

func TestNewClient_DeepSeek(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider: "deepseek",
		Model:    "deepseek-chat",
		APIKey:   "test-api-key",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	oaiClient, ok := client.(*OpenAICompatibleClient)
	if !ok {
		t.Fatalf("expected *OpenAICompatibleClient, got %T", client)
	}

	if oaiClient.provider != providerconfig.Provider(config.ProviderDeepSeek) {
		t.Errorf("expected provider deepseek, got %s", oaiClient.provider)
	}
	if oaiClient.model != "deepseek-chat" {
		t.Errorf("expected model deepseek-chat, got %s", oaiClient.model)
	}
	if oaiClient.contextWindowTokenCount != core.DefaultContextWindowTokenCount {
		t.Errorf("expected fallback context window %d, got %d", core.DefaultContextWindowTokenCount, oaiClient.contextWindowTokenCount)
	}
}

func TestNewClient_YoloAuto(t *testing.T) {
	client, err := NewClient(&config.ResolvedConfig{
		Provider: config.ProviderYoloAuto,
		Model:    "yolo",
		APIKey:   "yolo_test_key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	oaiClient, ok := client.(*OpenAICompatibleClient)
	if !ok {
		t.Fatalf("expected *OpenAICompatibleClient, got %T", client)
	}
	if oaiClient.provider != providerconfig.Provider(config.ProviderYoloAuto) {
		t.Fatalf("expected provider yolo-auto, got %s", oaiClient.provider)
	}
	if oaiClient.model != "yolo" {
		t.Fatalf("expected model yolo, got %s", oaiClient.model)
	}
	if oaiClient.contextWindowTokenCount != 131072 {
		t.Fatalf("expected context window 131072, got %d", oaiClient.contextWindowTokenCount)
	}
}

func TestNewClient_RegisteredModelsRequestParameters(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	standardEfforts := []string{"", "low", "medium", "high", "xhigh", "max"}
	for _, tt := range []struct {
		provider string
		model    string
		protocol string
		context  int
		efforts  []string
	}{
		{config.ProviderAnthropic, "claude-sonnet-5-5", "anthropic", 1000000, standardEfforts},
		{config.ProviderBedrock, "global.anthropic.claude-sonnet-5-5", "bedrock", 1000000, standardEfforts},
		{config.ProviderBedrock, "global.anthropic.claude-opus-5-5", "bedrock", 1000000, standardEfforts},
		{config.ProviderBedrock, "global.anthropic.claude-fable-5-1", "bedrock", 1000000, standardEfforts},
		{config.ProviderBedrock, "global.anthropic.claude-opus-5", "bedrock", 1000000, standardEfforts},
		{config.ProviderOpenAI, "gpt-6.1-sol", "responses", 1050000, standardEfforts},
		{config.ProviderOpenAICodex, "gpt-6.1-sol", "codex", 272000, []string{"", "low", "medium", "high", "xhigh", "max", "ultra"}},
		{config.ProviderZAI, "glm-5.3-flashx", "chat", 1000000, []string{"", "low", "high", "max"}},
		{config.ProviderDeepSeek, "deepseek-flash", "chat", 1000000, []string{"", "low", "high", "max"}},
		{config.ProviderMiniMax, "MiniMax-M3.1-Flash-Preview", "anthropic", 1000000, standardEfforts},
	} {
		t.Run(tt.provider+"/"+tt.model, func(t *testing.T) {
			for _, effort := range tt.efforts {
				name := effort
				if name == "" {
					name = "default"
				}
				t.Run(name, func(t *testing.T) {
					client, err := NewClient(&config.ResolvedConfig{
						Provider: tt.provider, Model: tt.model, APIKey: "test-key", ThinkingEffort: effort,
					})
					if err != nil {
						t.Fatalf("NewClient: %v", err)
					}
					var body []byte
					var marshalErr error
					var contextWindow int
					captureResponses := func(ctx context.Context, params responses.ResponseNewParams, opts ...option.RequestOption) responseStream {
						body, marshalErr = json.Marshal(params)
						return &fakeResponseStream{events: []responses.ResponseStreamEventUnion{
							mustResponseEvent(t, `{"type":"response.completed","response":{"id":"r1","object":"response","output":[]}}`),
						}}
					}
					switch c := client.(type) {
					case *AnthropicClient:
						if tt.protocol != "anthropic" {
							t.Fatalf("unexpected Anthropic client for %s", tt.protocol)
						}
						contextWindow = c.contextWindowTokenCount
						c.streamImpl = func(ctx context.Context, params anthropic.MessageNewParams, opts ...anthropicoption.RequestOption) anthropicStream {
							body, marshalErr = json.Marshal(params)
							return &mockAnthropicStream{}
						}
					case *BedrockClient:
						if tt.protocol != "bedrock" {
							t.Fatalf("unexpected Bedrock client for %s", tt.protocol)
						}
						contextWindow = c.contextWindowTokenCount
						c.streamImpl = func(ctx context.Context, params *bedrockruntime.ConverseStreamInput) (bedrockStream, error) {
							request := map[string]any{"model": aws.ToString(params.ModelId)}
							if params.AdditionalModelRequestFields != nil {
								body, marshalErr = params.AdditionalModelRequestFields.MarshalSmithyDocument()
								if marshalErr == nil {
									marshalErr = json.Unmarshal(body, &request)
								}
							}
							if marshalErr == nil {
								body, marshalErr = json.Marshal(request)
							}
							if params.InferenceConfig.Temperature != nil || params.InferenceConfig.TopP != nil {
								t.Error("Bedrock request must omit sampling parameters")
							}
							if params.ToolConfig == nil || params.ToolConfig.ToolChoice != nil {
								t.Error("expected tools without forced tool choice")
							}
							return &mockBedrockStream{}, nil
						}
					case *OpenAIResponsesClient:
						if tt.protocol != "responses" {
							t.Fatalf("unexpected Responses client for %s", tt.protocol)
						}
						contextWindow = c.contextWindowTokenCount
						c.responseStreamImpl = captureResponses
					case *OpenAICodexClient:
						if tt.protocol != "codex" {
							t.Fatalf("unexpected Codex client for %s", tt.protocol)
						}
						contextWindow = c.contextWindowTokenCount
						c.authManager = newTestCodexClient(t).authManager
						c.responseStreamImpl = captureResponses
					case *OpenAICompatibleClient:
						if tt.protocol != "chat" {
							t.Fatalf("unexpected Chat Completions client for %s", tt.protocol)
						}
						contextWindow = c.contextWindowTokenCount
						c.streamImpl = func(ctx context.Context, params openai.ChatCompletionNewParams, opts ...option.RequestOption) chatStream {
							body, marshalErr = json.Marshal(params)
							return &fakeChatStream{}
						}
					default:
						t.Fatalf("unexpected client type %T", client)
					}
					if contextWindow != tt.context {
						t.Errorf("context window = %d, want %d", contextWindow, tt.context)
					}
					registry := tools.NewRegistry()
					if err := registry.Register(&successTool{}); err != nil {
						t.Fatal(err)
					}
					ch, err := client.StreamChat(context.Background(), []core.Message{{Role: core.RoleUser, Content: "hi"}}, registry)
					if err != nil {
						t.Fatal(err)
					}
					for event := range ch {
						if event.Type == core.StreamEventTypeError {
							t.Errorf("stream error: %v", event.Error)
						}
					}
					if marshalErr != nil {
						t.Fatalf("marshal request: %v", marshalErr)
					}
					var request map[string]any
					if err := json.Unmarshal(body, &request); err != nil {
						t.Fatalf("decode request: %v", err)
					}
					if request["model"] != tt.model {
						t.Errorf("model = %v, want %s", request["model"], tt.model)
					}
					for _, field := range []string{"temperature", "top_p", "tool_choice"} {
						if _, exists := request[field]; exists {
							t.Errorf("unexpected %s", field)
						}
					}
					want := map[string]any{}
					if effort != "" {
						switch tt.protocol {
						case "anthropic", "bedrock":
							want["thinking"] = map[string]any{"type": "adaptive"}
							want["output_config"] = map[string]any{"effort": effort}
						case "responses", "codex":
							want["reasoning"] = map[string]any{"effort": effort}
						case "chat":
							want["thinking"] = map[string]any{"type": "enabled"}
							want["reasoning_effort"] = effort
						}
					}
					for _, field := range []string{"thinking", "output_config", "reasoning", "reasoning_effort"} {
						if !reflect.DeepEqual(request[field], want[field]) {
							t.Errorf("%s = %#v, want %#v", field, request[field], want[field])
						}
					}
				})
			}
			for _, effort := range []string{"none", "enabled", "disabled", "minimal", "medium", "xhigh", "ultra", "unsupported"} {
				if slices.Contains(tt.efforts, effort) {
					continue
				}
				if _, err := NewClient(&config.ResolvedConfig{
					Provider: tt.provider, Model: tt.model, APIKey: "test-key", ThinkingEffort: effort,
				}); err == nil {
					t.Errorf("expected validation error for %q", effort)
				}
			}
		})
	}
}
