# Architecture

Keen Code is a terminal-based AI coding agent that follows an event-driven, tool-based architecture.

## High-Level Overview

```
User Input → REPL (Bubble Tea) → Session → LLM Client → Provider (API)
                                    ↓
                              Tool Registry
                                    ↓
                              Tools (read_file, bash, etc.)
                                    ↓
                              Filesystem Guard (Permission System)
```

## Entry Point

```
cmd/main.go
    └── NewRootCommand(version)
            └── cobra.Command
                    └── Runs REPL via repl.RunREPL()
```

## Core Components

### CLI Command (`internal/cli/cmd/root.go`)

The root command initializes:
1. Provider registry (`providers.Load()`)
2. Config loader (`config.NewLoader()`)
3. Global config (`loader.Load()`)
4. Resolves configuration and starts the REPL

### REPL (`internal/cli/repl/`)

The interactive terminal interface built with Bubble Tea:

```
repl.RunREPL()
    └── Bubble Tea application
            ├── StreamHandler - handles LLM response streaming
            ├── CommandHandlers - processes slash commands (/btw, /model, /compact, etc.)
            ├── SessionState - manages session context
            └── UI Widgets - renders output, permissions, viewport
```

Key files:
- `repl.go` - Main REPL loop and state management
- `stream_handler.go` - Processes LLM stream events
- `command_handlers.go` - Slash commands
- `repl_queue.go` - Input queuing while agent is streaming
- `session_state.go` - Session context management

### LLM Layer (`internal/llm/`)

Unified interface for multiple AI providers:

```go
type LLMClient interface {
    StreamChat(ctx context.Context, messages []Message, toolRegistry *tools.Registry, opts ...StreamOptions) (<-chan StreamEvent, error)
    Reset()
}
```

Implementations:
- `AnthropicClient` - Direct Anthropic SDK integration
- `OpenAIResponsesClient` - OpenAI Responses API (GPT models)
- `OpenAICompatibleClient` - OpenAI-compatible API (DeepSeek, Moonshot, Z.ai, OpenCode Go compatible models)
- `GenkitClient` - Firebase Genkit for Google AI

MiniMax uses `AnthropicClient` with `https://api.minimax.io/anthropic`.
OpenCode Go is routed by model format: `minimax-m2.*` and `qwen3.7-max` use `AnthropicClient`, while GLM, Kimi, DeepSeek, MiMo, and other Qwen models use `OpenAICompatibleClient`.

### Tools (`internal/tools/`)

Built-in tools the LLM can call:

| Tool | File | Purpose |
|------|------|---------|
| read_file | `read_file.go` | Read file contents |
| write_file | `write_file.go` | Create/overwrite files |
| edit_file | `edit_file.go` | Targeted string replacement |
| glob | `glob.go` | Find files by pattern |
| grep | `grep.go` | Search file contents |
| bash | `bash.go` | Execute shell commands |
| web_fetch | `web_fetch.go` | Fetch public web content |
| call_mcp_tool | `call_mcp_tool.go` | Call a tool on a connected MCP server |
| delegate_task | `delegate_tool.go` | Delegate a task to a configured subagent |

All tools use `filesystem.Guard` for permission checks and `PermissionRequester` for user prompts.

### Side Questions (`/btw`)

The `/btw <question>` command starts a one-shot helper stream for quick side questions. It builds a request from up to the last 10 messages of the current conversation (5 user + 5 agent) plus `llm.BuildBtwPrompt()`, disables tool access, and renders the answer inline below the current conversation. The side-question exchange is kept out of the main conversation and session transcript. Running `/btw` with no question shows usage help.

### Session (`internal/session/`)

Event-sourced session management:

```
Sessions stored in ~/.keen/sessions/
    └── {namespace}/
            └── {session-id}/
                    └── transcript_events.jsonl
```

Events:
- `session_started` - Session initialization
- `user_message` - User input
- `assistant_turn` - AI response with transcript
- `compaction_applied` - Context compaction

### Permission System (`internal/filesystem/`)

```go
type Guard struct {
    workingDir   string
    blockedPaths []string
    gitignore    *GitAwareness
}
```

Policy:
- Working directory: Granted for reads, Pending for writes
- System paths (`/etc`, `/usr`, etc.): Denied
- Gitignored paths: Blocked
- Skills directories: Always allowed

### Skills (`internal/skills/`)

Discoverable skill modules with YAML frontmatter:

