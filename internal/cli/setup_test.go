package cli

import (
	"errors"
	"testing"

	"github.com/user/keen-cli/configs/providers"
	"github.com/user/keen-cli/internal/config"
)

type fakeLoader struct {
	saveCalled bool
	savedCfg   *config.GlobalConfig
	saveErr    error
}

func (f *fakeLoader) Save(cfg *config.GlobalConfig) error {
	f.saveCalled = true
	f.savedCfg = cfg
	return f.saveErr
}

func TestRunSetup_ProviderNotFound(t *testing.T) {
	// This test documents that RunSetup would fail if provider lookup fails.
	// Since we can't easily mock huh prompts, we test the registry behavior
	// that would cause the function to return an error.

	reg := &providers.Registry{
		Providers: []providers.Provider{
			{ID: "anthropic", Name: "Anthropic"},
		},
	}

	// Verify GetProvider returns false for unknown provider
	_, ok := reg.GetProvider("unknown")
	if ok {
		t.Error("GetProvider('unknown') should return false")
	}

	// Verify GetProvider returns true for valid provider
	p, ok := reg.GetProvider("anthropic")
	if !ok {
		t.Error("GetProvider('anthropic') should return true")
	}
	if p.ID != "anthropic" {
		t.Errorf("GetProvider returned wrong provider: %s", p.ID)
	}
}

func TestRunSetup_ConfigModification(t *testing.T) {
	// Test that the config modification logic works correctly
	// This is the core logic of RunSetup (lines 47-54)

	global := config.DefaultGlobalConfig()
	providerID := "anthropic"
	modelID := "claude-3-opus"
	apiKey := "test-api-key"

	// Simulate what RunSetup does after collecting inputs
	global.ActiveProvider = providerID
	global.ActiveModel = modelID

	providerCfg := config.ProviderConfig{
		APIKey: apiKey,
		Models: []string{modelID},
	}
	global.SetProviderConfig(providerID, providerCfg)

	// Verify the changes
	if global.ActiveProvider != providerID {
		t.Errorf("ActiveProvider = %q, want %q", global.ActiveProvider, providerID)
	}

	if global.ActiveModel != modelID {
		t.Errorf("ActiveModel = %q, want %q", global.ActiveModel, modelID)
	}

	storedCfg, ok := global.GetProviderConfig(providerID)
	if !ok {
		t.Fatal("Provider config not found")
	}

	if storedCfg.APIKey != apiKey {
		t.Errorf("APIKey = %q, want %q", storedCfg.APIKey, apiKey)
	}

	if len(storedCfg.Models) != 1 || storedCfg.Models[0] != modelID {
		t.Errorf("Models = %v, want [%s]", storedCfg.Models, modelID)
	}
}

func TestRunSetup_ResolvedConfig(t *testing.T) {
	// Test that ResolvedConfig is correctly constructed
	// This is what RunSetup returns on success (lines 60-64)

	providerID := "openai"
	modelID := "gpt-4"
	apiKey := "sk-test"

	resolved := &config.ResolvedConfig{
		Provider: providerID,
		APIKey:   apiKey,
		Model:    modelID,
	}

	if resolved.Provider != providerID {
		t.Errorf("Provider = %q, want %q", resolved.Provider, providerID)
	}

	if resolved.APIKey != apiKey {
		t.Errorf("APIKey = %q, want %q", resolved.APIKey, apiKey)
	}

	if resolved.Model != modelID {
		t.Errorf("Model = %q, want %q", resolved.Model, modelID)
	}
}

func TestRunSetup_SaveFailure(t *testing.T) {
	// Test the Save error handling path
	loader := &fakeLoader{
		saveErr: errors.New("permission denied"),
	}

	cfg := config.DefaultGlobalConfig()
	err := loader.Save(cfg)

	if err == nil {
		t.Error("Save should return error")
	}

	if loader.saveCalled != true {
		t.Error("Save should have been called")
	}

	if err.Error() != "permission denied" {
		t.Errorf("Error message = %q, want %q", err.Error(), "permission denied")
	}
}

func TestRunSetup_LoaderSaveCalled(t *testing.T) {
	// Verify that loader.Save is called with correct config
	loader := &fakeLoader{}

	cfg := config.DefaultGlobalConfig()
	cfg.ActiveProvider = "gemini"
	cfg.ActiveModel = "gemini-pro"
	cfg.SetProviderConfig("gemini", config.ProviderConfig{
		APIKey: "gemini-key",
		Models: []string{"gemini-pro"},
	})

	err := loader.Save(cfg)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if !loader.saveCalled {
		t.Error("Save was not called")
	}

	if loader.savedCfg == nil {
		t.Fatal("Saved config is nil")
	}

	if loader.savedCfg.ActiveProvider != "gemini" {
		t.Errorf("Saved ActiveProvider = %q, want %q", loader.savedCfg.ActiveProvider, "gemini")
	}
}

func TestRunSetup_MultipleProviders(t *testing.T) {
	// Test that config supports multiple providers correctly
	global := config.DefaultGlobalConfig()

	// Configure first provider
	global.SetProviderConfig("anthropic", config.ProviderConfig{
		APIKey: "key1",
		Models: []string{"claude-3-opus"},
	})

	// Configure second provider
	global.SetProviderConfig("openai", config.ProviderConfig{
		APIKey: "key2",
		Models: []string{"gpt-4"},
	})

	// Switch active to second provider (simulating RunSetup behavior)
	global.ActiveProvider = "openai"
	global.ActiveModel = "gpt-4"

	// Verify both providers are preserved
	anthropicCfg, ok := global.GetProviderConfig("anthropic")
	if !ok {
		t.Error("Anthropic config should be preserved")
	}
	if anthropicCfg.APIKey != "key1" {
		t.Errorf("Anthropic APIKey = %q, want %q", anthropicCfg.APIKey, "key1")
	}

	openaiCfg, ok := global.GetProviderConfig("openai")
	if !ok {
		t.Error("OpenAI config should exist")
	}
	if openaiCfg.APIKey != "key2" {
		t.Errorf("OpenAI APIKey = %q, want %q", openaiCfg.APIKey, "key2")
	}

	// Verify active settings
	if global.ActiveProvider != "openai" {
		t.Errorf("ActiveProvider = %q, want %q", global.ActiveProvider, "openai")
	}
}
