package cli

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/user/keen-cli/configs/providers"
	"github.com/user/keen-cli/internal/config"
)

var errorStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#EF4444")).
	Bold(true)

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

	var modelID string
	err = huh.NewSelect[string]().
		Title("Select a model:").
		Options(registry.ModelOptions(providerID)...).
		Value(&modelID).
		Run()
	if err != nil {
		return nil, fmt.Errorf("model selection failed: %w", err)
	}

	existingKey := ""
	if providerCfg, exists := global.GetProviderConfig(providerID); exists {
		existingKey = providerCfg.APIKey
	}

	apiKey, err := promptAPIKey(provider.Name, existingKey)
	if err != nil {
		return nil, err
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

func promptAPIKey(providerName, existingKey string) (string, error) {
	for {
		var apiKey string

		title := fmt.Sprintf("Enter API key for %s", providerName)
		if existingKey != "" {
			title = fmt.Sprintf("Enter API key for %s (press Enter to keep existing)", providerName)
		}

		err := huh.NewInput().
			Title(title).
			EchoMode(huh.EchoModePassword).
			Value(&apiKey).
			Run()
		if err != nil {
			return "", fmt.Errorf("api key input failed: %w", err)
		}

		if apiKey != "" {
			return apiKey, nil
		}

		if existingKey != "" {
			return existingKey, nil
		}

		fmt.Println(errorStyle.Render("  ✗ API key is required. Please provide a valid API key."))
	}
}
