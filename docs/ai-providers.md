# AI Providers

Keen Code supports multiple AI providers through a plugin-like architecture. The provider system handles model discovery, authentication, and communication with different LLM backends.

## Supported Providers

| Provider | ID | Authentication | Models |
|----------|-----|----------------|--------|
| Anthropic | `anthropic` | API Key | Claude Opus 5, Fable 5, Sonnet 5, Opus 4.8, Haiku 4.5 |
| OpenAI | `openai` | API Key | GPT-5.6 Sol, GPT-5.6 Luna, GPT-5.6 Terra, GPT-5.5, GPT-5.4 |
| Codex | `openai-codex` | OAuth | GPT-5.6 Sol, GPT-5.6 Terra, GPT-5.6 Luna, GPT-5.5, GPT-5.4 |
| Google AI | `googleai` | API Key | Gemini 3.1 Pro, 3.6 Flash, 3.5 Flash, 3.5 Flash-Lite |
| Moonshot AI | `moonshotai` | API Key | Kimi K3, K2.7 Code, K2.7 Code High-Speed, K2.6, K2.5 |
| Z.ai | `zai` | API Key | GLM-5.2, GLM-5.1 |
| DeepSeek | `deepseek` | API Key | DeepSeek V4 Flash, V4 Pro |
| MiniMax | `minimax` | API Key | MiniMax M3, M2.7, M2.7 High-Speed |
| Amazon Bedrock | `amazon-bedrock` | AWS credentials | Claude Fable 5, Sonnet 5, Opus 4.8, Opus 4.6, Sonnet 4.6, Haiku 4.5 |
| OpenCode Go | `opencode-go` | API Key | Grok 4.5, GPT-5.6 Luna, GLM-5.2, GLM-5.1, Kimi K3, Kimi K2.7 Code, Kimi K2.6, MiMo V2.5 Pro, MiMo V2.5, Qwen3.8 Max, Qwen3.7 Max, Qwen3.7 Plus, Qwen3.6 Plus, MiniMax M3, MiniMax M2.7, DeepSeek V4 Pro, DeepSeek V4 Flash, Hy3 |
| OpenAI Compatible | `openai-compatible` | API Key | Any OpenAI-compatible model |

## OpenAI-Compatible Providers

Keen supports arbitrary OpenAI-compatible endpoints through the `openai-compatible` provider. This is config-file only: set `active_provider`, `active_model`, and the provider entry in `~/.keen/configs.json`, then restart Keen.

### OpenRouter Example

```json
{
  "active_provider": "openai-compatible",
  "active_model": "tencent/hy3:free",
  "providers": {
    "openai-compatible": {
      "models": ["tencent/hy3:free"],
      "api_key": "sk-or-v1-...",
      "base_url": "https://openrouter.ai/api/v1",
      "headers": {
        "HTTP-Referer": "https://your-site.com",
        "X-Title": "Keen Code"
      }
    }
  }
}
```

Notes:

- `active_provider` must be `"openai-compatible"`.
- `active_model` must match one of the entries in `providers.openai-compatible.models`.
- `base_url` is required and must be the OpenAI-compatible chat completions endpoint base (usually ending in `/v1`).
- `api_key` is sent as the `Authorization: Bearer ...` token.
- Custom headers in `headers` are sent with every request.

## Provider Registry

Provider and model metadata is stored in `providers/registry.yaml`. This includes:
- Context window sizes
- Supported thinking efforts
- Model display names

```go
// providers/loader.go
type Registry struct {
    Providers []Provider `yaml:"providers"`
}

type Provider struct {
    ID     string  `yaml:"id"`
    Name   string  `yaml:"name"`
    Models []Model `yaml:"models"`
}

type Model struct {
    ID              string   `yaml:"id"`
    Name            string   `yaml:"name"`
    ContextWindow   int      `yaml:"context_window"`
    ThinkingEfforts []string `yaml:"thinking_efforts"`
}
```

## Configuration

### Global Config (`~/.keen/configs.json`)

```json
{
  "active_provider": "opencode-go",
  "active_model": "kimi-k2.6",
  "thinking_effort": "enabled",
  "show_thinking": true,
  "adversary_provider": "anthropic",
  "adversary_model": "claude-sonnet-5",
  "providers": {
    "opencode-go": {
      "models": ["kimi-k2.6"],
      "api_key": "oc_..."
    }
  }
}
```

