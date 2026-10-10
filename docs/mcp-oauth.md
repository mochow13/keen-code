# MCP Servers via OAuth

Keen supports OAuth-authenticated MCP servers for streamable HTTP MCP transports. OAuth is used when an MCP server entry in the selected MCP configuration file has `auth.type` set to `oauth`.

## Configuration

OAuth MCP servers are configured in the MCP config file, which defaults to:

```text
~/.keen/mcp/configs.json
```

Use `--mcp-config <file_path>` to select a different MCP configuration file for `keen` or `keen run`. It replaces the default file without merging servers and does not change OAuth token storage. Use `--model-config` separately for model/provider settings. See [MCP configuration location](mcp-servers.md#configuration-location).

Example:

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

OAuth config fields:

| Field | Required | Meaning |
| --- | --- | --- |
| `auth.type` | Yes | Must be `oauth`. |
| `auth.scopes` | No | Requested OAuth scopes. Keen joins this list with spaces for dynamic client registration. |

OAuth is only valid for HTTP MCP servers. If a server uses `command` for stdio, `auth.type: "oauth"` is rejected during config validation because stdio servers do not use Keen's HTTP OAuth integration.

## Supported OAuth flow

Keen's MCP OAuth support is built around the MCP Go SDK authorization-code handler.

| Capability | Support |
| --- | --- |
| Transport | Streamable HTTP MCP servers. |
| OAuth grant | Authorization Code. |
| User interaction | Browser-based login. |
| Callback listener | Local HTTP callback server. |
| Token persistence | Stored in Keen's auth store and reused on later startups. |
| Dynamic client registration | Supported by the SDK handler when no pre-registered client options are supplied. |
| Scopes | Supported through `auth.scopes`. |
| Startup with existing token | Supported. |
| Startup with no token | Does not open a browser; server becomes `auth_required`. |
| Forced re-authentication | Supported through `/mcp connect <server>`. |

## Not supported in user config

The current `configs.json` format intentionally keeps OAuth config small. These values are not configurable in the JSON file:

- OAuth authorization URL.
- OAuth token URL.
- OAuth client ID.
- OAuth client secret.
- Redirect URL.
- Per-server OAuth callback port.
- Device Code flow.
- Client Credentials flow.
- OAuth for stdio MCP servers.

The `internal/mcp` package has code-level options for alternate redirect URLs, pre-registered clients, client metadata document URLs, and client names, but Keen's CLI starts the MCP manager with defaults and does not expose those options through `configs.json`.

## Redirect URL

The default MCP OAuth redirect URL is:

```text
http://localhost:1456/auth/mcp/callback
```

This is defined by `DefaultOAuthRedirectURL` in `internal/mcp/oauth.go`.

When `/mcp connect <server>` starts OAuth, Keen:

1. Uses this redirect URL.
2. Starts a temporary local HTTP server on `localhost:1456`.
3. Registers a callback handler at `/auth/mcp/callback`.
4. Opens the browser to the authorization URL supplied by the MCP SDK auth flow.
5. Waits for the authorization server to redirect back with `code` and `state`.

If the callback address is unavailable, OAuth fails and `/mcp connect` reports the error.

## Startup behavior

At process startup, Keen creates and starts the MCP manager:

1. `internal/mcp/config.go` loads the file selected with `--mcp-config`, or `~/.keen/mcp/configs.json` by default.
2. `internal/mcp/manager.go` creates one runtime entry per configured server.
3. `internal/mcp/oauth.go` loads persisted OAuth tokens from Keen's auth store.
4. The manager connects to all configured MCP servers concurrently.
5. Connected servers have their tools listed and later generate MCP skills.

For OAuth servers, startup is non-interactive. Keen does not open a browser during startup.

Startup outcomes:

| Condition | Outcome |
| --- | --- |
| Stored valid token exists | Keen connects using the stored bearer token. |
| No stored token exists | Server usually becomes `auth_required`. |
| Stored token exists but is invalid/rejected | Server usually becomes `auth_failed`. |
| Server cannot be reached | Server becomes `disconnected`. |
| Config is invalid | MCP startup is skipped for the default config; the command fails if `--mcp-config` is supplied. |

This lets Keen start normally even when an OAuth MCP server needs user login. The user can authenticate later with `/mcp connect <server>`.

## Token loading and persistence

Keen stores OAuth credentials in:

```text
~/.keen/auth.json
```

MCP OAuth credentials use provider IDs of this form:

```text
mcp:<server>
```

For example, a server named `posthog` is stored under:

```text
mcp:posthog
```

The stored credential contains:

```json
{
  "type": "oauth",
  "access_token": "...",
  "refresh_token": "...",
  "expires_at": "2026-05-21T12:00:00Z"
}
```

The auth store is written with `0600` permissions.

Token loading rules:

- Keen reads all credentials from `~/.keen/auth.json`.
- It only loads providers prefixed with `mcp:`.
- It only loads tokens for servers that are still configured in `configs.json`.
- It only loads tokens for servers whose configured auth type is `oauth`.
- Removed servers' old auth entries may remain in `auth.json`, but they are ignored because there is no matching configured runtime entry.

## HTTP transport integration

For OAuth HTTP servers, `Manager.transportFor` builds a streamable HTTP client transport with an OAuth handler:

```go
transport := &mcpsdk.StreamableClientTransport{
    Endpoint:             server.URL,
    HTTPClient:           client,
    MaxRetries:           opts.streamableMaxRetries,
    DisableStandaloneSSE: opts.disableStandaloneSSE,
}
transport.OAuthHandler = oauthHandler
```

The OAuth handler is a `cachingOAuthHandler` wrapping the MCP SDK's `AuthorizationCodeHandler` when interactive OAuth is available.

The handler has two modes:

| Mode | Used when | Behavior |
| --- | --- | --- |
| Non-interactive | Startup, or refresh without an OAuth code fetcher | Reuses a valid stored token if available. If the server challenges and no interactive handler exists, returns `ErrAuthRequired` or `ErrAuthFailed`. |
| Interactive | `/mcp connect <server>` | Opens the browser, receives an authorization code, lets the MCP SDK complete OAuth, then saves the resulting token. |

## Interactive login with `/mcp connect`

Use this command to authenticate or re-authenticate an OAuth MCP server:

```text
/mcp connect <server>
```

The REPL implementation does the following:

1. Creates a 5-minute context for the connection attempt.
2. Uses `DefaultOAuthRedirectURL`.
3. Creates a browser authorization-code fetcher with `NewBrowserOAuthCodeFetcher`.
4. Calls `mcp.Refresh` with:
   - `WithRefreshConnectTimeout(5 * time.Minute)`
   - `WithRefreshOAuthRedirectURL(DefaultOAuthRedirectURL)`
   - `WithRefreshOAuthAuthorizationCodeFetcher(fetcher)`
   - `WithRefreshOAuthForceReauth(true)`
5. On success, regenerates the MCP skill and enables it.
6. On failure, disables the MCP skill and shows the error.

`WithRefreshOAuthForceReauth(true)` is important: Keen clears the stored token before reconnecting, so `/mcp connect` starts a fresh OAuth login instead of silently reusing the old token.

## Browser authorization-code fetcher

`NewBrowserOAuthCodeFetcher` returns an MCP SDK `AuthorizationCodeFetcher`.

When the SDK needs user authorization, the fetcher:

1. Receives an authorization URL from the SDK.
2. Starts a local callback server for the configured redirect URL.
3. Opens the authorization URL in the default browser.
4. Waits for the callback request.
5. Extracts `code` and `state` from the query string.
6. Returns them to the MCP SDK as an `AuthorizationResult`.

Callback handling details:

| Callback input | Behavior |
| --- | --- |
| `?code=<code>&state=<state>` | Returns the code and state to the SDK and shows a success page. |
| `?error=<error>` | Returns an OAuth authorization error and shows a failure page. |
| Missing `code` | Returns `OAuth callback missing code`. |
| No callback within 5 minutes | Returns `OAuth login timed out`. |
| Callback response blocks too long | Browser gets a timeout page after 30 seconds. |

## Client registration and token exchange

`newOAuthHandler` configures the MCP SDK authorization-code handler.

The handler config includes:

- Redirect URL: defaults to `http://localhost:1456/auth/mcp/callback`.
- Authorization-code fetcher: only supplied for interactive `/mcp connect`.
- HTTP client: Keen's manager HTTP client.

Client identity behavior:

1. If a client metadata document URL is supplied through code-level options, Keen passes it to the SDK handler.
2. Else, if a pre-registered client ID is supplied through code-level options, Keen passes those credentials to the SDK handler.
3. Else, Keen enables dynamic client registration with metadata:
   - `client_name`: `Keen Code` by default.
   - `redirect_uris`: the configured redirect URL.
   - `grant_types`: `["authorization_code"]`.
   - `response_types`: `["code"]`.
   - `token_endpoint_auth_method`: `client_secret_post` (the OAuth 2.1-preferred method; some servers reject the RFC 7591 default `client_secret_basic`).
   - `scope`: the space-joined `auth.scopes` from `configs.json`.

In normal CLI usage, Keen uses option 3 because the CLI does not expose pre-registered OAuth client config.

The MCP SDK handler performs the OAuth protocol work required by the MCP server. After authorization succeeds, Keen obtains the token from the SDK token source and saves it in `~/.keen/auth.json`.

## Reusing tokens

When a valid token is available, `cachingOAuthHandler.TokenSource` returns an `oauth2.StaticTokenSource` for that token. The MCP SDK transport can then send authenticated requests without prompting the user.

If a stored token exists but is no longer valid or is rejected by the server:

- Startup or non-interactive refresh cannot repair it because no browser fetcher is available.
- The server state becomes `auth_failed` in most OAuth rejection cases.
- Running `/mcp connect <server>` clears the stored token and starts a fresh browser login.

## Failure states

OAuth failures map into MCP server states shown by `/mcp status`.

| State | Common OAuth cause | Recovery |
| --- | --- | --- |
| `auth_required` | No token is available and the server requires OAuth. | Run `/mcp connect <server>`. |
| `auth_failed` | Stored token rejected, OAuth callback error, OAuth-related SDK error, or callback setup failure. | Run `/mcp connect <server>` again after fixing the cause. |
| `disconnected` | Network failure, server unavailable, protocol error not identified as auth-related. | Check server URL/network, then run `/mcp connect <server>` or restart Keen. |

Keen also reports failed MCP startup statuses in the REPL with a hint like:

```text
MCP connection failed for posthog. Try `/mcp connect posthog` to connect.
```

## Skill integration after OAuth

OAuth state controls whether the generated MCP skill is visible to the LLM.

| OAuth/MCP outcome | Skill behavior |
| --- | --- |
| OAuth succeeds and tools are listed | Keen generates or refreshes `~/.keen/skills/mcp:<server>`, enables it, and reloads skills. |
| OAuth is required or failed | Keen disables `mcp:<server>` and reloads skills. |
| Server removed from config | Previously enabled generated MCP skill is disabled on the next startup sync. |

This prevents the LLM from seeing OAuth-protected MCP tools until Keen has a connected, authenticated server and current tool schemas.

## Headless behavior

`keen run` starts the MCP runtime, but it does not have an interactive `/mcp connect` command. Therefore:

- Headless runs can use OAuth MCP servers only if a valid token is already stored.
- If no valid token exists, the server is unavailable for that run.
- Authenticate first in the interactive REPL with `/mcp connect <server>`.

## Security notes

- OAuth tokens are persisted in `~/.keen/auth.json` with file mode `0600`.
- MCP OAuth tokens are namespaced as `mcp:<server>` to avoid mixing them with LLM-provider OAuth credentials.
- `call_mcp_tool` still requires user approval before invoking a remote MCP tool.
- OAuth access tokens are not printed by Keen's normal MCP status output.
- API-key redaction logic is separate; OAuth error strings are classified but not token-redacted by key value because OAuth tokens are not stored in the MCP server config.

## Implementation references

| Concern | Code |
| --- | --- |
| MCP OAuth constants and handler | `internal/mcp/oauth.go` |
| MCP config auth type validation | `internal/mcp/config.go` |
| OAuth transport wiring | `internal/mcp/manager.go` |
| MCP manager options for OAuth | `internal/mcp/options.go` |
| Auth store format and permissions | `internal/auth/store.go` |
| Browser callback server | `internal/auth/oauth.go` |
| `/mcp connect` command | `internal/cli/repl/command_handlers.go` |
| MCP skill sync after connect/startup | `internal/cli/repl/repl_helpers.go` |
