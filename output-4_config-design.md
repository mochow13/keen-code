# Keen CLI Configuration Design

This document describes the two-level configuration system for Keen CLI.

---

## Two-Level Configuration

### Level 1: Global Config (Defaults)
- **Stored in:** `~/.config/keen/config.yaml`
- **Purpose:** Default settings for all sessions
- **Access:** Go API via `config.Loader`

### Level 2: Session Config (Overrides)
- **Stored in:** Memory only (not persisted)
- **Purpose:** Override for current session only
- **Access:** Go API via `config.SessionConfig` struct

### Resolution Order (Highest to Lowest Priority)

1. **Session config** (`SessionConfig` struct) - Session-specific overrides
2. **Global config** (`~/.config/keen/config.yaml`) - Defaults
3. **Built-in defaults** - Fallbacks

---

## Global Config File

### Location
```
~/.config/keen/config.yaml
```

### Structure
```yaml
# Active default provider
provider: anthropic

# Per-provider configurations
anthropic:
  model: claude-3-sonnet
  api_key: sk-ant-xxxxx

openai:
  model: gpt-4o
  api_key: sk-xxxxx

gemini:
  model: gemini-1.5-pro
  api_key: xxxxxx
```

---

## Go Implementation

### Config Structs

```go
package config

// GlobalConfig is persisted to ~/.config/keen/config.yaml
type GlobalConfig struct {
    ActiveProvider string `yaml:"provider" mapstructure:"provider"`
    
    Anthropic ProviderConfig `yaml:"anthropic"`
    OpenAI    ProviderConfig `yaml:"openai"`
    Gemini    ProviderConfig `yaml:"gemini"`
}

type ProviderConfig struct {
    Model  string `yaml:"model"`
    APIKey string `yaml:"api_key"`
}

// SessionConfig holds runtime overrides for the current session only
type SessionConfig struct {
    Provider string
    APIKey   string
    Model    string
}

// ResolvedConfig is the final merged configuration
type ResolvedConfig struct {
    Provider string
    APIKey   string
    Model    string
}
```

### Config Access Methods

```go
// GetProviderConfig returns the ProviderConfig for a given provider name
func (g *GlobalConfig) GetProviderConfig(provider string) (ProviderConfig, error)

// SetProviderConfig sets the ProviderConfig for a given provider name
func (g *GlobalConfig) SetProviderConfig(provider string, cfg ProviderConfig) error
```

### Resolution Logic

```go
// Resolve merges global and session configs into the final ResolvedConfig.
// Resolution order: Session > Global > Default
func Resolve(global *GlobalConfig, session *SessionConfig) (*ResolvedConfig, error)
```

Resolution rules:
- **Provider**: `session.Provider` → `global.ActiveProvider` → error
- **API Key**: `session.APIKey` → `global.GetProviderConfig().APIKey` → error
- **Model**: `session.Model` → `global.GetProviderConfig().Model` → `defaultModel(provider)`

### Config Storage

```go
package config

// Loader handles loading and saving the global configuration.
type Loader struct{}

// NewLoader creates a new configuration loader.
func NewLoader() *Loader

// Load loads the global configuration from disk.
// Returns default config if file doesn't exist.
func (l *Loader) Load() (*GlobalConfig, error)

// Save persists the global configuration to disk.
// Creates config directory if needed. Uses 0600 permissions.
func (l *Loader) Save(cfg *GlobalConfig) error

// Exists checks if the config file exists.
func (l *Loader) Exists() bool

// ConfigPath returns the full path to the config file.
func ConfigPath() string

// ConfigDir returns the directory containing the config file.
func ConfigDir() string
```

---

## Security Considerations

### API Key Storage

| Method | Storage | Use Case |
|--------|---------|----------|
| Global config | `~/.config/keen/config.yaml` | Personal daily use |
| Session config | Memory only | Shared machines, CI/CD |

### File Permissions

```go
// Config file is readable only by owner
os.WriteFile(path, data, 0600) // -rw-------
```

---

## Summary

| Aspect | Implementation |
|--------|----------------|
| **Config levels** | 2 (global + session) |
| **Global storage** | `~/.config/keen/config.yaml` |
| **Session storage** | In-memory struct only |
| **Resolution** | Session > Global > Default |
| **Persistence** | YAML file loader |
| **Security** | 0600 file permissions |

---

## Future Work

- CLI flags for session overrides (`--provider`, `--api-key`, `--model`)
- REPL `/provider` command for interactive configuration