`adversary_provider` and `adversary_model` are set via `/adversary model` and control which model is used for adversarial reviews. They are independent of `active_provider`/`active_model` and can point to any configured provider.

### Custom Headers

You can attach custom HTTP headers to a provider by adding a `headers` object to that provider's config in `~/.keen/configs.json`. These headers are sent with every request to that provider.

```json
{
  "active_provider": "deepseek",
  "active_model": "deepseek-v4-pro",
  "providers": {
    "deepseek": {
      "models": ["deepseek-v4-pro"],
      "api_key": "sk-...",
      "headers": {
        "x_header_1": "val1",
        "x_header_2": "val2"
      }
    }
  }
}
```

Notes:

- Header names and values are plain strings.
- They are set per-provider; different providers can have different headers.
- Custom headers must be added by editing the config file directly. The `/model` UI does not provide a field for them.
- Applied to all clients: Anthropic, OpenAI, Codex, DeepSeek, Moonshot AI, Z.ai, MiniMax, OpenCode Go, Google AI (Genkit), and Amazon Bedrock.

### Config Resolution

Keen loads `~/.keen/configs.json` through `internal/config.Loader`, then builds the runtime `ResolvedConfig` in `internal/cli/cmd/root.go`.

Resolution order for the default interactive and headless paths:
1. Provider: `global.active_provider`
2. Model: `global.active_model`
3. API key: `providers.{provider}.api_key_helper` → `providers.{provider}.api_key`

For `keen run --provider`, the selected provider's config replaces the active provider for that invocation. If `--model` is omitted, Keen uses the selected provider's first configured model. The API key is still resolved through `api_key_helper` first, then `api_key`.

## Authentication

### API Key Authentication

Most providers use API key authentication. Keys are stored in the global config under `providers.{provider}.api_key`.

Instead of storing a key, a provider can define `api_key_helper`. Keen executes the helper locally when resolving the provider config, trims stdout, and uses that value as the in-memory API key for the current run/session. When `api_key_helper` is set, it always wins over `api_key`; `api_key` can be empty and Keen does not write the helper output back to `~/.keen/configs.json`.

```json
{
  "active_provider": "anthropic",
  "active_model": "claude-sonnet-5",
  "providers": {
    "anthropic": {
      "models": ["claude-sonnet-5"],
      "api_key": "",
      "api_key_helper": "example-auth token || (example-auth login > /dev/null 2>&1 && example-auth token)"
    }
  }
}
```

> **Security note:** `api_key_helper` is executed as a shell command with the privileges of the running process. Treat it as executable code: never paste untrusted strings into this field, audit any helper script before use, and keep `~/.keen` permissions strict (e.g. `chmod 700 ~/.keen` and `chmod 600 ~/.keen/configs.json`) so other local users cannot inject or read its contents.

MiniMax uses its Anthropic-compatible API. Users normally leave `base_url` unset. Keen uses `https://api.minimax.io/anthropic`, which the Anthropic SDK extends to `/v1/messages`.

OpenCode Go also uses API key authentication. Users normally leave `base_url` unset. Keen follows OpenCode's model-specific endpoints: GPT-5.6 Luna uses `/v1/responses`, MiniMax and Qwen use `/v1/messages`, and the other curated models use `/v1/chat/completions`.

### OAuth Authentication (OpenAI Codex)

OpenAI Codex uses OAuth with PKCE flow:

```go
// internal/auth/oauth.go
type OAuthManager struct {
    Store       *Store
    HTTPClient  *http.Client
    OpenBrowser BrowserOpener
}
```

Flow:
1. Generate PKCE verifier/challenge and state
2. Start local HTTP server on port 1455
3. Open browser to authorization URL
4. Receive callback, exchange code for tokens
5. Store refresh/access tokens

Token refresh is automatic when the access token expires.

### AWS Authentication (Amazon Bedrock)

Amazon Bedrock uses AWS credential authentication via the AWS SDK:

```go
// internal/config/config.go
const AuthModeAWS = "aws"
```

Credentials are loaded from the standard AWS credential chain (`~/.aws/credentials`, environment variables, IAM roles, etc.). No API key is stored in Keen's global config. An optional custom `base_url` can be configured to override the Bedrock endpoint.

The default AWS region is `us-east-1` if none is configured in the environment.

## LLM Client Architecture

