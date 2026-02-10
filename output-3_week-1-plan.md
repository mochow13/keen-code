# Week 1 Implementation Plan: Phase 1 - Foundation

Based on the RFC (`output-1_rfc.md`) and PRD (`prd.md`), this document outlines the step-by-step tasks for Week 1 (Phase 1: Foundation).

---

## Overview

Phase 1 focuses on building the foundational infrastructure before any LLM integration or tool implementation. The key deliverables are:

1. Project structure and Go module setup
2. Configuration system with YAML
3. FileGuard for secure file access
4. **GitAwareness component (CRITICAL)** - Respects `.gitignore` to avoid wasting tokens
5. Basic CLI structure with Cobra
6. Structured logging with `log/slog`

---

## Project Structure

```
keen-cli/
├── cmd/agent/
│   └── main.go                    # Entry point
├── internal/
│   ├── config/
│   │   ├── config.go              # Config struct and defaults
│   │   └── loader.go              # YAML config loading
│   ├── filesystem/
│   │   ├── guard.go               # FileGuard - path security
│   │   └── gitawareness.go        # GitAwareness - .gitignore handling
│   ├── cli/
│   │   ├── root.go                # Cobra root command
│   │   └── repl.go                # Interactive REPL command (stub)
│   └── logger/
│       └── logger.go              # Structured logging setup
├── configs/
│   └── system_prompts/            # Default system prompts
├── go.mod
├── go.sum
└── README.md
```

---

## Task 1: Initialize Go Module and Project Structure

**Objective:** Set up the project foundation and dependencies.

**Steps:**
1. Run `go mod init github.com/user/keen-cli`
2. Create directory structure as outlined above
3. Add core dependencies:
   - `github.com/spf13/cobra` - CLI framework
   - `gopkg.in/yaml.v3` - YAML marshal/unmarshal
   - `github.com/go-git/go-git/v5` - For .gitignore parsing
   - `github.com/go-git/go-git/v5/plumbing/format/gitignore` - Gitignore matcher

**Deliverables:**
- `go.mod` with all dependencies
- Directory structure created
- Empty placeholder files to satisfy imports

**Testing Strategy:**
- Verify `go build ./...` succeeds
- Verify all packages compile

---

## Task 2: Implement Config System

**Package:** `internal/config/`

**Files:**
- `config.go` - Config structs, resolution logic, and defaults
- `loader.go` - YAML loading and saving

**Two-Level Configuration:**

1. **Global Config** - Persisted to `~/.config/keen/config.yaml`
   - Set via `/provider` command in REPL
   - Contains per-provider settings (model, API key)
   
2. **Session Config** - CLI flag overrides (not persisted)
   - Set via `--provider`, `--api-key`, `--model` flags
   - Overrides global config for current session only

**Key Components:**

```go
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

// SessionConfig holds CLI flag overrides (not persisted)
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

**Resolution Order (Session > Global > Default):**

```go
func Resolve(global *GlobalConfig, session *SessionConfig) (*ResolvedConfig, error)
```

1. Provider: `session.Provider` → `global.ActiveProvider` → error
2. API Key: `session.APIKey` → `global.GetProviderConfig().APIKey` → error
3. Model: `session.Model` → `global.GetProviderConfig().Model` → `defaultModel(provider)`

**Loader:**

```go
type Loader struct{}

