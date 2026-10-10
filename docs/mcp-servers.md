# MCP Servers in Keen

Keen supports MCP (Model Context Protocol) servers as external tool providers. MCP servers are loaded from a JSON config (`~/.keen/mcp/configs.json` by default, or a file selected with `--mcp-config`), connected at startup, and exposed to the LLM through Keen's `call_mcp_tool` tool.

## What is supported

| Area | Support |
| --- | --- |
| MCP capability exposed to the LLM | Tools: Keen lists MCP tools and calls MCP tools. |
| Transports | Streamable HTTP and stdio. |
| HTTP auth | `none`, `api_key`, and `oauth`. |
| Stdio auth | No HTTP auth. Use process environment variables for stdio server credentials. |
| Tool discovery | Keen lists tools during startup and on manual refresh/connect. Paginated tool lists are supported. |
| Tool calls | Keen calls a named tool on a connected configured server with JSON object arguments. |
| OAuth | Browser-based OAuth during `/mcp connect <server>` and persisted OAuth tokens under Keen's auth store. |
| Permissions | Every `call_mcp_tool` invocation asks for user approval before calling the remote tool. |
| Status UI | `/mcp status` shows configured servers, connection state, auth type, and errors. |

## What is not supported

Keen's current MCP integration is intentionally tool-focused:

- MCP resources and prompts are not exposed to the LLM.
- Server-initiated tool-list changes are logged, but they do not automatically regenerate skills until refresh/connect or restart.
- There is no automatic project-local MCP config discovery; use `--mcp-config <file_path>` to select a project-local file.
- There is no live reload of MCP configuration; restart Keen after adding or removing servers.
- There is no explicit `transport` field in config. Keen infers transport from the presence of `command`.
- Legacy SSE is not a first-class config transport. Keen uses the SDK's streamable HTTP client for HTTP MCP servers.
- WebSocket, raw TCP, and Unix socket transports are not configured by Keen.
- Per-server timeouts and retry settings are not configurable in the JSON file.
- `call_mcp_tool` does not locally validate arguments against the schema; the LLM is instructed to read the generated schema and the MCP server remains the final authority.
- Stdio servers cannot use Keen's HTTP auth config. Put credentials in `env` for the subprocess instead.

## Configuration location

By default, Keen reads MCP server configuration from:

```text
~/.keen/mcp/configs.json
```

If the default file does not exist, Keen starts with no configured MCP servers.

Use `--mcp-config <file_path>` to load a different file instead:

```bash
keen --mcp-config ./mcp-config.json
keen run --mcp-config ./mcp-config.json "Explain this project"
```

Relative paths are resolved against the current working directory; absolute paths are also supported. The selected file replaces the default configuration entirely, without merging servers. It must exist, be readable, and contain valid MCP configuration; otherwise startup fails without falling back to the default. OAuth credentials and other Keen data keep their normal locations.

