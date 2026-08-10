package config

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/mochow13/keen-code/internal/providers"
)

const (
	ProviderAnthropic        = "anthropic"
	ProviderOpenAI           = "openai"
	ProviderOpenAICodex      = "openai-codex"
	ProviderGoogleAI         = "googleai"
	ProviderMoonshotAI       = "moonshotai"
	ProviderDeepSeek         = "deepseek"
	ProviderZAI              = "zai"
	ProviderMiniMax          = "minimax"
	ProviderOpenCodeGo       = "opencode-go"
	ProviderBedrock          = "amazon-bedrock"
	ProviderOpenAICompatible = "openai-compatible"
)

const ConfigFixHint = "To fix configs manually, check ~/.keen/configs.json"

type GlobalConfig struct {
	ActiveProvider    string                    `json:"active_provider"`
	ActiveModel       string                    `json:"active_model"`
	ThinkingEffort    string                    `json:"thinking_effort,omitempty"`
	ShowThinking      *bool                     `json:"show_thinking,omitempty"`
	AdversaryProvider string                    `json:"adversary_provider,omitempty"`
	AdversaryModel    string                    `json:"adversary_model,omitempty"`
	Providers         map[string]ProviderConfig `json:"providers"`
}

type ProviderConfig struct {
	Models       []string          `json:"models"`
	APIKey       string            `json:"api_key"`
	APIKeyHelper string            `json:"api_key_helper,omitempty"`
	BaseURL      string            `json:"base_url,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
}

type ResolvedConfig struct {
	Provider       string
	APIKey         string
	APIKeyHelper   string
	Model          string
	ThinkingEffort string
	BaseURL        string
	AuthMode       string
	Headers        map[string]string
}

const (
	AuthModeAPIKey = "api_key"
	AuthModeOAuth  = "oauth"
	AuthModeAWS    = "aws"
)

func (g *GlobalConfig) GetProviderConfig(provider string) (ProviderConfig, bool) {
	cfg, ok := g.Providers[provider]
	return cfg, ok
}

func (g *GlobalConfig) SetProviderConfig(provider string, cfg ProviderConfig) {
	if g.Providers == nil {
		g.Providers = make(map[string]ProviderConfig)
	}
	mergedModels := make([]string, 0)
	mergedModels = append(mergedModels, cfg.Models...)
	mergedModels = append(mergedModels, g.Providers[provider].Models...)
	sort.Strings(mergedModels)
	mergedModels = slices.Compact(mergedModels)

	g.Providers[provider] = ProviderConfig{
		Models:       mergedModels,
		APIKey:       cfg.APIKey,
		APIKeyHelper: cfg.APIKeyHelper,
		BaseURL:      cfg.BaseURL,
		Headers:      cfg.Headers,
	}
}

func (g *GlobalConfig) AddModel(provider string, model string) {
	if model == "" {
		return
	}
	cfg, ok := g.GetProviderConfig(provider)
	if !ok {
		cfg = ProviderConfig{}
	}
	if slices.Contains(cfg.Models, model) {
		return
	}
	cfg.Models = append(cfg.Models, model)
	g.SetProviderConfig(provider, cfg)
}

func RequiresAPIKey(provider string) bool {
	return AuthModeForProvider(provider) == AuthModeAPIKey
}

func SupportsAPIKey(provider string) bool {
	return RequiresAPIKey(provider) || provider == ProviderBedrock
}

func AuthModeForProvider(provider string) string {
	if provider == ProviderBedrock {
		return AuthModeAWS
	}
	if provider == ProviderOpenAICodex {
		return AuthModeOAuth
	}
	return AuthModeAPIKey
}

func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		Providers: make(map[string]ProviderConfig),
	}
}

func ResolveProviderAPIKey(provider string, providerCfg ProviderConfig) (string, error) {
	if strings.TrimSpace(providerCfg.APIKeyHelper) != "" {
		apiKey, err := runAPIKeyHelper(provider, providerCfg.APIKeyHelper)
		if err != nil {
			return "", err
		}
		return apiKey, nil
	}

	apiKey := normalizeAPIKey(providerCfg.APIKey)
	if RequiresAPIKey(provider) && apiKey == "" {
		return "", fmt.Errorf("no API key configured for %s. %s", provider, ConfigFixHint)
	}
	return apiKey, nil
}

func runAPIKeyHelper(provider string, helper string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", helper)
	return runAPIKeyHelperCommand(ctx, provider, cmd)
}

func runAPIKeyHelperCommand(ctx context.Context, provider string, cmd *exec.Cmd) (string, error) {
	output, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("apiKeyHelper timed out for %s", provider)
	}
	if err != nil {
		return "", fmt.Errorf("apiKeyHelper failed for %s: %w", provider, err)
	}

	apiKey := normalizeAPIKey(string(output))
	if apiKey == "" {
		return "", fmt.Errorf("apiKeyHelper returned empty API key for %s", provider)
	}
	return apiKey, nil
}

func ResolveProvider(global *GlobalConfig, provider, model, thinkingEffort string) (*ResolvedConfig, error) {
	if global == nil {
		return nil, fmt.Errorf("global config not initialized")
	}
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "" || model == "" {
		return nil, fmt.Errorf("provider and model are required")
	}
	provCfg, ok := global.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("provider %q is not configured", provider)
	}
	apiKey, err := ResolveProviderAPIKey(provider, provCfg)
	if err != nil {
		return nil, err
	}
	thinkingEffort, err = providers.ResolveThinkingEffort(provider, model, thinkingEffort)
	if err != nil {
		return nil, err
	}
	return &ResolvedConfig{
		Provider:       provider,
		Model:          model,
		ThinkingEffort: thinkingEffort,
		APIKey:         apiKey,
		APIKeyHelper:   provCfg.APIKeyHelper,
		BaseURL:        provCfg.BaseURL,
		AuthMode:       AuthModeForProvider(provider),
		Headers:        cloneHeaders(provCfg.Headers),
	}, nil
}

func ResolveAdversary(global *GlobalConfig) (*ResolvedConfig, error) {
	if global.AdversaryProvider == "" || global.AdversaryModel == "" {
		return nil, fmt.Errorf("adversary model not configured")
	}
	return ResolveProvider(global, global.AdversaryProvider, global.AdversaryModel, "")
}

func cloneHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".keen", "configs.json")
}

func ConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".keen")
}

func normalizeAPIKey(key string) string {
	return strings.TrimSpace(key)
}