func NewLoader() *Loader
func (l *Loader) Load() (*GlobalConfig, error)    // Load from ~/.config/keen/config.yaml
func (l *Loader) Save(cfg *GlobalConfig) error    // Save with 0600 permissions
func (l *Loader) Exists() bool                    // Check if config exists
```

**Deliverables:**
- `GlobalConfig`, `SessionConfig`, `ResolvedConfig` structs
- `Resolve()` function with proper error handling
- `Loader` for YAML persistence
- Helper methods: `GetProviderConfig()`, `SetProviderConfig()`, `ConfigPath()`
- Unit tests with 80%+ coverage

---

## Task 3: Implement FileGuard

**Package:** `internal/filesystem/`

**File:** `guard.go`

**Purpose:** Path security - prevent directory traversal and access to sensitive paths.

**Interface:**
```go
type Guard interface {
    ValidatePath(path string) error
    ResolvePath(path string) (string, error)
}
```

**Key Methods:**

1. **ValidatePath(path string) error**
   - Block paths containing `../` or `..\`
   - Block absolute paths outside working directory
   - Block paths matching blocked patterns (`~/.ssh`, `/etc`, etc.)
   - Return descriptive errors

2. **ResolvePath(path string) (string, error)**
   - Resolve relative paths against working directory
   - Clean path (remove `.`, `..`, extra slashes)
   - Return absolute path

**Security Rules:**
| Rule | Implementation |
|------|----------------|
| Path traversal | Reject paths containing `..` |
| Absolute paths | Convert to relative from working dir |
| Sensitive paths | Blocklist: `~/.ssh`, `/etc`, `~/.aws`, etc. |
| Symlinks | Resolve and validate target |

**Testable Design:**
- Pure functions for path validation
- Constructor injection of working directory and blocked paths
- No global state

**Test Cases:**
```go
func TestValidatePath(t *testing.T) {
    tests := []struct {
        name    string
        path    string
        wantErr bool
    }{
        {"valid file", "main.go", false},
        {"valid nested", "src/main.go", false},
        {"path traversal", "../etc/passwd", true},
        {"absolute outside", "/etc/passwd", true},
        {"dot slash", "./main.go", false},
        {"double slash", "src//main.go", false},
    }
    // ... table-driven test
}
```

**Deliverables:**
- FileGuard implementation
- Comprehensive unit tests
- Security audit of edge cases

---

## Task 4: Implement GitAwareness (CRITICAL)

**Package:** `internal/filesystem/`

**File:** `gitawareness.go`

**Purpose:** Prevent wasting tokens and confusing the LLM by filtering out files that should be ignored according to `.gitignore` rules.

**Interface:**
```go
type GitAwareness interface {
    LoadGitignore(path string) error
    IsIgnored(filePath string) bool
    FilterPaths(paths []string) []string
}
```

**Key Methods:**

1. **LoadGitignore(path string) error**
   - Load `.gitignore` from project root
   - Recursively load nested `.gitignore` files from subdirectories
   - Support global gitignore (`~/.gitignore_global`)
   - Cache parsed patterns for performance

2. **IsIgnored(filePath string) bool**
   - Check if path matches any loaded ignore pattern
   - Respect negation patterns (`!important.log`)
   - Handle directory vs file patterns correctly

3. **FilterPaths(paths []string) []string**
   - Filter a list of paths, returning only non-ignored ones
   - Efficient batch operation

**Caching Strategy:**
- Cache parsed ignore matchers per directory
- Cache `IsIgnored` results for frequently checked paths
- Invalidate cache when `.gitignore` files change

**Implementation Notes:**
- Use `github.com/go-git/go-git/v5/plumbing/format/gitignore` for pattern matching
- Handle edge cases:
  - Empty .gitignore files
  - Comments and blank lines
  - Glob patterns (`*.log`, `node_modules/`)
  - Directory patterns (`build/`)
  - Negation patterns (`!important.log`)

**Testable Design:**
- Interface-based for mocking
- Separate parser from matcher logic
- In-memory implementation for tests

**Test Cases:**
```go
func TestGitAwareness(t *testing.T) {
    tests := []struct {
        name     string
        patterns []string
        path     string
        ignored  bool
    }{
        {"node_modules dir", []string{"node_modules/"}, "node_modules/lodash", true},
        {"log files", []string{"*.log"}, "debug.log", true},
        {"nested path", []string{"build/"}, "build/output.js", true},
        {"negation", []string{"*.log", "!important.log"}, "important.log", false},
        {"not ignored", []string{"*.log"}, "main.go", false},
    }
    // ... table-driven test
}
```

**Deliverables:**
- GitAwareness implementation
- Support for nested .gitignore files
- Caching for performance
- Comprehensive unit tests
- Benchmark tests for large path lists

---

## Task 5: Basic CLI Structure

**Package:** `internal/cli/`

**Files:**
- `root.go` - Root command and global flags
- `repl.go` - REPL command (initial stub)

**Commands to Implement:**

1. **Root Command (`keen`)**
   ```bash
   keen                    # Start REPL
   keen "create fibonacci" # One-shot mode
   keen --version          # Show version
   keen --config ~/.keen.yaml # Use custom config
   ```

2. **Flags:**
   - `-c, --config string` - Config file path
   - `-v, --verbose` - Enable debug logging
   - `--version` - Show version

3. **Config Subcommand (stub)**
   ```bash
   keen config get llm.provider
   keen config set llm.provider openai
   ```

**Integration:**
- Initialize config on startup
- Initialize logger
- Wire up FileGuard and GitAwareness

**Testable Design:**
- Use Cobra's command testing utilities
- Dependency injection for config and logger
- Separate command logic from execution

**Deliverables:**
- Working CLI with help text
- Config flag handling
- Version command
- Basic error handling

---

## Task 6: Structured Logging

**Package:** `internal/logger/`

**File:** `logger.go`

**Purpose:** Centralized, configurable logging using Go's standard `log/slog`.

**Features:**
- Levels: debug, info, warn, error
- Formats: text (development), JSON (production)
- Output: stderr (default) or file
- Structured fields using slog.Attr

**Interface:**
```go
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
    With(args ...any) Logger
}
```

**Configuration:**
```yaml
logging:
  level: info      # debug | info | warn | error
  format: text     # text | json
  file: ""         # empty = stderr
