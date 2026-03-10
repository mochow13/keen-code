package llm

import (
	"testing"

	"github.com/user/keen-code/internal/config"
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

	if err.Error() != "API key is required" {
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

	if err.Error() != "model is required" {
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

	expectedMsg := "unsupported provider: unknown-provider"
	if err.Error() != expectedMsg {
		t.Errorf("expected %q, got %q", expectedMsg, err.Error())
	}
}

func TestNewClient_Anthropic(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider: "anthropic",
		Model:    "claude-3-haiku",
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

	if genkitClient.provider != Provider(config.ProviderAnthropic) {
		t.Errorf("expected provider anthropic, got %s", genkitClient.provider)
	}

	if genkitClient.model != "anthropic/claude-3-haiku" {
		t.Errorf("expected model anthropic/claude-3-haiku, got %s", genkitClient.model)
	}
}

func TestNewClient_OpenAI(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider: "openai",
		Model:    "gpt-4",
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

	if genkitClient.provider != Provider(config.ProviderOpenAI) {
		t.Errorf("expected provider openai, got %s", genkitClient.provider)
	}

	if genkitClient.model != "openai/gpt-4" {
		t.Errorf("expected model openai/gpt-4, got %s", genkitClient.model)
	}
}

func TestNewClient_Gemini(t *testing.T) {
	cfg := &config.ResolvedConfig{
		Provider: "googleai",
		Model:    "gemini-pro",
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

	if genkitClient.model != "googleai/gemini-pro" {
		t.Errorf("expected model googleai/gemini-pro, got %s", genkitClient.model)
	}
}

func TestProviderConstants(t *testing.T) {
	tests := []struct {
		provider Provider
		expected string
	}{
		{Provider(config.ProviderAnthropic), "anthropic"},
		{Provider(config.ProviderOpenAI), "openai"},
		{Provider(config.ProviderGoogleAI), "googleai"},
	}

	for _, tt := range tests {
		if string(tt.provider) != tt.expected {
			t.Errorf("expected provider %q, got %q", tt.expected, tt.provider)
		}
	}
}
