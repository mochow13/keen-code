package config

import (
	"os"
	"testing"
)

func TestNewLoader(t *testing.T) {
	loader := NewLoader()
	if loader == nil {
		t.Fatal("expected non-nil loader, got nil")
	}
}

func TestLoader_Load_NoConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Unsetenv("HOME")

	loader := NewLoader()
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config, got nil")
	}
	if cfg.ActiveProvider != "" {
		t.Errorf("expected empty ActiveProvider, got %q", cfg.ActiveProvider)
	}
}

func TestLoader_Load_ExistingConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Unsetenv("HOME")

	os.MkdirAll(ConfigDir(), 0755)
	configPath := ConfigPath()
	content := `{
	"active_provider": "anthropic",
		"providers": {
			"anthropic": {
			"models": ["claude-3-sonnet"],
			"api_key": "sk-test"
			}
		}
	}`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	loader := NewLoader()
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ActiveProvider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", cfg.ActiveProvider)
	}

	pc, ok := cfg.GetProviderConfig("anthropic")
	if !ok {
		t.Fatal("expected to find anthropic provider config")
	}
	if len(pc.Models) != 1 || pc.Models[0] != "claude-3-sonnet" {
		t.Errorf("expected models ['claude-3-sonnet'], got %v", pc.Models)
	}
}

func TestLoader_Save(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Unsetenv("HOME")

	loader := NewLoader()
	cfg := &GlobalConfig{
		ActiveProvider: ProviderOpenAI,
		Providers: map[string]ProviderConfig{
			ProviderOpenAI: {Models: []string{"gpt-4o"}, APIKey: "sk-test"},
		},
	}

	err := loader.Save(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(ConfigPath()); os.IsNotExist(err) {
		t.Error("expected config file to exist, but it does not")
	}
}

func TestLoader_Load_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Unsetenv("HOME")

	os.MkdirAll(ConfigDir(), 0755)
	configPath := ConfigPath()
	if err := os.WriteFile(configPath, []byte("invalid json content"), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	loader := NewLoader()
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected error when loading invalid JSON, got nil")
	}
}

func TestLoader_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Unsetenv("HOME")

	loader := NewLoader()
	cfg := &GlobalConfig{
		ActiveProvider: ProviderOpenAI,
		ActiveModel:    "gpt-4o",
		Providers: map[string]ProviderConfig{
			ProviderOpenAI: {Models: []string{"gpt-4o"}, APIKey: "sk-test"},
		},
	}

	if err := loader.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := loader.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded.ActiveProvider != ProviderOpenAI {
		t.Errorf("expected provider %q, got %q", ProviderOpenAI, loaded.ActiveProvider)
	}
	if loaded.ActiveModel != "gpt-4o" {
		t.Errorf("expected active model 'gpt-4o', got %q", loaded.ActiveModel)
	}

	pc, ok := loaded.GetProviderConfig(ProviderOpenAI)
	if !ok {
		t.Fatal("expected to find openai provider config")
	}
	if len(pc.Models) != 1 || pc.Models[0] != "gpt-4o" {
		t.Errorf("expected models ['gpt-4o'], got %v", pc.Models)
	}
}

func TestLoader_Exists_False(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Unsetenv("HOME")

	loader := NewLoader()
	if loader.Exists() {
		t.Error("expected Exists() to return false, got true")
	}
}

func TestLoader_Exists_True(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	defer os.Unsetenv("HOME")

	os.MkdirAll(ConfigDir(), 0755)
	configPath := ConfigPath()
	if err := os.WriteFile(configPath, []byte("{}"), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	loader := NewLoader()
	if !loader.Exists() {
		t.Error("expected Exists() to return true, got false")
	}
}
