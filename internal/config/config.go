package config

import (
	"fmt"
	"os"
	"path/filepath"
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

/* SessionConfig holds CLI flag overrides for the current session.
 * These are not persisted and apply only to the current session.
 */
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
	case "anthropic":
		return g.Anthropic, nil
	case "openai":
		return g.OpenAI, nil
	case "gemini":
		return g.Gemini, nil
	default:
		return ProviderConfig{}, fmt.Errorf("unknown provider: %s", provider)
	}
}

func (g *GlobalConfig) SetProviderConfig(provider string, cfg ProviderConfig) error {
	switch provider {
	case "anthropic":
		g.Anthropic = cfg
	case "openai":
		g.OpenAI = cfg
	case "gemini":
		g.Gemini = cfg
	default:
		return fmt.Errorf("unknown provider: %s", provider)
	}
	return nil
}

/* Resolve merges global and session configs into the final ResolvedConfig.
 * Resolution order: Session > Global
 * Returns an error if no provider is configured.
 */
func Resolve(global *GlobalConfig, session *SessionConfig) (*ResolvedConfig, error) {
	provider := session.Provider
	if provider == "" {
		provider = global.ActiveProvider
	}
	if provider == "" {
		return nil, fmt.Errorf("no provider configured. Run /provider to set up a provider")
	}

	providerGlobal, err := global.GetProviderConfig(provider)
	if err != nil {
		return nil, err
	}
	apiKey := firstNonEmpty(session.APIKey, providerGlobal.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for %s. Run /provider to set up", provider)
	}
	resolved := &ResolvedConfig{
		Provider: provider,
		APIKey:   apiKey,
		Model:    firstNonEmpty(session.Model, providerGlobal.Model, defaultModel(provider)),
	}

	return resolved, nil
}

/* DefaultGlobalConfig returns an empty GlobalConfig.
 * Users must explicitly configure a provider using /provider command.
 */
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
	case "anthropic":
		return "claude-3-sonnet"
	case "openai":
		return "gpt-4o"
	case "gemini":
		return "gemini-1.5-pro"
	default:
		return ""
	}
}
