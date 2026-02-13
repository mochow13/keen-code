# Task 5: Basic CLI Structure - Implementation Plan

## Overview

Implement the simplified CLI experience with interactive setup flow using arrow key selection. Provider and model mappings are loaded from a dedicated config file inside the project.

## New CLI Behavior

```bash
keen                    # Start REPL (interactive setup on first run)
keen --version          # Show version
```

**No flags** - All configuration done via interactive prompts.

---

## Interactive Setup Flow (First Run)

When `keen` is run for the first time (no config exists), user is guided through:

### Step 1: Select Provider (arrow key selection)
```
Select a provider:
> Anthropic
  OpenAI
  Google Gemini
```

### Step 2: Enter API Key (password input, hidden)
```
Enter API key for Anthropic: ****
```

### Step 3: Select Model (arrow key selection, provider-specific)
```
Select a model for Anthropic:
> Claude 3 Opus
  Claude 3 Sonnet
  Claude 3 Haiku
```

### Step 4: Save Config
User config saved to `~/.keen/configs.json` with:
- `active_provider`: selected provider ID
- `active_model`: selected model ID
- Provider config with API key and models list

---

## Provider/Model Mapping Config

**Location:** `configs/providers/registry.yaml` (inside project)

```yaml
providers:
  - id: anthropic
    name: Anthropic
    models:
      - id: claude-3-opus
        name: Claude 3 Opus
      - id: claude-3-sonnet
        name: Claude 3 Sonnet
      - id: claude-3-haiku
        name: Claude 3 Haiku
        
  - id: openai
    name: OpenAI
    models:
      - id: gpt-4o
        name: GPT-4o
      - id: gpt-4-turbo
        name: GPT-4 Turbo
      - id: gpt-3.5-turbo
        name: GPT-3.5 Turbo
        
  - id: gemini
    name: Google Gemini
    models:
      - id: gemini-1.5-pro
        name: Gemini 1.5 Pro
      - id: gemini-1.5-flash
        name: Gemini 1.5 Flash
```

### Structs

```go
package providers

type Registry struct {
    Providers []Provider `yaml:"providers"`
}

type Provider struct {
    ID     string  `yaml:"id"`
    Name   string  `yaml:"name"`
    Models []Model `yaml:"models"`
}

type Model struct {
    ID   string `yaml:"id"`
    Name string `yaml:"name"`
}
```

### Loader

```go
package providers

import (
    "embed"
    "gopkg.in/yaml.v3"
)

//go:embed registry.yaml
var registryYAML embed.FS

func Load() (*Registry, error) {
    data, err := registryYAML.ReadFile("registry.yaml")
    if err != nil {
        return nil, err
    }
    
    var reg Registry
    if err := yaml.Unmarshal(data, &reg); err != nil {
        return nil, err
    }
    return &reg, nil
}
```

### Helper Methods

```go
func (r *Registry) GetProvider(id string) *Provider
func (r *Registry) GetModel(providerID, modelID string) *Model
func (r *Registry) GetProviderOptions() []huh.Option[string]  // For huh select
func (r *Registry) GetModelOptions(providerID string) []huh.Option[string]
```

---

## Implementation Steps

### Step 1: Add charmbracelet/huh dependency

```bash
go get github.com/charmbracelet/huh
```

### Step 2: Create `configs/providers/registry.yaml`

Create the directory and YAML file with default providers and models.

### Step 3: Create `configs/providers/loader.go`

```go
package providers

import (
    "embed"
    
    "github.com/charmbracelet/huh"
    "gopkg.in/yaml.v3"
)

//go:embed registry.yaml
var registryFS embed.FS

type Registry struct {
    Providers []Provider `yaml:"providers"`
}

type Provider struct {
    ID     string  `yaml:"id"`
    Name   string  `yaml:"name"`
    Models []Model `yaml:"models"`
}

type Model struct {
    ID   string `yaml:"id"`
    Name string `yaml:"name"`
}

func Load() (*Registry, error) {
    data, err := registryFS.ReadFile("registry.yaml")
    if err != nil {
        return nil, err
    }
    
    var reg Registry
    if err := yaml.Unmarshal(data, &reg); err != nil {
        return nil, err
    }
    return &reg, nil
}

func (r *Registry) GetProvider(id string) *Provider {
    for i := range r.Providers {
        if r.Providers[i].ID == id {
            return &r.Providers[i]
        }
    }
    return nil
}

func (r *Registry) GetModel(providerID, modelID string) *Model {
    p := r.GetProvider(providerID)
    if p == nil {
        return nil
    }
    for i := range p.Models {
        if p.Models[i].ID == modelID {
            return &p.Models[i]
        }
    }
    return nil
}

func (r *Registry) ProviderOptions() []huh.Option[string] {
    opts := make([]huh.Option[string], len(r.Providers))
    for i, p := range r.Providers {
        opts[i] = huh.NewOption(p.Name, p.ID)
    }
    return opts
}

func (r *Registry) ModelOptions(providerID string) []huh.Option[string] {
    p := r.GetProvider(providerID)
    if p == nil {
        return nil
    }
    opts := make([]huh.Option[string], len(p.Models))
    for i, m := range p.Models {
        opts[i] = huh.NewOption(m.Name, m.ID)
    }
    return opts
}
```

### Step 4: Create `internal/cli/setup.go`

Interactive setup using registry:

