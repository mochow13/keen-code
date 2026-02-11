package config

import (
	"testing"
)

func TestGlobalConfig_GetProviderConfig(t *testing.T) {
	g := &GlobalConfig{
		Providers: map[string]ProviderConfig{
			ProviderAnthropic: {Models: []string{"claude-3-sonnet"}, APIKey: "sk-ant-test"},
			ProviderOpenAI:    {Models: []string{"gpt-4o"}, APIKey: "sk-test"},
			ProviderGemini:    {Models: []string{"gemini-1.5-pro"}, APIKey: "test-key"},
		},
	}

	pc, ok := g.GetProviderConfig(ProviderAnthropic)
	if !ok {
		t.Fatal("expected to find provider config")
	}
	if pc.APIKey != "sk-ant-test" {
		t.Errorf("expected api key 'sk-ant-test', got %q", pc.APIKey)
	}
	if len(pc.Models) != 1 || pc.Models[0] != "claude-3-sonnet" {
		t.Errorf("expected models ['claude-3-sonnet'], got %v", pc.Models)
	}
}

func TestGlobalConfig_GetProviderConfig_NotFound(t *testing.T) {
	g := &GlobalConfig{}

	_, ok := g.GetProviderConfig("unknown")
	if ok {
		t.Error("expected not to find provider config")
	}
}

func TestGlobalConfig_SetProviderConfig(t *testing.T) {
	g := &GlobalConfig{}
	cfg := ProviderConfig{Models: []string{"gpt-4o"}, APIKey: "sk-test"}

	g.SetProviderConfig(ProviderOpenAI, cfg)

	pc, ok := g.GetProviderConfig(ProviderOpenAI)
	if !ok {
		t.Fatal("expected to find provider config")
	}
	if len(pc.Models) != 1 || pc.Models[0] != "gpt-4o" {
		t.Errorf("expected models ['gpt-4o'], got %v", pc.Models)
	}
	if pc.APIKey != "sk-test" {
		t.Errorf("expected api key 'sk-test', got %q", pc.APIKey)
	}
}

func TestGlobalConfig_AddModel(t *testing.T) {
	g := &GlobalConfig{}

	g.AddModel(ProviderAnthropic, "claude-3-sonnet")

	pc, _ := g.GetProviderConfig(ProviderAnthropic)
	if len(pc.Models) != 1 || pc.Models[0] != "claude-3-sonnet" {
		t.Errorf("expected models ['claude-3-sonnet'], got %v", pc.Models)
	}

	g.AddModel(ProviderAnthropic, "claude-3-sonnet")
	pc, _ = g.GetProviderConfig(ProviderAnthropic)
	if len(pc.Models) != 1 {
		t.Errorf("expected 1 model, got %d", len(pc.Models))
	}

	g.AddModel(ProviderAnthropic, "claude-3-opus")
	pc, _ = g.GetProviderConfig(ProviderAnthropic)
	if len(pc.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(pc.Models))
	}
}

func TestGlobalConfig_GetFirstModel(t *testing.T) {
	g := &GlobalConfig{
		Providers: map[string]ProviderConfig{
			ProviderAnthropic: {Models: []string{"claude-3-sonnet", "claude-3-opus"}},
		},
	}

	first := g.GetFirstModel(ProviderAnthropic)
	if first != "claude-3-sonnet" {
		t.Errorf("expected first model 'claude-3-sonnet', got %q", first)
	}

	first = g.GetFirstModel(ProviderOpenAI)
	if first != "" {
		t.Errorf("expected empty string for no models, got %q", first)
	}
}

func TestProviderConfig_hasModel(t *testing.T) {
	pc := ProviderConfig{Models: []string{"claude-3-sonnet", "claude-3-opus"}}

	if !pc.hasModel("claude-3-sonnet") {
		t.Error("expected hasModel('claude-3-sonnet') to be true")
	}
	if !pc.hasModel("claude-3-opus") {
		t.Error("expected hasModel('claude-3-opus') to be true")
	}
	if pc.hasModel("gpt-4o") {
		t.Error("expected hasModel('gpt-4o') to be false")
	}
}

func TestResolve(t *testing.T) {
	global := &GlobalConfig{
		ActiveProvider: ProviderAnthropic,
		Providers: map[string]ProviderConfig{
			ProviderAnthropic: {Models: []string{"claude-3-sonnet"}, APIKey: "sk-ant-test"},
		},
	}
	session := &SessionConfig{}

	resolved, err := Resolve(global, session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Provider != ProviderAnthropic {
		t.Errorf("expected provider %q, got %q", ProviderAnthropic, resolved.Provider)
	}
	if resolved.APIKey != "sk-ant-test" {
		t.Errorf("expected api key 'sk-ant-test', got %q", resolved.APIKey)
	}
	if resolved.Model != "claude-3-sonnet" {
		t.Errorf("expected model 'claude-3-sonnet', got %q", resolved.Model)
	}
}

func TestResolve_WithSessionOverrides(t *testing.T) {
	global := &GlobalConfig{
		ActiveProvider: ProviderAnthropic,
		ActiveModel:    "claude-3-sonnet",
		Providers: map[string]ProviderConfig{
			ProviderAnthropic: {Models: []string{"claude-3-sonnet"}, APIKey: "sk-ant-test"},
		},
	}
	session := &SessionConfig{
		Provider: ProviderOpenAI,
		APIKey:   "sk-openai-test",
		Model:    "gpt-4o",
	}

	resolved, err := Resolve(global, session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Provider != ProviderOpenAI {
		t.Errorf("expected provider %q, got %q", ProviderOpenAI, resolved.Provider)
	}
	if resolved.APIKey != "sk-openai-test" {
		t.Errorf("expected api key 'sk-openai-test', got %q", resolved.APIKey)
	}
	if resolved.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", resolved.Model)
	}
}

func TestResolve_MissingProvider(t *testing.T) {
	global := &GlobalConfig{}
	session := &SessionConfig{}

	_, err := Resolve(global, session)
	if err == nil {
		t.Fatal("expected error for missing provider, got nil")
	}
}

func TestResolve_MissingAPIKey(t *testing.T) {
	global := &GlobalConfig{
		ActiveProvider: ProviderAnthropic,
	}
	session := &SessionConfig{}

	_, err := Resolve(global, session)
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}
}

func TestDefaultGlobalConfig(t *testing.T) {
	cfg := DefaultGlobalConfig()

	if cfg == nil {
		t.Fatal("expected non-nil config, got nil")
	}
	if cfg.ActiveProvider != "" {
		t.Errorf("expected empty ActiveProvider, got %q", cfg.ActiveProvider)
	}
	if cfg.ActiveModel != "" {
		t.Errorf("expected empty ActiveModel, got %q", cfg.ActiveModel)
	}
	if cfg.Providers == nil {
		t.Error("expected non-nil Providers map")
	}
}

func TestConfigPath(t *testing.T) {
	path := ConfigPath()

	if path == "" {
		t.Error("expected non-empty path, got empty string")
	}
}

func TestConfigDir(t *testing.T) {
	dir := ConfigDir()

	if dir == "" {
		t.Error("expected non-empty directory, got empty string")
	}
}
