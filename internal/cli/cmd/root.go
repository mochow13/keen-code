package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	keenauth "github.com/user/keen-code/internal/auth"
	"github.com/user/keen-code/internal/cli/repl"
	"github.com/user/keen-code/internal/config"
	"github.com/user/keen-code/internal/llm"
	keenmcp "github.com/user/keen-code/internal/mcp"
	"github.com/user/keen-code/internal/providers"
	"github.com/user/keen-code/internal/session"
)

var newMCPManager = func(opts ...keenmcp.Option) (keenmcp.Runtime, error) {
	return keenmcp.NewManager(opts...)
}

func NewRootCommand(version string) *cobra.Command {
	var resumeSessionID string

	cmd := &cobra.Command{
		Use:   "keen",
		Short: "Keen - A coding agent CLI",
		Long:  `Keen is a terminal-based coding agent that provides AI-assisted code editing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, loader, globalCfg, resolvedCfg, needsSetup, err := loadRootRuntime()
			if err != nil {
				return err
			}
			wd, err := os.Getwd()
			if err != nil {
				wd = "."
			}

			var resumeSession *session.LoadedSession
			if resumeSessionID != "" {
				resumeSession, err = loadResumeSession(wd, resumeSessionID)
				if err != nil {
					return err
				}
			}

			mcpManager, closeMCP, mcpErr := startMCPRuntime(context.Background())
			defer closeMCP()
			if mcpErr != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "MCP unavailable: %v\n", mcpErr)
			}

			sessionID, err := repl.RunREPL(version, wd, resolvedCfg, loader, globalCfg, registry, needsSetup, mcpManager, resumeSession)
			if err != nil {
				return err
			}
			if sessionID != "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nRun `keen --resume %s` to resume the session\n", sessionID)
			}
			return nil
		},
	}

	cmd.Version = version
	cmd.Flags().StringVar(&resumeSessionID, "resume", "", "resume a specific Keen session by ID")
	cmd.AddCommand(newRunCommand())
	return cmd
}

func startMCPRuntime(ctx context.Context) (keenmcp.Runtime, func(), error) {
	manager, err := newMCPManager()
	if err != nil {
		return nil, func() {}, err
	}
	if err := manager.Start(ctx); err != nil {
		if closeErr := manager.Close(); closeErr != nil {
			slog.Warn("MCP shutdown failed after startup error", "error", closeErr)
		}
		return nil, func() {}, err
	}
	slog.Debug("MCP manager started")
	return manager, func() {
		if err := manager.Close(); err != nil {
			slog.Warn("MCP shutdown failed", "error", err)
		}
	}, nil
}

func newRunCommand() *cobra.Command {
	var sessionID string
	var format string
	var providerID string
	var modelID string

	runCmd := &cobra.Command{
		Use:   "run [flags] <message...>",
		Short: "Run one non-interactive Keen turn",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _, globalCfg, resolvedCfg, _, err := loadRootRuntime()
			if err != nil {
				return err
			}
			if err := applyRunOverrides(globalCfg, resolvedCfg, providerID, modelID); err != nil {
				return err
			}
			if resolvedCfg.Provider == "" {
				return fmt.Errorf("LLM client not initialized. Run keen to configure a provider")
			}
			if resolvedCfg.AuthMode == config.AuthModeOAuth && !keenauth.NewOAuthManager(nil).HasCredential(resolvedCfg.Provider) {
				return fmt.Errorf("LLM client not initialized. Run keen to configure a provider")
			}

			stdin := ""
			if shouldReadStdin(os.Stdin) {
				data, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}
				stdin = string(data)
			}
			prompt := buildRunPrompt(args, stdin)
			if prompt == "" {
				return fmt.Errorf("prompt is required")
			}

			wd, err := os.Getwd()
			if err != nil {
				wd = "."
			}
			client, err := llm.NewClient(resolvedCfg)
			if err != nil {
				return err
			}
			mcpRuntime, closeMCP, mcpErr := startMCPRuntime(context.Background())
			defer closeMCP()
			if mcpErr != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "MCP unavailable: %v\n", mcpErr)
			}

			_, err = repl.RunHeadless(context.Background(), repl.HeadlessRunOptions{
				WorkingDir:   wd,
				Config:       resolvedCfg,
				GlobalConfig: globalCfg,
				MCPRuntime:   mcpRuntime,
				Client:       client,
				SessionID:    sessionID,
				Prompt:       prompt,
				Format:       format,
				Out:          cmd.OutOrStdout(),
				Progress:     cmd.ErrOrStderr(),
			})
			return err
		},
	}
	runCmd.Flags().StringVar(&sessionID, "session", "", "resume an existing Keen session")
	runCmd.Flags().StringVar(&format, "format", repl.HeadlessFormatText, "output format: text or json")
	runCmd.Flags().StringVar(&providerID, "provider", "", "provider to use for this run")
	runCmd.Flags().StringVar(&modelID, "model", "", "model to use for this run")
	return runCmd
}

func loadRootRuntime() (*providers.Registry, *config.Loader, *config.GlobalConfig, *config.ResolvedConfig, bool, error) {
	registry, err := providers.Load()
	if err != nil {
		return nil, nil, nil, nil, false, fmt.Errorf("failed to load provider registry: %w", err)
	}
	loader := config.NewLoader()
	globalCfg, err := loader.Load()
	if err != nil {
		return nil, nil, nil, nil, false, fmt.Errorf("failed to load config: %w", err)
	}

	if globalCfg.ActiveProvider == "" {
		return registry, loader, globalCfg, &config.ResolvedConfig{}, true, nil
	}

	_, ok := registry.GetProvider(globalCfg.ActiveProvider)
	if !ok {
		return nil, nil, nil, nil, false, fmt.Errorf("configured provider %q not found in registry", globalCfg.ActiveProvider)
	}
	providerCfg, ok := globalCfg.GetProviderConfig(globalCfg.ActiveProvider)
	if !ok {
		return nil, nil, nil, nil, false, fmt.Errorf("failed to get provider config for %q", globalCfg.ActiveProvider)
	}
	apiKey, err := config.ResolveProviderAPIKey(globalCfg.ActiveProvider, providerCfg)
	if err != nil {
		return nil, nil, nil, nil, false, err
	}
	activeModel := globalCfg.ActiveModel
	if activeModel == "" && len(providerCfg.Models) > 0 {
		activeModel = providerCfg.Models[0]
	}
	resolvedCfg := &config.ResolvedConfig{
		Provider:       globalCfg.ActiveProvider,
		Model:          activeModel,
		APIKey:         apiKey,
		APIKeyHelper:   providerCfg.APIKeyHelper,
		ThinkingEffort: globalCfg.ThinkingEffort,
		BaseURL:        providerCfg.BaseURL,
		AuthMode:       config.AuthModeForProvider(globalCfg.ActiveProvider),
		Headers:        providerCfg.Headers,
	}
	needsSetup := resolvedCfg.AuthMode == config.AuthModeOAuth && !keenauth.NewOAuthManager(nil).HasCredential(globalCfg.ActiveProvider)
	return registry, loader, globalCfg, resolvedCfg, needsSetup, nil
}

func applyRunOverrides(globalCfg *config.GlobalConfig, resolvedCfg *config.ResolvedConfig, providerID string, modelID string) error {
	if providerID != "" {
		providerCfg, ok := globalCfg.GetProviderConfig(providerID)
		if !ok {
			return fmt.Errorf("provider %q is not configured", providerID)
		}
		apiKey, err := config.ResolveProviderAPIKey(providerID, providerCfg)
		if err != nil {
			return err
		}
		resolvedCfg.Provider = providerID
		resolvedCfg.APIKey = apiKey
		resolvedCfg.BaseURL = providerCfg.BaseURL
		resolvedCfg.AuthMode = config.AuthModeForProvider(providerID)
		resolvedCfg.Headers = providerCfg.Headers
		if modelID == "" && len(providerCfg.Models) > 0 {
			resolvedCfg.Model = providerCfg.Models[0]
		}
	}
	if modelID != "" {
		resolvedCfg.Model = modelID
	}
	return nil
}

func buildRunPrompt(args []string, stdin string) string {
	argText := strings.TrimSpace(strings.Join(args, " "))
	stdin = strings.TrimSpace(stdin)
	switch {
	case argText != "" && stdin != "":
		return argText + "\n" + stdin
	case argText != "":
		return argText
	default:
		return stdin
	}
}

func shouldReadStdin(stdin *os.File) bool {
	info, err := stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}