```go
package cli

import (
    "github.com/charmbracelet/huh"
    "github.com/user/keen-cli/configs/providers"
    "github.com/user/keen-cli/internal/config"
)

func RunSetup(loader *config.Loader, global *config.GlobalConfig, registry *providers.Registry) (*config.ResolvedConfig, error) {
    // 1. Select provider
    var providerID string
    err := huh.NewSelect[string]().
        Title("Select a provider:").
        Options(registry.ProviderOptions()...).
        Value(&providerID).
        Run()
    if err != nil {
        return nil, err
    }
    
    // 2. Enter API key
    var apiKey string
    err = huh.NewInput().
        Title("Enter API key for " + registry.GetProvider(providerID).Name).
        EchoMode(huh.EchoModePassword).
        Value(&apiKey).
        Run()
    if err != nil {
        return nil, err
    }
    
    // 3. Select model
    var modelID string
    err = huh.NewSelect[string]().
        Title("Select a model:").
        Options(registry.ModelOptions(providerID)...).
        Value(&modelID).
        Run()
    if err != nil {
        return nil, err
    }
    
    // 4. Save config
    global.ActiveProvider = providerID
    global.ActiveModel = modelID
    
    providerCfg := config.ProviderConfig{
        APIKey: apiKey,
        Models: []string{modelID},
    }
    if err := global.SetProviderConfig(providerID, providerCfg); err != nil {
        return nil, err
    }
    
    if err := loader.Save(global); err != nil {
        return nil, err
    }
    
    return &config.ResolvedConfig{
        Provider: providerID,
        APIKey:   apiKey,
        Model:    modelID,
    }, nil
}
```

### Step 5: Update `internal/cli/root.go`

```go
package cli

import (
    "fmt"
    "os"
    
    "github.com/spf13/cobra"
    "github.com/user/keen-cli/configs/providers"
    "github.com/user/keen-cli/internal/config"
)

func NewRootCommand(version string) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "keen",
        Short: "Keen - A coding agent CLI",
        Long:  `Keen is a terminal-based coding agent that provides AI-assisted code editing.`,
        RunE: func(cmd *cobra.Command, args []string) error {
            // Load provider registry (from embedded config)
            registry, err := providers.Load()
            if err != nil {
                return fmt.Errorf("failed to load provider registry: %w", err)
            }
            
            // Load user config
            loader := config.NewLoader()
            globalCfg, err := loader.Load()
            if err != nil {
                return fmt.Errorf("failed to load config: %w", err)
            }
            
            var resolvedCfg *config.ResolvedConfig
            
            // Check if setup is needed
            if globalCfg.ActiveProvider == "" {
                resolvedCfg, err = RunSetup(loader, globalCfg, registry)
                if err != nil {
                    return fmt.Errorf("setup failed: %w", err)
                }
            } else {
                // Verify provider exists in registry
                p := registry.GetProvider(globalCfg.ActiveProvider)
                if p == nil {
                    return fmt.Errorf("configured provider %q not found in registry", globalCfg.ActiveProvider)
                }
                
                providerCfg, _ := globalCfg.GetProviderConfig(globalCfg.ActiveProvider)
                resolvedCfg = &config.ResolvedConfig{
                    Provider: globalCfg.ActiveProvider,
                    Model:    globalCfg.ActiveModel,
                    APIKey:   providerCfg.APIKey,
                }
            }
            
            wd, _ := os.Getwd()
            return RunREPL(version, wd, resolvedCfg)
        },
    }
    
    cmd.Version = version
    return cmd
}
```

### Step 6: Update `internal/config/config.go`

Ensure `ProviderConfig` has proper JSON tags and add helper if needed:

```go
type ProviderConfig struct {
    Models []string `json:"models"`
    APIKey string   `json:"api_key"`
}
```

### Step 7: Remove obsolete code

- Remove `internal/config/prompt.go`
- Remove `ResolveWithPrompt` function

### Step 8: Update tests

- Add tests for `providers.Registry`
- Update existing tests

### Step 9: Test the flow

```bash
# First run - triggers setup
go run ./cmd/agent

# Should show interactive prompts:
# Select a provider:
# > Anthropic
#   OpenAI
#   Google Gemini
#
# Enter API key for Anthropic: ****
#
# Select a model:
# > Claude 3 Opus
#   Claude 3 Sonnet
#   Claude 3 Haiku
```

---

## Files to Modify

| File | Changes |
|------|---------|
| `go.mod` | Add `github.com/charmbracelet/huh` dependency |
| `configs/providers/registry.yaml` | **New file** - Provider/model definitions |
| `configs/providers/loader.go` | **New file** - Registry loader with embed |
| `internal/cli/setup.go` | **New file** - Interactive setup flow |
| `internal/cli/root.go` | Remove flags, load registry, add setup flow |
| `internal/config/prompt.go` | **Delete** - No longer needed |

---

## Benefits of Project-Level Config

1. **Version controlled** - Provider/model changes tracked in git
2. **Consistent** - All users get same defaults
3. **Customizable** - Users can fork and modify `registry.yaml`
4. **Bundled** - Embedded in binary, no external files needed
5. **Simple** - No runtime file creation/management

---

## Future: `/model` Command (REPL)

When REPL is fully implemented, add a `/model` command:
- Loads providers from embedded `configs/providers/registry.yaml`
- Shows friendly names from registry
- Allows switching to different provider/model
- If API key exists, shows masked version (***...)
- If API key missing, prompts for it
- Updates `active_provider` and `active_model` in user config
