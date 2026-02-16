package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
)

const (
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
	ProviderGemini    = "gemini"
)

type GlobalConfig struct {
	ActiveProvider string                    `json:"active_provider"`
	ActiveModel    string                    `json:"active_model"`
	Providers      map[string]ProviderConfig `json:"providers"`
}

type ProviderConfig struct {
	Models []string `json:"models"`
	APIKey string   `json:"api_key"`
}

func (p ProviderConfig) hasModel(model string) bool {
	return slices.Contains(p.Models, model)
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

func (g *GlobalConfig) GetProviderConfig(provider string) (ProviderConfig, bool) {
	cfg, ok := g.Providers[provider]
	return cfg, ok
}

func (g *GlobalConfig) SetProviderConfig(provider string, cfg ProviderConfig) {
	if g.Providers == nil {
		g.Providers = make(map[string]ProviderConfig)
	}
	g.Providers[provider] = cfg
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

func (g *GlobalConfig) GetFirstModel(provider string) string {
	cfg, ok := g.GetProviderConfig(provider)
	if !ok {
		return ""
	}
	if len(cfg.Models) > 0 {
		return cfg.Models[0]
	}
	return ""
}

func Resolve(global *GlobalConfig, session *SessionConfig) (*ResolvedConfig, error) {
	provider := session.Provider
	if provider == "" {
		provider = global.ActiveProvider
	}
	if provider == "" {
		return nil, fmt.Errorf("no provider configured")
	}

	providerGlobal, ok := global.GetProviderConfig(provider)
	if !ok {
		providerGlobal = ProviderConfig{}
	}
	apiKey := firstNonEmpty(session.APIKey, providerGlobal.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for %s", provider)
	}

	model := firstNonEmpty(
		session.Model,
		global.ActiveModel,
		global.GetFirstModel(provider),
	)

	resolved := &ResolvedConfig{
		Provider: provider,
		APIKey:   apiKey,
		Model:    model,
	}

	slog.Debug("config resolved", "provider", resolved.Provider, "model", resolved.Model)
	return resolved, nil
}

func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		Providers: make(map[string]ProviderConfig),
	}
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