```

**Usage:**
```go
logger.Info("config loaded", "path", configPath, "provider", cfg.LLM.Provider)
logger.Debug("file read", "path", path, "size", len(content))
logger.Error("tool execution failed", "tool", name, "error", err)
```

**Testable Design:**
- Interface-based for mocking in tests
- Capture logs in tests using `slogtest` or custom handler
- No global logger instance

**Deliverables:**
- Logger initialization from config
- Text and JSON formatters
- File output support
- Unit tests for log levels

---

## Implementation Order

| Order | Task | Depends On | Priority |
|-------|------|------------|----------|
| 1 | Project Structure | - | Critical |
| 2 | Config System | - | Critical |
| 3 | Logger | Config | High |
| 4 | FileGuard | Config | Critical |
| 5 | GitAwareness | Config | Critical |
| 6 | CLI | Config, Logger | High |

**Rationale:**
- Config is needed by almost all other components
- Logger should be available early for debugging
- FileGuard and GitAwareness are independent and can be done in parallel
- CLI comes last as it integrates everything

---

## Testing Strategy

### Unit Tests
- Each package should have `*_test.go` files
- Target: 80%+ code coverage
- Use table-driven tests for validation logic
- Mock interfaces for isolation

### Integration Tests
- Test config loading from multiple sources
- Test FileGuard with real filesystem (temp dir)
- Test GitAwareness with sample .gitignore files

### Test Structure
```
internal/
├── config/
│   ├── config.go
│   ├── config_test.go
│   └── loader_test.go
├── filesystem/
│   ├── guard.go
│   ├── guard_test.go
│   ├── gitawareness.go
│   └── gitawareness_test.go
```

---

## Dependencies to Add

```go
// go.mod requirements:
require (
    github.com/spf13/cobra v1.8.0
    gopkg.in/yaml.v3 v3.0.1
    github.com/go-git/go-git/v5 v5.11.0
)
```

**Standard Library Only:**
- `log/slog` - Structured logging
- `os`, `os/exec` - File operations
- `path/filepath` - Cross-platform paths
- `testing` - Unit tests

---

## Success Criteria for Week 1

- [ ] `go build ./...` succeeds with no errors
- [ ] All unit tests pass (`go test ./...`)
- [ ] CLI shows help and version
- [ ] Config loads from multiple sources correctly
- [ ] FileGuard blocks path traversal attempts
- [ ] GitAwareness correctly filters node_modules, .git, etc.
- [ ] Logging works at all levels
- [ ] Code follows Go best practices (gofmt, golint)
- [ ] 80%+ test coverage on core packages

---

## Next Steps (Week 2 Preview)

After Week 1 foundation is complete, Week 2 will focus on:
- LLM Provider Interface (Anthropic first)
- Tool System (read_file, list_dir)
- Basic Orchestrator loop
