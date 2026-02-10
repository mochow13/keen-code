package config

import (
	"testing"
)

func TestGlobalConfig_GetProviderConfig(t *testing.T) {
	g := &GlobalConfig{
		Anthropic: ProviderConfig{Model: "claude-3-sonnet", APIKey: "sk-ant-test"},
		OpenAI:    ProviderConfig{Model: "gpt-4o", APIKey: "sk-test"},
		Gemini:    ProviderConfig{Model: "gemini-1.5-pro", APIKey: "test-key"},
	}

	pc, err := g.GetProviderConfig(ProviderAnthropic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pc.Model != "claude-3-sonnet" {
		t.Errorf("expected model 'claude-3-sonnet', got %q", pc.Model)
	}
	if pc.APIKey != "sk-ant-test" {
		t.Errorf("expected api key 'sk-ant-test', got %q", pc.APIKey)
	}
}

func TestGlobalConfig_SetProviderConfig(t *testing.T) {
	g := &GlobalConfig{}
	cfg := ProviderConfig{Model: "gpt-4o", APIKey: "sk-test"}

	err := g.SetProviderConfig(ProviderOpenAI, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.OpenAI.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", g.OpenAI.Model)
	}
	if g.OpenAI.APIKey != "sk-test" {
		t.Errorf("expected api key 'sk-test', got %q", g.OpenAI.APIKey)
	}
}

func TestResolve(t *testing.T) {
	global := &GlobalConfig{
		ActiveProvider: ProviderAnthropic,
		Anthropic:      ProviderConfig{Model: "claude-3-sonnet", APIKey: "sk-ant-test"},
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

func TestDefaultGlobalConfig(t *testing.T) {
	cfg := DefaultGlobalConfig()

	if cfg == nil {
		t.Fatal("expected non-nil config, got nil")
	}
	if cfg.ActiveProvider != "" {
		t.Errorf("expected empty ActiveProvider, got %q", cfg.ActiveProvider)
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
