package llm

import (
	"fmt"

	"github.com/user/keen-cli/internal/config"
)

type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderGemini    Provider = "gemini"
)

type ClientConfig struct {
	Provider Provider
	APIKey   string
	Model    string
}

func NewClient(cfg *config.ResolvedConfig) (LLMClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	switch Provider(cfg.Provider) {
	case ProviderAnthropic, ProviderOpenAI, ProviderGemini:
		return NewGenkitClient(&ClientConfig{
			Provider: Provider(cfg.Provider),
			APIKey:   cfg.APIKey,
			Model:    cfg.Model,
		})
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}