```go
type Skill struct {
    Name        string
    Description string
    Location    string
}
```

Discovery roots:
- `<working-dir>/.agents/skills/`
- `<working-dir>/.keen/skills/`
- `<working-dir>/.claude/skills/`
- `~/.agents/skills/`
- `~/.keen/skills/`
- `~/.claude/skills/`
- Bundled (embedded in binary)

### Config (`internal/config/`)

Hierarchical configuration:
1. Session config (command-line arguments)
2. Global config (`~/.keen/configs.json`)
3. Provider defaults

```go
type GlobalConfig struct {
    ActiveProvider string
    ActiveModel    string
    ThinkingEffort string
    Providers      map[string]ProviderConfig
}
```

### Auth (`internal/auth/`)

Authentication management:
- API key storage and lookup
- OAuth flow for OpenAI Codex (PKCE-based)

## Data Flow: User Message to Response

1. **User Input** - User types message in REPL
2. **Command Check** - Check if it's a slash command
3. **Session Append** - Append `user_message` event to session
4. **Build Context** - Reconstruct conversation from events using `projection.BuildConversation()`
5. **LLM Request** - Call `StreamChat()` with messages and tool registry
6. **Stream Events** - Process events: chunks, tool calls, usage stats
7. **Tool Execution** - Execute tools, emit results back to LLM
8. **Response Complete** - Append `assistant_turn` event with transcript
9. **UI Update** - Render response in terminal

## Key Interfaces

### LLMClient

```go
// internal/llm/client.go
type LLMClient interface {
    StreamChat(ctx context.Context, messages []Message, toolRegistry *tools.Registry) (<-chan StreamEvent, error)
    Reset()
}
```

### Tool

```go
// internal/tools/tool.go
type Tool interface {
    Name() string
    Description() string
    InputSchema() map[string]any
    Execute(ctx context.Context, input any) (any, error)
}
```

### PermissionRequester

```go
// internal/tools/permission.go
type PermissionRequester interface {
    RequestPermission(ctx context.Context, toolName, path, resolvedPath string, isDangerous bool) (bool, error)
}
```

### DiffEmitter

```go
// internal/tools/diff.go
type DiffEmitter interface {
    EmitDiff(lines []EditDiffLine)
}
```

## Stream Events

LLM clients emit a unified stream of events:

```go
type StreamEvent struct {
    Type       StreamEventType
    Content    string           // text/reasoning chunks
    ToolCall   *ToolCall        // tool start/end
    Usage      *TokenUsage      // token usage stats
    Error      error            // errors
    Attempt    int              // retry attempt
}
```

Types:
- `StreamEventTypeChunk` - Text content
- `StreamEventTypeReasoningChunk` - Thinking content
- `StreamEventTypeToolStart` - Tool execution started
- `StreamEventTypeToolEnd` - Tool execution completed
- `StreamEventTypeUsage` - Token usage
- `StreamEventTypeDone` - Response complete
- `StreamEventTypeError` - Unrecoverable error
- `StreamEventTypeRetry` - Retrying after error
- `StreamEventTypeIncomplete` - Turn limit reached

## Directory Structure

```
keen-code/
├── cmd/
│   └── main.go              # Entry point
├── internal/
│   ├── auth/                # Authentication (OAuth, API keys)
│   ├── cli/
│   │   ├── cmd/             # Cobra commands
│   │   └── repl/            # Bubble Tea REPL
│   ├── config/              # Configuration management
│   ├── filesystem/          # Guard and GitAwareness
│   ├── llm/                 # LLM client implementations
│   ├── providers/           # Provider registry and model metadata
│   ├── session/             # Session management
│   ├── skills/              # Skills system
│   └── tools/               # Built-in tools
├── docs/                    # Documentation
└── npm/                      # npm wrapper package
```

## Dependencies

Key external libraries:
- **Bubble Tea** (`github.com/charmbracelet/bubbletea`) - TUI framework
- **Cobra** (`github.com/spf13/cobra`) - CLI framework
- **Anthropic SDK** (`github.com/anthropics/anthropic-sdk-go`) - Claude integration
- **OpenAI Go** (`github.com/openai/openai-go`) - OpenAI-compatible APIs
- **Genkit** (`github.com/firebase/genkit/go`) - Google AI integration
- **go-git** (`github.com/go-git/go-git/v5`) - Gitignore parsing
- **go-udiff** (`github.com/aymanbagabas/go-udiff`) - Diff computation
