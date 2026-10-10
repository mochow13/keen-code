package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoader_Load_NoConfigFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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
	t.Setenv("HOME", t.TempDir())

	os.MkdirAll(ConfigDir(), 0755)
	configPath := ConfigPath()
	content := `{
	"active_provider": "anthropic",
		"providers": {
			"anthropic": {
			"models": ["claude-3-sonnet"],
			"api_key": "sk-test",
			"api_key_helper": "printf helper-key"
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
	if pc.APIKeyHelper != "printf helper-key" {
		t.Errorf("expected apiKeyHelper to load, got %q", pc.APIKeyHelper)
	}
}

func TestLoader_Load_InvalidJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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
	t.Setenv("HOME", t.TempDir())

	loader := NewLoader()
	cfg := &GlobalConfig{
		ActiveProvider: ProviderOpenAI,
		ActiveModel:    "gpt-4o",
		Providers: map[string]ProviderConfig{
			ProviderOpenAI: {
				Models:       []string{"gpt-4o"},
				APIKey:       "sk-test",
				APIKeyHelper: "example-auth login > /dev/null 2>&1 && example-auth token",
				Headers:      map[string]string{"x-custom": "value"},
			},
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
	if pc.Headers["x-custom"] != "value" {
		t.Errorf("expected header x-custom=value, got %v", pc.Headers)
	}
	if pc.APIKeyHelper != "example-auth login > /dev/null 2>&1 && example-auth token" {
		t.Errorf("expected apiKeyHelper to round-trip, got %q", pc.APIKeyHelper)
	}
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}
	if !strings.Contains(string(data), `"api_key_helper": "example-auth login > /dev/null 2>&1 && example-auth token"`) {
		t.Fatalf("expected unescaped api_key_helper, got %s", string(data))
	}
}

func TestLoader_Exists_False(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	loader := NewLoader()
	if loader.Exists() {
		t.Error("expected Exists() to return false, got true")
	}
}

func TestLoader_Exists_True(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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

func TestLoader_SaveAndLoadDecisionConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	loader := NewLoader()
	cfg := DefaultGlobalConfig()
	cfg.Decision = &DecisionConfig{
		ActiveProvider: ProviderTypeSafe,
		ActiveModel:    "jev-1.13.0",
		Providers: map[string]ProviderConfig{
			ProviderTypeSafe: {APIKey: "test-key"},
		},
	}
	if err := loader.Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Decision == nil || loaded.Decision.ActiveProvider != ProviderTypeSafe || loaded.Decision.ActiveModel != "jev-1.13.0" {
		t.Fatalf("decision config = %#v", loaded.Decision)
	}
	providerCfg := loaded.Decision.Providers[ProviderTypeSafe]
	if providerCfg.APIKey != "test-key" {
		t.Fatalf("provider config = %#v", providerCfg)
	}
}

func TestLoader_CustomPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaultLoader := NewLoader()
	defaultCfg := DefaultGlobalConfig()
	defaultCfg.ActiveModel = "default-model"
	if err := defaultLoader.Save(defaultCfg); err != nil {
		t.Fatal(err)
	}
	defaultData, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	t.Chdir(dir)
	for _, path := range []string{"special configs.json", filepath.Join(dir, "nested", "configs.json")} {
		t.Run(path, func(t *testing.T) {
			loader := NewLoaderWithPath(path)
			if loader.Exists() {
				t.Fatal("custom config should not exist yet")
			}
			if _, err := loader.Load(); err == nil {
				t.Fatal("missing custom config should fail instead of using defaults")
			}
			cfg := DefaultGlobalConfig()
			cfg.ActiveModel = "custom-model"
			if err := loader.Save(cfg); err != nil {
				t.Fatal(err)
			}
			if !loader.Exists() {
				t.Fatal("saved custom config should exist")
			}
			loaded, err := loader.Load()
			if err != nil {
				t.Fatal(err)
			}
			if loaded.ActiveModel != "custom-model" {
				t.Fatalf("model = %q, want custom-model", loaded.ActiveModel)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0600 {
				t.Fatalf("config permissions = %o, want 600", info.Mode().Perm())
			}
		})
	}
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(defaultData) {
		t.Fatal("custom config changed the default config")
	}
}

func TestLoader_CustomPathErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, content := range []string{"", "{", "{} {}", "[]", `{"providers": "invalid"}`} {
		t.Run(content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid.json")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := NewLoaderWithPath(path).Load()
			if err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("Load() error = %v, want error with custom path", err)
			}
			if strings.Contains(err.Error(), "~/.keen/configs.json") {
				t.Fatal("error should not direct users to the default config")
			}
		})
	}
	path := t.TempDir()
	if _, err := NewLoaderWithPath(path).Load(); err == nil {
		t.Fatal("loading a directory should fail")
	}
}


func TestLoader_SaveRestrictsExistingFilePermissions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "existing-config.json")
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := NewLoaderWithPath(path).Save(DefaultGlobalConfig()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("config permissions = %o, want 600", got)
	}
}
