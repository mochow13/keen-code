package llm

import (
	"fmt"
	"strings"

	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/providers"
)

type Provider string

const (
	deepSeekBaseURL   = "https://api.deepseek.com/"
	moonshotAIBaseURL = "https://api.moonshot.ai/v1/"
	zaiBaseURL        = "https://api.z.ai/api/paas/v4/"
	miniMaxBaseURL    = "https://api.minimax.io/anthropic"
	openCodeGoBaseURL = "https://opencode.ai/zen/go"
)

type ClientConfig struct {
	Provider            Provider
	APIKey              string
	APIKeyHelper        string
	Model               string
	ThinkingEffort      string
	BaseURL             string
	MaxRetries          int
	ContextWindowTokens int
	Headers             map[string]string
}

func NewClient(cfg *config.ResolvedConfig) (LLMClient, error) {
	if config.RequiresAPIKey(cfg.Provider) && cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required. %s", config.ConfigFixHint)
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required. %s", config.ConfigFixHint)
	}
	thinkingEffort, err := providers.ResolveThinkingEffort(cfg.Provider, cfg.Model, cfg.ThinkingEffort)
	if err != nil {
		return nil, err
	}
	resolved := *cfg
	resolved.ThinkingEffort = thinkingEffort
	cfg = &resolved

	contextWindowTokenCount := contextWindowForProviderModel(Provider(cfg.Provider), cfg.Model)

	switch cfg.Provider {
	case config.ProviderAnthropic:
		return NewAnthropicClient(&ClientConfig{
			Provider:            Provider(cfg.Provider),
			APIKey:              cfg.APIKey,
			APIKeyHelper:        cfg.APIKeyHelper,
			Model:               cfg.Model,
			ThinkingEffort:      cfg.ThinkingEffort,
			BaseURL:             cfg.BaseURL,
			ContextWindowTokens: contextWindowTokenCount,
			Headers:             cfg.Headers,
		})
	case config.ProviderMiniMax:
		return NewAnthropicClient(&ClientConfig{
			Provider:            Provider(cfg.Provider),
			APIKey:              cfg.APIKey,
			Model:               cfg.Model,
			ThinkingEffort:      cfg.ThinkingEffort,
			BaseURL:             cfg.BaseURL,
			ContextWindowTokens: contextWindowTokenCount,
			Headers:             cfg.Headers,
		})
	case config.ProviderGoogleAI:
		return NewGenkitClient(&ClientConfig{
			Provider:            Provider(cfg.Provider),
			APIKey:              cfg.APIKey,
			Model:               cfg.Model,
			ThinkingEffort:      cfg.ThinkingEffort,
			BaseURL:             cfg.BaseURL,
			ContextWindowTokens: contextWindowTokenCount,
			Headers:             cfg.Headers,
		})
	case config.ProviderOpenAI:
		return NewOpenAIResponsesClient(&ClientConfig{
			Provider:            Provider(cfg.Provider),
			APIKey:              cfg.APIKey,
			Model:               cfg.Model,
			ThinkingEffort:      cfg.ThinkingEffort,
			BaseURL:             cfg.BaseURL,
			ContextWindowTokens: contextWindowTokenCount,
			Headers:             cfg.Headers,
		})
	case config.ProviderOpenAICodex:
		return NewOpenAICodexClient(&ClientConfig{
			Provider:            Provider(cfg.Provider),
			Model:               cfg.Model,
			ThinkingEffort:      cfg.ThinkingEffort,
			ContextWindowTokens: contextWindowTokenCount,
			Headers:             cfg.Headers,
		})
	case config.ProviderBedrock:
		return NewBedrockClient(&ClientConfig{
			Provider:            Provider(cfg.Provider),
			APIKey:              cfg.APIKey,
			Model:               cfg.Model,
			BaseURL:             cfg.BaseURL,
			ThinkingEffort:      cfg.ThinkingEffort,
			ContextWindowTokens: contextWindowTokenCount,
			Headers:             cfg.Headers,
		})
	case config.ProviderDeepSeek,
		config.ProviderMoonshotAI,
		config.ProviderZAI,
		config.ProviderOpenAICompatible:
		return NewOpenAICompatibleClient(&ClientConfig{
			Provider:            Provider(cfg.Provider),
			APIKey:              cfg.APIKey,
			Model:               cfg.Model,
			ThinkingEffort:      cfg.ThinkingEffort,
			BaseURL:             cfg.BaseURL,
			ContextWindowTokens: contextWindowTokenCount,
			Headers:             cfg.Headers,
		})
	case config.ProviderOpenCodeGo:
		if cfg.Model == "gpt-5.6-luna" {
			baseURL := cfg.BaseURL
			if baseURL == "" {
				baseURL = openCodeGoBaseURL + "/v1/"
			}
			return NewOpenAIResponsesClient(&ClientConfig{
				Provider:            Provider(cfg.Provider),
				APIKey:              cfg.APIKey,
				Model:               cfg.Model,
				ThinkingEffort:      cfg.ThinkingEffort,
				BaseURL:             baseURL,
				ContextWindowTokens: contextWindowTokenCount,
				Headers:             cfg.Headers,
			})
		}
		if isOpenCodeGoAnthropicModel(cfg.Model) {
			return NewAnthropicClient(&ClientConfig{
				Provider:            Provider(cfg.Provider),
				APIKey:              cfg.APIKey,
				Model:               cfg.Model,
				BaseURL:             cfg.BaseURL,
				ThinkingEffort:      cfg.ThinkingEffort,
				ContextWindowTokens: contextWindowTokenCount,
				Headers:             cfg.Headers,
			})
		}
		return NewOpenAICompatibleClient(&ClientConfig{
			Provider:            Provider(cfg.Provider),
			APIKey:              cfg.APIKey,
			Model:               cfg.Model,
			ThinkingEffort:      cfg.ThinkingEffort,
			BaseURL:             cfg.BaseURL,
			ContextWindowTokens: contextWindowTokenCount,
			Headers:             cfg.Headers,
		})
	default:
		return nil, fmt.Errorf("unsupported provider: %s. %s", cfg.Provider, config.ConfigFixHint)
	}
}

func contextWindowForProviderModel(provider Provider, model string) int {
	registry, err := providers.Load()
	if err != nil {
		return defaultContextWindowTokenCount
	}
	contextWindowTokenCount, ok := registry.GetModelContextWindow(string(provider), model)
	if !ok {
		return defaultContextWindowTokenCount
	}
	return contextWindowTokenCount
}

func isOpenCodeGoMiniMaxModel(model string) bool {
	return strings.HasPrefix(model, "minimax-")
}

func isOpenCodeGoAnthropicModel(model string) bool {
	return isOpenCodeGoMiniMaxModel(model) || isOpenCodeGoQwenModel(model)
}

func isOpenCodeGoDeepSeekModel(model string) bool {
	return strings.HasPrefix(model, "deepseek-")
}

func isOpenCodeGoGLMModel(model string) bool {
	return strings.HasPrefix(model, "glm-")
}

func isOpenCodeGoKimiModel(model string) bool {
	return strings.HasPrefix(model, "kimi-")
}

func isOpenCodeGoQwenModel(model string) bool {
	return strings.HasPrefix(model, "qwen")
}

func isOpenCodeGoReasoningEffortModel(model string) bool {
	return model == "grok-4.5" ||
		model == "glm-5.2" ||
		model == "kimi-k3" ||
		model == "hy3"
}
