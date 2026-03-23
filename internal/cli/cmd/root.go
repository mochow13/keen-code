package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/user/keen-code/internal/cli/repl"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/providers"
)

func NewRootCommand(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keen",
		Short: "Keen - A coding agent CLI",
		Long:  `Keen is a terminal-based coding agent that provides AI-assisted code editing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := providers.Load()
			if err != nil {
				return fmt.Errorf("failed to load provider registry: %w", err)
			}
			loader := config.NewLoader()
			globalCfg, err := loader.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			var resolvedCfg *config.ResolvedConfig
			needsSetup := false

			if globalCfg.ActiveProvider == "" {
				needsSetup = true
				resolvedCfg = &config.ResolvedConfig{}
			} else {
				_, ok := registry.GetProvider(globalCfg.ActiveProvider)
				if !ok {
					return fmt.Errorf("configured provider %q not found in registry", globalCfg.ActiveProvider)
				}
				providerCfg, ok := globalCfg.GetProviderConfig(globalCfg.ActiveProvider)
				if !ok {
					return fmt.Errorf("failed to get provider config for %q", globalCfg.ActiveProvider)
				}
				resolvedCfg = &config.ResolvedConfig{
					Provider: globalCfg.ActiveProvider,
					Model:    globalCfg.ActiveModel,
					APIKey:   providerCfg.APIKey,
				}
			}

			wd, err := os.Getwd()
			if err != nil {
				wd = "."
			}

			return repl.RunREPL(version, wd, resolvedCfg, loader, globalCfg, registry, needsSetup)
		},
	}

	cmd.Version = version
	return cmd
}
