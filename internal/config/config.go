package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

var configLog = slog.New(slog.NewTextHandler(os.Stderr, nil))

const (
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
	ProviderGemini    = "gemini"
)

type GlobalConfig struct {
	ActiveProvider string `yaml:"provider" mapstructure:"provider"`

	Anthropic ProviderConfig `yaml:"anthropic"`
	OpenAI    ProviderConfig `yaml:"openai"`
	Gemini    ProviderConfig `yaml:"gemini"`
}

type ProviderConfig struct {
	Model  string `yaml:"model"`
	APIKey string `yaml:"api_key"`
}

type SessionConfig struct {
	Provider string
	APIKey   string
	Model    string
}

type ResolvedConfig struct {
	Provider string
	APIKey   string
	Model    string
}

func (g *GlobalConfig) GetProviderConfig(provider string) (ProviderConfig, error) {
	switch provider {
	case ProviderAnthropic:
		return g.Anthropic, nil
	case ProviderOpenAI:
		return g.OpenAI, nil
	case ProviderGemini:
		return g.Gemini, nil
	default:
		return ProviderConfig{}, fmt.Errorf("unknown provider: %s", provider)
	}
}

func (g *GlobalConfig) SetProviderConfig(provider string, cfg ProviderConfig) error {
	switch provider {
	case ProviderAnthropic:
		g.Anthropic = cfg
	case ProviderOpenAI:
		g.OpenAI = cfg
	case ProviderGemini:
		g.Gemini = cfg
	default:
		return fmt.Errorf("unknown provider: %s", provider)
	}
	return nil
}

func Resolve(global *GlobalConfig, session *SessionConfig) (*ResolvedConfig, error) {
	provider := session.Provider
	if provider == "" {
		provider = global.ActiveProvider
	}
	if provider == "" {
		return nil, fmt.Errorf("no provider configured")
	}

	providerGlobal, err := global.GetProviderConfig(provider)
	if err != nil {
		return nil, err
	}
	apiKey := firstNonEmpty(session.APIKey, providerGlobal.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for %s", provider)
	}
	resolved := &ResolvedConfig{
		Provider: provider,
		APIKey:   apiKey,
		Model:    firstNonEmpty(session.Model, providerGlobal.Model, defaultModel(provider)),
	}

	configLog.Info("config resolved", "provider", resolved.Provider, "model", resolved.Model)
	return resolved, nil
}

func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{}
}

func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "keen", "config.yaml")
}

func ConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "keen")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func defaultModel(provider string) string {
	switch provider {
	case ProviderAnthropic:
		return "claude-3-sonnet"
	case ProviderOpenAI:
		return "gpt-4o"
	case ProviderGemini:
		return "gemini-1.5-pro"
	default:
		return ""
	}
}
