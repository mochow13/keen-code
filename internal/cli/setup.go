package cli

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/user/keen-cli/configs/providers"
	"github.com/user/keen-cli/internal/config"
)

func RunSetup(loader *config.Loader, global *config.GlobalConfig, registry *providers.Registry) (*config.ResolvedConfig, error) {
	var providerID string
	err := huh.NewSelect[string]().
		Title("Select a provider:").
		Options(registry.ProviderOptions()...).
		Value(&providerID).
		Run()
	if err != nil {
		return nil, fmt.Errorf("provider selection failed: %w", err)
	}

	provider, ok := registry.GetProvider(providerID)
	if !ok {
		return nil, fmt.Errorf("selected provider %q not found in registry", providerID)
	}

	var apiKey string
	err = huh.NewInput().
		Title(fmt.Sprintf("Enter API key for %s", provider.Name)).
		EchoMode(huh.EchoModePassword).
		Value(&apiKey).
		Run()
	if err != nil {
		return nil, fmt.Errorf("api key input failed: %w", err)
	}

	var modelID string
	err = huh.NewSelect[string]().
		Title("Select a model:").
		Options(registry.ModelOptions(providerID)...).
		Value(&modelID).
		Run()
	if err != nil {
		return nil, fmt.Errorf("model selection failed: %w", err)
	}

	global.ActiveProvider = providerID
	global.ActiveModel = modelID

	providerCfg := config.ProviderConfig{
		APIKey: apiKey,
		Models: []string{modelID},
	}
	global.SetProviderConfig(providerID, providerCfg)

	if err := loader.Save(global); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	return &config.ResolvedConfig{
		Provider: providerID,
		APIKey:   apiKey,
		Model:    modelID,
	}, nil
}
