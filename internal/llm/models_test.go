package llm

import (
	"testing"

	"github.com/mochow13/keen-code/internal/config"
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

	if responsesClient.provider != Provider(config.ProviderOpenAI) {
		t.Errorf("expected provider openai, got %s", responsesClient.provider)
	}

	if responsesClient.model != "gpt-5.4-mini" {
		t.Errorf("expected model gpt-5.4-mini, got %s", responsesClient.model)
	}
	if responsesClient.contextWindowTokenCount != defaultContextWindowTokenCount {
		t.Errorf("expected fallback context window %d, got %d", defaultContextWindowTokenCount, responsesClient.contextWindowTokenCount)
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

	if genkitClient.provider != Provider(config.ProviderGoogleAI) {
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
	if oaiClient.provider != Provider(config.ProviderOpenCodeGo) {
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
		{name: "qwen max", model: "qwen3.7-max", thinkingEffort: "enabled"},
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
	client, err := NewClient(&config.ResolvedConfig{
		Provider:       config.ProviderOpenCodeGo,
		Model:          "gpt-5.6-luna",
		APIKey:         "test-api-key",
		ThinkingEffort: "max",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	responsesClient, ok := client.(*OpenAIResponsesClient)
	if !ok {
		t.Fatalf("expected *OpenAIResponsesClient, got %T", client)
	}
	if responsesClient.provider != Provider(config.ProviderOpenCodeGo) {
		t.Fatalf("expected provider opencode-go, got %s", responsesClient.provider)
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
	if anthropicClient.provider != Provider(config.ProviderMiniMax) {
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

	if oaiClient.provider != Provider(config.ProviderZAI) {
		t.Errorf("expected provider zai, got %s", oaiClient.provider)
	}
	if oaiClient.model != "glm-4-plus" {
		t.Errorf("expected model glm-4-plus, got %s", oaiClient.model)
	}
	if oaiClient.contextWindowTokenCount != defaultContextWindowTokenCount {
		t.Errorf("expected fallback context window %d, got %d", defaultContextWindowTokenCount, oaiClient.contextWindowTokenCount)
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

	if oaiClient.provider != Provider(config.ProviderDeepSeek) {
		t.Errorf("expected provider deepseek, got %s", oaiClient.provider)
	}
	if oaiClient.model != "deepseek-chat" {
		t.Errorf("expected model deepseek-chat, got %s", oaiClient.model)
	}
	if oaiClient.contextWindowTokenCount != defaultContextWindowTokenCount {
		t.Errorf("expected fallback context window %d, got %d", defaultContextWindowTokenCount, oaiClient.contextWindowTokenCount)
	}
}