```go
// internal/llm/client.go
type LLMClient interface {
    StreamChat(ctx context.Context, messages []Message, toolRegistry *tools.Registry) (<-chan StreamEvent, error)
    Reset()
}
```

Three client implementations:

### AnthropicClient (`internal/llm/anthropic.go`)

Direct integration with Anthropic SDK:
- Streaming via `ssestream.Stream`
- Tool conversion to Anthropic tool format
- Adaptive thinking and model-specific effort support
- Cached token tracking
- MiniMax models through MiniMax's Anthropic-compatible `/messages` endpoint
- OpenCode Go MiniMax and Qwen models through the Anthropic-compatible `/messages` endpoint

### BedrockClient (`internal/llm/bedrock.go`)

AWS SDK integration for Amazon Bedrock:
- Streaming via `bedrockruntime.ConverseStream`
- Tool conversion to Bedrock tool format
- Reasoning content support (thinking text, signatures, redacted content)
- Prompt caching with cache points on system prompts, tools, and messages
- Cached token tracking
- Adaptive thinking through `additionalModelRequestFields`

### OpenAIResponsesClient (`internal/llm/openai_responses.go`)

OpenAI Responses API for:
- OpenAI (GPT models)
- OpenCode Go GPT-5.6 Luna

### OpenAICompatibleClient (`internal/llm/openai.go`)

OpenAI-compatible API for:
- DeepSeek
- Moonshot AI (Kimi)
- Z.ai (GLM)
- OpenCode Go Grok, GLM, Kimi, DeepSeek, MiMo, and Hy3 models

OpenCode Go Qwen and MiniMax models use the Anthropic-compatible client.

Handles provider-specific features like the `reasoning_content` extension and thinking controls for compatible providers.

### GenkitClient (`internal/llm/genkit.go`)

Firebase Genkit integration for Google AI (Gemini). Currently the only provider using Genkit.

## Stream Events

All clients emit a unified stream of events:

```go
type StreamEvent struct {
    Type       StreamEventType
    Content    string           // for Chunk, ReasoningChunk
    ToolCall   *ToolCall        // for ToolStart, ToolEnd
    Usage      *TokenUsage      // for Usage
    Error      error            // for Error, Retry
    Attempt    int              // for Retry
}
```

Event types:
- `StreamEventTypeChunk` - Text content delta
- `StreamEventTypeReasoningChunk` - Thinking/reasoning content
- `StreamEventTypeToolStart` - Tool execution begins
- `StreamEventTypeToolEnd` - Tool execution completes
- `StreamEventTypeUsage` - Token usage stats
- `StreamEventTypeDone` - Response complete
- `StreamEventTypeError` - Unrecoverable error
- `StreamEventTypeRetry` - Retrying after error
- `StreamEventTypeIncomplete` - Turn limit reached with pending state

## Thinking Efforts

Models expose their provider-specific thinking values without normalizing them to a common scale:

| Provider | Efforts |
|----------|---------|
| Anthropic | low, medium, high, xhigh, max |
| OpenAI | none, low, medium, high, xhigh, max |
| Google AI | low, medium, high, minimal |
| Moonshot AI | K3: low, high, max; K2.6: enabled, disabled |
| DeepSeek | disabled, high, max |
| Amazon Bedrock | low, medium, high, xhigh, max |
| Z.ai | GLM-5.2: disabled, high, max; GLM-5.1: enabled, disabled |
| MiniMax | M3: enabled, adaptive, disabled |
| OpenCode Go | Model-specific; see `internal/providers/registry.yaml` |

The selected effort is stored in `thinking_effort` and passed to the provider without changing its meaning.

OpenCode Go thinking controls are model-family specific:
- GPT-5.6 Luna uses the Responses API. Kimi K3 and Hy3 send their documented `reasoning_effort` values.
- DeepSeek sends `thinking.type` plus `reasoning_effort`.
- Qwen uses the Anthropic-compatible endpoint with an enabled/disabled thinking toggle.
- MiniMax M3 sends `thinking.type` as enabled, adaptive, or disabled.
- Models without a registry `thinking_efforts` entry do not receive a Keen-sent thinking control; returned reasoning is still streamed when the provider exposes it.

GPT-5.4 in Codex OAuth and Kimi K2.5 are retained for existing users and are scheduled for removal after their August 31, 2026 retirements.