`--mcp-config` selects only MCP server configuration. Use `--model-config <file_path>` for model and provider settings; both flags can be used together. See [CLI usage](cli-usage.md#custom-model-configuration-file).

The top-level format is:

```json
{
  "servers": {
    "server-name": {
      "url": "https://example.com/mcp",
      "auth": { "type": "none" }
    }
  }
}
```

Server names must be 1-128 characters and may contain letters, numbers, underscores, dashes, and dots. They must start with a letter or number.

## Transport selection

Keen infers the transport from each server entry:

| Config field | Transport used |
| --- | --- |
| `command` is present and non-empty | stdio |
| `command` is absent or empty | streamable HTTP |

If both `command` and `url` are present, `command` wins and Keen treats the server as stdio.

## Streamable HTTP examples

### No auth

```json
{
  "servers": {
    "deepwiki": {
      "url": "https://mcp.example.com/mcp",
      "auth": { "type": "none" }
    }
  }
}
```

`auth` may also be omitted for no-auth HTTP servers.

### API key auth with default bearer header

When `type` is `api_key` and no `header` is provided, Keen sends:

```text
Authorization: Bearer <key>
```

```json
{
  "servers": {
    "docs": {
      "url": "https://docs.example.com/mcp",
      "auth": {
        "type": "api_key",
        "key": "YOUR_API_KEY"
      }
    }
  }
}
```

### API key auth with a custom header

When `header` is provided and `scheme` is empty, Keen sends the raw key value in that header.

```json
{
  "servers": {
    "context7": {
      "url": "https://mcp.context7.com/mcp",
      "auth": {
        "type": "api_key",
        "header": "CONTEXT7_API_KEY",
        "scheme": "",
        "key": "YOUR_CONTEXT7_API_KEY"
      }
    }
  }
}
```

To use a custom scheme with a custom header, set `scheme` explicitly:

```json
{
  "servers": {
    "example": {
      "url": "https://mcp.example.com/mcp",
      "auth": {
        "type": "api_key",
        "header": "Authorization",
        "scheme": "Token",
        "key": "YOUR_TOKEN"
      }
    }
  }
}
```

This sends:

```text
Authorization: Token <key>
```

### OAuth auth

```json
{
  "servers": {
    "posthog": {
      "url": "https://mcp.posthog.com/mcp",
      "auth": {
        "type": "oauth",
        "scopes": ["read", "write"]
      }
    }
  }
}
```

OAuth behavior:

1. At startup, Keen can reuse a stored OAuth token for the server.
2. If no valid token is available, the server enters `auth_required` or `auth_failed`.
3. Run `/mcp connect <server>` to start browser-based OAuth.
4. Keen listens for the callback at:

```text
http://localhost:1456/auth/mcp/callback
```

OAuth tokens are stored in Keen's auth store under provider names like `mcp:<server>`.

## Stdio example

Use `command`, optional `args`, and optional `env` for stdio MCP servers.

```json
{
  "servers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "YOUR_TOKEN"
      }
    }
  }
}
```

Stdio behavior:

- Keen starts the command as a subprocess.
- `args` are passed as command arguments.
- `env` is merged into Keen's process environment and overrides matching variables.
- Stdio server stderr is logged at debug level and truncated.
- `auth` must be omitted or set to `{ "type": "none" }`.

## Runtime behavior

When Keen starts:

1. It reads the file selected with `--mcp-config`, or `~/.keen/mcp/configs.json` when the flag is omitted.
2. It creates one runtime entry per configured server.
3. It connects to all configured servers concurrently.
4. It lists each connected server's tools.
5. It stores each server state for `/mcp status` and `call_mcp_tool`.

Connection states include:

| State | Meaning |
| --- | --- |
| `configured` | Server exists in config but has not connected yet. |
| `connecting` | Keen is connecting or refreshing the server. |
| `connected` | Server is connected and tools were discovered. |
| `disconnected` | Server failed to connect or a session closed. |
| `auth_required` | OAuth authentication is needed. |
| `auth_failed` | Authentication failed, such as an invalid token or API key. |

## Commands

| Command | Purpose |
| --- | --- |
| `/mcp status` | Show all configured MCP servers and their state. |
| `/mcp connect <server>` | Refresh/connect a server. For OAuth servers, starts browser auth and forces re-authentication. |

`/mcp connect` accepts either a server name or a unique tool name. If multiple servers expose the same tool name, Keen asks for the server name.

## Calling MCP tools

Keen registers one built-in LLM tool when MCP runtime starts successfully:

```text
call_mcp_tool
```

The LLM must provide:

```json
{
  "server": "github",
  "tool": "create_issue",
  "arguments": {
    "title": "Bug report",
    "body": "Details..."
  }
}
```

Before execution, Keen asks the user to approve the remote tool call.

## Failure behavior

| Scenario | Behavior |
| --- | --- |
| Missing default `~/.keen/mcp/configs.json` (no `--mcp-config`) | Keen starts with no configured MCP servers. |
| Default config is unreadable or contains invalid JSON or server config | MCP startup is skipped; `call_mcp_tool` is not registered. Keen continues with a warning. |
| File selected with `--mcp-config` is missing, unreadable, or invalid | The command fails without falling back to the default config. |
| One invalid server entry | Validation fails for the whole config: MCP startup is skipped for the default file; the command fails when `--mcp-config` is supplied. |
| HTTP connection fails | Server state becomes `disconnected`; `/mcp status` shows the error. |
| API key is missing in config | Config validation fails. |
| API key is rejected by server | Server usually becomes `auth_failed`. API key values are redacted from errors/logs. |
| OAuth token is missing | Server becomes `auth_required`; run `/mcp connect <server>`. |
| OAuth token is rejected | Server becomes `auth_failed`; run `/mcp connect <server>` to re-authenticate. |
| Stdio command exits or cannot start | Server becomes `disconnected`. |
| Server removed from config | After restart, it is no longer configured; generated MCP skills may remain on disk but are disabled by sync when previously enabled. |
