# Keen CLI Configuration Design

This document describes the two-level configuration system for Keen CLI.

---

## Two-Level Configuration

### Level 1: Global Config (Defaults)
- **Set via:** `/config` command inside REPL
- **Stored in:** `~/.config/keen/config.yaml`
- **Purpose:** Default settings for all sessions

### Level 2: Session Config (Overrides)
- **Set via:** CLI flags at startup
- **Stored in:** Memory only (not persisted)
- **Purpose:** Override for current session only

### Resolution Order (Highest to Lowest Priority)

1. **CLI flags** (`--provider`, `--api-key`) - Session-specific
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

## Setting Up Global Config

### Via REPL `/provider` Command

```bash
$ keen
🤖 Keen v0.1.0

> /provider

Current configuration:
  Provider: anthropic
  Model: claude-3-sonnet

Options:
  [1] Change provider
  [2] Update API key
  [3] Change model
  [4] View all settings
  [q] Quit

Select: 1

Available providers:
  [1] anthropic
  [2] openai
  [3] gemini

Select provider: 2

Configure openai:
  API key: sk-xxxxx
  Model [gpt-4o]: 

✓ Configuration saved to ~/.config/keen/config.yaml
> 
```

### Direct Commands

```bash
# Interactive setup (menu-driven)
> /provider

# Set provider and API key interactively
> /provider set gemini
Enter API key: xxxxxx
Select model: gemini-1.5-pro
✓ Saved

# View current config
> /provider show

# Quick switch to pre-configured provider
> /provider use gemini   # Uses saved gemini config
```

---

## Session-Specific Config (CLI Flags)

Override global defaults for current session only:

```bash
# Start with specific provider
keen --provider=gemini

# Start with specific provider and API key
keen --provider=openai --api-key=sk-xxxxx

# Full override
keen --provider=anthropic --model=claude-3-opus --api-key=$ANTHROPIC_KEY
```

### Available Flags

| Flag | Description | Example |
|------|-------------|---------|
| `--provider` | LLM provider | `--provider=gemini` |
| `--api-key` | API key for provider | `--api-key=xxxxx` |
| `--model` | Model name | `--model=gpt-4o` |

---

## Use Cases

### Use Case 1: First-Time Setup (No Default Config)

```bash
# First run - no config exists
$ keen
🤖 Keen v0.1.0

⚠️  No provider configured.

Run /provider to set up a provider:
> /provider

Available providers:
  [1] anthropic
  [2] openai
  [3] gemini

Select provider: 1
Enter API key: sk-xxxxx
Select model [claude-3-sonnet]: 

✓ Configuration saved to ~/.config/keen/config.yaml

> explain this code   # Now works with anthropic
```

### Use Case 2: Testing Different Providers

```bash
# Terminal 1 - Use Claude for complex tasks
$ keen
> /provider set anthropic
> refactor this architecture

# Terminal 2 - Try Gemini for comparison
$ keen --provider=gemini --api-key=$GEMINI_KEY
> refactor this architecture
```

### Use Case 3: CI/CD with Different Keys

```bash
# CI script uses session-specific key
keen --provider=openai --api-key=$OPENAI_API_KEY "generate tests"
```

### Use Case 4: Quick One-Off Tasks

```bash
# Use cheaper model for simple task
keen --provider=gemini --model=gemini-1.5-flash "fix this typo"
```

---

## Go Implementation

### Config Struct

```go
package config

// GlobalConfig is persisted to ~/.config/keen/config.yaml
type GlobalConfig struct {
    Provider string `yaml:"provider" mapstructure:"provider"`
    
    Anthropic ProviderConfig `yaml:"anthropic"`
    OpenAI    ProviderConfig `yaml:"openai"`
    Gemini    ProviderConfig `yaml:"gemini"`
}

type ProviderConfig struct {
    Model  string `yaml:"model"`
    APIKey string `yaml:"api_key"`
}

// SessionConfig overrides for current session only
type SessionConfig struct {
    Provider string
    APIKey   string
    Model    string
}

// ResolvedConfig is the final merged config
type ResolvedConfig struct {
    Provider string
    APIKey   string
    Model    string
}
```

### Resolution Logic

```go
func Resolve(global *GlobalConfig, session *SessionConfig) (*ResolvedConfig, error) {
    // Determine provider (session > global > default)
    provider := session.Provider
    if provider == "" {
        provider = global.Provider
    }
    
    // Check if provider is configured
    if provider == "" {
        return nil, fmt.Errorf("no provider configured. Run /provider to set up a provider")
    }
    
    // Get provider-specific global config
    providerGlobal := global.GetProviderConfig(provider)
    
    // API Key: session > global
    apiKey := firstNonEmpty(session.APIKey, providerGlobal.APIKey)
    if apiKey == "" {
        return nil, fmt.Errorf("no API key configured for %s. Run /provider to set up", provider)
    }
    
    // Resolve each field (session > global > default)
    resolved := &ResolvedConfig{
        Provider: provider,
        APIKey:   apiKey,
        Model:    firstNonEmpty(session.Model, providerGlobal.Model, defaultModel(provider)),
    }
    
    return resolved, nil
}
```

---

## Config Storage

### Global Config Storage

```go
package config

import (
    "os"
    "path/filepath"
)

const configDir = ".config/keen"
const configFile = "config.yaml"

// LoadGlobal loads the global config from disk
func LoadGlobal() (*GlobalConfig, error) {
    home, _ := os.UserHomeDir()
    path := filepath.Join(home, configDir, configFile)
    
    // Read and unmarshal YAML
    // Return defaults if file doesn't exist
}

// SaveGlobal persists the global config to disk
func SaveGlobal(cfg *GlobalConfig) error {
    home, _ := os.UserHomeDir()
    dir := filepath.Join(home, configDir)
    
    // Create dir if needed
    os.MkdirAll(dir, 0755)
    
    // Marshal and write YAML
    path := filepath.Join(dir, configFile)
    return writeYAML(path, cfg)
}
```

---

## Security Considerations

### API Key Storage

| Method | Storage | Use Case |
|--------|---------|----------|
| Global config | `~/.config/keen/config.yaml` | Personal daily use |
| Session flag | Memory only | Shared machines, CI/CD |

### File Permissions

```go
// Config file should be readable only by owner
os.WriteFile(path, data, 0600) // -rw-------
```

---

## Summary

| Aspect | Implementation |
|--------|----------------|
| **Config levels** | 2 (global + session) |
| **Global storage** | `~/.config/keen/config.yaml` |
| **Session storage** | CLI flags, memory only |
| **Resolution** | Session > Global > Default |
| **Multi-session** | Each can use different providers via flags |
| **Interactive setup** | `/config` command in REPL |
| **Quick override** | `--provider`, `--api-key` flags |

---

## Commands Reference

### REPL Commands

| Command | Description |
|---------|-------------|
| `/provider` | Open interactive provider menu |
| `/provider show` | Display current provider config |
| `/provider set <name>` | Configure and save new provider |
| `/provider use <name>` | Switch to pre-configured provider (session only) |

### CLI Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--provider` | `-p` | Provider for this session |
| `--api-key` | `-k` | API key for this session |
| `--model` | `-m` | Model for this session |
